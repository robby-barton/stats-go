.PHONY: server

fmt:
	go fmt ./...

server:
	go run ./cmd/server

build:
	go build ./cmd/server
