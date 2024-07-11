generate:
	docker buildx build --platform linux/arm64 --target app -t app .
	docker run --platform linux/arm64 -e AWS_PROFILE -v ~/.aws:/root/.aws -v $$PWD/data:/data app
