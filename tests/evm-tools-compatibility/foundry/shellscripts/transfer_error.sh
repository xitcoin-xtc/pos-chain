#!/usr/bin/env bash
set -euo pipefail

# Usage: ./error_edge_case.sh
# Requires in .env: CUSTOM_RPC, PRIVATE_KEY, ACCOUNT_2 (recipient), CONTRACT
# shellcheck source=../.env
# shellcheck disable=SC1091
# The suite runner may export these variables instead (COMPAT_LOCAL_KEYS).
if [ -f ../.env ]; then
	source ../.env
fi
export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

RPC_URL=${CUSTOM_RPC:-http://127.0.0.1:8545}
PK=${PRIVATE_KEY:?}
RECIPIENT=${ACCOUNT_2:?}
CHAIN_ID=${CHAIN_ID:-262144}

# Ensure CONTRACT is set
if [ -z "${CONTRACT:-}" ]; then
	echo "Error: CONTRACT environment variable not set."
	exit 1
fi

# Amount exceeding typical balance (2000 tokens)
AMOUNT=2000000000000000000000

echo "🔄 Sending transfer exceeding balance..."

# 1) Send via cast and capture output (suppress exit)
echo "❌ Attempting transfer that should fail:"
echo "$ cast send \"$CONTRACT\" 'transfer(address,uint256)' \"$RECIPIENT\" \"$AMOUNT\" --rpc-url \"$RPC_URL\" --private-key \"[HIDDEN]\" --chain-id \"$CHAIN_ID\" --json"
set +e
OUTPUT=$(cast send \
	"$CONTRACT" \
	'transfer(address,uint256)' "$RECIPIENT" "$AMOUNT" \
	--rpc-url "$RPC_URL" \
	--private-key "$PK" \
	--chain-id "$CHAIN_ID" \
	--json 2>&1)
STATUS=$?
set -e

# Decide on the exit status and the revert reason, not on the shape of the
# output: recent cast versions print errors as JSON log lines.
# 0xe450d38c is the ERC20InsufficientBalance selector.
if [ "$STATUS" -eq 0 ]; then
	# A receipt: the transfer must have failed on chain.
	if echo "$OUTPUT" | jq -e '.status == "0x0"' >/dev/null 2>&1; then
		echo "✅ Transaction reverted on chain as expected"
	else
		echo "❌ Expected a revert, but the transfer succeeded:"
		echo "$OUTPUT"
		exit 1
	fi
elif echo "$OUTPUT" | grep -q -e 'execution reverted' -e 'ERC20InsufficientBalance' -e '0xe450d38c'; then
	echo "✅ Transaction reverted as expected"
	echo "$OUTPUT" | tail -1
else
	echo "❌ Unexpected failure (no revert reason):"
	echo "$OUTPUT"
	exit 1
fi
