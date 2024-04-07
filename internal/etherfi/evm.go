package etherfi

import (
	"context"
	"log/slog"
	"math/big"
	"strings"

	"github.com/D8-X/d8x-etherfi/internal/contracts"
	"github.com/D8-X/d8x-etherfi/internal/utils"
	d8xcontracts "github.com/D8-X/d8x-futures-go-sdk/pkg/contracts"
	"github.com/D8-X/d8x-futures-go-sdk/pkg/d8x_futures"
	d8xutils "github.com/D8-X/d8x-futures-go-sdk/utils"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/forta-network/go-multicall"
)

func CreateErc20Instance(tokenAddr string, rpc *ethclient.Client) (*contracts.Erc20, error) {
	tknAddr := common.HexToAddress(tokenAddr)
	instance, err := contracts.NewErc20(tknAddr, rpc)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func QueryTokenBalance(tknCtrct *contracts.Erc20, tknOwnerAddr string, blockNumber *big.Int) (*big.Int, error) {
	ownerAddr := common.HexToAddress(tknOwnerAddr)
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	bal, err := tknCtrct.BalanceOf(&bind.CallOpts{}, ownerAddr)
	return bal, err
}

func QueryMultiTokenBalance(client *ethclient.Client, tknAddr string, addrs []string, blockNumber *big.Int) ([]*big.Int, error) {
	contract, err := multicall.NewContract(ERC20_BALANCE_ABI, tknAddr)
	if err != nil {
		return nil, err
	}
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	caller, err := multicall.New(client)
	if err != nil {
		return nil, err
	}
	type balanceOutput struct {
		Bal *big.Int
	}
	balances := make([]*big.Int, 0, len(addrs))
	inc := 500
	to := min(len(addrs), inc)
	from := 0
	for {
		calls := make([]*multicall.Call, 0, inc)
		for k := from; k < to; k++ {
			c := contract.NewCall(new(balanceOutput), "balanceOf", common.HexToAddress(addrs[k]))
			calls = append(calls, c)
		}
		res, err := caller.Call(opts, calls...)
		if err != nil {
			return nil, err
		}

		for _, call := range res {
			balances = append(balances, call.Outputs.(*balanceOutput).Bal)
		}
		from = to
		to = min(len(addrs), to+inc)
		if from >= len(addrs) {
			break
		}
	}
	return balances, nil
}

func QueryTokenTotalSupply(tknCtrct *contracts.Erc20, blockNumber *big.Int) (*big.Int, error) {
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	bal, err := tknCtrct.TotalSupply(opts)
	return bal, err
}

// QueryTraderBalances queries the available cash for
// each active perpetual account. Available cash is defined as the
// cash minus unpaid funding. The function returns a mapping of lower-case trader-addr to its
// entire available cash balance (in all perpetuals of the relevant WEETH pool) and the total
func (app *App) QueryTraderBalances(blockNumber *big.Int) (map[string]*big.Int, *big.Int, error) {
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
		opts.Pending = false
		opts.Context = context.Background()
	}
	allTraders := make([]common.Address, 0)
	allCash := make([]*big.Int, 0)

	for _, perpId := range app.PerpIds {
		// query all active addresses in the given perp with re-trying on rpc failure
		traders, err := app.queryActiveAddr(opts, perpId)
		if err != nil {
			if err.Error() == "no contract code at given address" {
				return nil, big.NewInt(0), nil
			}
			return nil, nil, err
		}

		// query cash for the given traders in the current perp with re-trying on rpc failure
		// unit is decimal N, aligned with pool token decimals
		cash, err := app.queryAvailCash(opts, perpId, traders)
		if err != nil {
			return nil, nil, err
		}
		allTraders = append(allTraders, traders...)
		allCash = append(allCash, cash...)
	}
	// re-organize data
	bal := make(map[string]*big.Int)
	tot := big.NewInt(0)
	proxyAddr := strings.ToLower((app.PerpProxy.Hex()))
	for k, trader := range allTraders {
		addr := strings.ToLower(trader.Hex())
		if addr == proxyAddr {
			slog.Info("oops perp trader found, we don't add the perp")
			continue
		}
		if _, exists := bal[addr]; !exists {
			bal[addr] = new(big.Int).Set(allCash[k])
		} else {
			bal[addr] = new(big.Int).Add(bal[addr], allCash[k])
		}
		tot = new(big.Int).Add(tot, allCash[k])
	}
	return bal, tot, nil
}

