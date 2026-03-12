.PHONY: test test-integration test-all lint coverage bench bench-compare clean

# Unit tests (CI default, no external dependencies)
test:
	go test -race -count=1 ./...

# Integration tests (requires Aerospike on localhost:3000)
test-integration:
	go test -tags=integration -race -count=1 -v ./...

# All tests
test-all: test test-integration

# Linter
lint:
	golangci-lint run ./...

# Unit coverage with threshold check
coverage:
	go test -coverprofile=coverage.out -cover -race ./...
	go-test-coverage --config=./.testcoverage.yml

# Full coverage including integration tests
coverage-integration:
	go test -tags=integration -coverprofile=coverage.out -cover -race ./...
	go-test-coverage --config=./.testcoverage.yml

# Run benchmarks and save to benchmarks/ with timestamp
bench:
	@mkdir -p benchmarks
	go test -tags=integration -bench=. -benchmem -count=6 -run="^$$" -timeout=300s ./... \
		| tee benchmarks/$$(date +%Y%m%d_%H%M%S).txt

# Compare latest benchmark against baseline
bench-compare:
	@LATEST=$$(ls -t benchmarks/*.txt | head -1); \
	BASELINE=$$(ls -t benchmarks/*.txt | head -2 | tail -1); \
	echo "Comparing: $$BASELINE (old) vs $$LATEST (new)"; \
	benchstat $$BASELINE $$LATEST

# Start Aerospike for integration tests
aerospike-start:
	@podman run -d --name aerospike-test -p 3000:3000 \
		-v $(CURDIR)/aerospike-test.conf:/opt/aerospike/etc/aerospike.conf:ro \
		--entrypoint asd docker.io/aerospike/aerospike-server:latest \
		--config-file /opt/aerospike/etc/aerospike.conf \
		&& echo "Aerospike started on localhost:3000"

# Stop Aerospike
aerospike-stop:
	@podman stop aerospike-test && podman rm aerospike-test 2>/dev/null; true

# Full CI check (unit tests + lint + coverage)
ci: lint coverage
