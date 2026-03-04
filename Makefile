.PHONY: build test clean golden-update docker-mcp docker-cli fmt lint tidy

build:
	go build -o bin/evidra ./cmd/evidra
	go build -o bin/evidra-mcp ./cmd/evidra-mcp

test:
	go test ./... -v -count=1

golden-update:
	EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update ./internal/canon/...

docker-mcp:
	docker build -t evidra-mcp:dev -f Dockerfile .

docker-cli:
	docker build -t evidra:dev -f Dockerfile.cli .

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin/
