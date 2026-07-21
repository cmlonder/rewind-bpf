#!/usr/bin/env bash
set -euo pipefail

# Public-repository gate. This is intentionally conservative: it checks the
# index and working tree for accidental credentials, personal absolute paths,
# private-note filenames, and generated runtime data. It does not claim to
# replace a hosted secret scanner or a full git-history review.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail=0

if git grep -n -I -E '(-----BEGIN .*PRIVATE KEY-----|sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16})' -- ':!go.sum' ':!scripts/public_repo_audit.sh'; then
  echo 'PUBLIC_AUDIT_FAIL=credential-like literal found' >&2
  fail=1
fi

personal_paths="$(git grep -n -I -E '/Users/[A-Za-z0-9._-]+/|/home/[A-Za-z0-9._-]+/Dev/' -- ':!docs/ARCHITECTURE.md' ':!README.md' || true)"
personal_paths="$(printf '%s\n' "$personal_paths" | grep -v '/Users/Shared/' || true)"
if [[ -n "$personal_paths" ]]; then
  printf '%s\n' "$personal_paths"
  echo 'PUBLIC_AUDIT_FAIL=personal absolute path found' >&2
  fail=1
fi

if git ls-files | grep -E '(^|/)(PRIVATE_|.*_PRIVATE\.md$|docs/private/|notes/private/|.*\.token$|.*\.pem$|.*\.key$)' ; then
  echo 'PUBLIC_AUDIT_FAIL=private or secret file tracked' >&2
  fail=1
fi

if git status --short --ignored | grep -E '(^!!|^\?\?) .*\.(events\.jsonl|audit\.jsonl|history\.json|record\.json|sqlite3?|db|token)$' ; then
  echo 'PUBLIC_AUDIT_INFO=runtime artifacts are ignored' >&2
fi

git diff --check
test -s README.md
test -s policies/example.yaml

if (( fail != 0 )); then
  exit 1
fi

echo 'PUBLIC_REPO_AUDIT=PASS'
