.PHONY: build test e2e clean golden-update docker-mcp docker-cli fmt lint tidy \
	benchmark-validate benchmark-coverage bench-add \
	test-mcp-inspector test-mcp-inspector-ci test-mcp-inspector-local-rest test-mcp-inspector-hosted test-mcp-inspector-hosted-rest

build:
	go build -o bin/evidra ./cmd/evidra
	go build -o bin/evidra-mcp ./cmd/evidra-mcp

test:
	go test ./... -v -count=1

e2e: build
	go test -tags e2e ./tests/e2e/ -v -count=1 -timeout=120s

test-mcp-inspector:
	bash tests/inspector/run_inspector_tests.sh

test-mcp-inspector-ci:
	mkdir -p tests/inspector/out
	bash -o pipefail -c 'bash tests/inspector/run_inspector_tests.sh | tee tests/inspector/out/latest.log'

test-mcp-inspector-local-rest:
	EVIDRA_TEST_MODE=local-rest bash tests/inspector/run_inspector_tests.sh

test-mcp-inspector-hosted:
	EVIDRA_TEST_MODE=hosted-mcp bash tests/inspector/run_inspector_tests.sh

test-mcp-inspector-hosted-rest:
	EVIDRA_TEST_MODE=hosted-rest bash tests/inspector/run_inspector_tests.sh

benchmark-validate:
	bash tests/benchmark/scripts/validate-dataset.sh

benchmark-coverage:
	bash tests/benchmark/scripts/generate-coverage.sh > tests/benchmark/COVERAGE.md

bench-add:
	bash scripts/bench-add.sh $(CASE_ID) $(if $(ARTIFACT),--artifact $(ARTIFACT)) $(if $(SOURCE),--source $(SOURCE))

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
