package main

import (
	pb "asseto-bebop/proto"
	"asseto-bebop/utils"

	"google.golang.org/protobuf/proto"
)

// PairLevel 定义单个交易对的价格层级
//
// 包含：
// - 基础代币和报价代币的地址和精度
// - 买单层级（Bids）：价格从低到高，数量递增
// - 卖单层级（Asks）：价格从高到低，数量递增
type PairLevel struct {
	BaseAddress   string
	BaseDecimals  uint32
	QuoteAddress  string
	QuoteDecimals uint32
	Bids          [][2]float64
	Asks          [][2]float64
}

// PricingData 定义完整的定价数据
//
// 包含：
// - 链 ID（用于标识区块链网络）
// - Market Maker 地址（用于标识报价来源）
// - 所有交易对的价格层级
type PricingData struct {
	ChainID      uint32
	MakerAddress string
	Levels       []PairLevel
}

// ExecutionMode 执行模式
type ExecutionMode int

const (
	GaslessMode     ExecutionMode = iota // Bebop 执行（默认）
	SelfExecuteMode                      // 用户自己执行
)

func CreateSamplePricing() PricingData {
	return CreateSamplePricingWithMode(SelfExecuteMode)
}

func CreateSamplePricingWithMode(mode ExecutionMode) PricingData {
	// === 步骤 1: 获取市场价格 ===

	// === 步骤 2: 设置价差 ===
	spread := 0.005 // 0.5%

	if mode == SelfExecuteMode {
		// Self Execution 模式：用户支付 gas，价差可以更小
		spread = 0.003 // 0.3%
	} else {
		// Gasless 模式：需要在价差中包含执行/gas 风险
		gasUsd := 10.0
		// 下面会为每个交易对单独应用 gas 影响
		_ = gasUsd
	}

	// 每对价差调整（无气债券增加气体影响）
	spreadFor := func(mid float64) float64 {
		if mid <= 0 {
			return spread
		}
		if mode == GaslessMode {
			gasUsd := 10.0
			gasImpact := gasUsd / mid
			return spread + gasImpact
		}
		return spread
	}
	// Maker address
	makerAddr := "0x95B89a3bB25FCBeD8E30052a8BDf77f106c1B554"

	// --- Addresses/decimals ---
	// WBNB (Wrapped BNB)
	wbnbAddress := "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
	wbnbDecimals := uint32(18)
	// USDC (USD Coin)
	usdcAddress := "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d"
	usdcDecimals := uint32(18)
	// USDT (Binance-Peg BSC-USD)
	usdtAddress := "0x55d398326f99059fF775485246999027B3197955"
	usdtDecimals := uint32(18)
	// CASH+
	cashPlusAddress := "0x1775504c5873e179Ea2f8ABFcE3861EC74D159bc"
	cashPlusDecimals := uint32(18)

	chainIDStr := "56"
	chainID := uint32(56)
	if v, err := utils.ParseUint32(chainIDStr); err == nil {
		chainID = v
	}

	// --- Build levels ---
	// 注意：这些价格应与 rfq_handler.go 中的 getTokenPrice 保持一致
	wbnbUsdtMid := 940.12     // WBNB 当前市场价格
	cashPlusUsdtMid := 106.70 // CASH+ 当前市场价格

	wbnbSpread := spreadFor(wbnbUsdtMid)
	cashPlusSpread := spreadFor(cashPlusUsdtMid)

	return PricingData{
		ChainID:      chainID,
		MakerAddress: makerAddr,
		Levels: []PairLevel{
			{
				// WBNB/USDT
				BaseAddress:   wbnbAddress,
				BaseDecimals:  wbnbDecimals,
				QuoteAddress:  usdtAddress,
				QuoteDecimals: usdtDecimals,
				Bids: [][2]float64{
					{wbnbUsdtMid * (1 - wbnbSpread), 0.2},   //  0.2 WBNB
					{wbnbUsdtMid * (1 - wbnbSpread*2), 0.3}, //  0.3 WBNB
					{wbnbUsdtMid * (1 - wbnbSpread*3), 0.4}, //  0.4 WBNB (总: 0.9 WBNB)
				},
				Asks: [][2]float64{
					{wbnbUsdtMid * (1 + wbnbSpread), 0.2},   //  0.2 WBNB
					{wbnbUsdtMid * (1 + wbnbSpread*2), 0.3}, //  0.3 WBNB
					{wbnbUsdtMid * (1 + wbnbSpread*3), 0.4}, //  0.4 WBNB (总: 0.9 WBNB)
				},
			},
			{
				// WBNB/USDC
				BaseAddress:   wbnbAddress,
				BaseDecimals:  wbnbDecimals,
				QuoteAddress:  usdcAddress,
				QuoteDecimals: usdcDecimals,
				Bids: [][2]float64{
					{wbnbUsdtMid * (1 - wbnbSpread), 0.2},   //  0.2 WBNB
					{wbnbUsdtMid * (1 - wbnbSpread*2), 0.3}, //  0.3 WBNB
					{wbnbUsdtMid * (1 - wbnbSpread*3), 0.4}, //  0.4 WBNB (总: 0.9 WBNB)
				},
				Asks: [][2]float64{
					{wbnbUsdtMid * (1 + wbnbSpread), 0.2},   //  0.2 WBNB
					{wbnbUsdtMid * (1 + wbnbSpread*2), 0.3}, //  0.3 WBNB
					{wbnbUsdtMid * (1 + wbnbSpread*3), 0.4}, //  0.4 WBNB (总: 0.9 WBNB)
				},
			},
			{
				// CASH+/USDC
				BaseAddress:   cashPlusAddress,
				BaseDecimals:  cashPlusDecimals,
				QuoteAddress:  usdcAddress,
				QuoteDecimals: usdcDecimals,
				Bids: [][2]float64{
					// 买入 CASH+，支付 USDC
					{cashPlusUsdtMid * (1 - cashPlusSpread), 0.1},   // 0.1 CASH+ = ~$10.67
					{cashPlusUsdtMid * (1 - cashPlusSpread*2), 0.2}, // 0.2 CASH+ = ~$21.34
					{cashPlusUsdtMid * (1 - cashPlusSpread*3), 0.3}, // 0.3 CASH+ = ~$32.01
				},
				Asks: [][2]float64{
					// 买入 USDC，支付 CASH+
					{cashPlusUsdtMid * (1 + cashPlusSpread), 0.1},   // 0.1 CASH+ = ~$10.67
					{cashPlusUsdtMid * (1 + cashPlusSpread*2), 0.2}, // 0.2 CASH+ = ~$21.34
					{cashPlusUsdtMid * (1 + cashPlusSpread*3), 0.3}, // 0.3 CASH+ = ~$32.01
				},
			},
			{
				// cash+/USDT
				BaseAddress:   cashPlusAddress,
				BaseDecimals:  cashPlusDecimals,
				QuoteAddress:  usdtAddress,
				QuoteDecimals: usdtDecimals,
				Bids: [][2]float64{
					// 买入 CASH+，支付 USDC
					{cashPlusUsdtMid * (1 - cashPlusSpread), 0.1},   // 0.1 CASH+ = ~$10.67
					{cashPlusUsdtMid * (1 - cashPlusSpread*2), 0.2}, // 0.2 CASH+ = ~$21.34
					{cashPlusUsdtMid * (1 - cashPlusSpread*3), 0.3}, // 0.3 CASH+ = ~$32.01
				},
				Asks: [][2]float64{
					// 买入 USDC，支付 CASH+
					{cashPlusUsdtMid * (1 + cashPlusSpread), 0.1},   // 0.1 CASH+ = ~$10.67
					{cashPlusUsdtMid * (1 + cashPlusSpread*2), 0.2}, // 0.2 CASH+ = ~$21.34
					{cashPlusUsdtMid * (1 + cashPlusSpread*3), 0.3}, // 0.3 CASH+ = ~$32.01
				},
			},
		},
	}
}

