#!/usr/bin/env bash
# Gate 2 evidence script for cb365 #42 (ADR-0057). Run on the target
# toolchain (Linux OpenClaw VM). Produces evidence/build-<host>.txt with
# toolchain versions, pinned dependency checksums, both build variants and
# their runtime probe output, and a secret-sentinel check.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p evidence bin

HOST_TAG="${1:-linux-openclaw-vm}"
OUT="evidence/build-${HOST_TAG}.txt"

{
	echo "# Gate 2 build evidence — cb365 #42 / ADR-0057"
	echo "date_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "host: $(uname -srm)"
	echo "go: $(go version)"
	echo "cc: $(gcc --version | head -1)"
	echo

	echo "## Pinned dependencies (go.sum)"
	go mod download
	go mod verify
	grep -E 'bitwarden|AzureAD' go.sum
	echo

	echo "## Native artefact digest (statically linked by cgo build)"
	LIBA="$(go env GOMODCACHE)/github.com/bitwarden/sdk-go/v2@v2.1.0/internal/cinterface/lib/linux-x64/libbitwarden_c.a"
	sha256sum "$LIBA"
	echo

	echo "## Variant 1: CGO_ENABLED=1 (managed provider must construct)"
	CGO_ENABLED=1 go build -o bin/buildprobe-cgo ./cmd/buildprobe
	./bin/buildprobe-cgo
	echo "exit_code: $?"
	echo

	echo "## Variant 2: CGO_ENABLED=0 (fail-closed stub must compile and refuse)"
	CGO_ENABLED=0 go build -o bin/buildprobe-nocgo ./cmd/buildprobe
	./bin/buildprobe-nocgo
	echo "exit_code: $?"
	echo

	echo "## vet (both variants)"
	CGO_ENABLED=1 go vet ./...
	CGO_ENABLED=0 go vet ./...
	echo "vet: ok"
} 2>&1 | tee "$OUT"

# JWT sentinel must not appear anywhere in spike artefacts (issue #42 validation).
if grep -R --binary-files=without-match -n 'eyJ' evidence/ ; then
	echo "SENTINEL FAILURE: 'eyJ' found in evidence output" >&2
	exit 1
fi
echo "sentinel_check: no 'eyJ' in evidence/" | tee -a "$OUT"
