package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"log"
	"strings"
	"time"
)

var cfnClient *cloudformation.Client

var awsCfn, _ = config.LoadDefaultConfig(context.TODO(), config.WithRetryer(func() aws.Retryer {
	retrier := retry.AddWithMaxAttempts(retry.NewStandard(), 10)
	retrier = retry.AddWithMaxBackoffDelay(retrier, time.Second*30)
	return retrier
}))

var activeStati = []types.StackStatus{
	types.StackStatusCreateComplete,
	types.StackStatusCreateInProgress,
	types.StackStatusRollbackComplete,
	types.StackStatusRollbackFailed,
	types.StackStatusImportComplete,
	types.StackStatusUpdateComplete,
	types.StackStatusUpdateFailed,
	types.StackStatusUpdateInProgress,
	types.StackStatusUpdateRollbackComplete,
	types.StackStatusUpdateRollbackFailed,
	types.StackStatusUpdateRollbackInProgress,
	types.StackStatusUpdateRollbackCompleteCleanupInProgress,
}

type Stack struct {
	types.Stack
	Exports       map[string]string
	PutParameters []string
	GetParameters []string
}

type Stacks map[string]Stack

func (s Stacks) Add(stack types.Stack, putParameters []string, getParameters []string) {
	exports := make(map[string]string)
	for _, output := range stack.Outputs {
		if output.ExportName != nil {
			exports[*output.ExportName] = *output.OutputValue
		}
	}
	s[*stack.StackId] = Stack{Stack: stack, Exports: exports, PutParameters: putParameters, GetParameters: getParameters}
}

func GetStacks() (Stacks, error) {
	retVal := make(Stacks)
	cfnClient = cloudformation.NewFromConfig(awsCfn)
	var nextToken *string
	out, err := cfnClient.ListStacks(context.Background(), &cloudformation.ListStacksInput{StackStatusFilter: activeStati})
	if err != nil {
		return nil, err
	}
	for _, stack := range out.StackSummaries {
		s, putParameters, getParameters, err := describe(stack)
		if err != nil {
			return retVal, err
		}
		retVal.Add(*s, putParameters, getParameters)
	}
	nextToken = out.NextToken
	for nextToken != nil {
		out, err := cfnClient.ListStacks(context.Background(), &cloudformation.ListStacksInput{NextToken: nextToken, StackStatusFilter: activeStati})
		if err != nil {
			return nil, err
		}
		nextToken = out.NextToken
		fmt.Println(nextToken)
		for _, stack := range out.StackSummaries {
			s, putParameters, getParameters, err := describe(stack)
			if err != nil {
				return retVal, err
			}
			retVal.Add(*s, putParameters, getParameters)
		}
	}
	return retVal, nil
}

//func GetImports(stacks Stacks) {
//	for _, stack := range stacks {
//		for _, export := range stack.Exports {
//
//		}
//	}
//
//}

func describe(stack types.StackSummary) (retStack *types.Stack, putParameters []string, getParameters []string, err error) {
	fmt.Printf("reading stack %s\n", *stack.StackId)
	describeStacksOutput, err := cfnClient.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{StackName: stack.StackId})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(describeStacksOutput.Stacks) != 1 {
		return nil, nil, nil, fmt.Errorf("stack missing for %s", stack.StackId)
	}

	describeStackResourcesOutput, err := cfnClient.DescribeStackResources(context.Background(), &cloudformation.DescribeStackResourcesInput{StackName: stack.StackId})
	if err != nil {
		return nil, nil, nil, err
	}

	for _, resource := range describeStackResourcesOutput.StackResources {
		if *resource.ResourceType != "AWS::SSM::Parameter" {
			continue
		}
		putParameters = append(putParameters, *resource.PhysicalResourceId)
	}

	getStackTemplateSummaryOutput, err := cfnClient.GetTemplateSummary(context.Background(), &cloudformation.GetTemplateSummaryInput{StackName: stack.StackId})
	if err != nil {
		return nil, nil, nil, err
	}

	for _, getParam := range getStackTemplateSummaryOutput.Parameters {
		if strings.HasPrefix(*getParam.ParameterType, "AWS::SSM::Parameter::Value") {
			getParameters = append(getParameters, *getParam.DefaultValue)
		}
	}

	return &describeStacksOutput.Stacks[0], putParameters, getParameters, nil
}

