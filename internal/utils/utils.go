package utils

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type RpcHandler struct {
	RpcClients []*ethclient.Client
	lastIdx    int
}

type APIBalancesPayload struct {
	BlockNumber int64            `json:"blockNumber"`
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
	PerpAddr         common.Address `json:"perpAddr"`
	PoolTknAddr      common.Address `json:"poolTknAddr"`
	PoolShareTknAddr common.Address `json:"poolShareTknAddr"`
	PoolTknDecimals  uint8          `json:"poolTknDecimals"`
	RpcUrls          []string       `json:"rpcUrl"`
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

func (h *RpcHandler) Init(rpcUrls []string) error {
	h.RpcClients = make([]*ethclient.Client, 0)
	for _, url := range rpcUrls {
		rpc, err := ethclient.Dial(url)
		if err != nil {
			slog.Error("failed to connect to the Ethereum client " + url + " (skipping this one):" + err.Error())
			continue
		}
		h.RpcClients = append(h.RpcClients, rpc)
	}
	if len(h.RpcClients) == 0 {
		return errors.New("failed to create rpcs")
	}
	return nil
}

func (h *RpcHandler) GetRpc() *ethclient.Client {
	return h.RpcClients[h.lastIdx]
}

func (h *RpcHandler) GetNextRpc() *ethclient.Client {
	h.lastIdx = (h.lastIdx + 1) % len(h.RpcClients)
	return h.RpcClients[h.lastIdx]
}
