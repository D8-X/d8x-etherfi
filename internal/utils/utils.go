package utils

import (
	"encoding/json"
	"log/slog"
	"os"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
)

type APIBalancesPayload struct {
	BlockNumber uint64           `json:"blockNumber"`
	Addresses   []common.Address `json:"addresses"`
}

type APIBalancesResponse struct {
	Result []Balance `json:"Result"`
}

type Balance struct {
	Address    string  `json:"address"`
	EffBalance float64 `json:"effective_balance"`
}

type FSResultSet struct {
	ColumnNames      []string               `json:"columnNames"`
	ColumnTypes      []string               `json:"columnTypes"`
	Rows             []interface{}          `json:"rows"`
	Page             FSPageStats            `json:"page"`
	Sql              string                 `json:"sql"`
	Format           string                 `json:"format"`
	OriginalQueryRun map[string]interface{} `json:"originalQueryRun"`
}

type FSPageStats struct {
	CurrentPageNumber float64 `json:"currentPageNumber"`
	CurrentPageSize   float64 `json:"currentPageSize"`
	TotalRows         float64 `json:"totalRows"`
	TotalPages        float64 `json:"totalPages"`
}

type Config struct {
	PerpAddr    common.Address   `json:"perpAddr"`
	PoolTknAddr []common.Address `json:"poolTknAddr"`
	RpcUrls     []string         `json:"rpcUrl"`
}

func LoadConfig(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error(err.Error())
		return Config{}, err
	}
	var conf Config
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return Config{}, err
	}
	return conf, nil
}

func IsValidEvmAddr(addr string) bool {
	// Define a regular expression pattern for Ethereum addresses
	// It should start with "0x" followed by 40 hexadecimal characters
	pattern := "^0x[0-9a-fA-F]{40}$"

	// Compile the regular expression
	re := regexp.MustCompile(pattern)

	// Check if the address matches the pattern
	return re.MatchString(addr)
}
