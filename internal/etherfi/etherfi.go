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
	PerpProxy        common.Address
	PoolShareTknAddr common.Address
	PoolTknAddr      common.Address
	PoolId           uint16  // weeth pool
	PerpIds          []int32 // relevant perpetual ids
	PoolTknDecimals  uint8
	RpcMngr          utils.RpcHandler
	FlipsideKey      string
	Mutex            sync.Mutex
	Sdk              *d8x_futures.SdkRO
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
		PoolId:           uint16(config.PoolId),
		PerpIds:          perpIds,
		PoolShareTknAddr: shareTkn,
		PoolTknAddr:      marginTkn,
		Sdk:              &sdkRo,
	}

	if app.PoolShareTknAddr == (common.Address{}) || app.PoolTknAddr == (common.Address{}) {
		return nil, errors.New("invalid token address")
	}
	err = app.RpcMngr.Init(config.RpcUrls)
	if err != nil {
		return nil, err
	}
	dec, err := QueryTokenDecimals(marginTkn.Hex(), app.RpcMngr.GetRpc())
	if err != nil {
		return nil, err
	}
	app.PoolTknDecimals = dec

	app.FlipsideKey = v.GetString(env.FLIPSIDE_API_KEY)
	if app.FlipsideKey == "" {
		return nil, errors.New("no flipside key found")
	}
	return &app, nil
}

