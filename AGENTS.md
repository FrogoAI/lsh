# LSH — Locality-Sensitive Hashing Library (v2)

## Project overview

Go library for **Locality-Sensitive Hashing** with two built-in use cases:

- **String deduplication** (`dedup/`) — MinHash signatures + Jaccard similarity
- **Vector behavioural ID** (`vector/`) — random hyperplane signatures + cosine similarity

Both share a common core (banding, pooling, sharding) and a pluggable storage layer.
Buckets store **representatives** (one per cluster, set semantics) — not full member lists.

## Architecture

```
lsh (root)                Core toolkit: banding, signature pools, lock sharding, prefix hashing
├── dedup/                String dedup use case (MinHash + Jaccard). Service has Upsert().
├── vector/               Vector behavioural ID use case (Hyperplane + Cosine). Service has Upsert().
├── repositories/         Storage interface + implementations
│   ├── memory/           In-memory (for tests)
│   └── aerospike/        Aerospike (for production)
├── source/               Reference methodology documents (read-only, not Go code)
├── testdata/
│   └── aerospike.conf    Aerospike config for integration tests
└── benchmarks/
    └── baseline.txt      Tracked benchmark baseline (rebuild with `make bench-baseline`)
```

### Key files per use case

| File | dedup/ | vector/ |
|---|---|---|
| Service (Upsert) | `service.go` | `service.go` |
| Hasher | `hasher.go` (MinHash) | `hasher.go` (Hyperplane) |
| Similarity | `similarity.go` (Jaccard) | `similarity.go` (Cosine) |
| Config | `config.go` (prefix LSH) | `config.go` (prefix VLSH) |
| Record serde | `record.go` | `record.go` |
| Errors | `error.go` | `error.go` |

## source/ directory

The `source/` directory contains **reference methodology documents** that describe the engineering principles, coding standards, and architectural patterns this library follows. They are **not Go code** — they are markdown guides:

- `Go Library.md` — Blueprint for building reusable Go vendor libraries (folder structure, config, testing, CI)
- `Go Best Practice.md` — Go idioms and coding conventions
- `Clean Code.md` — General clean code principles
- `Engineering Principles.md` — Core engineering values
- `CTO Methodology Guide.md` — Decision-making framework
- `Twelve-Factor App.md` — 12-factor methodology adapted for libraries

**When to consult**: before making architectural decisions, adding new packages, changing the public API, or setting up CI/testing patterns. These documents define the "why" behind the project structure.

## Commands

All shortcuts are in the `Makefile`:

```sh
make test                # Unit tests (no external deps)
make test-integration    # Integration tests (requires Aerospike)
make test-all            # Both
make lint                # golangci-lint
make coverage            # Unit coverage + threshold check (>=70%)
make coverage-integration # Full coverage including Aerospike
make bench               # Run benchmarks -> benchmarks/current.txt (gitignored)
make bench-baseline      # Rebuild tracked baseline (commit after running)
make bench-compare       # Compare current.txt vs baseline.txt via benchstat
make aerospike-start     # Start Aerospike via podman
make aerospike-stop      # Stop Aerospike
make ci                  # lint + coverage (what CI runs)
```

## Testing conventions

- **Table-driven tests only**: define a `cases` (or `testcases`) slice of structs, iterate with `t.Run`.
- Each test must have **at least 5 edge cases** (more is better) plus **1-2 normal behavioral tests**.
- Use `memory.NewRepository()` as the storage backend in unit tests.
- **Integration tests** use build tag `//go:build integration`. CI runs only unit tests.
- The `repositories/aerospike` package is covered by integration tests only.
- Coverage threshold: **70%** total (configured in `.testcoverage.yml`).

## Linting rules

- Linter config: `.golangci.yml` (golangci-lint v2).
- Import order enforced by `gci`: standard -> blank -> dot -> default -> `github.com/FrogoAI` prefix.
- `forbidigo`: no `fmt.Print*`, `log.Print*`, or bare `print` in production code.
- `wsl_v5`: blank line required before `return` when preceded by multi-line blocks.
- `mnd` (magic number detector): use named constants; suppress with `// nolint:mnd` where necessary.
- Line length limit: 120 characters.
- Tests are exempt from: `bodyclose`, `dogsled`, `dupl`, `errcheck`, `lll`, `mnd`, `wsl`.

## Code style

- Avoid `fmt.Print*` / `log.Print*` — the linter will reject them.
- Always run `golangci-lint run ./...` before considering work done.
- Nolint directives must specify the linter: `// nolint:gosec`.

## Error handling

- **Never ignore errors.** Every `error` return must be checked. Using `_ = fn()` is forbidden.
- If an error is **critical** (breaks the operation), return it to the caller.
- If an error is **non-critical** (e.g. cache write failure), log it as a warning with `slog.Warn` and continue execution. Use structured logging with context fields (`slog.String`, `slog.Any`).
- Use `log/slog` (standard library) for logging. Do not use `fmt.Print*` or `log.Print*` (blocked by linter).

## Rules
1. **Use `#AI-assisted` in commit messages**.
2. **Domain packages must have zero infrastructure imports** (no fiber, pgx, etc.).
3. **New reusable infrastructure goes in `pkg/`**, not inside a service.
4. **Consult `source/` documents** before making architectural or API design decisions.
