.PHONY: fmt run-server run-updater refresh-modules build clean

fmt:
	@echo "Running go fmt"
	@go fmt ./...

run-server:
	@echo "Starting server"
	@go run ./cmd/server

run-updater:
	@echo "Starting updater"
	@go run ./cmd/updater

refresh-modules:
	@echo "Updating go modules"
	@go get -u ./...
	@go mod tidy

server:
	@go build ./cmd/server

updater:
	@go build ./cmd/updater

build: server updater
	@echo "Building all projects"

clean:
	@rm -rf server updater > /dev/null 2>&1
