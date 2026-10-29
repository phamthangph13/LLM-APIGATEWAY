.PHONY: run build test vet fmt tidy doctor check

BINARY=bin/llm-api-gateway.exe

run:
	go run ./cmd/server

build:
	go build -trimpath -o $(BINARY) ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

doctor:
	go run ./cmd/doctor

check: fmt vet test build
