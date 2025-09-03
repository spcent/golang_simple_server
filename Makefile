# Go 环境
GOOS ?= $(shell $(GO) env GOOS) # linux, darwin, windows
GOARCH ?= $(shell $(GO) env GOARCH) # amd64, arm64
GO ?= go
BINARY_NAME ?= golang_simple_server
PORT ?= 8080

.PHONY: all run test coverage fmt lint clean

all: fmt lint test build

# 构建可执行文件
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o $(BINARY_NAME) .

# 启动服务器
run: build
	./$(BINARY_NAME) -addr ":$(PORT)"

# 运行测试
test:
	$(GO) test -v ./...

# 测试并生成覆盖率报告
coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# 格式化代码
fmt:
	$(GO) fmt ./...

# 代码静态检查
lint:
	$(GO) vet ./...

# 清理构建产物
clean:
	rm -f $(BINARY_NAME) coverage.out coverage.html
