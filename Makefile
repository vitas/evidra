.PHONY: build test clean canon-fixtures-update docker-mcp docker-cli docker-up docker-down fmt lint tidy \
	prompts-generate prompts-verify test-signals \
	ui-build docker-hosted

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X samebits.com/evidra/pkg/version.Version=$(VERSION) -X samebits.com/evidra/pkg/version.Commit=$(COMMIT) -X samebits.com/evidra/pkg/version.Date=$(BUILD_DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/evidra ./cmd/evidra
	go build -ldflags "$(LDFLAGS)" -o bin/evidra-mcp ./cmd/evidra-mcp

test:
	go test ./... -v -count=1

test-signals: build
	PATH="$(PWD)/bin:$$PATH" bash tests/signal-validation/validate-signals-engine.sh

prompts-generate:
	bash scripts/prompts-generate.sh

prompts-verify:
	bash scripts/prompts-verify.sh

canon-fixtures-update:
	EVIDRA_UPDATE_CANON_FIXTURES=1 go test -run TestCanonFixtures -update ./internal/canon/...

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
	cd ui && npm install && npm run build

