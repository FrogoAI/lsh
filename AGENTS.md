# LSH — Locality-Sensitive Hashing Library

## Project overview

Go library for near-duplicate detection using MinHash + LSH banding.
Core flow: text → shingle → MinHash signature → LSH bands → bucket lookup → Jaccard verification.

## Architecture

```
service.go          – SimilarityService (Upsert entry point, orchestration)
hasher.go           – MinHash signature computation and band hashing
utils.go            – Shingle, Jaccard similarity (exact + estimated)
config.go           – LSH parameters (Bands, Rows, ShingleSize, JaccardThreshold, etc.)
error.go            – Sentinel errors
model/record.go     – Record struct (ID, Input, GroupID, Signature)
repositories/
  storage.go        – Storage interface
  memory/           – In-memory implementation (used in tests)
  aerospike/        – Aerospike implementation (requires external server)
```

## Commands

```sh
# Run tests with coverage
go test -coverprofile=coverage.out -cover -race ./...

# Check coverage threshold (must be ≥70%)
go-test-coverage --config=./.testcoverage.yml

# Lint
golangci-lint run -v ./...
```

## Testing conventions

- **Table-driven tests only**: define a `cases` (or `testcases`) slice of structs, iterate with `t.Run`.
- Use `github.com/FrogoAI/testutils` for assertions (`testutils.Equal`, `testutils.NotEqual`).
- Use `memory.NewRepository()` as the storage backend in unit tests.
- The `repositories/aerospike` package is excluded from coverage (requires external infrastructure).
- Coverage threshold: **70%** total (configured in `.testcoverage.yml`).

## Linting rules

- Linter config: `.golangci.yml` (golangci-lint v2).
- Import order enforced by `gci`: standard → blank → dot → default → `github.com/FrogoAI` prefix.
- `forbidigo`: no `fmt.Print*`, `log.Print*`, or bare `print` in production code.
- `wsl_v5`: blank line required before `return` when preceded by multi-line blocks.
- `mnd` (magic number detector): use named constants; suppress with `// nolint:mnd` where necessary.
- Line length limit: 120 characters.
- Tests are exempt from: `bodyclose`, `dogsled`, `dupl`, `errcheck`, `lll`, `mnd`, `wsl`.

## Code style

- Avoid `fmt.Print*` / `log.Print*` — the linter will reject them.
- Always run `golangci-lint run ./...` before considering work done.
- Nolint directives must specify the linter: `// nolint:gosec`.

## 9. Rules
1. **Use `#AI-assisted` in commit messages**.
2. **Domain packages must have zero infrastructure imports** (no fiber, pgx, etc.).
3. **New reusable infrastructure goes in `pkg/`**, not inside a service.