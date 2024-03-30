package etherfi

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/viper"
)

type App struct {
	Db          *sql.DB
	PerpProxy   common.Address
	PoolTknAddr []common.Address
	RpcClients  []*ethclient.Client
	FlipsideKey string
}

func NewApp(v *viper.Viper) (*App, error) {
	config, err := utils.LoadConfig(v.GetString(env.CONFIG_PATH))
	if err != nil {
		return nil, err
	}
	app := App{
		PerpProxy:   config.PerpAddr,
		PoolTknAddr: config.PoolTknAddr,
	}
	app.RpcClients = make([]*ethclient.Client, 0)
	for _, url := range config.RpcUrls {
		rpc, err := ethclient.Dial(url)
		if err != nil {
			slog.Error("failed to connect to the Ethereum client " + url + " (skipping this one):" + err.Error())
			continue
		}
		app.RpcClients = append(app.RpcClients, rpc)
	}
	if len(app.RpcClients) == 0 {
		return nil, errors.New("failed to create rpcs")
	}
	app.FlipsideKey = v.GetString(env.FLIPSIDE_API_KEY)
	if app.FlipsideKey == "" {
		return nil, errors.New("no flipside key found")
	}
	return &app, nil
}

func (app *App) Balances(req utils.APIBalancesPayload) (utils.APIBalancesResponse, error) {

	fromBlock := app.DbGetStartBlock()
	fsSet, err := app.flipsideGetAddresses(fromBlock, req.BlockNumber)
	if err != nil {
		return utils.APIBalancesResponse{}, err
	}
	//fmt.Println(fsSet)
	err = app.DBFillReceivers(fsSet, fromBlock, req.BlockNumber)
	if err != nil {
		return utils.APIBalancesResponse{}, errors.New("failed inserting flipside result to DB:" + err.Error())
	}
	return utils.APIBalancesResponse{}, nil
}

// DbGetStartBlock looks up the latest block for which
// we have stored receiver addresses
func (app *App) DbGetStartBlock() uint64 {
	query := `SELECT coalesce(0, max(to_block)) FROM receivers`
	var block uint64
	err := app.Db.QueryRow(query).Scan(&block)
	if err == sql.ErrNoRows {
		return block
	}
	if err != nil {
		slog.Error("Error for DbGetStartBlock" + err.Error())
		return block
	}
	return max(191682586, block+1)
}

func (app *App) DBFillReceivers(fsSet *utils.FSResultSet, fromBlock, toBlock uint64) error {
	// Prepare the insert statement
	stmt, err := app.Db.Prepare("INSERT INTO receivers(addr, from_block, to_block) VALUES($1, $2, $3)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Insert each address
	for _, row := range fsSet.Rows {
		addr := row.([]interface{})[0]
		_, err := stmt.Exec(addr, fromBlock, toBlock)
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
