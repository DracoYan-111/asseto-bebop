package main

import (
	"math/big"
	"strings"
)

// =============================================================================
// 价格计算模块 - 使用动态 Bid/Ask 价格
// =============================================================================

// 预计算的 10 的幂次方缓存，避免重复计算
var (
	pow10Cache = make(map[int64]*big.Int)
)

// =============================================================================
// 公共函数
// =============================================================================

// CalculateMakerAmount 根据 TakerAmount 计算 MakerAmount (卖出 taker 换取 maker)
//
// 场景: 用户卖出 takerAmount 的 takerToken，获得多少 makerToken
// 使用运营设定的 Bid 价格（你买入 taker 的价格）
//
// 示例: 用户卖 1 WBNB → 获得 USDT
// - WBNB/USDT 交易对 Bid = 870 (你愿意以 870 USDT 买入 1 WBNB)
// - makerAmount = 1 × 870 = 870 USDT
func CalculateMakerAmount(takerToken, makerToken string, takerAmount *big.Int) *big.Int {
	// 尝试获取 taker/quote 的报价（taker 是 base）
	price, ok := globalPriceStore.GetBidPrice(takerToken, makerToken)
	if !ok {
		// 尝试反向：maker/taker 的报价（maker 是 base）
		// 用户卖 taker 买 maker = 你卖 maker，使用 Ask 价格的倒数
		askPrice, askOk := globalPriceStore.GetAskPrice(makerToken, takerToken)
		if !askOk {
			// 价格未知，返回原值（兜底）
			return new(big.Int).Set(takerAmount)
		}
		// price = 1 / askPrice (反向计算)
		price = 1.0 / askPrice
	}

	// 计算: takerAmount × price
	result := new(big.Float).SetInt(takerAmount)
	result.Mul(result, big.NewFloat(price))

	// 调整精度差异
	takerDecimals := GetTokenDecimals(takerToken)
	makerDecimals := GetTokenDecimals(makerToken)
	adjustDecimals(result, makerDecimals, takerDecimals)

	amount, _ := result.Int(nil)
	return amount
}

// CalculateTakerAmount 根据 MakerAmount 计算 TakerAmount (需要多少 taker 换取 maker)
//
// 场景: 用户想获得 makerAmount 的 makerToken，需要卖出多少 takerToken
// 使用运营设定的 Ask 价格（你卖出 maker 的价格）
//
// 示例: 用户想获得 1000 USDT → 需付多少 WBNB?
// - WBNB/USDT 交易对 Ask = 888 (你愿意以 888 USDT 卖出 1 WBNB)
// - takerAmount = 1000 / 888 = 1.126 WBNB
func CalculateTakerAmount(takerToken, makerToken string, makerAmount *big.Int) *big.Int {
	// 尝试获取 taker/quote 的报价（taker 是 base）
	// 用户买 maker 卖 taker = 你买 taker 卖 maker
	price, ok := globalPriceStore.GetAskPrice(takerToken, makerToken)
	if !ok {
		// 尝试反向：maker/taker 的报价（maker 是 base）
		// 用户买 maker = MM 卖 maker = 使用 Ask 价格的倒数
		askPrice, askOk := globalPriceStore.GetAskPrice(makerToken, takerToken)
		if !askOk {
			// 价格未知，返回原值（兜底）
			return new(big.Int).Set(makerAmount)
		}
		// price = 1 / askPrice (反向计算)
		price = 1.0 / askPrice
	}

	// 计算: makerAmount / price
	result := new(big.Float).SetInt(makerAmount)
	result.Quo(result, big.NewFloat(price))

	// 调整精度差异
	takerDecimals := GetTokenDecimals(takerToken)
	makerDecimals := GetTokenDecimals(makerToken)
	adjustDecimals(result, takerDecimals, makerDecimals)

	amount, _ := result.Int(nil)
	return amount
}

