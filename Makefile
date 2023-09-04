.PHONY: fmt refresh-modules build-server build-ranker build-updater
.PHONY: download-modules modules build clean server updater ranker

fmt:
	@go fmt ./...

server:
	@go run ./cmd/server

updater:
	@go run ./cmd/updater ${OPTS}

update-all-rankings:
	@go run ./cmd/updater -r -a

ranker:
	@go run ./cmd/ranker ${OPTS}

refresh-modules: download-modules modules
	@go get -u ./...
	@go mod tidy

download-modules:
	@go get -u ./...

modules:
	@go mod tidy

build-server:
	@go build ./cmd/server

build-updater:
	@go build ./cmd/updater

build-ranker:
	@go build ./cmd/ranker

build: build-server build-updater build-ranking

clean:
	@rm -rf server updater ranker > /dev/null 2>&1

lint:
	@golangci-lint run --config=.golangci.yml ./...
