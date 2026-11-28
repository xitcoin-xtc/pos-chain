package types

import (
	"math/big"

	sdkmath "cosmossdk.io/math"
)

// maxAmountDigits is the number of decimal digits of 2^256-1.
const maxAmountDigits = 78

// ParseAmount is the only parser of bridge amounts and route limits. It
// accepts exactly the canonical decimal form of an integer in [1, 2^256-1]:
// ASCII digits only, no sign, no leading zero, no whitespace, no base prefix
// or separator, so ParseAmount(s).String() == s for every accepted s. This is
// also the only form the relayer accepts (^[1-9][0-9]*$) and fits the
// uint256 amounts of the Cronos vault.
//
// big.Int.SetString(s, 10) accepts "010" and "+10" as 10, and
// sdkmath.NewIntFromString (SetString(s, 0)) reads "010" as octal 8, so the
// two must never be mixed for consensus amounts.
func ParseAmount(s string) (sdkmath.Int, error) {
	if s == "" || len(s) > maxAmountDigits || s[0] < '1' || s[0] > '9' {
		return sdkmath.Int{}, ErrNonCanonicalAmount
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return sdkmath.Int{}, ErrNonCanonicalAmount
		}
	}
	value, ok := new(big.Int).SetString(s, 10)
	if !ok || value.Sign() <= 0 || value.String() != s {
		return sdkmath.Int{}, ErrNonCanonicalAmount
	}
	if value.BitLen() > sdkmath.MaxBitLen {
		return sdkmath.Int{}, ErrAmountOutOfRange
	}
	return sdkmath.NewIntFromBigInt(value), nil
}

// ParseNonNegativeAmount accepts "0" or a canonical positive amount. It is
// used only for the exported outstanding amount, which may be zero.
func ParseNonNegativeAmount(s string) (sdkmath.Int, error) {
	if s == "0" {
		return sdkmath.ZeroInt(), nil
	}
	return ParseAmount(s)
}
