# hf-to-local

Create local model folders that point at files already downloaded in the Hugging Face Hub cache.

The main use case is reusing downloaded HF models with local runtimes without copying multi-GB files:

- LM Studio: creates a folder tree under a models directory and links `.gguf` files there.
- Ollama: creates import folders with a linked model file or safetensors snapshot plus a `Modelfile`.
- Generic mirror: creates a readable tree of links for each selected HF snapshot.

Hugging Face stores Hub downloads under `~/.cache/huggingface/hub` by default. Model repositories appear as folders like `models--org--repo`, with `refs`, `snapshots`, and `blobs` below them. This tool reads that layout and creates links in a target directory.

## Install

On macOS, install from the Homebrew tap:

```bash
brew install jrenno-kantic/tap/hf-to-local
```

Or tap first:

```bash
brew tap jrenno-kantic/tap
brew install hf-to-local
```

Homebrew supports casks on macOS only; on Linux and Windows use the release
archive below.

Download an archive for your platform from the
[releases page](https://github.com/jrenno-kantic/hf_to_local/releases), then
extract and place the binary on your `PATH`:

```bash
VERSION=0.2.0
OS=$(uname -s | tr '[:upper:]' '[:lower:]')   # darwin or linux
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

curl -sSfL -o hf-to-local.tar.gz \
  "https://github.com/jrenno-kantic/hf_to_local/releases/download/v$VERSION/hf-to-local_${VERSION}_${OS}_${ARCH}.tar.gz"
tar -xzf hf-to-local.tar.gz
sudo install hf-to-local /usr/local/bin/
```

Windows archives are `.zip` rather than `.tar.gz`.

### Verifying a download

Every release publishes `checksums.txt`, a cosign signature over it, and an SBOM
per archive. Check the archive against the checksum file:

```bash
curl -sSfLO "https://github.com/jrenno-kantic/hf_to_local/releases/download/v$VERSION/checksums.txt"
shasum -a 256 -c checksums.txt --ignore-missing
```

The checksum file itself is signed with [cosign](https://docs.sigstore.dev/) in
keyless mode, so the signature is tied to the release workflow's identity rather
than to a long-lived key:

```bash
curl -sSfLO "https://github.com/jrenno-kantic/hf_to_local/releases/download/v$VERSION/checksums.txt.sig"
curl -sSfLO "https://github.com/jrenno-kantic/hf_to_local/releases/download/v$VERSION/checksums.txt.pem"

cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp '^https://github.com/jrenno-kantic/hf_to_local/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Building from source is covered under [Build](#build).

## Cache Location

By default the tool reads the Hub cache resolved in this order:

1. `HF_HUB_CACHE` if set.
2. `$HF_HOME/hub` if `HF_HOME` is set.
3. `~/.cache/huggingface/hub`.

Override it explicitly with `--cache` on either command:

```bash
bin/hf-to-local list --cache /data/hf/hub
bin/hf-to-local --target ~/LocalModels --cache /data/hf/hub
```

## Build

```bash
make build
```

The binary is written to:

```text
bin/hf-to-local
```

Run the test suite:

```bash
make test
```

Run without installing:

```bash
go run ./cmd/hf-to-local --help
```

Install into your Go binary path:

```bash
go install ./cmd/hf-to-local
```

## Examples

Inspect cached models before creating links:

```bash
bin/hf-to-local list
bin/hf-to-local list --repo TheBloke/Llama-2-7B-GGUF
bin/hf-to-local list --json
bin/hf-to-local list --llama-cpp-only
bin/hf-to-local list --resolve-names
```

Some models were cached under a legacy org-less id (for example `camembert-base`
before it moved under the `almanach` organization on the Hub). The cache folder
keeps its original name, so `list` reports the bare id by default. Pass
`--resolve-names` to look the canonical `org/name` up via the Hugging Face API;
resolved names are memoized on disk, and the bare id is kept as-is when offline
or when the lookup is ambiguous. The same flag is available when creating links,
where the canonical id also determines the `namespace/name` of the generated
target folders.

Preview all cached model links:

```bash
bin/hf-to-local --target ~/LocalModels --dry-run
bin/hf-to-local --target ~/LocalModels --dry-run --resolve-names
bin/hf-to-local --target ~/LocalModels --runtime ollama --plan json
```

Create LM Studio folders for cached GGUF models:

```bash
bin/hf-to-local --target ~/LocalModels --runtime lmstudio
bin/hf-to-local --target ~/LocalModels --runtime lmstudio --llama-cpp-only
```

Then set LM Studio's models directory to:

```text
~/LocalModels/lmstudio
```

Create Ollama import folders:

```bash
bin/hf-to-local --target ~/LocalModels --runtime ollama
```

Follow progress on a large cache:

```bash
bin/hf-to-local --target ~/LocalModels --verbose
```

`--verbose` prints a per-repo progress line to stderr, so it stays out of
`--plan json` when that is piped. It applies to the create command; `list` does
its work without writing anything and has no progress output.

Create the Ollama models immediately after generating import folders:

```bash
bin/hf-to-local --target ~/LocalModels --runtime ollama --create
```

Customize generated Ollama model names:

```bash
bin/hf-to-local --target ~/LocalModels --runtime ollama --ollama-name-template "local-{repo}-{quant}"
```

Supported template fields are `{repo}`, `{repo_slug}`, `{quant}`, and `{revision}`.

For GGUF files, each generated folder contains:

```text
model.gguf -> /path/to/hf/cache/.../model.gguf
mmproj-*.gguf -> /path/to/hf/cache/.../mmproj-*.gguf
mtp-*.gguf -> /path/to/hf/cache/.../mtp-*.gguf
Modelfile
```

Companion GGUF files are included when present. They stay beside the primary model file and are not turned into separate Ollama models.

Import one into Ollama:

```bash
cd ~/LocalModels/ollama/org--repo/q4_k_m
ollama create org-repo-q4-k-m -f Modelfile
ollama run org-repo-q4-k-m
```

The command also writes `create-all.sh` under `~/LocalModels/ollama` with the suggested `ollama create` commands.

## Selecting Models

Link one repository:

```bash
bin/hf-to-local --target ~/LocalModels --repo mistralai/Mistral-7B-Instruct-v0.3
```

Link more than one repository:

```bash
bin/hf-to-local --target ~/LocalModels --repo TheBloke/Llama-2-7B-GGUF --repo bartowski/Qwen2.5-7B-Instruct-GGUF
```

Use a specific ref or snapshot:

```bash
bin/hf-to-local --target ~/LocalModels --revision main
bin/hf-to-local --target ~/LocalModels --revision 0123456789abcdef
```

By default, the tool uses `refs/main` when available, otherwise the newest cached snapshot folder.

With `--resolve-names`, `--repo` accepts either the id the cache folder encodes
or the canonical one:

```bash
bin/hf-to-local list --repo camembert-base
bin/hf-to-local list --repo almanach/camembert-base --resolve-names
```

Process every cached snapshot instead of a single one per repo:

```bash
bin/hf-to-local --target ~/LocalModels --all-snapshots
bin/hf-to-local list --revision main --all-snapshots
```

On `list`, `--all-snapshots` shows every cached snapshot when `--revision` is set; on the create command it links all snapshots rather than the selected one.

Keep only text-generation GGUF models recognized by llama.cpp:

```bash
bin/hf-to-local list --llama-cpp-only
bin/hf-to-local --target ~/LocalModels --runtime lmstudio --llama-cpp-only
bin/hf-to-local --target ~/LocalModels --runtime ollama --llama-cpp-only
```

This reads GGUF metadata from each primary `.gguf` file and keeps only known llama.cpp text-generation architectures, such as Llama, Mistral, Qwen, Gemma, Phi, Falcon, MPT, and similar decoder or recurrent language models. It ignores BERT/ModernBERT/Nomic BERT, CLIP, embedding-only architectures, tokenizer-only snapshots, generic unknown architectures, and Ollama's safetensors fallback.

Companion files named `mmproj-*.gguf` or `mtp-*.gguf` are kept when a snapshot also contains a compatible primary model. These files are linked alongside the primary model for LM Studio and Ollama, but they do not make a snapshot compatible by themselves and do not create standalone Ollama `Modelfile` entries.

## Link Modes

The default mode is `symlink`.

```bash
bin/hf-to-local --target ~/LocalModels --link-mode symlink
bin/hf-to-local --target ~/LocalModels --link-mode hardlink
bin/hf-to-local --target ~/LocalModels --link-mode copy
```

Use `copy` only when you really want another physical copy of the model files.

## Conflict Handling

Existing destination files are skipped by default.

```bash
bin/hf-to-local --target ~/LocalModels --on-conflict skip
bin/hf-to-local --target ~/LocalModels --on-conflict replace
bin/hf-to-local --target ~/LocalModels --on-conflict fail
```

`--overwrite` is kept as shorthand for `--on-conflict replace`.

## Reporting

Every run reports linked files, generated files, skipped destinations, referenced bytes, and broken source symlinks. `list --json` and `--plan json` provide machine-readable data for scripts or GUI wrappers.

A cache entry whose snapshot symlink no longer resolves to a blob — common after
an interrupted download — is counted, reported on stderr, and skipped. It does
not stop the run.

Exit codes:

```text
0    success
1    error
2    usage error
130  interrupted (SIGINT/SIGTERM)
```

On interrupt the tool stops at the next file boundary and still prints the
summary for the work already done, so the state of the target directory is
never a mystery.

## Release Builds

Build local platform binary:

```bash
make build
```

Build local release archives for supported platforms:

```bash
make checksums
```

This writes platform archives and `checksums.txt` under `dist/`. The target
list, archive naming, and archive formats match `.goreleaser.yml`, so this is a
faithful rehearsal of a tagged release. It does not produce signatures or SBOMs,
which are generated only by the release workflow.

Tagged releases are handled by GoReleaser in GitHub Actions:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The release workflow additionally generates an SBOM per archive and signs
`checksums.txt` with keyless cosign. `.goreleaser.yml` is validated by `goreleaser
check` on every CI run, so a broken release config surfaces before tag time.

Print the embedded version:

```bash
bin/hf-to-local --version
```

## Notes

- Small metadata files (`README.md`, `LICENSE*`, `config.json`, `generation_config.json`, and the `tokenizer*` files) are linked beside GGUF models by default. Pass `--no-include-metadata` to link only the model files.
- LM Studio primarily consumes GGUF model files in this workflow.
- Ollama can import a GGUF file with `FROM /path/to/file.gguf` or safetensors weights with `FROM /path/to/safetensors/directory`.
- `--llama-cpp-only` disables the Ollama safetensors fallback and keeps only compatible primary GGUF models plus companion `mmproj-*.gguf` and `mtp-*.gguf` files.
- `--resolve-names` (available on both `list` and the create command) is the sole feature that reaches the network; every other operation is fully offline. Lookups are retried once on a rate-limit or server error, the whole resolution pass is bounded by an overall time budget, and anything unresolved keeps its cached id and is retried on a later run. It only queries the Hub for org-less cache folders, follows the API redirect to the canonical `org/name`, and caches the mapping under the user cache dir (`repo-names.json`). On the create command it also drives the `namespace/name` layout of the generated target folders.
- Hardlinks cannot cross filesystems. On Windows, symlink creation may require Developer Mode or elevated privileges.
- The tool does not download models. Use Hugging Face tooling first, then run this tool against the populated cache.
