FROM golang:1.26-alpine as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

ADD cmd ./cmd
ADD internal ./internal
RUN go build -o updater ./cmd/updater

FROM node:24-alpine AS updater
WORKDIR /

RUN apk add --no-cache tzdata git
ENV TZ=America/New_York
RUN corepack enable

COPY scripts/deploy-web.sh /scripts/deploy-web.sh
RUN chmod +x /scripts/deploy-web.sh

COPY --from=builder /app/updater .
ENTRYPOINT ["/updater", "schedule"]
