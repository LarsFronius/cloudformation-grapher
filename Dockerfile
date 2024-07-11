FROM golang:buster as deps

RUN apt-get update && apt-get install -y graphviz
WORKDIR /app

FROM deps as app

COPY . /app
RUN go mod vendor
RUN go build main.go

ENTRYPOINT ["/app/main"]