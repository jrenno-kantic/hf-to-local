# Contributing

Thanks for your interest in hf-to-local. This is a small, focused tool, and the
bar for changes is mostly about not breaking the guarantees people rely on.

## Getting started

```bash
git clone https://github.com/jrenno-kantic/hf_to_local
cd hf_to_local
make build          # -> bin/hf-to-local
make test
```

Go 1.22 or newer. No dependencies to install: the project is standard-library
only, and that is deliberate — a new dependency needs to clearly pay for itself.

Run a single test:

```bash
go test ./internal/hftolocal -run TestOllamaCreatesSurviveRerun -v
```

Before opening a pull request:

```bash
make fmt && make vet && go test -race ./...
```

CI runs the same on Linux, macOS, and Windows, plus a `gofmt` gate, a build
against the minimum Go version, and `goreleaser check`.

## Guarantees to preserve

These are load-bearing. Breaking one is a bug even when the tests still pass.

- **The tool never downloads models and never modifies the Hugging Face cache.**
  It reads the cache and writes elsewhere. That is the whole contract.
- **Nothing is written outside `--target`.** Cache folder names encode `/` as
  `--`, so a repo id can contain path separators and `..`. Every path element
  goes through `safeSegment` or `safeRelPath` before being joined.
- **All filesystem writes go through `applyLink` or `applyWrite`** in
  `internal/hftolocal/link.go`. They are the single place handling dry-run,
  conflict policy, summary counters, and the JSON plan. Calling `os.Symlink` or
  `os.WriteFile` directly makes `--dry-run` lie about what a real run does.
- **A broken source is skipped, never fatal.** Interrupted downloads leave
  dangling snapshot→blob symlinks routinely; one must not sink the whole run.
- **Only `--resolve-names` touches the network.** Everything else works offline,
  and no test may reach the real Hugging Face API.

## Testing

Build fake cache trees in `t.TempDir()` with the existing helpers rather than
pointing tests at a real user cache. Fixtures needing symlinks call
`requireSymlinks`, which skips where symlink creation is unprivileged — that is
what keeps the Windows CI leg meaningful instead of permanently red.

When changing filesystem logic, cover symlink, hardlink, copy, dry-run, and each
conflict policy. When fixing a bug, add a guard to `regression_test.go`.

## Pull requests

Use concise imperative commit subjects, for example
`Add Ollama import script generation`. In the description, cover the purpose,
the behavior change, your test results, and any macOS/Linux/Windows filesystem
differences.

CLI flag names and output formats are documented in `README.md` and treated as
stable API — update the README in the same change, and include example
invocations when flags or output layouts change.
