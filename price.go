package main

import (
	"math/big"
	"strings"
)

// TokenPrice 存储代币的 USD 价格和精度
type TokenPrice struct {
	PriceUSD float64 // 代币的 USD 价格
	Decimals uint32  // 代币精度
}

// priceMap 存储所有代币的价格信息
// 注意：地址需要小写以便比较
var priceMap = map[string]TokenPrice{
	// WBNB - 价格约 879 USD
	strings.ToLower(wbnbAddress): {
		PriceUSD: wbnbUsdtMid,
		Decimals: wbnbDecimals,
	},
	// USDC - 稳定币，价格 1 USD
	strings.ToLower(usdcAddress): {
		PriceUSD: 1.0,
		Decimals: usdcDecimals,
	},
	// USDT - 稳定币，价格 1 USD
	strings.ToLower(usdtAddress): {
		PriceUSD: 1.0,
		Decimals: usdtDecimals,
	},
	// CASH+ - 价格约 106.71 USD
	strings.ToLower(cashPlusAddress): {
		PriceUSD: cashPlusUsdtMid,
		Decimals: cashPlusDecimals,
	},
}

// GetTokenPrice 获取代币价格信息
func GetTokenPrice(tokenAddress string) (TokenPrice, bool) {
	price, ok := priceMap[strings.ToLower(tokenAddress)]
	return price, ok
}

// CalculateMakerAmount 根据 TakerAmount 计算 MakerAmount
// 例如：用 1 WETH 换 USDC，价格是 1 ETH = 4000 USDC
// taker_amount = 1 WETH (with decimals), maker_amount = 4000 USDC (with decimals)
//
// 公式：maker_amount = taker_amount * (taker_price / maker_price) * (1 - spread)
// 例如：1 WBNB -> USDC: maker_amount = 1 * (940.12 / 1.0) * 0.993 = 933.5 USDC
func CalculateMakerAmount(takerToken, makerToken string, takerAmount *big.Int) *big.Int {
	takerPrice, takerOk := GetTokenPrice(takerToken)
	makerPrice, makerOk := GetTokenPrice(makerToken)

	if !takerOk || !makerOk {
		// 如果找不到价格，返回一个安全的默认值
		return new(big.Int).Set(takerAmount)
	}

	// 点差：Market Maker 需要一些利润
	// 对于 RFQ 报价，使用与 pricing levels 相同的点差
	spread := 0.007 // 0.7% 点差（比 levels 稍高以确保安全）

	// 使用 big.Float 进行精确计算
	// maker_amount = taker_amount * (taker_price / maker_price) * (1 - spread)
	takerAmountFloat := new(big.Float).SetInt(takerAmount)
	priceRatio := new(big.Float).SetFloat64(takerPrice.PriceUSD / makerPrice.PriceUSD)
	spreadFactor := new(big.Float).SetFloat64(1 - spread)

	// 计算结果
	makerAmountFloat := new(big.Float).Mul(takerAmountFloat, priceRatio)
	makerAmountFloat.Mul(makerAmountFloat, spreadFactor)

	// 处理精度差异
	// 如果 taker decimals != maker decimals，需要调整
	if takerPrice.Decimals != makerPrice.Decimals {
		decimalDiff := int64(makerPrice.Decimals) - int64(takerPrice.Decimals)
		if decimalDiff > 0 {
			// maker 精度更高，需要乘以 10^diff
			multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimalDiff), nil))
			makerAmountFloat.Mul(makerAmountFloat, multiplier)
		} else if decimalDiff < 0 {
			// maker 精度更低，需要除以 10^(-diff)
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(-decimalDiff), nil))
			makerAmountFloat.Quo(makerAmountFloat, divisor)
		}
	}

	// 转换为 big.Int（向下取整）
	makerAmount, _ := makerAmountFloat.Int(nil)
	return makerAmount
}

// CalculateTakerAmount 根据 MakerAmount 计算 TakerAmount
// 例如：用 ? WBNB 换 20 USDC，价格是 1 WBNB = 940.12 USDC
// maker_amount = 20 USDC, taker_amount = 20 / 940.12 = 0.02127 WBNB
//
// 公式：taker_amount = maker_amount * (maker_price / taker_price) * (1 + spread)
func CalculateTakerAmount(takerToken, makerToken string, makerAmount *big.Int) *big.Int {
	takerPrice, takerOk := GetTokenPrice(takerToken)
	makerPrice, makerOk := GetTokenPrice(makerToken)

	if !takerOk || !makerOk {
		// 如果找不到价格，返回一个安全的默认值
		return new(big.Int).Set(makerAmount)
	}

	// 点差：Market Maker 需要一些利润
	// 当计算 taker 需要支付多少时，要多收一点
	spread := 0.007 // 0.7% 点差

	// 使用 big.Float 进行精确计算
	// taker_amount = maker_amount * (maker_price / taker_price) * (1 + spread)
	makerAmountFloat := new(big.Float).SetInt(makerAmount)
	priceRatio := new(big.Float).SetFloat64(makerPrice.PriceUSD / takerPrice.PriceUSD)
	spreadFactor := new(big.Float).SetFloat64(1 + spread)

	// 计算结果
	takerAmountFloat := new(big.Float).Mul(makerAmountFloat, priceRatio)
	takerAmountFloat.Mul(takerAmountFloat, spreadFactor)

	// 处理精度差异
	if takerPrice.Decimals != makerPrice.Decimals {
		decimalDiff := int64(takerPrice.Decimals) - int64(makerPrice.Decimals)
		if decimalDiff > 0 {
			multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimalDiff), nil))
			takerAmountFloat.Mul(takerAmountFloat, multiplier)
		} else if decimalDiff < 0 {
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(-decimalDiff), nil))
			takerAmountFloat.Quo(takerAmountFloat, divisor)
		}
	}

	// 转换为 big.Int（向上取整，对 maker 有利）
	takerAmount, _ := takerAmountFloat.Int(nil)
	return takerAmount
}

// ConvertNativeFeeToToken 将原生代币（BNB）费用转换为指定代币的数量
// feeNative: BNB 数量（浮点数）
// tokenAddress: 目标代币地址
// 返回: 目标代币数量（带精度）
func ConvertNativeFeeToToken(feeNative float64, tokenAddress string) *big.Int {
	if feeNative <= 0 {
		return big.NewInt(0)
	}

	// 获取 BNB 价格（即 WBNB 价格）
	bnbPrice, bnbOk := GetTokenPrice(wbnbAddress)
	if !bnbOk {
		return big.NewInt(0)
	}

	// 计算费用的 USD 价值
	feeUSD := feeNative * bnbPrice.PriceUSD

	// 获取目标代币价格
	tokenPrice, tokenOk := GetTokenPrice(tokenAddress)
	if !tokenOk {
		return big.NewInt(0)
	}

	// 计算目标代币数量: feeUSD / tokenPriceUSD
	tokenAmount := feeUSD / tokenPrice.PriceUSD

	// 应用精度（乘以 10^decimals）
	decimalMultiplier := new(big.Float).SetInt(
		new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokenPrice.Decimals)), nil),
	)
	tokenAmountFloat := new(big.Float).SetFloat64(tokenAmount)
	tokenAmountFloat.Mul(tokenAmountFloat, decimalMultiplier)

	result, _ := tokenAmountFloat.Int(nil)
	return result
}
