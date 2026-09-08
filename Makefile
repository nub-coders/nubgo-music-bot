.PHONY: build test race run clean

BINARY := bin/musicbot

build:
	mkdir -p bin
	CGO_ENABLED=1 go build -trimpath -o $(BINARY) ./cmd/musicbot

test:
	CGO_ENABLED=1 go test ./...

race:
	CGO_ENABLED=1 go test -race ./internal/...

run: build
	./$(BINARY)

clean:
	rm -rf bin cache
