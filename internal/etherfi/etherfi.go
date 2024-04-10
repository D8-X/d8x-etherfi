package etherfi

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/filterer"
	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/D8-X/d8x-futures-go-sdk/pkg/d8x_futures"
	d8xutils "github.com/D8-X/d8x-futures-go-sdk/utils"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/viper"
)

type App struct {
	Db               *sql.DB
	Genesis          uint64 // starting block when no data
	PerpProxy        common.Address
	PoolShareTknAddr common.Address
	PoolTknAddr      common.Address
	PoolId           uint16  // weeth pool
	PerpIds          []int32 // relevant perpetual ids
	PoolTknDecimals  uint8
	RpcMngr          utils.RpcHandler
	Filterer         *filterer.Filterer
	Mutex            sync.Mutex
	Sdk              *d8x_futures.SdkRO
	LastBlockTo      [2]uint64 // last block-to query. 0:delegates 1: transfers
}

func NewApp(v *viper.Viper) (*App, error) {
	config, err := utils.LoadConfig(v.GetString(env.CONFIG_PATH))
	if err != nil {
		return nil, err
	}
	var sdkRo d8x_futures.SdkRO
	err = sdkRo.New(strconv.Itoa(int(config.ChainId)))
	if err != nil {
		return nil, err
	}
	perpIds := make([]int32, 0)
	for _, perp := range sdkRo.Info.Perpetuals {
		if perp.PoolId != config.PoolId {
			continue
		}
		perpIds = append(perpIds, perp.Id)
	}
	var marginTkn, shareTkn common.Address
	for _, pool := range sdkRo.Info.Pools {
		if pool.PoolId == config.PoolId {
			marginTkn = pool.PoolMarginTokenAddr
			shareTkn = pool.ShareTokenAddr
			break
		}
	}

	app := App{
		PerpProxy:        config.PerpAddr,
		Genesis:          config.Genesis,
		PoolId:           uint16(config.PoolId),
		PerpIds:          perpIds,
		PoolShareTknAddr: shareTkn,
		PoolTknAddr:      marginTkn,
		Sdk:              &sdkRo,
	}

	if app.PoolShareTknAddr == (common.Address{}) || app.PoolTknAddr == (common.Address{}) {
		return nil, errors.New("invalid token address")
	}
	f, err := filterer.NewFilterer(config.RpcUrlsFltr, config.PerpAddr, app.PoolShareTknAddr)
	if err != nil {
		return nil, errors.New("failed to create filterer:" + err.Error())
	}
	app.Filterer = f
	err = app.RpcMngr.Init(config.RpcUrls, 5, 5)
	if err != nil {
		return nil, err
	}
	dec, err := QueryTokenDecimals(marginTkn.Hex(), app.RpcMngr.GetRpc())
	if err != nil {
		return nil, err
	}
	app.PoolTknDecimals = dec

	return &app, nil
}

