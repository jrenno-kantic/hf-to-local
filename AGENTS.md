# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI for linking Hugging Face cache snapshots into local model folders for LM Studio and Ollama.

- `cmd/hf-to-local/main.go`: CLI entrypoint, flags, version output, and process exit handling.
- `internal/hftolocal/`: focused packages-by-file for cache discovery, layout generation, link/copy logic, Ollama integration, listing, and shared helpers.
- `internal/hftolocal/hftolocal_test.go`: unit tests using temporary fake HF cache trees.
- `Makefile`: build, test, format, vet, and clean commands.
- `.github/workflows/ci.yml`: Go CI for test, vet, and build.

## Build, Test, and Development Commands

- `make build`: builds `bin/hf-to-local` with trimpath and release-style linker flags.
- `make test`: runs `go test ./...` with a repository-local `GOCACHE`.
- `make vet`: runs `go vet ./...`.
- `make fmt`: runs `gofmt -w cmd internal`.
- `make checksums`: builds cross-platform release archives under `dist/` and writes SHA-256 checksums.
- `go run ./cmd/hf-to-local --help`: runs the CLI locally without building.
- `bin/hf-to-local --target ~/LocalModels --dry-run`: previews filesystem operations.

## Coding Style & Naming Conventions

Use idiomatic Go and keep the project standard-library only unless a dependency clearly pays for itself. Format all Go files with `gofmt`. Public types and functions should use clear exported names, such as `Options`, `Summary`, and `Run`; private helpers should stay narrow and lower camel case. Keep CLI flag names stable and kebab-case, for example `--link-mode` and `--all-snapshots`.

## Testing Guidelines

Tests use Go’s built-in `testing` package. Add tests beside the implementation in `internal/hftolocal/` and name them `Test...`. Prefer temporary directories and fake HF cache layouts over touching real user caches. Cover symlink, copy, hardlink, dry-run, missing repo, and generated `Modelfile` behavior when changing filesystem logic.

## Commit & Pull Request Guidelines

This repository has no established commit history yet. Use concise imperative commit subjects, for example `Add Ollama import script generation`. Pull requests should include a short purpose, key behavior changes, test results, and any compatibility notes for macOS/Linux/Windows filesystem behavior. Include CLI examples when flags or output layouts change.

## Security & Configuration Tips

Do not commit real Hugging Face tokens, downloaded model files, or generated `bin/`/`dist/` artifacts. The tool should not download models or modify the HF cache. Default to `--dry-run` in examples that inspect real user model folders.
