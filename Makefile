.PHONY: server

fmt:
	@echo "Running go fmt"
	@go fmt ./...

server:
	@echo "Starting server"
	@go run ./cmd/server

updater:
	@echo "Starting updater"
	@go run ./cmd/updater

refresh-modules:
	@echo "Updating go modules"
	@go get -u ./...
	@go mod tidy

build:
	@echo "Building all projects"
	@go build ./cmd/server
	@go build ./cmd/game-updater
