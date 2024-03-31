package etherfi

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestQueryMultiTokenBalance(t *testing.T) {
	url := "https://arb1.arbitrum.io/rpc"
	rpc, err := ethclient.Dial(url)
	if err != nil {
		t.FailNow()
	}
	usdc := "0xaf88d065e77c8cc2239327c5edb3a432268e5831"
	addrs := []string{"0x337a3778244159f37c016196a8e1038a811a34c9", "0x7fcdc35463e3770c2fb992716cd070b63540b947",
		"0xe37e799d5077682fa0a244d46e5649f71457bd09", "0x0e4831319a50228b9e450861297ab92dee15b44f",
		"0xfc99f58a8974a4bc36e60e2d490bb8d72899ee9f"}
	bals, err := QueryMultiTokenBalance(rpc, usdc, addrs, big.NewInt(195984604))
	if err != nil {
		t.FailNow()
	}
	for k, b := range bals {
		fmt.Printf("Balance of %s is %.2f\n", addrs[k], utils.DecNToFloat(b, 6))
	}
}
