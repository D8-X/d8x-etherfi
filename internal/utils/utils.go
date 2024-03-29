package utils

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

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
