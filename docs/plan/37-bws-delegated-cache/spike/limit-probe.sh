#!/usr/bin/env bash
# Gate 1 support script for cb365 #42 (ADR-0057): empirically verify the
# effective BWS secret-value PLAINTEXT limit against the EU tenancy.
#
# Safety contract:
#   * Uses ONLY non-secret filler data ('A' repeated N times) — never
#     credential material, so ADR-0057's "no cache via BWS CLI/argv" rule is
#     not engaged.
#   * Operates on ONE dedicated throwaway secret record in the approved test
#     project, and deletes it at the end (verified).
#   * Requires explicit operator (Mark) approval before running — see issue
#     cb365 #42 "Security/secrets impact".
#
# Usage: BWS_ACCESS_TOKEN=... ./limit-probe.sh <THROWAWAY_PROJECT_ID>
# The access token comes from the VM's existing BWS EU runtime injection
# boundary; this script never prints it. Server endpoint comes from the
# machine's existing bws profile (EU per ADR-0010).
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p evidence

PROJECT_ID="${1:?usage: limit-probe.sh <THROWAWAY_PROJECT_ID>}"
OUT="evidence/bws-limit-probe.txt"
KEY="cb365-spike42-limit-probe-throwaway"

filler() { head -c "$1" /dev/zero | tr '\0' 'A'; }

echo "# BWS secret-value plaintext limit probe — cb365 #42" | tee "$OUT"
echo "date_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$OUT"
echo "bws: $(bws --version)" | tee -a "$OUT"
echo "project_id: $PROJECT_ID" | tee -a "$OUT"

SECRET_ID=$(bws secret create "$KEY" "probe-init" "$PROJECT_ID" -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
echo "throwaway_secret_id: $SECRET_ID" | tee -a "$OUT"

cleanup() {
	bws secret delete "$SECRET_ID" -o none && echo "throwaway_record_deleted: yes" | tee -a "$OUT" || echo "throwaway_record_deleted: NO — DELETE MANUALLY" | tee -a "$OUT"
	if bws secret get "$SECRET_ID" -o none 2>/dev/null; then
		echo "delete_verified: NO — record still exists" | tee -a "$OUT"
	else
		echo "delete_verified: yes" | tee -a "$OUT"
	fi
}
trap cleanup EXIT

try_size() {
	local n=$1
	if bws secret edit --value "$(filler "$n")" "$SECRET_ID" -o none 2>>"$OUT.err"; then
		echo "attempt plaintext_bytes=$n result=accepted" | tee -a "$OUT"
		return 0
	else
		echo "attempt plaintext_bytes=$n result=rejected" | tee -a "$OUT"
		return 1
	fi
}

LO=1000; HI=35000
try_size "$LO" || { echo "FATAL: lower bound $LO rejected" | tee -a "$OUT"; exit 1; }
if try_size "$HI"; then
	echo "verified_plaintext_limit_bytes: >=$HI (upper bound accepted)" | tee -a "$OUT"
	exit 0
fi
while [ $((HI - LO)) -gt 64 ]; do
	MID=$(((LO + HI) / 2))
	if try_size "$MID"; then LO=$MID; else HI=$MID; fi
done
echo "verified_plaintext_limit_bytes: $LO (largest accepted; smallest rejected: $HI)" | tee -a "$OUT"
