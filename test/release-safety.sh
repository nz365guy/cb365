#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release.yml"
expected="          args: release --clean --parallelism 1"

matches="$(grep -Fxc "$expected" "$workflow" || true)"
if [[ "$matches" != "1" ]]; then
  echo "release workflow must contain exactly one serialized GoReleaser invocation: $expected" >&2
  exit 1
fi

if grep -Eq '^[[:space:]]*args:[[:space:]]+release[[:space:]]+--clean[[:space:]]*$' "$workflow"; then
  echo "unbounded GoReleaser invocation detected" >&2
  exit 1
fi

echo "release concurrency guardrail verified"
