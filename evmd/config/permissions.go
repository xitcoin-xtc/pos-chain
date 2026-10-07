package config

import (
	"maps"
	"sort"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	transfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
	corevm "github.com/ethereum/go-ethereum/core/vm"
	cosmosevmutils "github.com/xitcoin-xtc/pos-chain/utils"
	bridgetypes "github.com/xitcoin-xtc/pos-chain/x/bridge/types"
	erc20types "github.com/xitcoin-xtc/pos-chain/x/erc20/types"
	feemarkettypes "github.com/xitcoin-xtc/pos-chain/x/feemarket/types"
	validatorincentivestypes "github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
	vmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

// BlockedAddresses returns all the app's blocked account addresses.
//
// Note, this includes:
//   - module accounts
//   - Ethereum's native precompiled smart contracts
//   - Cosmos EVM' available static precompiled contracts
func BlockedAddresses() map[string]bool {
	blockedAddrs := make(map[string]bool)

	maccPerms := GetMaccPerms()
	accs := make([]string, 0, len(maccPerms))
	for acc := range maccPerms {
		accs = append(accs, acc)
	}
	sort.Strings(accs)

	for _, acc := range accs {
		// The validator incentive treasury is intentionally prefunded with
		// existing XTC. It remains a non-minting and non-burning module
		// account, but must accept ordinary bank transfers.
		if acc == validatorincentivestypes.TreasuryAccountName {
			continue
		}

		blockedAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}

	blockedPrecompilesHex := vmtypes.AvailableStaticPrecompiles
	for _, addr := range corevm.PrecompiledAddressesPrague {
		blockedPrecompilesHex = append(blockedPrecompilesHex, addr.Hex())
	}

	for _, precompile := range blockedPrecompilesHex {
		blockedAddrs[cosmosevmutils.Bech32StringFromHexAddress(precompile)] = true
	}

	return blockedAddrs
}

// module account permissions
var maccPerms = map[string][]string{
	authtypes.FeeCollectorName:     nil,
	distrtypes.ModuleName:          nil,
	transfertypes.ModuleName:       {authtypes.Minter, authtypes.Burner},
	minttypes.ModuleName:           {authtypes.Minter},
	stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
	stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
	govtypes.ModuleName:            {authtypes.Burner},

	// Cosmos EVM modules
	vmtypes.ModuleName:                  {authtypes.Minter, authtypes.Burner},
	feemarkettypes.ModuleName:           nil,
	erc20types.ModuleName:               {authtypes.Minter, authtypes.Burner},
	bridgetypes.ModuleName:              {authtypes.Minter, authtypes.Burner},
	validatorincentivestypes.ModuleName: nil,
}

// GetMaccPerms returns a copy of the module account permissions
func GetMaccPerms() map[string][]string {
	return maps.Clone(maccPerms)
}
