# Minimal Go HTTP server with routing, middleware, and graceful shutdown

## Overview

A lightweight HTTP server skeleton built with the Go standard library.
It includes dynamic routing, middleware, graceful shutdown, and environment-based configuration — ideal for quickly building simple APIs or learning HTTP server development.

## Documentation

* **[English](docs/en/usage.md)**: Comprehensive guide with examples
* **[中文](docs/cn/usage.md)**: 中文文档，包含详细的使用说明和示例

## Features

* **Dynamic Routing**: Supports path parameters like `/hello/:name`
* **Middleware System**: Built-in Logging and Auth middlewares, with support for custom extensions
* **Graceful Shutdown**: Cleans up connections within 5 seconds after receiving an interrupt signal
* **Logging**: Add glog logging library for structured logging
* **Env Configuration**: Supports `.env` files (e.g., log level, etc.)
* **Test Coverage**: Includes tests for routing, middleware, 404 handling, and more
* **Developer Toolchain**: Comes with a `Makefile` and `dev.sh` script for easy build/run/test workflows

## Getting Started

### Requirements

* Go 1.18+

### Build & Run

```bash
# Using Makefile
make run   # Build and start the server (default port: 8080)

# Or using the dev.sh script
./dev.sh run
```

### Custom Port

```bash
./golang_simple_server -addr :9090  # Start on port 9090
```

## Routing

Register parameterized routes with `router.(Get|Post|Delete|Put)` (see `main.go` for examples):

```go
r.Get("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // Access params["name"]
})
r.Get("/users/:id/posts/:postID", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // Access params["id"] and params["postID"]
})
```

## Route Testing
After starting the service, test the routes using curl:
```bash
curl http://localhost:8080/ping # pong
curl http://localhost:8080/hello # {"message":"Hello, World!"}
curl -H "X-Token: secret" http://localhost:8080/hello/Alice # {"message":"Hello, Alice!"}
```

## Middleware

* **LoggingMiddleware**: Logs request duration (`[time] METHOD PATH (duration)`)
* **AuthMiddleware**: Validates `X-Token` header
* **CorsMiddleware**: Allow cross-domain requests

Combine middlewares (see `main.go` for usage):

```go
app.Use(app.Logging(), app.Auth())
```

## Testing

```bash
make test       # Run all tests
make coverage   # Generate coverage report (outputs to coverage.html)
```

## License
MIT