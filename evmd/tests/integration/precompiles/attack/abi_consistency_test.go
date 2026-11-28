package attack

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
)

// A precompile refuses its transaction methods in read-only calls and runs
// every other method as a query whose writes are discarded, so its
// IsTransaction must name exactly the methods its ABI declares as state
// changing (neither view nor pure).
func TestPrecompileTransactionMethodsMatchTheABI(t *testing.T) {
	h := sharedCalldataHarness(t)
	ctx := h.base.WithGasMeter(storetypes.NewInfiniteGasMeter())
	for _, target := range allTargets {
		if target.abi == nil {
			continue
		}
		precompiles, found, err := h.app.EVMKeeper.GetPrecompileInstance(ctx, target.address)
		if err != nil || !found {
			t.Fatalf("%s: precompile not active: %v", target.name, err)
		}
		classifier, ok := precompiles.Map[target.address].(interface{ IsTransaction(*abi.Method) bool })
		methods := append([]abi.Method{}, target.methods...)
		if target.abi.HasFallback() {
			methods = append(methods, target.abi.Fallback)
		}
		if target.abi.HasReceive() {
			methods = append(methods, target.abi.Receive)
		}
		for i := range methods {
			method := &methods[i]
			if !ok {
				// A stateless precompile (bech32) has no context to write to.
				continue
			}
			if classifier.IsTransaction(method) == method.IsConstant() {
				t.Errorf("%s.%s: IsTransaction=%v but the ABI state mutability is %q", target.name, method.Name, classifier.IsTransaction(method), method.StateMutability)
			}
		}
	}
}
