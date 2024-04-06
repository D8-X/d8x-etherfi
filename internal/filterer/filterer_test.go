package filterer

import (
	"fmt"
	"testing"

	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
)

func TestFilterDelegateEvts(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(".env")
	c, err := utils.LoadConfig("../../config/config.json")
	if err != nil {
		t.FailNow()
	}
	fmt.Println(c.PerpAddr.Hex())
	f, err := NewFilterer(c.RpcUrlsFltr, c.PerpAddr, common.Address{})
	if err != nil {
		t.FailNow()
	}
	delegates, nowblock, err := f.FilterDelegateEvts(30021418, 0)
	if err != nil {
		t.FailNow()
	}
	fmt.Printf("found %d events up to block %d\n", len(delegates), nowblock)
	for _, dlgt := range delegates {
		d := dlgt.(*Delegate)
		fmt.Printf("from %s to %s with index %d at block %d\n", d.Addr, d.Delegate, d.Index, d.BlockNr)
	}
}
