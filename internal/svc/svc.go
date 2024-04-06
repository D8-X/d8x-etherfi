package svc

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/D8-X/d8x-etherfi/internal/api"
	"github.com/D8-X/d8x-etherfi/internal/db"
	"github.com/D8-X/d8x-etherfi/internal/env"
	"github.com/D8-X/d8x-etherfi/internal/etherfi"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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
	if err != nil {
		slog.Error("Error:" + err.Error())
		return
	}
	// connect db before running migrations
	if err := app.ConnectDB(v.GetString(env.DATABASE_DSN)); err != nil {
		slog.Error("connecting to db", "error", err)
		return
	}
	// Run migrations on startup. If migrations fail - exit.
	if err := runMigrations(v.GetString(env.DATABASE_DSN)); err != nil {
		slog.Error("running migrations", "error", err)
		os.Exit(1)
		return
	} else {
		slog.Info("migrations run completed")
	}
	// start go routine to periodically filter for events
	go app.RunFilter()

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
		env.DATABASE_DSN,
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

func runMigrations(postgresDSN string) error {

	source, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance(
		"MigrationsFS",
		source,
		postgresDSN,
	)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil {
		// Only return error if it's not "no change" error
		if err.Error() != "no change" {
			return err
		}
	}

	e1, e2 := m.Close()
	if e1 != nil {
		return e1
	}
	if e2 != nil {
		return e2
	}
	return nil
}
