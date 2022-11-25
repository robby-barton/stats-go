FROM golang:1.18-alpine as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

ADD cmd ./cmd
ADD internal ./internal
RUN go build -o server ./cmd/server
RUN go build -o updater ./cmd/updater

FROM alpine:latest as updater
WORKDIR /

RUN apk add --no-cache tzdata
ENV TZ=America/New_York

COPY --from=builder /app/updater .
ENTRYPOINT ["/updater", "-s"]
