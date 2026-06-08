# Repository Guidelines

## Project Structure & Module Organization

Wikimesh is a Go CLI and SDK project. The CLI entrypoint is `cmd/wikimesh`, while command orchestration lives under `internal/cli`. Application services are in `internal/app`, and shared CLI UI, help text, and i18n strings are centralized in `internal/ui`. The public qmd search SDK is in `pkg/qmd`; lower-level packages such as `pkg/qmd/embed`, `pkg/qmd/extract`, `pkg/qmd/index`, and `pkg/qmd/llamaruntime` support it. Runtime skill assets live in `skills/devwiki`. Tests sit beside implementation files as `*_test.go`.

## Build, Test, and Development Commands

- `make build`: build the default binary at `.wikimesh/bin/wikimesh`.
- `make test`: run `go test ./...` with the repository-local Go cache.
- `make package`: package the binary and optional llama.cpp runtime files.
- `go run ./cmd/wikimesh --help`: run the CLI locally without installing it.
- `env -u GOROOT GOCACHE=$(pwd)/.cache/go-build go test ./...`: use this when local Go toolchain caches report version mismatches.

## Coding Style & Naming Conventions

Run `gofmt` on changed Go files. Keep package names short and lowercase. Prefer narrow command packages under `internal/cli/<command>` and public SDK APIs under `pkg/qmd`. Do not expose SQLite, FTS, chunk, vector table, or llama runtime internals through the qmd public API. New or updated Go comments should use Chinese unless an external API or specification requires English. User-facing CLI text belongs in `internal/ui`, not inline in command implementations.

## Testing Guidelines

Use Go's standard `testing` package. Name tests `Test...` and keep them near the code they validate. Add focused tests for CLI behavior, config read/write, qmd search semantics, and public API changes. Before handing off code changes, run `make test` or the explicit `go test ./...` command above.

## Commit & Pull Request Guidelines

The current history is short and mixed (`init`, `finit`, `fix: action编译失败`). Prefer concise, imperative commit messages; use a Conventional Commit prefix such as `fix:` or `feat:` when it clarifies scope. Pull requests should describe the problem, summarize the change, list verification commands, and note any CLI behavior, config, or documentation impact. Include screenshots only for UI-facing changes.

## Agent-Specific Instructions

Check for deeper `AGENTS.md` files before editing subdirectories. Do not overwrite user changes in a dirty worktree. Keep documentation examples aligned with actual `wikimesh --help` output and current Go code.
