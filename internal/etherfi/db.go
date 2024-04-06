package etherfi

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/filterer"
)

// dbGetShareTokenHolders looks for all addresses that have
// ever received a pool share token up to the given block
func (app *App) dbGetShareTokenHolders(blockNum uint64) ([]string, error) {

	query := `SELECT distinct("to") FROM sh_tkn_transfer WHERE block <= $1 AND chain_id=$2`
	rows, err := app.Db.Query(query, blockNum, app.Sdk.ChainConfig.ChainId)
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

// DBGetLatestBlock looks for the last block for which data has been
// collected for both the delegation and transfer events
func (app *App) DBGetLatestBlock() uint64 {
	return min(app.DbGetDelegateStartBlock(), app.DbGetShTknTransferStartBlock())
}

// DbGetStartBlock looks up the latest block for which
// we have stored receiver addresses
func (app *App) DbGetShTknTransferStartBlock() uint64 {
	query := `SELECT coalesce(max(to_block),0) FROM sh_tkn_transfer WHERE chain_id=$1`
	var block uint64
	err := app.Db.QueryRow(query, app.Sdk.ChainConfig.ChainId).Scan(&block)
	if err == sql.ErrNoRows {
		return block
	}
	if err != nil {
		slog.Error("Error for DbGetStartBlock" + err.Error())
		return block
	}
	return max(app.Genesis, block+1)
}

// DbGetStartBlock looks up the latest block for which
// we have stored receiver addresses
func (app *App) DbGetDelegateStartBlock() uint64 {
	query := `SELECT coalesce(max(to_block),0) FROM delegates WHERE chain_id=$1`
	var block uint64
	err := app.Db.QueryRow(query, app.Sdk.ChainConfig.ChainId).Scan(&block)
	if err == sql.ErrNoRows {
		return block
	}
	if err != nil {
		slog.Error("Error for DbGetStartBlock" + err.Error())
		return block
	}
	return max(app.Genesis, block+1)
}

// DBInsertShTknTransfer inserts the results FSResultSet for flipsGetPoolShrTknHolders into the database
func (app *App) DBInsertShTknTransfer(transfers []interface{}, toBlock uint64) error {
	// Prepare the insert statement
	stmt, err := app.Db.Prepare(`INSERT INTO sh_tkn_transfer("from", "to", block, to_block, sh_tkn, chain_id) VALUES($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	chainId := app.Sdk.ChainConfig.ChainId
	tkn_addr := app.PoolShareTknAddr.Hex()
	// Insert each address
	for _, row := range transfers {
		transfer := row.(filterer.Transfer)
		_, err := stmt.Exec(transfer.From, transfer.To, transfer.BlockNr, toBlock, tkn_addr, chainId)
		if err != nil {
			return err
		}
	}

	return nil
}

// DBInsertDelegates inserts the delegate array into the database
func (app *App) DBInsertDelegates(delegates []interface{}, toBlock uint64) error {
	// Prepare the insert statement
	stmt, err := app.Db.Prepare("INSERT INTO delegates(addr, delegate, block, index, to_block, chain_id) VALUES($1, $2, $3, $4, $5, $6)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	chainId := app.Sdk.ChainConfig.ChainId
	// Insert each event
	for _, row := range delegates {
		dlgt := row.(filterer.Delegate)
		_, err := stmt.Exec(dlgt.Addr, dlgt.Delegate, dlgt.BlockNr, dlgt.Index, toBlock, chainId)
		if err != nil {
			return err
		}
	}
	return nil
}

// DbFindStrategyDelegates finds addresses for which we have to
// re-assign the tokens from "addr" to "delegate"
func (app *App) DbFindStrategyDelegates(toBlock uint64) ([]string, []string, error) {
	query := `SELECT DISTINCT addr, delegate FROM delegates where block <= $1 and chain_id=$2 and index=$3`
	rows, err := app.Db.Query(query, toBlock, app.Sdk.ChainConfig.ChainId, env.DELEGATE_IDX_STRATEGY)
	if err != nil {
		return nil, nil, errors.New("DbFindStrategyDelegates" + err.Error())
	}
	defer rows.Close()
	addr := make([]string, 0)
	delegate := make([]string, 0)
	for rows.Next() {
		var a, b string
		rows.Scan(&a, &b)
		addr = append(addr, a)
		delegate = append(delegate, b)
	}
	return addr, delegate, nil
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
