package main

import (
	"math/big"
	"strings"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	// defaultSpread 默认点差 (0.7%)
	// Market Maker 通过点差获取利润，RFQ 报价使用与 pricing levels 相同的点差
	defaultSpread = 0.007
)

// 预计算的 10 的幂次方缓存，避免重复计算
var (
	pow10Cache = make(map[int64]*big.Int)
)

// =============================================================================
// 数据结构
// =============================================================================

// TokenPrice 代币价格信息
type TokenPrice struct {
	PriceUSD float64 // USD 价格
	Decimals uint32  // 代币精度 (例如: 18 表示 10^18 最小单位 = 1 代币)
}

// =============================================================================
// 价格映射表
// =============================================================================

// priceMap 代币价格映射表 (地址小写 -> 价格信息)
var priceMap = map[string]TokenPrice{
	strings.ToLower(wbnbAddress):     {PriceUSD: wbnbUsdtMid, Decimals: wbnbDecimals},
	strings.ToLower(usdcAddress):     {PriceUSD: 1.0, Decimals: usdcDecimals},
	strings.ToLower(usdtAddress):     {PriceUSD: 1.0, Decimals: usdtDecimals},
	strings.ToLower(cashPlusAddress): {PriceUSD: cashPlusUsdtMid, Decimals: cashPlusDecimals},
}

// =============================================================================
// 公共函数
// =============================================================================

// GetTokenPrice 根据代币地址获取价格信息
func GetTokenPrice(tokenAddress string) (TokenPrice, bool) {
	price, ok := priceMap[strings.ToLower(tokenAddress)]
	return price, ok
}

// CalculateMakerAmount 根据 TakerAmount 计算 MakerAmount (卖出 taker 换取 maker)
//
// 场景: 用户卖出 takerAmount 的 takerToken，获得多少 makerToken
// 公式: makerAmount = takerAmount × (takerPrice / makerPrice) × (1 - spread)
//
// 示例: 1 WBNB → USDC (价格 879 USDC/WBNB, 点差 0.7%)
//
//	makerAmount = 1 × (879 / 1) × 0.993 = 872.847 USDC
func CalculateMakerAmount(takerToken, makerToken string, takerAmount *big.Int) *big.Int {
	takerPrice, takerOk := GetTokenPrice(takerToken)
	makerPrice, makerOk := GetTokenPrice(makerToken)

	if !takerOk || !makerOk {
		return new(big.Int).Set(takerAmount) // 价格未知时返回原值
	}

	// 计算: takerAmount × (takerPrice / makerPrice) × (1 - spread)
	priceRatio := takerPrice.PriceUSD / makerPrice.PriceUSD
	spreadFactor := 1 - defaultSpread

	result := new(big.Float).SetInt(takerAmount)
	result.Mul(result, big.NewFloat(priceRatio*spreadFactor))

	// 调整精度差异
	adjustDecimals(result, makerPrice.Decimals, takerPrice.Decimals)

	amount, _ := result.Int(nil)
	return amount
}

// CalculateTakerAmount 根据 MakerAmount 计算 TakerAmount (需要多少 taker 换取 maker)
//
// 场景: 用户想获得 makerAmount 的 makerToken，需要卖出多少 takerToken
// 公式: takerAmount = makerAmount × (makerPrice / takerPrice) × (1 + spread)
//
// 示例: ? WBNB → 20 USDC (价格 879 USDC/WBNB, 点差 0.7%)
//
//	takerAmount = 20 × (1 / 879) × 1.007 = 0.0229 WBNB
func CalculateTakerAmount(takerToken, makerToken string, makerAmount *big.Int) *big.Int {
	takerPrice, takerOk := GetTokenPrice(takerToken)
	makerPrice, makerOk := GetTokenPrice(makerToken)

	if !takerOk || !makerOk {
		return new(big.Int).Set(makerAmount) // 价格未知时返回原值
	}

	// 计算: makerAmount × (makerPrice / takerPrice) × (1 + spread)
	priceRatio := makerPrice.PriceUSD / takerPrice.PriceUSD
	spreadFactor := 1 + defaultSpread

	result := new(big.Float).SetInt(makerAmount)
	result.Mul(result, big.NewFloat(priceRatio*spreadFactor))

	// 调整精度差异
	adjustDecimals(result, takerPrice.Decimals, makerPrice.Decimals)

	amount, _ := result.Int(nil)
	return amount
}

// ConvertNativeFeeToToken 将原生代币 (BNB) 费用转换为目标代币数量
//
// 参数:
//   - feeNative: BNB 费用数量 (浮点数，例如 0.017 表示 0.017 BNB)
//   - tokenAddress: 目标代币地址
//
// 返回: 目标代币数量 (带精度的 big.Int)
//
// 示例: 0.017 BNB → USDC (BNB 价格 $879)
//
//	feeUSD = 0.017 × 879 = $14.94
//	tokenAmount = 14.94 / 1.0 = 14.94 USDC
//	返回值 = 14.94 × 10^18 = 14940000000000000000
func ConvertNativeFeeToToken(feeNative float64, tokenAddress string) *big.Int {
	if feeNative <= 0 {
		return big.NewInt(0)
	}

	// 获取 BNB 和目标代币价格
	bnbPrice, bnbOk := GetTokenPrice(wbnbAddress)
	tokenPrice, tokenOk := GetTokenPrice(tokenAddress)
	if !bnbOk || !tokenOk {
		return big.NewInt(0)
	}

	// 计算: (feeNative × bnbPrice / tokenPrice) × 10^decimals
	tokenAmount := (feeNative * bnbPrice.PriceUSD) / tokenPrice.PriceUSD

	result := new(big.Float).SetFloat64(tokenAmount)
	result.Mul(result, new(big.Float).SetInt(getPow10(int64(tokenPrice.Decimals))))

	amount, _ := result.Int(nil)
	return amount
}

// =============================================================================
// 内部辅助函数
// =============================================================================

// adjustDecimals 调整精度差异
// targetDecimals: 目标代币精度
// sourceDecimals: 源代币精度
func adjustDecimals(value *big.Float, targetDecimals, sourceDecimals uint32) {
	if targetDecimals == sourceDecimals {
		return
	}

	diff := int64(targetDecimals) - int64(sourceDecimals)
	factor := new(big.Float).SetInt(getPow10(abs(diff)))

	if diff > 0 {
		value.Mul(value, factor) // 目标精度更高，乘以 10^diff
	} else {
		value.Quo(value, factor) // 目标精度更低，除以 10^|diff|
	}
}

// getPow10 获取 10^n (带缓存)
func getPow10(n int64) *big.Int {
	if n < 0 {
		n = -n
	}
	if cached, ok := pow10Cache[n]; ok {
		return cached
	}
	result := new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
	pow10Cache[n] = result
	return result
}

// abs 返回 int64 的绝对值
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
