package utils

import (
	"encoding/json"
	"log/slog"
	"os"
	"regexp"

	config "github.com/D8-X/d8x-futures-go-sdk/config"
	"github.com/ethereum/go-ethereum/common"
)

type APIBalancesPayload struct {
	BlockNumber int64    `json:"blockNumber"`
	Addresses   []string `json:"addresses"`
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
	ConfigFile
	PerpAddr common.Address `json:"perpAddr"`
}

type ConfigFile struct {
	ChainId     int64    `json:"chainId"`
	PoolId      int32    `json:"poolId"`
	Genesis     uint64   `json:"genesisBlock"`
	RpcUrls     []string `json:"rpcUrl"`
	RpcUrlsFltr []string `json:"rpcUrlFilterer"`
}

func LoadConfig(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error(err.Error())
		return Config{}, err
	}
	var conf ConfigFile
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return Config{}, err
	}
	// Assign ConfigFile to Config and fill remaining values
	c, err := config.GetDefaultChainConfigFromId(int64(conf.ChainId))
	if err != nil {
		return Config{}, err
	}
	config := Config{
		ConfigFile: conf,
		PerpAddr:   c.ProxyAddr,
	}

	return config, nil
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
