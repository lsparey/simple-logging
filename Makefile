.PHONY: generate build test lint tidy

## generate: regenerate gRPC/protobuf Go stubs from proto/
##   Requires: buf (https://buf.build/docs/installation)
generate:
	buf generate

## build: compile the server binary
build:
	go build -trimpath -ldflags="-s -w" -o bin/simple-logging ./cmd/server

## test: run all tests
test:
	go test ./...

## lint: run go vet
lint:
	go vet ./...

## tidy: tidy go modules
tidy:
	go mod tidy
