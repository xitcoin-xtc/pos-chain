package rpc

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/debug"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/eth"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/eth/filters"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/miner"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/net"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/personal"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/txpool"
	"github.com/xitcoin-xtc/pos-chain/rpc/namespaces/ethereum/web3"
)

// JSON-RPC must never accept raw private keys: the upstream
// personal_importRawKey method was removed and no namespace may expose it.
// The JSON-RPC server publishes every exported method of a service, so a
// declaration is enough to expose it.
func TestNoRawPrivateKeyImportMethod(t *testing.T) {
	services := map[string][]any{
		EthNamespace:      {&eth.PublicAPI{}, &filters.PublicFilterAPI{}},
		Web3Namespace:     {&web3.PublicAPI{}},
		NetNamespace:      {&net.PublicAPI{}},
		PersonalNamespace: {&personal.PrivateAccountAPI{}},
		TxPoolNamespace:   {&txpool.PublicAPI{}},
		DebugNamespace:    {&debug.API{}},
		MinerNamespace:    {&miner.API{}},
	}
	// A new namespace must be listed above.
	require.ElementsMatch(t, slices.Collect(maps.Keys(apiCreators)), slices.Collect(maps.Keys(services)))

	for namespace, list := range services {
		for _, service := range list {
			typ := reflect.TypeOf(service)
			for i := range typ.NumMethod() {
				name := typ.Method(i).Name
				require.False(t, strings.EqualFold(name, "ImportRawKey"),
					"%s exposes %s_%s%s", typ, namespace, strings.ToLower(name[:1]), name[1:])
			}
		}
	}
}
