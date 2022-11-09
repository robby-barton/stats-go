.PHONY: fmt run-server run-updater run-ranking refresh-modules build clean

fmt:
	@echo "Running go fmt"
	@go fmt ./...

run-server:
	@echo "Starting server"
	@go run ./cmd/server

run-updater:
	@echo "Starting updater"
	@go run ./cmd/updater

run-ranking:
	@echo "Starting ranking"
	@go run ./cmd/ranking

refresh-modules:
	@echo "Updating go modules"
	@go get -u ./...
	@go mod tidy

server:
	@go build ./cmd/server

updater:
	@go build ./cmd/updater

ranking:
	@go build ./cmd/ranking

build: server updater ranking
	@echo "Building all projects"

clean:
	@rm -rf server updater ranking > /dev/null 2>&1
