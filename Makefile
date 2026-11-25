# Wormhole — simple build helpers.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
IMAGE   ?= wormhole:$(VERSION)

.PHONY: build test vet run docker clean install

build: ## Build the binary for the host platform
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o wormhole .

test: ## Run tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

run: build ## Build and run on :8080
	./wormhole

docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

install: ## Install into GOBIN / GOPATH
	go install -trimpath -ldflags "$(LDFLAGS)" .

clean: ## Remove build artifacts
	rm -rf wormhole dist/