// queryAvailCash queries via multicall the available cash for the traders in the addrs slice.
// Retries on RPC failure.
func (app *App) queryAvailCash(opts *bind.CallOpts, perpId int32, addrs []common.Address) ([]*big.Int, error) {
	id := big.NewInt(int64(perpId))
	var cash []*big.Int
	var err error
	for trial := 0; trial < 3; trial++ {
		cash, err = app.tryQueryAvailCash(opts, id, addrs)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return cash, err
}

// tryQueryAvailCash uses multicall to get available cash (cash-funding payments due) for the
// given trader addresses
func (app *App) tryQueryAvailCash(opts *bind.CallOpts, id *big.Int, addrs []common.Address) ([]*big.Int, error) {
	contract, err := multicall.NewContract(AVAIL_CASH_ABI, app.PerpProxy.Hex())
	if err != nil {
		return nil, err
	}
	client := app.RpcMngr.GetNextRpc()
	app.RpcMngr.WaitForToken(client)
	caller, err := multicall.New(client)
	if err != nil {
		return nil, err
	}
	type availCashOutput struct {
		Cash *big.Int
	}
	cash := make([]*big.Int, 0, len(addrs))
	inc := 500
	to := min(len(addrs), inc)
	from := 0
	for {
		app.RpcMngr.WaitForToken(client)
		calls := make([]*multicall.Call, 0, inc)
		for k := from; k < to; k++ {
			c := contract.NewCall(new(availCashOutput), "getAvailableCash", id, addrs[k])
			calls = append(calls, c)
		}
		res, err := caller.Call(opts, calls...)
		if err != nil {
			return nil, err
		}

		for _, call := range res {
			// convert to decN
			cash64 := call.Outputs.(*availCashOutput).Cash
			cashDecN := d8xutils.ABDKToDecN(cash64, app.PoolTknDecimals)
			cash = append(cash, cashDecN)
		}
		from = to
		to = min(len(addrs), to+inc)
		if from >= len(addrs) {
			break
		}
	}
	return cash, nil
}

func (app *App) queryActiveAddr(opts *bind.CallOpts, perpId int32) ([]common.Address, error) {
	traders := make([]common.Address, 0)
	id := big.NewInt(int64(perpId))
	var err error
	for trial := 0; trial < 3; trial++ {
		client := app.RpcMngr.GetNextRpc()
		app.RpcMngr.WaitForToken(client)
		ctrct := d8x_futures.CreatePerpetualManagerInstance(client, app.Sdk.Info.ProxyAddr)
		traders, err = tryQueryActiveAddr(&app.RpcMngr, client, id, ctrct, opts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// tryQueryActiveAddr gets all addresses that have an active perpetual account in the given perpetual
func tryQueryActiveAddr(rpcMngr *utils.RpcHandler, client *ethclient.Client, perpId *big.Int, ctrct *d8xcontracts.IPerpetualManager, opts *bind.CallOpts) ([]common.Address, error) {
	traders := make([]common.Address, 0)
	nb, err := ctrct.CountActivePerpAccounts(opts, perpId)
	if err != nil {
		return nil, err
	}
	n := int(nb.Int64())
	var inc int = 100
	to := min(n, inc)
	from := 0
	for {
		rpcMngr.WaitForToken(client)
		addr, err := ctrct.GetActivePerpAccountsByChunks(opts, perpId, big.NewInt(int64(from)), big.NewInt(int64(to)))
		if err != nil {
			return nil, err
		}
		traders = append(traders, addr...)
		from = to
		to = min(n, to+inc)
		if from >= n {
			break
		}
	}
	return traders, nil
}
