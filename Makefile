.PHONY: test test-integration test-all lint coverage bench bench-baseline bench-compare clean

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

# Run benchmarks on fresh Aerospike, save as current result (gitignored)
bench: aerospike-stop aerospike-start
	@mkdir -p benchmarks
	@sleep 4
	go test -tags=integration -bench=. -benchmem -count=6 -run="^$$" -timeout=300s ./... \
		| tee benchmarks/current.txt

# Rebuild the tracked baseline on fresh Aerospike (commit after running)
bench-baseline: aerospike-stop aerospike-start
	@mkdir -p benchmarks
	@sleep 4
	go test -tags=integration -bench=. -benchmem -count=6 -run="^$$" -timeout=300s ./... \
		| tee benchmarks/baseline.txt
	@echo "\nBaseline updated. Run 'git add benchmarks/baseline.txt && git commit' to persist."

# Compare current benchmark against baseline
bench-compare:
	@if [ ! -f benchmarks/current.txt ]; then echo "Run 'make bench' first."; exit 1; fi
	benchstat benchmarks/baseline.txt benchmarks/current.txt

# Start Aerospike for integration tests
aerospike-start:
	@podman run -d --name aerospike-test -p 3000:3000 \
		-v $(CURDIR)/testdata/aerospike.conf:/opt/aerospike/etc/aerospike.conf:ro \
		--entrypoint asd docker.io/aerospike/aerospike-server:latest \
		--config-file /opt/aerospike/etc/aerospike.conf \
		&& echo "Aerospike started on localhost:3000"

# Stop Aerospike
aerospike-stop:
	@podman stop aerospike-test && podman rm aerospike-test 2>/dev/null; true

# Full CI check (unit tests + lint + coverage)
ci: lint coverage
