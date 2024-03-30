package utils

import "math/big"

// DecNToFloat converts a decimal N number to
// the corresponding float number
func DecNToFloat(num *big.Int, decN uint8) float64 {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decN)), nil))
	numf := new(big.Float).SetInt(num)
	smallFloat := new(big.Float).Quo(numf, divisor)
	f, _ := smallFloat.Float64()
	return f
}
