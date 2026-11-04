.PHONY: build test vet run fmt

build:
	go build -trimpath -ldflags "-s -w" -o bin/go-bot ./cmd/go-bot

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run ./cmd/go-bot
