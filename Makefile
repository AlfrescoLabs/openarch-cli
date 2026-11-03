BINARY  := openarch
PKG     := github.com/AlfrescoLabs/openarch-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)

.PHONY: build test lint fmt tidy generate snapshot clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/openarch

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

# Regenerates the OpenArch API client from api/openapi.json.
generate:
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.json

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
