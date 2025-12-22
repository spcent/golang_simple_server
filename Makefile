# Go environment
GOOS ?= $(shell $(GO) env GOOS) # linux, darwin, windows
GOARCH ?= $(shell $(GO) env GOARCH) # amd64, arm64
GO ?= go
BINARY_NAME ?= golang_simple_server
PORT ?= 8080

.PHONY: all run test coverage fmt lint clean deps install uninstall help

## all: Default target - format, lint, test, and build
all: fmt lint test build

## build: Build the executable
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o $(BINARY_NAME) .

## run: Build and run the server
run: build
	./$(BINARY_NAME) -addr ":$(PORT)"

## test: Run all tests
test:
	$(GO) test -v ./...

## coverage: Test with coverage report
coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## fmt: Format code with go fmt
fmt:
	$(GO) fmt ./...

## lint: Run static analysis with go vet
lint:
	$(GO) vet ./...

## clean: Clean build artifacts
clean:
	rm -f $(BINARY_NAME) coverage.out coverage.html

## deps: Download dependencies
deps:
	$(GO) mod download

## install: Install binary to $GOPATH/bin
install:
	$(GO) install

## uninstall: Remove binary from $GOPATH/bin
uninstall:
	$(GO) clean -i

## help: Display this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
	@echo ""
	@echo "Variables:"
	@echo "  GOOS       Operating system (default: $(GOOS))"
	@echo "  GOARCH     Architecture (default: $(GOARCH))"
	@echo "  GO         Go command (default: $(GO))"
	@echo "  BINARY_NAME Executable name (default: $(BINARY_NAME))"
	@echo "  PORT       Server port (default: $(PORT))"