func (app *App) Balances(req utils.APIBalancesPayload) (utils.APIBalancesResponse, error) {
	addr := req.Addresses
	var err error
	if len(addr) == 0 {
		// user did not provide any addresses, that means the entire
		// holder universe must be queried
		// 1. Update token holders via flipside query
		err = app.refreshReceivers(req.BlockNumber)
		if err != nil {
			return utils.APIBalancesResponse{}, err
		}
		// 2. Get list of all token holders
		addr, err = app.dbGetTokenHolders(req.BlockNumber)
		if err != nil {
			return utils.APIBalancesResponse{}, err
		}
	}
	traderBalcs, total, err := app.QueryTraderBalances(big.NewInt(int64(req.BlockNumber)))
	if err != nil {
		slog.Error("Unable to get trader balances:" + err.Error())
		return utils.APIBalancesResponse{}, err
	}
	fmt.Println("found ", len(traderBalcs), "traders")
	lpBalcs, err := app.QueryLpBalances(addr, total, req.BlockNumber)
	if err != nil {
		return utils.APIBalancesResponse{}, err
	}
	// combine balances. If addresses were provided we report the balance for each of those addresses,
	// even if zero.
	balances := combineBalances(addr, len(req.Addresses) > 0, lpBalcs, traderBalcs, app.PoolTknDecimals)
	var r utils.APIBalancesResponse
	r.Result = balances
	// create
	return r, nil
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

func retryQuery(blockNumber int64, rpcManager *utils.RpcHandler, queryFunc func(int64, *ethclient.Client) (*big.Int, error)) (*big.Int, error) {
	var result *big.Int
	var err error
	for trial := 0; trial < 4; trial++ {
		rpc := rpcManager.GetNextRpc()
		rpcManager.WaitForToken(rpc)
		result, err = queryFunc(blockNumber, rpc)
		if err == nil {
			break
		}
		slog.Info("query failed, retrying")
	}
	return result, err
}

// QueryLpBalances gets the attributed WEETH balances of sharepooltoken holders with given addresses
// We supply the total trader margin account balance 'traderTotal' to this function
func (app *App) QueryLpBalances(addrs []string, traderTotal *big.Int, blockNumber int64) ([]*big.Int, error) {
	var err error
	var total *big.Int
	total, err = retryQuery(blockNumber, &app.RpcMngr, app.queryShareTknSupply)
	if err != nil {
		return nil, err
	}
	if total.Cmp(big.NewInt(0)) == 0 {
		return nil, nil
	}

	// weeth pool balance:
	var poolBalance *big.Int
	poolBalance, err = retryQuery(blockNumber, &app.RpcMngr, app.queryPoolTknTotalBalance)
	if err != nil {
		return nil, err
	}
	poolBalance = new(big.Int).Sub(poolBalance, traderTotal)
	// attributed WEETH equals shareTknBal/totalShareTknSupply * (poolBalance-traderTotal)
	var balcs []*big.Int
	for trial := 0; trial < 3; trial++ {
		rpc := app.RpcMngr.GetNextRpc()
		app.RpcMngr.WaitForToken(rpc)
		balcs, err = QueryMultiTokenBalance(rpc, app.PoolShareTknAddr.Hex(), addrs, big.NewInt(blockNumber))
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		return nil, err
	}
	balances := make([]*big.Int, 0, len(balcs))
	for _, bal := range balcs {
		b := big.NewInt(0)
		if bal.Cmp(b) == 0 {
			balances = append(balances, b)
			continue
		}
		b = b.Mul(bal, poolBalance)
		b = b.Div(b, total)
		// b is in units of the poolTkn (WEETH)
		balances = append(balances, b)
	}
	return balances, nil
}

func (app *App) queryPoolTknTotalBalance(blockNumber int64, rpc *ethclient.Client) (*big.Int, error) {
	poolTkn, err := CreateErc20Instance(app.PoolTknAddr.Hex(), rpc)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	bal, err := QueryTokenBalance(poolTkn, app.PerpProxy.Hex(), big.NewInt(blockNumber))
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

func (app *App) queryShareTknSupply(blockNumber int64, rpc *ethclient.Client) (*big.Int, error) {
	shareTkn, err := CreateErc20Instance(app.PoolShareTknAddr.Hex(), rpc)
	if err != nil {
		slog.Error("queryBalances:" + err.Error())
		return nil, err
	}
	b := big.NewInt(blockNumber)
	total, err := QueryTokenTotalSupply(shareTkn, b)
	if err != nil {
		slog.Error("queryBalances:" + err.Error())
		return nil, err
	}

	return total, nil
}

func (app *App) dbGetTokenHolders(blockNum int64) ([]string, error) {

	query := `SELECT distinct(addr) FROM receivers where from_block <= $1`
	rows, err := app.Db.Query(query, blockNum)
	if err != nil {
		return nil, errors.New("dbGetTokenHolders" + err.Error())
	}
	defer rows.Close()
	addr := make([]string, 0)
	for rows.Next() {
		var a string
		rows.Scan(&a)
		addr = append(addr, a)
	}
	return addr, nil
}

// refreshReceivers ensures we collected all current and past holders of the share token,
// up to the given block number
func (app *App) refreshReceivers(toBlockNumber int64) error {
	// prevent overlapping queries via flipside for block numbers
	app.Mutex.Lock()
	defer app.Mutex.Unlock()

	fromBlock := app.DbGetStartBlock()
	if toBlockNumber >= fromBlock {
		// we need to make a query to refresh the token holder addresses
		fsSet, err := app.flipsideGetAddresses(fromBlock, toBlockNumber)
		if err != nil {
			return err
		}
		//fmt.Println(fsSet)
		err = app.DBInsertReceivers(fsSet, fromBlock, toBlockNumber)
		if err != nil {
			return errors.New("failed inserting flipside result to DB:" + err.Error())
		}
	}
	return nil
}

// DbGetStartBlock looks up the latest block for which
// we have stored receiver addresses
func (app *App) DbGetStartBlock() int64 {
	query := `SELECT coalesce(max(to_block),0) FROM receivers`
	var block int64
	err := app.Db.QueryRow(query).Scan(&block)
	if err == sql.ErrNoRows {
		return block
	}
	if err != nil {
		slog.Error("Error for DbGetStartBlock" + err.Error())
		return block
	}
	return max(191382586, block+1)
}

// DBInsertReceivers inserts the results FSResultSet into the database
func (app *App) DBInsertReceivers(fsSet *utils.FSResultSet, fromBlock, toBlock int64) error {
	// Prepare the insert statement
	stmt, err := app.Db.Prepare("INSERT INTO receivers(addr, from_block, to_block, pool_tkn) VALUES($1, $2, $3, $4)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	tkn_addr := app.PoolShareTknAddr.Hex()
	// Insert each address
	for _, row := range fsSet.Rows {
		addr := row.([]interface{})[0]
		_, err := stmt.Exec(addr, fromBlock, toBlock, tkn_addr)
		if err != nil {
			return err
		}
	}

	return nil
}

// ConnectDB connects to the database and assigns the connection to the app struct
func (a *App) ConnectDB(connStr string) error {
	// Connect to database
	// From documentation: "The returned DB is safe for concurrent use by multiple goroutines and
	// maintains its own pool of idle connections. Thus, the Open function should be called just once.
	// It is rarely necessary to close a DB."
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	a.Db = db
	return nil
}
