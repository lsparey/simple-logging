.PHONY: generate build test lint tidy

## generate: regenerate gRPC/protobuf Go stubs from proto/
##   Requires: buf (https://buf.build/docs/installation)
##   Outputs Go stubs to gen/  (buf.gen.yaml lives in proto/)
generate:
	cd ../proto && buf generate

## build: compile the server binary into bin/ (embeds whatever is in
##   internal/ui/dist; run `make build` at the repo root to include the UI)
build:
	go build -trimpath -ldflags="-s -w" -o bin/simple-logging ./cmd/server

## test: run all server tests
test:
	go test ./...

## lint: run go vet on the server module
lint:
	go vet ./...

## tidy: tidy the server Go module
tidy:
	go mod tidy
