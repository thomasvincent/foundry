.PHONY: build test lint clean install fmt vet all

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"

build:
	go build $(LDFLAGS) -o bin/anvil ./cmd/anvil

install:
	go install $(LDFLAGS) ./cmd/anvil

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ .foundry/out/

fmt:
	gofmt -w .
	goimports -w .

vet:
	go vet ./...

all: lint test build
