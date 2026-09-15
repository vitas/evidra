.PHONY: build test clean docker-mcp docker-cli docker-up docker-down fmt lint tidy ui-build ui-lint docker-hosted

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X samebits.com/evidra/pkg/version.Version=$(VERSION) -X samebits.com/evidra/pkg/version.Commit=$(COMMIT) -X samebits.com/evidra/pkg/version.Date=$(BUILD_DATE)
GO_PACKAGES := ./cmd/... ./pkg/...

build:
	go build -ldflags "$(LDFLAGS)" -o bin/evidra ./cmd/evidra
	go build -ldflags "$(LDFLAGS)" -o bin/evidra-mcp ./cmd/evidra-mcp

test:
	go test $(GO_PACKAGES) -v -count=1

docker-mcp:
	docker build -t evidra-mcp:dev -f Dockerfile .

docker-cli:
	docker build -t evidra:dev -f Dockerfile.cli .

docker-hosted:
	docker build -t evidra-mcp-hosted:dev -f Dockerfile.hosted .

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin/

ui-build:
	cd ui && npm ci && npm run build

ui-lint:
	cd ui && npm run lint && npm run typecheck
