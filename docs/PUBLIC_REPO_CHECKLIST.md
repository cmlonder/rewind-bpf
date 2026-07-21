# Public Repository Checklist

This repository is intended to be public. The source, product documentation,
benchmark ledger, safe synthetic fixtures, release scripts, and public site are
safe to publish. Personal conversation exports, local runtime state, tokens,
private keys, evidence archives, and build outputs are not part of the public
source tree.

## Keep public

- `README.md`, `docs/`, `benchmarks/`, `policies/example.yaml`, `scripts/`
- Go source and tests under `cmd/` and `internal/`
- `ebpf/` source and build instructions (not generated `.o`/BTF files)
- `site/` and `ui/`
- CI and release metadata templates that contain no credentials

## Never commit

- `.env`, credentials, private keys, bearer tokens, token files, or keychain exports
- `dist/`, `bin/`, runtime directories, event JSONL, history/record databases,
  local logs, VM disks, or personal evidence archives
- private conversation exports or notes (`PRIVATE_*.md`, `docs/private/`,
  `notes/private/`, and conversation-export files)
- real project paths, real secret values, personal usernames, or customer data

## Before the first public push

From the repository root:

```bash
make public-audit
make hackathon-preflight
git diff --check
git status --short --ignored
```

Review the staged file list manually. The audit checks the current index and
working tree; it does not replace GitHub secret scanning or a review of old
commits. Existing token-shaped strings in tests must remain clearly synthetic
and must never be replaced with a real credential.

## What this project does not publish

The public repository does not contain the private Codex conversation transcript.
The Devpost `/feedback` Session ID is an intentional submission identifier, not
a credential. It is documented in `docs/DEVPOST_SUBMISSION.md` for the entry
form; no bearer token or secret is stored there.
