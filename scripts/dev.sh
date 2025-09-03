#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="golang_simple_server"
SERVER="wwww@x.x.x.x"
REMOTE_DIR="/home/wwwroot/project/simple"
PORT="${PORT:-8080}"

usage() {
    echo "Usage: $0 {build|run|test|coverage|fmt|lint|clean|deploy}"
    exit 1
}

build() {
    echo "Building ${BINARY_NAME}..."
    go build -o "${BINARY_NAME}" .
    echo "Build done."
}

run() {
    build
    echo "Starting server on port ${PORT}..."
    ./"${BINARY_NAME}"
}

test() {
    echo "Running unit tests..."
    go test -v ./...
}

coverage() {
    echo "Running tests with coverage..."
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report generated: coverage.html"
}

fmt() {
    echo "Formatting code..."
    go fmt ./...
}

lint() {
    echo "Running static analysis..."
    go vet ./...
}

clean() {
    echo "Cleaning build artifacts..."
    rm -f "${BINARY_NAME}" coverage.out coverage.html
    echo "Clean done."
}

deploy() {
    echo "Deploying ${BINARY_NAME} to ${SERVER}..."
    scp "${BINARY_NAME}" "${SERVER}:${REMOTE_DIR}"
    ssh "${SERVER}" "chmod +x ${REMOTE_DIR}/${BINARY_NAME}"
    echo "Deploy done."
}

if [ $# -lt 1 ]; then
    usage
fi

case "$1" in
    build) build ;;
    run) run ;;
    test) test ;;
    coverage) coverage ;;
    fmt) fmt ;;
    lint) lint ;;
    clean) clean ;;
    deploy) deploy ;;
    *) usage ;;
esac
