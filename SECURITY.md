# Security Policy

## Supported versions

Fixes land on the latest release. There are no maintained older release lines.

## Reporting a vulnerability

Please report privately rather than opening a public issue.

Use [GitHub private vulnerability reporting](https://github.com/jrenno-kantic/hf_to_local/security/advisories/new),
or email **jacky.renno@kantic-analytics.com**.

Please include what you were running, the cache and target layout involved, and
what happened versus what you expected. A reproducer built from a synthetic
cache tree is ideal — please do not send real model files.

Expect an initial response within about a week. This is a small project
maintained in spare time; there is no formal SLA.

## What is in scope

This tool reads a Hugging Face cache and writes links into a target directory,
so the interesting failure modes are about staying inside those boundaries:

- Writing anything outside `--target`, including via crafted cache folder names
  (these decode `--` to `/`, so a repo id can contain `..`).
- Modifying, deleting, or corrupting anything inside the Hugging Face cache.
- Unexpected network activity. Only `--resolve-names` should ever reach the
  network, and only to `huggingface.co`.
- Executing anything from cache contents. The only subprocess the tool ever runs
  is `ollama create`, and only under `--create`.

## What is not in scope

- The contents of models you downloaded. This tool links files; it does not
  inspect or vouch for them.
- Anything `ollama` does with a generated `Modelfile`.
- Requiring elevated privileges to create symlinks on Windows. That is a
  platform behavior, documented in the README.
