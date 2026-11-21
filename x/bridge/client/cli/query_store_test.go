package cli

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"strings"
	"testing"
)

func TestQueryCmdIncludesStoreQueries(t *testing.T) {
	cmd := GetQueryCmd()
	for _, name := range []string{"attestation-status", "route", "source-deposit-key"} {
		child, _, err := cmd.Find([]string{name})
		if err != nil || child == cmd || child.Name() != name {
			t.Fatalf("missing %s query command: %v", name, err)
		}
	}
}

func TestDecodeRouteStore(t *testing.T) {
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint64(nonce, 5)
	outstanding := new(big.Int).Mul(big.NewInt(1_000_000_000), big.NewInt(1_000_000_000_000_000_000)).Bytes()
	view, err := DecodeRouteStore(
		42,
		[]byte(`{"route_id":"cronos-xitcoin-xtc-v1","bridge_signers":[],"guardian":"","max_transfer_amount":"1","daily_limit":"1","max_outstanding_amount":"1","enabled":false,"source_chain_id":"25"}`),
		[]byte(`{"paused":true}`),
		outstanding,
		nonce,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Configured || !view.Paused || view.RouteConfig.SourceChainID != "25" || view.ControlNonce != 5 || view.OutboundNonce != 0 || view.Height != 42 {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.OutstandingAmount != "1000000000000000000000000000" {
		t.Fatalf("outstanding = %s", view.OutstandingAmount)
	}
	empty, err := DecodeRouteStore(1, nil, nil, nil, nil, nil)
	if err != nil || empty.Configured || empty.Paused || empty.OutstandingAmount != "0" {
		t.Fatalf("unexpected empty view: %+v err=%v", empty, err)
	}
	if _, err := DecodeRouteStore(1, nil, nil, nil, []byte{1}, nil); err == nil {
		t.Fatal("malformed nonce accepted")
	}
}

func TestSourceDepositKeyCommandMatchesCrossCheckVector(t *testing.T) {
	cmd := GetSourceDepositKeyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"338", "0x" + strings.Repeat("aa", 32)})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "0x60e1081b4354c658cfe771da1042e6122798577367f118f0cc589b081a535566" {
		t.Fatalf("source deposit key = %s", got)
	}
}
