package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-etherfi/internal/etherfi"
	"github.com/D8-X/d8x-etherfi/internal/utils"
)

func onHolderContracts(w http.ResponseWriter, r *http.Request, app *etherfi.App) {
	blockReq := r.URL.Query().Get("blockNumber")
	var block *big.Int
	if blockReq != "" {
		b, err := strconv.Atoi(blockReq)
		if err != nil {
			slog.Error("error in onHolderContracts")
			errMsg := "invalid block number"
			http.Error(w, string(formatError(errMsg)), http.StatusBadRequest)
			return
		}
		block = big.NewInt(int64(b))
	}

	type response struct {
		HolderContracts []string  `json:"holderContracts"`
		Balance         []float64 `json:"balance"`
		Status          string    `json:"status"`
	}
	res := response{
		HolderContracts: []string{app.PerpProxy.Hex()},
		Status:          "ok",
	}
	res.Balance = make([]float64, 0, len(res.HolderContracts))

	client := app.RpcMngr.GetNextRpc()
	app.RpcMngr.WaitForToken(client)
	var bal []*big.Int
	var err error
	for trial := 0; trial < 3; trial++ {
		rpc := app.RpcMngr.GetNextRpc()
		app.RpcMngr.WaitForToken(rpc)
		bal, err = etherfi.QueryMultiTokenBalance(client, strings.ToLower(app.PoolTknAddr.Hex()), res.HolderContracts, block)
		if err == nil {
			break
		}
		slog.Info("onHolderContracts: QueryMultiTokenBalance unavailable, retrying")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		slog.Error("onHolderContracts:" + err.Error())
		res.Status = "balance unavailable"
	} else {
		for k := range res.HolderContracts {
			res.Balance = append(res.Balance, utils.DecNToFloat(bal[k], app.PoolTknDecimals))
		}
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

func onGetBalances(w http.ResponseWriter, r *http.Request, app *etherfi.App) {
	blockReq := r.URL.Query().Get("blockNumber")
	addrs := r.URL.Query()["addresses"]
	block := app.DBGetLatestBlock()
	if blockReq != "" {
		blockNum, err := strconv.Atoi(blockReq)
		if err != nil {
			block = min(block, uint64(blockNum))
		}
	}
	// check input
	for k, addr := range addrs {
		if !utils.IsValidEvmAddr(addr) {
			http.Error(w, string(formatError("malformated address in request")), http.StatusBadRequest)
			slog.Info("malformated address in get request")
			return
		}
		addrs[k] = strings.ToLower(addrs[k])
	}
	req := utils.APIBalancesPayload{
		BlockNumber: block,
		Addresses:   addrs,
	}
	balanceResponse(req, w, app)
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
	for k, addr := range req.Addresses {
		if !utils.IsValidEvmAddr(addr) {
			http.Error(w, string(formatError("malformated address in request")), http.StatusBadRequest)
			slog.Info("malformated address in request")
			return
		}
		req.Addresses[k] = strings.ToLower(req.Addresses[k])
	}
	lb := app.DBGetLatestBlock()
	if uint64(req.BlockNumber) > lb {
		msg := fmt.Sprintf("queried block %d but only %d available", req.BlockNumber, lb)
		slog.Error(msg)
		http.Error(w, string(formatError("requested block not available")), http.StatusInternalServerError)
		return
	}
	balanceResponse(req, w, app)
}

// balanceResponse is shared between the GET and POST request
func balanceResponse(req utils.APIBalancesPayload, w http.ResponseWriter, app *etherfi.App) {
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
