package filterer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/D8-X/d8x-etherfi/internal/utils"
	d8xcontracts "github.com/D8-X/d8x-futures-go-sdk/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type Filterer struct {
	RpcMngr          utils.RpcHandler
	PerpProxy        common.Address
	PoolShareTknAddr common.Address
}

type Delegate struct {
	Addr     string
	Delegate string
	Index    int
	BlockNr  int
}

type Transfer struct {
	From    string
	To      string
	BlockNr int
}

func NewFilterer(rpcUrls []string, perpProxy, poolShareTknAddr common.Address) (*Filterer, error) {
	var F Filterer
	err := F.RpcMngr.Init(rpcUrls, 5, 5)
	if err != nil {
		return nil, err
	}
	F.PerpProxy = perpProxy
	F.PoolShareTknAddr = poolShareTknAddr
	return &F, nil
}

// processMultiPayEvents loops through blockchain events from the multipay contract and collects the data in
// the logs slice
func (F *Filterer) processDelegateEvents(iterator interface{}, logs *[]interface{}) {
	it := iterator.(*d8xcontracts.IPerpetualManagerSetDelegateIterator)
	for it.Next() {
		var dlgt Delegate
		event := it.Event
		dlgt.Addr = strings.ToLower(event.Trader.Hex())
		dlgt.Delegate = strings.ToLower(event.Delegate.Hex())
		dlgt.Index = int(event.Index.Uint64())
		dlgt.BlockNr = int(it.Event.Raw.BlockNumber)
		*logs = append(*logs, dlgt)
	}
}

// processMultiPayEvents loops through blockchain events from the multipay contract and collects the data in
// the logs slice
func (F *Filterer) processTransferEvents(iterator interface{}, logs *[]interface{}) {
	it := iterator.(*d8xcontracts.Erc20TransferIterator)
	for it.Next() {
		var transfer Transfer
		event := it.Event
		transfer.From = strings.ToLower(event.From.Hex())
		transfer.To = strings.ToLower(event.To.Hex())
		transfer.BlockNr = int(it.Event.Raw.BlockNumber)
		*logs = append(*logs, transfer)
	}
}

type EventType int

const (
	// SetDelegateEvent represents the SetDelegate event
	SetDelegateEvent EventType = iota
	// TokenTransferEvent represents some other event type
	TokenTransferEvent
)

func (F *Filterer) FilterTransferEvts(startBlock, endBlock uint64) ([]interface{}, uint64, error) {
	data, nowblock, err := F.FilterEvents(TokenTransferEvent, startBlock, endBlock)
	if err != nil {
		return nil, nowblock, errors.New("TransferEvents:" + err.Error())
	}
	return data, nowblock, nil
}

// FilterDelegates collects historical delegate events and updates the database
// set endBlock to zero to filter up to the latest block
func (F *Filterer) FilterDelegateEvts(startBlock, endBlock uint64) ([]interface{}, uint64, error) {
	data, nowblock, err := F.FilterEvents(SetDelegateEvent, startBlock, endBlock)
	if err != nil {
		return nil, nowblock, errors.New("TransferEvents:" + err.Error())
	}
	return data, nowblock, nil
}

func (F *Filterer) FilterEvents(eventType EventType, startBlock, endBlock uint64) ([]interface{}, uint64, error) {
	client := F.RpcMngr.GetNextRpc()
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return nil, 0, errors.New("failed to get block header: " + err.Error())
	}
	nowBlock := header.Number.Uint64()
	if endBlock == 0 {
		endBlock = nowBlock
	}
	var ctrct interface{}
	switch eventType {
	case SetDelegateEvent:
		ctrct, err = d8xcontracts.NewIPerpetualManager(F.PerpProxy, client)
	case TokenTransferEvent:
		ctrct, err = d8xcontracts.NewErc20(F.PoolShareTknAddr, client)
	default:
		return nil, 0, errors.New("unsupported event type")
	}

	if err != nil {
		return nil, 0, err
	}
	var logs []interface{}
	var reportCount int
	var pathLen = float64(nowBlock - startBlock)
	// filter payments in batches of 32_768 (and decreasing) blocks to avoid RPC limit
	deltaBlock := uint64(32_768)
	for trial := 0; trial < 7; trial++ {
		err = nil
		if trial > 0 {
			msg := fmt.Sprintf("Retrying with num blocks=%d (%d/%d)...", deltaBlock, trial, 7)
			slog.Info(msg)
			time.Sleep(time.Duration(5*trial) * time.Second)
		}
		for {
			endBlock := startBlock + deltaBlock
			if reportCount%100 == 0 {
				msg := fmt.Sprintf("Reading delegates from onchain: %.0f%%", 100-100*float64(nowBlock-startBlock)/pathLen)
				slog.Info(msg)
			}
			// Create an event iterator for events
			var endBlockPtr *uint64 = &endBlock
			if endBlock >= nowBlock {
				endBlockPtr = nil
			}
			opts := &bind.FilterOpts{
				Start:   startBlock,  // Starting block number
				End:     endBlockPtr, // Ending block (nil for latest)
				Context: context.Background(),
			}
			var iterator interface{}
			F.RpcMngr.WaitForToken(client)
			if eventType == SetDelegateEvent {
				iterator, err = ctrct.(*d8xcontracts.IPerpetualManager).FilterSetDelegate(opts, []common.Address{}, []common.Address{})
				if err != nil {
					break
				}
				F.processDelegateEvents(iterator, &logs)
			} else if eventType == TokenTransferEvent {
				iterator, err = ctrct.(*d8xcontracts.Erc20).FilterTransfer(opts, []common.Address{}, []common.Address{})
				if err != nil {
					break
				}
				F.processTransferEvents(iterator, &logs)
			} else {
				return nil, 0, errors.New("unknown event")
			}

			if endBlock >= nowBlock {
				break
			}
			startBlock = endBlock + 1
			reportCount += 1
		}
		if err == nil {
			break
		}
		slog.Info("Failed to create event iterator: " + err.Error())
		deltaBlock = deltaBlock / 2
	}
	if err != nil {
		return logs, 0, err
	}
	slog.Info("Reading events completed.")
	return logs, nowBlock, nil
}