func main() {
	fmt.Println("### Cloudformation Grapher")

	g := graphviz.New()
	//graph, err := g.Graph()
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer func() {
	//	if err := graph.Close(); err != nil {
	//		log.Fatal(err)
	//	}
	//	g.Close()
	//}()
	//n, err := graph.CreateNode("n")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//m, err := graph.CreateNode("m")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//e, err := graph.CreateEdge("e", n, m)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//e.SetLabel("e")
	//var buf bytes.Buffer
	//if err := g.Render(graph, "dot", &buf); err != nil {
	//	log.Fatal(err)
	//}
	//fmt.Println(buf.String())
	//
	//g.RenderFilename(graph, graphviz.PNG, "./graph.png")
	//
	//graphviz.ParseBytes(buf.Bytes())
	//
	stacks, err := GetStacks()
	if err != nil {
		log.Fatal(err)
	}

	//imports, err := GetImports(stacks)
	//if err != nil {
	//	log.Fatal(err)
	//}

	//awsDiagram, err := diagram.New(diagram.Label("my-diagram"), diagram.Filename("diagram"))
	awsGraph, err := g.Graph()
	if err != nil {
		log.Fatal(err)
	}
	//awsGraph.SetOverlap(false)

	//seenDiagramStacks := map[string]*diagram.Node{}
	seenStacks := map[string]*cgraph.Node{}
	//seenDiagramExports := map[string]*diagram.Node{}
	seenExports := map[string]*cgraph.Node{}
	seenParameters := map[string]*cgraph.Node{}

	for _, stack := range stacks {
		log.Printf("creating node %s", *stack.StackName)
		//seenDiagramStacks[*stack.StackName] = aws.Management.Cloudformation().Label(*stack.StackName)
		//awsDiagram.Add(seenDiagramStacks[*stack.StackName])
		seenStacks[*stack.StackName], err = awsGraph.CreateNode(*stack.StackName)
		seenStacks[*stack.StackName].SetArea(2).SetHeight(1)
		if err != nil {
			log.Fatal(err)
		}
		seenStacks[*stack.StackName].SetLabel(*stack.StackName)
		for name, export := range stack.Exports {
			log.Printf("creating export %s", name)
			if _, ok := seenExports[name]; !ok {
				//seenDiagramExports[name] = generic.Blank.Blank().Label(export)
				seenExports[name], _ = awsGraph.CreateNode(export)
				seenExports[name].SetFillColor("azure2").SetStyle(cgraph.SolidNodeStyle).SetShape(cgraph.BoxShape).SetArea(2).SetHeight(1)
			}

			log.Printf("creating edge %s -> %s", *stack.StackName, name)

			//awsDiagram.Connect(seenDiagramStacks[*stack.StackName], seenDiagramExports[name])

			e, err := awsGraph.CreateEdge(name, seenStacks[*stack.StackName], seenExports[name])
			if err != nil {
				log.Fatal(err)
			}
			e.SetLabel("Exports")
		}

		for _, putParameter := range stack.PutParameters {
			log.Printf("creating putParameter %s", putParameter)
			if _, ok := seenParameters[putParameter]; !ok {
				seenParameters[putParameter], _ = awsGraph.CreateNode(putParameter)
				seenParameters[putParameter].SetFillColor("azure2").SetStyle(cgraph.SolidNodeStyle).SetShape(cgraph.SquareShape).SetArea(2).SetHeight(1)
			}

			log.Printf("creating edge %s -> %s", *stack.StackName, putParameter)
			e, err := awsGraph.CreateEdge(putParameter, seenStacks[*stack.StackName], seenParameters[putParameter])
			if err != nil {
				log.Fatal(err)
			}
			e.SetLabel("PutParameter")
		}

		for _, getParameter := range stack.GetParameters {
			log.Printf("creating getParameter %s", getParameter)
			if _, ok := seenParameters[getParameter]; !ok {
				seenParameters[getParameter], _ = awsGraph.CreateNode(getParameter)
				seenParameters[getParameter].SetFillColor("azure2").SetStyle(cgraph.SolidNodeStyle).SetShape(cgraph.SquareShape).SetArea(2).SetHeight(1)
			}

			log.Printf("creating edge %s -> %s", getParameter, *stack.StackName)
			e, err := awsGraph.CreateEdge(getParameter, seenParameters[getParameter], seenStacks[*stack.StackName])
			if err != nil {
				log.Fatal(err)
			}
			e.SetLabel("GetParameter")
		}

	}

	for name, _ := range seenExports {
		out, err := cfnClient.ListImports(context.Background(), &cloudformation.ListImportsInput{
			ExportName: &name,
		})
		var apiError smithy.APIError
		if errors.As(err, &apiError) {
			if apiError.ErrorCode() != "ValidationError" {
				log.Fatal(err)
			} else {
				continue
			}
		}

		if err != nil {
			log.Fatal(err)
		}
		for _, i := range out.Imports {
			log.Printf("creating edge %s -> %s", name, i)
			if _, ok := seenExports[name]; !ok {
				log.Printf("export missing: %s", name)
				continue
			}
			if _, ok := seenStacks[i]; !ok {
				log.Printf("stack missing: %s", i)
				continue
			}
			e, err := awsGraph.CreateEdge(name, seenExports[name], seenStacks[i])
			if err != nil {
				log.Fatal(err)
			}
			e.SetLabel("Imports")
		}
	}

	err = g.RenderFilename(awsGraph, graphviz.XDOT, "/data/awsGraph.dot")
	if err != nil {
		log.Fatal(err)
	}
	err = g.RenderFilename(awsGraph, graphviz.PNG, "/data/awsGraph.png")
	if err != nil {
		log.Fatal(err)
	}

	//awsDiagram.Render()
	///// diagram test

	//d, err := diagram.New(diagram.Label("my-diagram"), diagram.Filename("diagram"))
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//fw := generic.Network.Firewall().Label("fw")
	//sw := generic.Network.Switch().Label("sw")
	//
	//d.Connect(fw, sw)
}
