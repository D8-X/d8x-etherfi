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
		delegateBlock := app.DbGetDelegateStartBlock() + 1
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
		app.LastBlockTo[0] = upToBlockD
	}()

	go func() {
		defer wg.Done()
		transferBlock := app.DbGetShTknTransferStartBlock() + 1
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
		app.LastBlockTo[1] = upToBlockT
	}()
	wg.Wait()
	slog.Info("Event filterer completed")
	// Schedule the next call of Scan in 2 minutes
	time.AfterFunc(2*time.Minute, app.RunFilter)
}
