BINARY     = ibmdocs
BUILD_DIR  = target/bin
CMD_DIR    = cmd/ibmdocs
MODULE     = github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli

VERSION    ?= dev
BUILD_DATE  = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT  = $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

LDFLAGS = -ldflags "-s -w \
  -X $(MODULE)/cmd.version=$(VERSION) \
  -X $(MODULE)/cmd.buildDate=$(BUILD_DATE) \
  -X $(MODULE)/cmd.gitCommit=$(GIT_COMMIT)"

GO      = go
GOBUILD = CGO_ENABLED=0 $(GO) build $(LDFLAGS)
GOTEST  = $(GO) test
GOLINT  = golangci-lint

.PHONY: all build build-all build-linux-amd64 build-linux-arm64 \
        build-darwin-arm64 build-darwin-amd64 build-windows \
        clean test test-verbose coverage lint deps docker help

all: clean test build

## build: build native binary (host OS/arch)
build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY) ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)"

## build-linux-amd64: static Linux amd64 binary
build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)-linux-amd64"

## build-linux-arm64: static Linux arm64 binary
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)-linux-arm64"

## build-darwin-arm64: macOS Apple Silicon binary
build-darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)-darwin-arm64"

## build-darwin-amd64: macOS Intel binary
build-darwin-amd64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)-darwin-amd64"

## build-windows: static Windows amd64 binary
build-windows:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) \
	  -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./$(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY)-windows-amd64.exe"

## build-all: build all five release targets + checksums
build-all: build-linux-amd64 build-linux-arm64 build-darwin-arm64 build-darwin-amd64 build-windows
	@echo "\nAll targets:"
	@ls -lh $(BUILD_DIR)/$(BINARY)-*
	@cd $(BUILD_DIR) && sha256sum $(BINARY)-* > checksums.txt
	@echo "Checksums: $(BUILD_DIR)/checksums.txt"

## docker: build multi-arch container image (requires buildx)
docker:
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  -t ibmdocs:$(VERSION) \
	  .

## docker-local: build single-arch image for local use
docker-local:
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  -t ibmdocs:$(VERSION) \
	  .

## test: run unit tests
test:
	$(GOTEST) -v ./...

## test-verbose: run tests with race detector
test-verbose:
	$(GOTEST) -v -race ./...

## coverage: generate HTML coverage report
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: run golangci-lint
lint:
	$(GOLINT) run ./...

## deps: tidy and download modules
deps:
	$(GO) mod tidy
	$(GO) mod download

## release-dry-run: build-all + print what would be released
release-dry-run: build-all
	@echo "\nRelease artefacts for VERSION=$(VERSION):"
	@ls -lh $(BUILD_DIR)/$(BINARY)-*
	@cat $(BUILD_DIR)/checksums.txt

## clean: remove build artefacts
clean:
	$(GO) clean
	rm -rf target/

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
