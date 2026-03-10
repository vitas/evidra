.PHONY: build test e2e clean golden-update docker-mcp docker-cli docker-api docker-up docker-down fmt lint tidy \
	benchmark-validate benchmark-coverage benchmark-process-artifact benchmark-refresh-contracts benchmark-check-contracts \
	benchmark-detect-duplicates bench-add \
	test-contracts test-mcp-inspector test-mcp-inspector-ci test-mcp-inspector-local-rest test-mcp-inspector-hosted test-mcp-inspector-hosted-rest \
	prompts-generate prompts-verify test-experiments test-signals \
	ui-build build-api

build:
	go build -o bin/evidra ./cmd/evidra
	go build -o bin/evidra-mcp ./cmd/evidra-mcp
	go build -o bin/evidra-api ./cmd/evidra-api

test:
	go test ./... -v -count=1

test-experiments:
	go test ./internal/experiments -count=1
	bash tests/experiments/runners/run_agent_experiments_clean_out_dir_test.sh
	bash tests/experiments/runners/run_agent_execution_experiments_test.sh

test-signals:
	@if [ ! -x bin/evidra ]; then $(MAKE) build; fi
	PATH="$(PWD)/bin:$$PATH" bash tests/signal-validation/validate-signals-engine.sh

e2e: build
	go test -tags e2e ./tests/e2e/ -v -count=1 -timeout=120s

test-contracts: build
	go test -tags e2e ./tests/contracts/ -v -count=1 -timeout=120s

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

benchmark-process-artifact:
	@test -n "$(ARTIFACT)" || (echo "ARTIFACT is required, e.g. make benchmark-process-artifact ARTIFACT=tests/inspector/fixtures/safe-nginx-deployment.yaml" >&2; exit 2)
	bash tests/benchmark/scripts/process-artifact.sh --artifact "$(ARTIFACT)" $(if $(TOOL),--tool $(TOOL)) $(if $(OPERATION),--operation $(OPERATION)) $(if $(OUT),--out $(OUT)) $(if $(EVIDRA_BIN),--evidra-bin $(EVIDRA_BIN))

benchmark-refresh-contracts:
	bash tests/benchmark/scripts/refresh-contracts.sh $(if $(CASE_ID),--case $(CASE_ID)) $(if $(OPERATION),--operation $(OPERATION)) $(if $(EVIDRA_BIN),--evidra-bin $(EVIDRA_BIN))

benchmark-check-contracts:
	bash tests/benchmark/scripts/refresh-contracts.sh --check $(if $(CASE_ID),--case $(CASE_ID)) $(if $(OPERATION),--operation $(OPERATION)) $(if $(EVIDRA_BIN),--evidra-bin $(EVIDRA_BIN))

benchmark-detect-duplicates:
	bash tests/benchmark/scripts/detect-duplicates.sh

bench-add:
	bash scripts/bench-add.sh $(CASE_ID) $(if $(ARTIFACT),--artifact $(ARTIFACT)) $(if $(SOURCE),--source $(SOURCE)) $(if $(TOOL),--tool $(TOOL)) $(if $(OPERATION),--operation $(OPERATION)) $(if $(EVIDRA_BIN),--evidra-bin $(EVIDRA_BIN)) $(if $(NO_PROCESS),--no-process)

prompts-generate:
	bash scripts/prompts-generate.sh

prompts-verify:
	bash scripts/prompts-verify.sh

golden-update:
	EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update ./internal/canon/...

docker-mcp:
	docker build -t evidra-mcp:dev -f Dockerfile .

docker-cli:
	docker build -t evidra:dev -f Dockerfile.cli .

docker-api:
	docker build -t evidra-api:dev -f Dockerfile.api .

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

build-api: ui-build
	go build -tags embed_ui -o bin/evidra-api ./cmd/evidra-api
