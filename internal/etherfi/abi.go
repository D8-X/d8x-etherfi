package etherfi

const ERC20_BALANCE_ABI = `[{"inputs":[{"internalType":"address","name":"account","type":"address"}],
							 "name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],
							 "stateMutability":"view","type":"function"}]`

const AVAIL_CASH_ABI = `[{
	"inputs": [
	  {
		"internalType": "uint24",
		"name": "_iPerpetualId",
		"type": "uint24"
	  },
	  {
		"internalType": "address",
		"name": "_traderAddr",
		"type": "address"
	  }
	],
	"name": "getAvailableCash",
	"outputs": [
	  {
		"internalType": "int128",
		"name": "",
		"type": "int128"
	  }
	],
	"stateMutability": "view",
	"type": "function"
  }]`
