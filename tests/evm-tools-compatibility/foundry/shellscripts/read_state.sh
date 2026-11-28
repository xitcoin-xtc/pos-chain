#!/usr/bin/env bash
set -euo pipefail

# Usage: ./read_state.sh <CONTRACT_ADDRESS>
# .env에 CUSTOM_RPC, ALICE_ADDRESS 를 정의해 두세요.

# shellcheck source=../.env
# shellcheck disable=SC1091
# The suite runner may export these variables instead (COMPAT_LOCAL_KEYS).
if [ -f ../.env ]; then
	source ../.env
fi
RPC_URL=${CUSTOM_RPC:-http://127.0.0.1:8545}
CONTRACT=$1
ALICE=${ALICE_ADDRESS:-0x0000000000000000000000000000000000000001}

if [ -z "$CONTRACT" ]; then
	echo "Usage: $0 <CONTRACT_ADDRESS>"
	exit 1
fi

# 1) Chain ID
echo "⛓ Chain ID:"
echo "$ cast chain-id --rpc-url \"$RPC_URL\""
cast chain-id --rpc-url "$RPC_URL"
echo

# 2) totalSupply()
echo "🔢 totalSupply:"
echo "$ cast call --rpc-url \"$RPC_URL\" \"$CONTRACT\" 'totalSupply()(uint256)'"
cast call --rpc-url "$RPC_URL" "$CONTRACT" 'totalSupply()(uint256)'
echo

# 3) balanceOf(alice)
echo "👤 balanceOf(alice=$ALICE):"
echo "$ cast call --rpc-url \"$RPC_URL\" \"$CONTRACT\" 'balanceOf(address)(uint256)' \"$ALICE\""
cast call --rpc-url "$RPC_URL" "$CONTRACT" 'balanceOf(address)(uint256)' "$ALICE"
echo

# 4) name()
echo "📛 name:"
echo "$ cast call --rpc-url \"$RPC_URL\" \"$CONTRACT\" 'name()(string)'"
cast call --rpc-url "$RPC_URL" "$CONTRACT" 'name()(string)'
echo

# 5) symbol()
echo "🔣 symbol:"
echo "$ cast call --rpc-url \"$RPC_URL\" \"$CONTRACT\" 'symbol()(string)'"
cast call --rpc-url "$RPC_URL" "$CONTRACT" 'symbol()(string)'
echo

# 6) decimals()
echo "🔢 decimals:"
echo "$ cast call --rpc-url \"$RPC_URL\" \"$CONTRACT\" 'decimals()(uint8)'"
cast call --rpc-url "$RPC_URL" "$CONTRACT" 'decimals()(uint8)'
