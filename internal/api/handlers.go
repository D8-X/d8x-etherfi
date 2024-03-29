package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/D8-X/d8x-etherfi/internal/etherfi"
)

func onHolderContracts(w http.ResponseWriter, app *etherfi.App) {
	type response struct {
		HolderContracts []string `json:"holderContracts"`
	}
	res := response{
		HolderContracts: []string{app.PerpProxy.Hex()},
	}
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(res)
	if err != nil {
		slog.Error("error in onHolderContracts")
		errMsg := "Unavailable"
		http.Error(w, string(formatError(errMsg)), http.StatusInternalServerError)
		return
	} else {
		slog.Info("onHolderContracts request answered")
	}
	w.Write(jsonResponse)
}
