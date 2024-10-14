.PHONY: fmt refresh-module download-modules modules clean migrate updater ranker

format:
	@go fmt ./...

migrate:
	@go run ./cmd/migrate

updater:
	@go run ./cmd/updater ${OPTS}

update-all-rankings:
	@go run ./cmd/updater -ranking -all

ranker:
	@go run ./cmd/ranker ${OPTS}

refresh-modules: download-modules modules
	@go get -u ./...
	@go mod tidy

download-modules:
	@go get -u ./...

modules:
	@go mod tidy

clean:
	@rm -rf migrate updater ranker > /dev/null 2>&1
	@rm -rf ranking team teams.json availRanks.json latest.json gameCount.json > /dev/null 2>&1

lint:
	@golangci-lint run --config=.golangci.yml ./cmd/... ./internal/...

local-deploy:
	docker compose up --detach --build --force-recreate updater
