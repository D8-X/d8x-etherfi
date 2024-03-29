package svc

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/D8-X/d8x-etherfi/internal/api"

	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/etherfi"
	"github.com/spf13/viper"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	})))
}

func Run() {
	v, err := loadEnv()
	if err != nil {
		slog.Error("Error:" + err.Error())
		return
	}
	app, err := etherfi.NewApp(v)
	api.StartApiServer(app, v.GetString(env.API_BIND_ADDR), v.GetString(env.API_PORT))
}

func loadEnv() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	if err := v.ReadInConfig(); err != nil {
		slog.Info("could not load .env file" + err.Error() + " using automatic envs")
	}
	v.AutomaticEnv()

	requiredEnvs := []string{
		env.CONFIG_PATH,
		env.API_BIND_ADDR,
		env.API_PORT,
	}

	for _, e := range requiredEnvs {
		if !v.IsSet(e) {
			return nil, fmt.Errorf("required environment variable not set %s", e)
		}
	}

	return v, nil
}
