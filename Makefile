build: ## Build binaries
	@echo "+ $@"
	CGO_ENABLED=0 go build -v -o bin/app ./cmd/app

image: ## Build docker image
	@echo "+ $@"
	docker buildx build --push --platform linux/amd64,linux/arm64 -t notnildev/port-forwarder:latest -f Dockerfile .