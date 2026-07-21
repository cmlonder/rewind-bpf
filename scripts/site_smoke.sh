#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

for file in site/app.js site/data.js site/sections/benchmarks.js site/sections/capabilities.js site/sections/footer.js site/sections/header.js site/sections/hero.js site/sections/landscape.js site/sections/roadmap.js site/sections/system.js; do
  node --check "$file"
done

PORT="${REWIND_SITE_SMOKE_PORT:-4179}"
LOG="$(mktemp "${TMPDIR:-/tmp}/rewind-site-smoke.XXXXXX.log")"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -f -- "$LOG"
}
trap cleanup EXIT

python3 -m http.server "$PORT" --directory site >"$LOG" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 30); do
  if body="$(curl -fsS "http://127.0.0.1:$PORT/" 2>/dev/null)" && printf '%s' "$body" | grep -qi 'rewind'; then
    echo "SITE_HTTP_PASS"
    echo "SITE_SMOKE_PASS"
    exit 0
  fi
  sleep 0.1
done

cat "$LOG" >&2
echo "site smoke: HTTP preview did not become ready" >&2
exit 1
