package etherfi

import (
	"math/big"

	"github.com/D8-X/d8x-etherfi/internal/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/forta-network/go-multicall"
)

const ERC20_BALANCE_ABI = `[{"inputs":[{"internalType":"address","name":"account","type":"address"}],
							 "name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],
							 "stateMutability":"view","type":"function"}]`

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

func QueryMultiTokenBalance(client *ethclient.Client, tknAddr string, addrs []string, blockNumber *big.Int) ([]*big.Int, error) {
	contract, err := multicall.NewContract(ERC20_BALANCE_ABI, tknAddr)
	if err != nil {
		return nil, err
	}
	var opts *bind.CallOpts
	if blockNumber != nil {
		opts = new(bind.CallOpts)
		opts.BlockNumber = blockNumber
	}
	caller, err := multicall.New(client)
	if err != nil {
		return nil, err
	}
	type balanceOutput struct {
		Bal *big.Int
	}
	balances := make([]*big.Int, 0, len(addrs))
	inc := 500
	to := min(len(addrs), inc)
	from := 0
	for {
		calls := make([]*multicall.Call, 0, inc)
		for k := from; k < to; k++ {
			c := contract.NewCall(new(balanceOutput), "balanceOf", common.HexToAddress(addrs[k]))
			calls = append(calls, c)
		}
		res, err := caller.Call(opts, calls...)
		if err != nil {
			return nil, err
		}

		for _, call := range res {
			balances = append(balances, call.Outputs.(*balanceOutput).Bal)
		}
		from = to
		to = min(len(addrs), to+inc)
		if from >= len(addrs) {
			break
		}
	}
	return balances, nil
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
