PROJECT_NAME := "signalr"
PKG := "github.com/philippseith/$(PROJECT_NAME)"
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
GO_FILES := $(shell find . -name '*.go' | grep -v /vendor/ | grep -v _test.go)

.PHONY: all dep lint vet test test-coverage build clean

all: build

dep: ## Get the dependencies
	@go mod download

lint: ## Lint Golang files
	@golangci-lint run

vet: ## Run go vet
	@go vet ${PKG_LIST}

test: ## Run unittests
	@go test -race -short -count=1 ${PKG_LIST}

test-coverage: ## Run tests with coverage
	@go test -race -short -count=1 -coverpkg=. -coverprofile cover.out -covermode=atomic ${PKG_LIST}
	@cat cover.out >> coverage.txt

build: dep ## Build the binary file
	@go build -i -o build/main $(PKG)

clean: ## Remove previous build
	@rm -f $(PROJECT_NAME)/build

run-chatsample: ## run the local ./chatsample server
	@go run ./chatsample/*.go

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