// ConvertNativeFeeToToken 将原生代币 (BNB) 费用转换为目标代币数量
//
// 使用 USD (USDT) 作为中间货币进行转换：BNB → USD → 目标代币
//
// 参数:
//   - feeNative: BNB 费用数量 (浮点数，例如 0.017 表示 0.017 BNB)
//   - tokenAddress: 目标代币地址
//   - useBidPrice: 是否使用 Bid 价格（true=用于扣除费用，false=用于添加费用）
//
// 价格选择逻辑:
//   - useBidPrice=true: 使用 Bid 价格（更高），扣除更多费用 → 对 MM 有利
//   - useBidPrice=false: 使用 Ask 价格（更低），添加更少费用 → 对用户稍有利
//
// 返回: 目标代币数量 (带精度的 big.Int)
func ConvertNativeFeeToToken(feeNative float64, tokenAddress string, useBidPrice bool) *big.Int {
	if feeNative <= 0 {
		return big.NewInt(0)
	}

	// 1. 获取 WBNB 的 USD 价格
	var wbnbUsdPrice float64
	var ok bool

	if useBidPrice {
		// 使用 Bid 价格（更高）→ 费用更大
		wbnbUsdPrice, ok = globalPriceStore.GetBidPrice(wbnbAddress, usdtAddress)
	} else {
		// 使用 Ask 价格（更低）→ 费用更小
		wbnbUsdPrice, ok = globalPriceStore.GetAskPrice(wbnbAddress, usdtAddress)
	}

	if !ok {
		// 备用：尝试反向
		if bid, bidOk := globalPriceStore.GetBidPrice(usdtAddress, wbnbAddress); bidOk {
			wbnbUsdPrice = 1.0 / bid
		} else if ask, askOk := globalPriceStore.GetAskPrice(usdtAddress, wbnbAddress); askOk {
			wbnbUsdPrice = 1.0 / ask
		} else {
			return big.NewInt(0)
		}
	}

	// 2. 计算费用的 USD 价值
	feeUSD := feeNative * wbnbUsdPrice

	// 3. 获取目标代币的 USD 价格
	var tokenUsdPrice float64
	normalizedAddr := strings.ToLower(tokenAddress)

	// 稳定币特殊处理：USDT/USDC 的 USD 价格为 1.0
	if normalizedAddr == strings.ToLower(usdtAddress) || normalizedAddr == strings.ToLower(usdcAddress) {
		tokenUsdPrice = 1.0
	} else if useBidPrice {
		// 使用 Bid 价格（更高）→ 转换后代币数量更少 → 对 MM 有利（扣除更少代币？）
		// 实际上：fee = feeUSD / tokenPrice，价格越高，代币数量越少
		// 对于 subtractFee，我们希望扣除更多代币，所以应该用更低的价格
		// 修正：subtractFee 应该用 Ask 价格（更低）让 fee 代币数量更大
		if tokenUsdAsk, ok := globalPriceStore.GetAskPrice(tokenAddress, usdtAddress); ok {
			tokenUsdPrice = tokenUsdAsk
		} else if tokenUsdAsk, ok := globalPriceStore.GetAskPrice(tokenAddress, usdcAddress); ok {
			tokenUsdPrice = tokenUsdAsk
		} else if tokenUsdBid, ok := globalPriceStore.GetBidPrice(tokenAddress, usdtAddress); ok {
			tokenUsdPrice = tokenUsdBid
		} else {
			return big.NewInt(0)
		}
	} else {
		// 使用 Bid 价格（更高）→ 转换后代币数量更少 → 对用户有利（少付）
		if tokenUsdBid, ok := globalPriceStore.GetBidPrice(tokenAddress, usdtAddress); ok {
			tokenUsdPrice = tokenUsdBid
		} else if tokenUsdBid, ok := globalPriceStore.GetBidPrice(tokenAddress, usdcAddress); ok {
			tokenUsdPrice = tokenUsdBid
		} else {
			return big.NewInt(0)
		}
	}

	// 4. 计算目标代币数量: feeUSD / tokenPriceUSD
	tokenAmount := feeUSD / tokenUsdPrice

	// 5. 应用代币精度
	tokenDecimals := GetTokenDecimals(tokenAddress)
	result := new(big.Float).SetFloat64(tokenAmount)
	result.Mul(result, new(big.Float).SetInt(getPow10(int64(tokenDecimals))))

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

// =============================================================================
// 兼容性函数（保持原有接口）
// =============================================================================

// TokenPrice 代币价格信息（兼容旧接口）
type TokenPrice struct {
	PriceUSD float64
	Decimals uint32
}

// GetTokenPrice 根据代币地址获取价格信息（兼容旧接口）
// 注意：此函数返回的是对 USDT 的中间价（Bid+Ask 的平均值）
func GetTokenPrice(tokenAddress string) (TokenPrice, bool) {
	addr := strings.ToLower(tokenAddress)

	// 稳定币直接返回 1.0
	if addr == strings.ToLower(usdtAddress) || addr == strings.ToLower(usdcAddress) {
		return TokenPrice{PriceUSD: 1.0, Decimals: GetTokenDecimals(tokenAddress)}, true
	}

	// 尝试获取对 USDT 的价格
	price, ok := globalPriceStore.GetPrice(tokenAddress, usdtAddress)
	if ok && len(price.Bids) > 0 && len(price.Asks) > 0 {
		midPrice := (price.Bids[0][0] + price.Asks[0][0]) / 2
		return TokenPrice{PriceUSD: midPrice, Decimals: GetTokenDecimals(tokenAddress)}, true
	}

	// 尝试获取对 USDC 的价格
	price, ok = globalPriceStore.GetPrice(tokenAddress, usdcAddress)
	if ok && len(price.Bids) > 0 && len(price.Asks) > 0 {
		midPrice := (price.Bids[0][0] + price.Asks[0][0]) / 2
		return TokenPrice{PriceUSD: midPrice, Decimals: GetTokenDecimals(tokenAddress)}, true
	}

	return TokenPrice{}, false
}
