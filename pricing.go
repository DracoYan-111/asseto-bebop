package main

import (
	pb "asseto-bebop/proto"
	"asseto-bebop/utils"

	"google.golang.org/protobuf/proto"
)

// =============================================================================
// 常量定义
// =============================================================================

// ExecutionMode 交易执行模式
type ExecutionMode int

const (
	GaslessMode     ExecutionMode = iota // Bebop 代执行（用户无需支付 gas）
	SelfExecuteMode                      // 用户自执行（用户自己支付 gas）
)

// 价差配置
const (
	selfExecuteSpread = 0.003 // 自执行模式价差 (0.3%)
	gaslessSpread     = 0.005 // 代执行模式基础价差 (0.5%)
	gasEstimateUSD    = 10.0  // 预估 gas 成本 (USD)
	defaultChainID    = 56    // 默认链 ID (BSC)
)

// =============================================================================
// 数据结构
// =============================================================================

// PairLevel 交易对价格层级
type PairLevel struct {
	BaseAddress   string       // 基础代币地址
	BaseDecimals  uint32       // 基础代币精度
	QuoteAddress  string       // 报价代币地址
	QuoteDecimals uint32       // 报价代币精度
	Bids          [][2]float64 // 买单 [[价格, 数量], ...]
	Asks          [][2]float64 // 卖单 [[价格, 数量], ...]
}

// PricingData 完整定价数据
type PricingData struct {
	ChainID      uint32      // 链 ID
	MakerAddress string      // Market Maker 地址
	Levels       []PairLevel // 所有交易对层级
}

// =============================================================================
// 公共函数
// =============================================================================

// CreateSamplePricing 创建示例定价数据（默认自执行模式）
func CreateSamplePricing() PricingData {
	return CreateSamplePricingWithMode(SelfExecuteMode)
}

// CreateSamplePricingWithMode 根据执行模式创建定价数据
func CreateSamplePricingWithMode(mode ExecutionMode) PricingData {
	// 计算价差
	wbnbSpread := calculateSpread(mode, wbnbUsdtMid)
	cashSpread := calculateSpread(mode, cashPlusUsdtMid)

	return PricingData{
		ChainID:      defaultChainID,
		MakerAddress: makerAddr,
		Levels: []PairLevel{
			buildPairLevel(wbnbAddress, wbnbDecimals, usdtAddress, usdtDecimals, wbnbUsdtMid, wbnbSpread),
			buildPairLevel(wbnbAddress, wbnbDecimals, usdcAddress, usdcDecimals, wbnbUsdtMid, wbnbSpread),
			buildPairLevel(cashPlusAddress, cashPlusDecimals, usdcAddress, usdcDecimals, cashPlusUsdtMid, cashSpread),
			buildPairLevel(cashPlusAddress, cashPlusDecimals, usdtAddress, usdtDecimals, cashPlusUsdtMid, cashSpread),
		},
	}
}

// BuildPricingMessage 将定价数据序列化为 Protobuf 消息
func BuildPricingMessage(data PricingData) ([]byte, error) {
	levels := make([]*pb.LevelInfo, 0, len(data.Levels))
	for _, level := range data.Levels {
		levels = append(levels, &pb.LevelInfo{
			BaseAddress:   utils.HexToBytes(level.BaseAddress),
			BaseDecimals:  level.BaseDecimals,
			QuoteAddress:  utils.HexToBytes(level.QuoteAddress),
			QuoteDecimals: level.QuoteDecimals,
			Bids:          flattenLevels(level.Bids),
			Asks:          flattenLevels(level.Asks),
		})
	}

	msg := &pb.LevelsSchema{
		ChainId:  data.ChainID,
		MsgTopic: "pricing",
		MsgType:  "update",
		Msg:      &pb.LevelMsg{MakerAddress: utils.HexToBytes(data.MakerAddress), Levels: levels},
	}

	return proto.Marshal(msg)
}

// =============================================================================
// 内部辅助函数
// =============================================================================

// calculateSpread 根据执行模式和中间价计算价差
func calculateSpread(mode ExecutionMode, midPrice float64) float64 {
	if mode == SelfExecuteMode {
		return selfExecuteSpread
	}
	// Gasless 模式：基础价差 + gas 影响
	if midPrice > 0 {
		return gaslessSpread + gasEstimateUSD/midPrice
	}
	return gaslessSpread
}

// buildPairLevel 构建交易对价格层级
// 默认生成 3 层深度：0.2, 0.3, 0.4 个基础代币
func buildPairLevel(baseAddr string, baseDec uint32, quoteAddr string, quoteDec uint32, midPrice, spread float64) PairLevel {
	return PairLevel{
		BaseAddress:   baseAddr,
		BaseDecimals:  baseDec,
		QuoteAddress:  quoteAddr,
		QuoteDecimals: quoteDec,
		Bids:          generateLevels(midPrice, spread, -1), // 买单：价格递减
		Asks:          generateLevels(midPrice, spread, +1), // 卖单：价格递增
	}
}

// generateLevels 生成价格层级
// direction: -1 表示买单（价格递减），+1 表示卖单（价格递增）
func generateLevels(midPrice, spread float64, direction int) [][2]float64 {
	amounts := [3]float64{0.2, 0.3, 0.4} // 各层数量
	levels := make([][2]float64, len(amounts))

	for i, amount := range amounts {
		multiplier := 1 + float64(direction)*spread*float64(i+1)
		levels[i] = [2]float64{midPrice * multiplier, amount}
	}
	return levels
}

// flattenLevels 将层级数组扁平化为 [price, amount, price, amount, ...]
func flattenLevels(levels [][2]float64) []float64 {
	result := make([]float64, 0, len(levels)*2)
	for _, l := range levels {
		result = append(result, l[0], l[1])
	}
	return result
}
