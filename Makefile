.PHONY: build test vet run fmt

build:
	VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.1); go build -trimpath -ldflags "-s -w -X main.Version=$$VERSION" -o bin/go-bot ./cmd/go-bot

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run ./cmd/go-bot
