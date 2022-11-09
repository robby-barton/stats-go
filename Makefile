.PHONY: fmt run-server run-updater run-ranker refresh-modules
.PHONY: download-modules modules build clean

fmt:
	@echo "Running go fmt"
	@go fmt ./...

run-server:
	@echo "Starting server"
	@go run ./cmd/server ${OPTS}

run-updater:
	@echo "Starting updater"
	@go run ./cmd/updater ${OPTS}

run-ranker:
	@echo "Starting ranker"
	@go run ./cmd/ranker ${OPTS}

refresh-modules: download-modules modules
	@go get -u ./...
	@go mod tidy

download-modules:
	@go get -u ./...

modules:
	@go mod tidy

server:
	@go build ./cmd/server

updater:
	@go build ./cmd/updater

ranker:
	@go build ./cmd/ranker

build: server updater ranking
	@echo "Building all projects"

clean:
	@rm -rf server updater ranker > /dev/null 2>&1
