# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI for linking Hugging Face cache snapshots into local model folders for LM Studio and Ollama. It is standard-library only and has no module dependencies.

- `cmd/hf-to-local/main.go`: CLI entrypoint. `main` only wires signals and streams; all behavior lives in `run(ctx, args, stdout, stderr) int` so it is testable.
- `cmd/hf-to-local/main_test.go`: flag parsing, exit codes, and output-shape tests.
- `internal/hftolocal/`: one concern per file for cache discovery, layout generation, link/copy logic, Ollama integration, listing, GGUF parsing, name resolution, and shared helpers.
- `internal/hftolocal/hftolocal_test.go`: unit tests over temporary fake HF cache trees.
- `internal/hftolocal/regression_test.go`: guards for previously fixed bugs — broken sources, path containment, repo filtering, Ollama re-runs, cancellation.
- `internal/hftolocal/resolve_test.go`, `resolve_http_test.go`: name resolution, with an injected resolver and `httptest` servers rather than live network calls.
- `Makefile`: build, test, format, vet, dist, checksums, clean.
- `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/`: contributor-facing docs. `CONTRIBUTING.md` restates the invariants below for humans; keep the two in sync.
- `.github/workflows/ci.yml`: test/vet/build across Linux, macOS, and Windows on the current toolchain, plus a build against the `go.mod` floor and `gofmt`/`goreleaser check` gates.
- `.github/workflows/release.yml` and `.goreleaser.yml`: tagged releases, SBOMs, cosign signing, and the Homebrew cask published to `jrenno-kantic/homebrew-tap`.

## Build, Test, and Development Commands

- `make build`: builds `bin/hf-to-local` with trimpath and release-style linker flags.
- `make test`: runs `go test ./...` with a repository-local `GOCACHE`.
- `make vet`: runs `go vet ./...`.
- `make fmt`: runs `gofmt -w cmd internal`. CI fails on unformatted files.
- `make checksums`: builds cross-platform release archives under `dist/` and writes SHA-256 checksums. The target list, naming, and formats intentionally mirror `.goreleaser.yml`; change them together.
- `go test ./internal/hftolocal -run TestName -v`: runs a single test.
- `go test -race ./...`: what CI runs on Linux and macOS.
- `go run ./cmd/hf-to-local --help`: runs the CLI locally without building.
- `bin/hf-to-local --target ~/LocalModels --dry-run`: previews filesystem operations.

## Coding Style & Naming Conventions

Target Go 1.22 (the floor declared in `go.mod`, enforced by the `minimum-go` CI job); do not raise it without a concrete reason, since it gates who can install and contribute. Use idiomatic Go and keep the project standard-library only unless a dependency clearly pays for itself. Format all Go files with `gofmt`. Public types and functions should use clear exported names, such as `Options`, `Summary`, and `Run`; private helpers should stay narrow and lower camel case. Keep CLI flag names stable and kebab-case, for example `--link-mode` and `--all-snapshots`. The create and `list` subcommands own separate `flag.FlagSet`s, so a flag that belongs to both must be registered twice — check for drift when adding one.

## Invariants

These are load-bearing; breaking one is a bug even when tests still pass.

- **All filesystem writes go through `applyLink` or `applyWrite` in `link.go`.** They are the single place that handles dry-run, conflict policy, summary counters, and `Operation` records. Calling `os.Symlink` or `os.WriteFile` directly makes the JSON plan diverge from what a real run does.
- **Nothing is written outside `--target`.** Cache folder names decode `--` to `/`, so a repo id can contain path separators and `..`. Every path element is sanitized through `safeSegment` or `safeRelPath` before being joined onto a destination root.
- **A broken source is skipped, never fatal.** Interrupted downloads routinely leave dangling snapshot→blob symlinks; one must not make the whole cache unusable. Count it, warn on stderr, continue.
- **Generated content that already exists still counts as present.** `applyWrite` reports `created` and `present` separately so re-running against a populated target still finds its Ollama models.
- **Long operations honor cancellation.** `Run` and `List` take a `context.Context` and check it at repo, snapshot, and per-file boundaries. Interrupt exits 130 after printing the partial summary.
- **Only `--resolve-names` touches the network.** Everything else works fully offline.

## Testing Guidelines

Tests use Go's built-in `testing` package. Add tests beside the implementation and name them `Test...`. Build fake HF cache trees in `t.TempDir()` rather than touching the real user cache; `makeHFCache` and `writeMinimalGGUF` synthesize valid fixtures. Fixtures that need symlinks call `requireSymlinks`, which skips rather than fails where symlink creation is unprivileged — this is what keeps Windows CI meaningful.

When changing filesystem logic, cover symlink, hardlink, copy, dry-run, each conflict policy, and generated `Modelfile` behavior. When fixing a bug, add the regression test to `regression_test.go`. Network behavior is tested against `httptest` servers and injected resolvers; no test may reach the real Hugging Face API.

## Commit & Pull Request Guidelines

Use concise imperative commit subjects, for example `Add Ollama import script generation`. Pull requests should include a short purpose, key behavior changes, test results, and any compatibility notes for macOS/Linux/Windows filesystem behavior. Include CLI examples when flags or output layouts change, and update `README.md` in the same change — flag names and output formats are documented there and treated as stable API.

## Security & Configuration Tips

Do not commit real Hugging Face tokens, downloaded model files, or generated `bin/`/`dist/` artifacts. The tool must not download models or modify the HF cache. Default to `--dry-run` in examples that inspect real user model folders. GitHub Actions are pinned to commit SHAs with the version in a trailing comment; keep them pinned when upgrading.