// Balances responds to the balance query. Precondition: event data
// has been gathered up to the requested block
func (app *App) Balances(req utils.APIBalancesPayload) (utils.APIBalancesResponse, error) {

	addr := req.Addresses
	var err error
	if len(addr) == 0 {
		// user did not provide any addresses, that means the entire
		// holder universe must be queried
		// Get list of all token holders
		addr, err = app.dbGetShareTokenHolders(req.BlockNumber)
		if err != nil {
			return utils.APIBalancesResponse{}, err
		}
	}
	time0 := time.Now()
	type TraderChan struct {
		TraderBal map[string]*big.Int
		Total     *big.Int
	}
	type LpChan struct {
		ShTknBal   []*big.Int
		ShTknTotal *big.Int
	}
	errChan := make(chan error)
	traderChan := make(chan TraderChan)
	lpChan := make(chan LpChan)
	go func() {
		traderBalcs, total, err := app.QueryTraderBalances(big.NewInt(int64(req.BlockNumber)))
		if err != nil {
			slog.Error("Unable to get trader balances:" + err.Error())
			errChan <- err
		}
		err = app.reassignTraderBalances(traderBalcs, req.BlockNumber)
		if err != nil {
			errChan <- err
		}
		traderChan <- TraderChan{TraderBal: traderBalcs, Total: total}
	}()

	go func() {
		lpBalcs, shTknTot, err := app.QueryLpBalances(addr, req.BlockNumber)
		if err != nil {
			errChan <- err
		}
		lpChan <- LpChan{ShTknBal: lpBalcs, ShTknTotal: shTknTot}
	}()

	var t TraderChan
	var lp LpChan
	for i := 0; i < 2; i++ {
		select {
		case t = <-traderChan:
			fmt.Println("found ", len(t.TraderBal), "traders")
		case lp = <-lpChan:
			// Use lpBalcs
			fmt.Println("found ", len(lp.ShTknBal), "LPs")
		case err := <-errChan:
			if err != nil {
				return utils.APIBalancesResponse{}, err
			}
		}
	}
	// attribute lp balances based on totals
	lpBal, err := app.attributeLpBalances(lp.ShTknBal, lp.ShTknTotal, t.Total, req.BlockNumber)
	if err != nil {
		return utils.APIBalancesResponse{}, err
	}
	// combine balances. If addresses were provided we report the balance for each of those addresses,
	// even if zero.
	balances := combineBalances(addr, len(req.Addresses) > 0, lpBal, t.TraderBal, app.PoolTknDecimals)
	fmt.Println("\ntime elapsed = ", time.Since(time0))
	var r utils.APIBalancesResponse
	r.Result = balances
	// create
	return r, nil
}

// reassignTraderBalances re-assigns balances from the 'trader-account' to the 'delegate' in
// case the event was emitted with index DELEGATE_IDX_STRATEGY = 2. As a result, the traders
// of the hedge-strategy (delegates) get assigned the WEETH that is owned by the strategy-wallet.
// The strategy wallet is a private key generated from the delegate wallet but the delegate does
// not have the keys (directly).
func (app *App) reassignTraderBalances(traderBal map[string]*big.Int, block uint64) error {
	addrs, delegates, err := app.DbFindStrategyDelegates(block)
	if err != nil {
		slog.Error("reassignTraderBalance did not succeed")
		return err
	}
	for k, d := range delegates {
		// re-assign
		if _, exists := traderBal[d]; exists {
			if _, exists := traderBal[addrs[k]]; exists {
				traderBal[d] = new(big.Int).Add(traderBal[d], traderBal[addrs[k]])
				traderBal[addrs[k]] = big.NewInt(0)
			}
		}

	}
	return nil
}

// combineBalances goes through all the addresses and reconciles the balances
func combineBalances(addrs []string, exactAddr bool, lpBal []*big.Int, traderBal map[string]*big.Int, decN uint8) []utils.Balance {
	balances := make([]utils.Balance, 0, len(addrs)+len(traderBal))
	z := big.NewInt(0)
	for k, addr := range addrs {
		bal := new(big.Int).Set(lpBal[k])
		if _, exists := traderBal[addr]; exists {
			bal = new(big.Int).Add(bal, traderBal[addr])
			traderBal[addr] = big.NewInt(0)
		}
		if bal.Cmp(z) == 0 {
			if exactAddr {
				balances = append(balances, utils.Balance{Address: addr, EffBalance: 0})
			}
			continue
		}
		balances = append(balances, utils.Balance{Address: addr, EffBalance: d8xutils.DecNToFloat(bal, decN)})
	}
	if exactAddr {
		return balances
	}
	// exactAddr=false and we have to add all WEETH owners to the list
	// hence, we also add the traders to the list. traders that are also LPs were set to zero in the
	// code above
	for addr, bal := range traderBal {
		if bal.Cmp(z) == 0 {
			continue
		}
		balances = append(balances, utils.Balance{Address: addr, EffBalance: d8xutils.DecNToFloat(bal, decN)})
	}
	return balances
}

