#!/usr/bin/env bash
# Fail on identifiers that do not exist — the bug class that ships a blank page.
#
# `vite build` happily bundles a component that calls an un-imported function:
# Svelte compiles the template, and the ReferenceError only appears at runtime,
# in the browser, as an endless loading skeleton. That is exactly how a missing
# `productIcon` import reached a review.
#
# The full type check on this untyped JS codebase reports ~58 pre-existing
# inference complaints, so it cannot gate anything yet. Filter to the one class
# that is always a real, page-breaking bug.
set -uo pipefail
cd "$(dirname "$0")/.."

out=$(npx --no-install svelte-check --tsconfig ./jsconfig.strict.json --output human 2>&1) || true

bad=$(grep -E "Cannot find name '" <<<"$out" || true)
if [[ -n "$bad" ]]; then
  echo "undefined identifiers (missing import?):"
  echo "$bad"
  exit 1
fi
echo "no undefined identifiers"
