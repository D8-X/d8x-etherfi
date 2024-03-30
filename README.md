# d8x-etherfi

Etherfi Integration

## GET Endpoint `/contracts`

- GET endpoint with no arguments
- Returns the relevant contracts that are directly holding WEETH

## POST Endpoint `/balances`

- Post endpoint with arguments blockNumber and a possibly empty list of addresses
- Returns the relevant contracts that are directly holding WEETH
- If addresses is empty, all token holders at the given block are returned

Payload example:

```
{
	"blockNumber": 195685403,
	"addresses": []
}
```

Result example:
The `effective_balance` is provided as a floating number.

```
{
    "Result": [
        {
            "address": "0x337a3778244159f37c016196a8e1038a811a34c9",
            "effective_balance": 3635.689148
        }
    ]
}
```

# Dev

## Contracts

Generate the ABI:
`abigen --abi internal/contracts/abi/ERC20.json --pkg contracts --type Erc20 --out erc20.go`

## Config

```
{
    "perpAddr":     "0x8f8BccE4c180B699F81499005281fA89440D1e95", <-- perpetual manager address
    "poolShareTknAddr": "0xc21950e41121C2c52DC8074713514ddBAD678258", <-- share token address for the relevant pool collateralized in WEETH
    "poolTknAddr" : "0xaf88d065e77c8cc2239327c5edb3a432268e5831", <-- address of the pool token (WEETH)
    "poolTknDecimals": 6, <-- number of decimals of the pool token to conver the ownership to float
    "rpcUrl": ["https://arbitrum.llamarpc.com", "https://arb1.arbitrum.io/rpc"] <-- RPC urls that will be used for queries
}
```
