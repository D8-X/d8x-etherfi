package etherfi

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
)

type App struct {
	Db               *sql.DB
	PerpProxy        common.Address
	PoolShareTknAddr common.Address
	PoolTknAddr      common.Address
	PoolTknDecimals  uint8
	RpcMngr          utils.RpcHandler
	FlipsideKey      string
	Mutex            sync.Mutex
}

func NewApp(v *viper.Viper) (*App, error) {
	config, err := utils.LoadConfig(v.GetString(env.CONFIG_PATH))
	if err != nil {
		return nil, err
	}
	app := App{
		PerpProxy:        config.PerpAddr,
		PoolShareTknAddr: config.PoolShareTknAddr,
		PoolTknAddr:      config.PoolTknAddr,
		PoolTknDecimals:  config.PoolTknDecimals,
	}
	if app.PoolTknDecimals <= 0 {
		return nil, errors.New("invalid pool tkn decimals")
	}
	if app.PoolShareTknAddr == (common.Address{}) || app.PoolTknAddr == (common.Address{}) {
		return nil, errors.New("invalid token address")
	}
	err = app.RpcMngr.Init(config.RpcUrls)
	if err != nil {
		return nil, err
	}
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

	balances, err := app.queryBalances(addr, req.BlockNumber)
	if err != nil {
		return utils.APIBalancesResponse{}, err
	}
	var r utils.APIBalancesResponse
	r.Result = balances
	// create
	return r, nil
}

func (app *App) queryBalances(addrs []string, blockNumber int64) ([]utils.Balance, error) {
	shareTknBal, total, err := app.queryShareTknBalances(addrs, blockNumber)
	if err != nil {
		return nil, err
	}
	if total.Cmp(big.NewInt(0)) == 0 {
		return []utils.Balance{}, nil
	}
	poolBalance, err := app.queryPoolTknTotalBalance(blockNumber)
	if err != nil {
		return nil, err
	}
	// attributed WEETH equals shareTknBal/total * poolBalance
	balances := make([]utils.Balance, 0, len(shareTknBal))
	for _, addr := range addrs {
		addrLower := strings.ToLower(addr)
		bal, exists := shareTknBal[addrLower]
		if !exists {
			entry := utils.Balance{Address: addr, EffBalance: 0}
			balances = append(balances, entry)
			continue
		}
		b := big.NewInt(0)
		b = b.Mul(bal, poolBalance)
		b = b.Div(b, total)
		// b is in units of the poolTkn (WEETH)
		entry := utils.Balance{Address: addr, EffBalance: utils.DecNToFloat(b, app.PoolTknDecimals)}
		balances = append(balances, entry)
	}
	return balances, nil
}

func (app *App) queryPoolTknTotalBalance(blockNumber int64) (*big.Int, error) {
	rpc := app.RpcMngr.GetNextRpc()
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

func (app *App) queryShareTknBalances(addrs []string, blockNumber int64) (map[string]*big.Int, *big.Int, error) {
	rpc := app.RpcMngr.GetNextRpc()
	bucket := utils.NewTokenBucket(5, 5)
	bucket.WaitForToken("bal", true)
	shareTkn, err := CreateErc20Instance(app.PoolShareTknAddr.Hex(), rpc)
	if err != nil {
		slog.Error("queryBalances:" + err.Error())
		return nil, nil, err
	}
	b := big.NewInt(blockNumber)
	balances := make(map[string]*big.Int)
	total, err := QueryTokenTotalSupply(shareTkn, b)
	if err != nil {
		slog.Error("queryBalances:" + err.Error())
		return nil, nil, err
	}
	zero := big.NewInt(0)
	for _, addr := range addrs {
		if addr == (common.Address{}).Hex() {
			// skip zero address (burning)
			continue
		}
		addr := strings.ToLower(addr)
		bucket.WaitForToken("bal", true)
		bal, err := QueryTokenBalance(shareTkn, addr, b)
		if err != nil {
			slog.Error(err.Error())
			return nil, nil, err
		}
		if bal.Cmp(zero) == 0 {
			fmt.Printf("account %s has zero balance, skipping", addr)
		}
		balances[addr] = bal
	}
	return balances, total, nil
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

// refreshReceivers ensures we collected all current and past token holders
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
