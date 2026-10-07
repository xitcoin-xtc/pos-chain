#!/usr/bin/env bash
set -euo pipefail

# Current bounded technical decision, linked to exact locks and explanation.
# Historical qualification retains its original expired review and false
# global acceptance. No independent approval or release acceptance is implied.
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
review_dates="$(python3 "$repo_root/scripts/verify-security-assessment.py")"
read -r assessed_on review_by <<< "$review_dates"
today="$(date -u +%F)"
if [[ ! "$today" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ || "$today" < "$assessed_on" ]]; then
  echo 'Current assessment is not yet valid or the date is invalid' >&2
  exit 1
fi
if [[ "$today" > "$review_by" ]]; then
  echo "govulncheck exception review expired on $review_by" >&2
  review_expired=true
else
  review_expired=false
fi

accepted=(
  GO-2023-1821 # Deprecated crisis code is outside the reviewed production imports/registration.
  GO-2023-1881 # ConstantFee defect: distinct advisory; crisis remains outside production.
  GO-2024-2584 # Fix source is present in SDK 0.54.4; Source/OSV discrepancy is assessed in SECURITY-ASSESSMENT.md.
  GO-2025-3442 # v0.39.4 contains the SetPeerRange fix; broad OSV range still reports it.
  GO-2026-4479 # Residual v2 module only; effective STUN v3/DTLS v3 imports are reviewed.
  GO-2026-5932 # SDK now uses Proton armor; obsolete OpenPGP is a test oracle only.
)

# Lock the same module that govulncheck scans, including calls from evmd.
export GOWORK=off
module_file="$(go env GOMOD)"
case "$module_file" in
  "$repo_root/go.mod"|"$repo_root/evmd/go.mod") ;;
  *) echo 'Run this gate from the root or evmd module' >&2; exit 1 ;;
esac
module_dir="$(dirname "$module_file")"

require_module() {
  local module=$1 expected=$2 actual
  actual="$(cd "$module_dir" && go list -m -f '{{.Version}}' "$module")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'Reviewed govulncheck exception invalidated: %s is %s, expected %s\n' \
      "$module" "$actual" "$expected" >&2
    exit 1
  fi
}

require_replacement() {
  local module=$1 expected_path=$2 expected_version=$3 actual
  actual="$(cd "$module_dir" && go list -m -f '{{if .Replace}}{{.Replace.Path}} {{.Replace.Version}}{{end}}' "$module")"
  if [[ "$actual" != "$expected_path $expected_version" ]]; then
    printf 'Reviewed govulncheck exception invalidated: replacement for %s is %s\n' \
      "$module" "${actual:-absent}" >&2
    exit 1
  fi
}

require_module github.com/cosmos/cosmos-sdk v0.54.4
require_module github.com/ethereum/go-ethereum v1.16.9
require_module github.com/cometbft/cometbft v0.39.4
require_module github.com/pion/dtls/v2 v2.2.12
require_module golang.org/x/crypto v0.56.0
require_replacement github.com/cosmos/cosmos-sdk github.com/xitcoin-xtc/cosmos-sdk v0.54.5-0.20260914091530-52ff14a25bee
require_replacement github.com/ethereum/go-ethereum github.com/xitcoin-xtc/go-ethereum v1.17.2-cosmos-0.0.20260913233706-e09f79643cd4
require_module github.com/pion/stun/v3 v3.1.5
require_module github.com/pion/dtls/v3 v3.1.4
require_module github.com/ProtonMail/go-crypto v1.4.1
# Pin both archive and go.mod sums for evmd's already qualified root library.
if [[ "$module_file" == "$repo_root/evmd/go.mod" ]]; then
  root_sums="$(go list -m -f '{{.Sum}} {{.GoModSum}}' github.com/xitcoin-xtc/pos-chain)"
  if [[ "$root_sums" != 'h1:XlNgJS1pkjUXWEfxbl+8+B+5KnmI/AOT2ggB9uy2low= h1:7EjObEwFuYAQRSd1W1j7D4hZCLB1SYLFazZHImpSZCE=' ]]; then
    echo 'evmd: root library checksums differ from the reviewed anchor' >&2
    exit 1
  fi
fi
# Check fork sums, effective v2 imports and evmd's immutable root anchor.
(cd "$module_dir" && python3 "$repo_root/scripts/verify-go-fork-provenance.py")
packages="$(cd "$module_dir" && go list -deps ./...)"
if printf '%s\n' "$packages" | grep -E '^golang.org/x/crypto/openpgp(/|$)|^(github.com/cosmos/cosmos-sdk/(contrib/)?x/crisis|cosmossdk.io/x/crisis)(/|$)' >/dev/null; then
  echo 'Unreviewed obsolete production package reintroduced' >&2
  exit 1
fi

# Do not reintroduce a JSON-RPC method accepting raw private keys. OpenPGP
# key armor uses the maintained Proton parser through Cosmos CLI/keyring.
if grep -R --line-number --include='*.go' -E 'ImportRawKey|personal_importRawKey' \
  "$repo_root/rpc"; then
  echo 'Raw private-key import must not be exposed by JSON-RPC' >&2
  exit 1
fi

# x/crisis is deprecated and has two no-fix advisories. It is present in the
# upstream repository, but must never be wired into this application.
if grep -R --line-number --include='*.go' 'cosmos-sdk/x/crisis' "$repo_root"; then
  echo 'The deprecated Cosmos x/crisis module must not be imported' >&2
  exit 1
fi

report="$(mktemp)"
trap 'rm -f "$report"' EXIT

set +e
govulncheck "$@" 2>&1 | tee "$report"
status=${PIPESTATUS[0]}
set -e

# After successful lock checks, collect findings but reject an expired review.
if [[ "$review_expired" == true ]]; then
  echo "govulncheck report collected; expired exception review still blocks this check" >&2
  exit 1
fi

if (( status == 0 )); then
  exit 0
fi
if (( status != 3 )); then
  exit "$status"
fi

mapfile -t found < <(
  sed -nE 's/^Vulnerability #[0-9]+: (GO-[0-9]{4}-[0-9]+)$/\1/p' "$report" |
    sort -u
)
mapfile -t allowed < <(printf '%s\n' "${accepted[@]}" | sort -u)
mapfile -t unexpected < <(comm -23 <(printf '%s\n' "${found[@]}") <(printf '%s\n' "${allowed[@]}"))

if (( ${#found[@]} == 0 )); then
  echo "govulncheck failed without a parseable advisory ID" >&2
  exit 1
fi
if (( ${#unexpected[@]} > 0 )); then
  printf 'Unaccepted Go vulnerability: %s\n' "${unexpected[@]}" >&2
  exit 1
fi

printf 'Only reviewed upstream exceptions remain (review by %s): %s\n' \
  "$review_by" "${found[*]}"
