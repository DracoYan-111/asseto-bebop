package main

import (
	pb "asseto-bebop/proto"
	"asseto-bebop/utils"
	"log"

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

// 默认配置
const (
	defaultChainID = 56 // 默认链 ID (BSC)
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

// CreateSamplePricing 创建定价数据（默认自执行模式）
func CreateSamplePricing() PricingData {
	return CreateSamplePricingWithMode(SelfExecuteMode)
}

// CreateSamplePricingWithMode 根据执行模式创建定价数据
// 从 PriceStore 获取动态价格
func CreateSamplePricingWithMode(mode ExecutionMode) PricingData {
	levels := []PairLevel{}

	// 获取所有已配置的价格
	prices := globalPriceStore.GetAllPrices()

	if len(prices) == 0 {
		log.Println("⚠️ 未配置任何价格，将生成空的 Pricing 数据")
	}

	for _, price := range prices {
		level := PairLevel{
			BaseAddress:   price.BaseToken,
			BaseDecimals:  price.BaseDecimals, // 使用运营上传的精度
			QuoteAddress:  price.QuoteToken,
			QuoteDecimals: price.QuoteDecimals, // 使用运营上传的精度
			Bids:          price.Bids,          // 直接使用运营上传的 levels
			Asks:          price.Asks,
		}
		levels = append(levels, level)
	}

	return PricingData{
		ChainID:      defaultChainID,
		MakerAddress: makerAddr,
		Levels:       levels,
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

// flattenLevels 将层级数组扁平化为 [price, amount, price, amount, ...]
func flattenLevels(levels [][2]float64) []float64 {
	result := make([]float64, 0, len(levels)*2)
	for _, l := range levels {
		result = append(result, l[0], l[1])
	}
	return result
}
