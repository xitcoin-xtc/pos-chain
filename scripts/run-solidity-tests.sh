#!/usr/bin/env bash
# Run the six Solidity suites against a disposable local node
# (local_node.sh). Needs Go, Node.js 24 and Yarn 1.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/tests_compatibility_common.sh
source "$SCRIPT_DIR/tests_compatibility_common.sh"

SUITES=(basic eip1559 exception opcode precompiles revert_cases)

(cd "$ROOT/contracts" && npm ci)
(cd "$ROOT/tests/solidity" && yarn install --frozen-lockfile --non-interactive)

# local_node.sh installs the binary and creates a fresh $HOME/.evmd.
start_node
trap cleanup_node EXIT
wait_for_node 10

load_test_evm_chain_id
load_test_validator_address
load_test_validator_cons_address

for suite in "${SUITES[@]}"; do
	echo "--- Solidity suite: $suite"
	(cd "$ROOT/tests/solidity/suites/$suite" && yarn test-cosmos)
done
