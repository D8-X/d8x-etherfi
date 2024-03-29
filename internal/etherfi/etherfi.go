package etherfi

import (
	"errors"
	"log/slog"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/viper"
)

type App struct {
	PerpProxy   common.Address
	PoolTknAddr []common.Address
	RpcClients  []*ethclient.Client
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
	return &app, nil
}
