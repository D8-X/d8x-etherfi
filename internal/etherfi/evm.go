package etherfi

import (
	"math/big"

	"github.com/D8-X/d8x-etherfi/internal/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func CreateErc20Instance(tokenAddr string, rpc *ethclient.Client) (*contracts.Erc20, error) {
	tknAddr := common.HexToAddress(tokenAddr)
	instance, err := contracts.NewErc20(tknAddr, rpc)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func QueryTokenBalance(tknCtrct *contracts.Erc20, tknOwnerAddr string, blockNumber *big.Int) (*big.Int, error) {
	ownerAddr := common.HexToAddress(tknOwnerAddr)
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	bal, err := tknCtrct.BalanceOf(&bind.CallOpts{}, ownerAddr)
	return bal, err
}

func QueryTokenTotalSupply(tknCtrct *contracts.Erc20, blockNumber *big.Int) (*big.Int, error) {
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	bal, err := tknCtrct.TotalSupply(&bind.CallOpts{})
	return bal, err
}
