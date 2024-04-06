package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/D8-X/d8x-etherfi/internal/etherfi"
	"github.com/D8-X/d8x-etherfi/internal/utils"
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

func onBalances(w http.ResponseWriter, r *http.Request, app *etherfi.App) {
	// Read the JSON data from the request body
	var jsonData []byte
	if r.Body != nil {
		defer r.Body.Close()
		jsonData, _ = io.ReadAll(r.Body)
	}
	var req utils.APIBalancesPayload
	err := json.Unmarshal(jsonData, &req)
	if err != nil {
		errMsg := `Wrong argument types. Usage:
		{
		   'blockNumber': 195374242,
		   'addresses': ['0xaCFe...']
	    }`
		errMsg = strings.ReplaceAll(errMsg, "\t", "")
		errMsg = strings.ReplaceAll(errMsg, "\n", "")
		http.Error(w, string(formatError(errMsg)), http.StatusBadRequest)
		slog.Info("onBalances invalid request:" + err.Error())
		return
	}
	// check input
	for _, addr := range req.Addresses {
		if !utils.IsValidEvmAddr(addr) {
			http.Error(w, string(formatError("malformated address in request")), http.StatusBadRequest)
			slog.Info("malformated address in request")
			return
		}
	}
	lb := app.DBGetLatestBlock()
	if uint64(req.BlockNumber) > lb {
		msg := fmt.Sprintf("queried block %d but only %d available", req.BlockNumber, lb)
		slog.Error(msg)
		http.Error(w, string(formatError("requested block not available")), http.StatusInternalServerError)
		return
	}
	res, err := app.Balances(req)
	if err != nil {
		slog.Error("Could not determine balances:" + err.Error())
		http.Error(w, string(formatError("request failed")), http.StatusInternalServerError)
		return
	}
	// Set the Content-Type header to application/json
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(res)
	if err != nil {
		slog.Error("Failed parsing balance response:" + err.Error())
		http.Error(w, string(formatError("request failed")), http.StatusInternalServerError)
		return
	}
	msg := fmt.Sprintf("Responding to balance request for %d addresses on block %d", len(req.Addresses), req.BlockNumber)
	slog.Info(msg)
	// Write the JSON response
	w.Write(jsonResponse)
}