func BuildPricingMessage(data PricingData) ([]byte, error) {
	msg := &pb.LevelsSchema{
		ChainId:  data.ChainID,
		MsgTopic: "pricing",
		MsgType:  "update",
		Msg: &pb.LevelMsg{
			MakerAddress: utils.HexToBytes(data.MakerAddress),
			Levels:       make([]*pb.LevelInfo, 0, len(data.Levels)),
		},
	}

	// 构建每个交易对的层级信息
	for _, level := range data.Levels {
		levelInfo := &pb.LevelInfo{
			BaseAddress:   utils.HexToBytes(level.BaseAddress),
			BaseDecimals:  level.BaseDecimals,
			QuoteAddress:  utils.HexToBytes(level.QuoteAddress),
			QuoteDecimals: level.QuoteDecimals,
			Bids:          flattenPriceLevels(level.Bids),
			Asks:          flattenPriceLevels(level.Asks),
		}
		msg.Msg.Levels = append(msg.Msg.Levels, levelInfo)
	}

	return proto.Marshal(msg)
}

// ParseServerResponse 解析服务器的 Protobuf 响应
func ParseServerResponse(data []byte) (*pb.WebSocketResponse, error) {
	resp := &pb.WebSocketResponse{}
	err := proto.Unmarshal(data, resp)
	return resp, err
}

// flattenPriceLevels 将价格层级数组扁平化为 [price, amount, price, amount, ...] 格式
func flattenPriceLevels(levels [][2]float64) []float64 {
	result := make([]float64, 0, len(levels)*2)
	for _, level := range levels {
		result = append(result, level[0], level[1]) // price, amount
	}
	return result
}
