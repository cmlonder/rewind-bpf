#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

node --check ui/app.js
node --check ui/components/layout.js
node --check ui/components/proof.js
node --check ui/data/fixture.js
node --check ui/data/supervisor-adapter.js
grep -q 'data-action="macos-test-guide"' ui/components/layout.js
grep -q 'make mac-native-smoke' ui/app.js
grep -q 'macos-native' ui/app.js
grep -q 'infoButton("runtime-defaults")' ui/components/layout.js
grep -q 'className = "modal-layer"' ui/app.js
git diff --check

echo "UI_SMOKE_PASS"