func retryQuery(blockNumber uint64, rpcManager *utils.RpcHandler, queryFunc func(uint64, *ethclient.Client) (*big.Int, error)) (*big.Int, error) {
	var result *big.Int
	var err error
	for trial := 0; trial < 4; trial++ {
		rpc := rpcManager.GetNextRpc()
		rpcManager.WaitForToken(rpc)
		result, err = queryFunc(blockNumber, rpc)
		if err == nil {
			break
		}
		if err.Error() == "no contract code at given address" {
			return big.NewInt(0), nil
		}
		slog.Info("query failed, retrying")
	}
	return result, err
}

// QueryLpBalances gets the share-token balances of given addresses
// also returns the total share token supply
func (app *App) QueryLpBalances(addrs []string, blockNumber uint64) ([]*big.Int, *big.Int, error) {
	var err error
	var total *big.Int
	total, err = retryQuery(blockNumber, &app.RpcMngr, app.queryShareTknSupply)
	if err != nil {
		return nil, nil, err
	}
	if total.Cmp(big.NewInt(0)) == 0 {
		return nil, total, nil
	}

	var balcs []*big.Int
	for trial := 0; trial < 3; trial++ {
		rpc := app.RpcMngr.GetNextRpc()
		app.RpcMngr.WaitForToken(rpc)
		balcs, err = QueryMultiTokenBalance(rpc, app.PoolShareTknAddr.Hex(), addrs, big.NewInt(int64(blockNumber)))
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, nil, err
	}
	return balcs, total, nil
}

// AttributeLpBalances attributes WEETH to LPs based on pool-available funds
// We supply the total trader margin account balance 'traderTotal' to this function
func (app *App) attributeLpBalances(balcs []*big.Int, shTknTotal *big.Int, traderTotal *big.Int, blockNumber uint64) ([]*big.Int, error) {
	if shTknTotal.Cmp(big.NewInt(0)) == 0 {
		return nil, nil
	}
	// weeth pool balance:
	var poolBalance *big.Int
	poolBalance, err := retryQuery(blockNumber, &app.RpcMngr, app.queryPoolTknTotalBalance)
	if err != nil {
		return nil, err
	}
	poolBalance = new(big.Int).Sub(poolBalance, traderTotal)
	// attributed WEETH equals shareTknBal/totalShareTknSupply * (poolBalance-traderTotal)
	balances := make([]*big.Int, 0, len(balcs))
	for _, bal := range balcs {
		b := big.NewInt(0)
		if bal.Cmp(b) == 0 {
			balances = append(balances, b)
			continue
		}
		b = b.Mul(bal, poolBalance)
		b = b.Div(b, shTknTotal)
		// b is in units of the poolTkn (WEETH)
		balances = append(balances, b)
	}
	return balances, nil
}

func (app *App) queryPoolTknTotalBalance(blockNumber uint64, rpc *ethclient.Client) (*big.Int, error) {
	poolTkn, err := CreateErc20Instance(app.PoolTknAddr.Hex(), rpc)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	bal, err := QueryTokenBalance(poolTkn, app.PerpProxy.Hex(), big.NewInt(int64(blockNumber)))
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return bal, nil
}

// QueryTokenDecimals gets the token decimals from an ERC-20 token
func QueryTokenDecimals(tokenAddr string, rpc *ethclient.Client) (uint8, error) {
	tkn, err := CreateErc20Instance(tokenAddr, rpc)
	if err != nil {
		slog.Error(err.Error())
		return 0, err
	}
	return tkn.Decimals(&bind.CallOpts{})
}

func (app *App) queryShareTknSupply(blockNumber uint64, rpc *ethclient.Client) (*big.Int, error) {
	shareTkn, err := CreateErc20Instance(app.PoolShareTknAddr.Hex(), rpc)
	if err != nil {
		slog.Error("queryBalances:" + err.Error())
		return nil, err
	}
	b := big.NewInt(int64(blockNumber))
	total, err := QueryTokenTotalSupply(shareTkn, b)
	if err != nil {
		msg := fmt.Sprintf("queryBalances for token %s failed at block %d: %s", app.PoolShareTknAddr.Hex(), blockNumber, err.Error())
		slog.Error(msg)
		return nil, err
	}

	return total, nil
}
