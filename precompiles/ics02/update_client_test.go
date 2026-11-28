package ics02

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// An empty client message (empty bytes, or an Any without a type URL)
// decodes to a nil interface without an error. updateClient used to call
// ValidateBasic on it and panicked with a nil pointer dereference instead of
// reverting. Found by FuzzICS02PrecompileCalldata.
func TestUpdateClientRejectsAnEmptyClientMessage(t *testing.T) {
	p := NewPrecompile(codec.NewProtoCodec(codectypes.NewInterfaceRegistry()), nil)
	method := p.Methods[UpdateClientMethod]
	for name, message := range map[string][]byte{
		"empty bytes":          {},
		"Any without type URL": {0x12, 0x01, 0x00}, // field 2 (value) only
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				_, err = p.UpdateClient(sdk.Context{}, nil, nil, &method, []interface{}{"07-tendermint-0", message})
			})
			require.ErrorContains(t, err, "empty client message")
		})
	}
}
