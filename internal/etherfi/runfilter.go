package etherfi

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

func (app *App) RunFilter() {
	var wg sync.WaitGroup
	wg.Add(2)
	slog.Info("Filter for events")
	go func() {
		defer wg.Done()
		delegateBlock := app.DbGetDelegateStartBlock()
		delegates, upToBlockD, err := app.Filterer.FilterDelegateEvts(delegateBlock, 0)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		msg := fmt.Sprintf("FilterDelegateEvts found %d delegation events", len(delegates))
		slog.Info(msg)
		err = app.DBInsertDelegates(delegates, upToBlockD)
		if err != nil {
			slog.Error(err.Error())
		}
	}()

	go func() {
		defer wg.Done()
		transferBlock := app.DbGetShTknTransferStartBlock()
		transfers, upToBlockT, err := app.Filterer.FilterTransferEvts(transferBlock, 0)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		msg := fmt.Sprintf("FilterTransferEvts found %d transfer events", len(transfers))
		slog.Info(msg)
		err = app.DBInsertShTknTransfer(transfers, upToBlockT)
		if err != nil {
			slog.Error(err.Error())
		}
	}()
	wg.Wait()
	slog.Info("Event filterer completed")
	// Schedule the next call of Scan in 2 minutes
	time.AfterFunc(2*time.Minute, app.RunFilter)
}
