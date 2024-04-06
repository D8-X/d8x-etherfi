package utils

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/ethereum/go-ethereum/ethclient"
)

type RpcHandler struct {
	RpcClients []*ethclient.Client
	lastIdx    int
	Buckets    map[*ethclient.Client]*TokenBucket
	mutex      *sync.Mutex
}

func (h *RpcHandler) Init(rpcUrls []string, capacity int, refillRate float64) error {
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
	h.mutex = new(sync.Mutex)
	h.Buckets = make(map[*ethclient.Client]*TokenBucket, len(h.RpcClients))
	for _, rpc := range h.RpcClients {
		h.Buckets[rpc] = NewTokenBucket(capacity, refillRate)
	}
	return nil
}

func (h *RpcHandler) WaitForToken(rpc *ethclient.Client) {
	h.Buckets[rpc].WaitForToken("", false)
}

func (h *RpcHandler) GetRpc() *ethclient.Client {
	return h.RpcClients[h.lastIdx]
}

func (h *RpcHandler) GetNextRpc() *ethclient.Client {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.lastIdx = (h.lastIdx + 1) % len(h.RpcClients)
	return h.RpcClients[h.lastIdx]
}
