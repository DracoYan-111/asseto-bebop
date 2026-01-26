package main

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	// 注意：这些价格应与 rfq_handler.go 中的 getTokenPrice 保持一致
	wbnbUsdtMid     = 879.00 // WBNB 价格（与 Bebop 测试价格一致）
	cashPlusUsdtMid = 106.71 // CASH+ 当前市场价格

	makerAddr = "0x95B89a3bB25FCBeD8E30052a8BDf77f106c1B554"

	// --- Addresses/decimals ---
	// WBNB (Wrapped BNB)
	wbnbAddress  = "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
	wbnbDecimals = uint32(18)
	// USDC (USD Coin)
	usdcAddress  = "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d"
	usdcDecimals = uint32(18)
	// USDT (Binance-Peg BSC-USD)
	usdtAddress  = "0x55d398326f99059fF775485246999027B3197955"
	usdtDecimals = uint32(18)
	// CASH+
	cashPlusAddress  = "0x1775504c5873e179Ea2f8ABFcE3861EC74D159bc"
	cashPlusDecimals = uint32(18)
)

// 常量定义
const (
	// WebSocket 配置
	handshakeTimeout      = 10 * time.Second // 握手超时
	pingInterval          = 10 * time.Second // 心跳间隔
	readDeadline          = 90 * time.Second // 读取超时时间
	writeDeadline         = 10 * time.Second // 写入超时时间
	pricingStreamInterval = 3 * time.Second  // 价格流间隔
	retryDelay            = 5 * time.Second  // 重试间隔

	// WebSocket URL
	pricingWSURL = "wss://api.bebop.xyz/pmm/%s/v3/maker/pricing?format=protobuf" // 价格流 URL
	quotesWSURL  = "wss://api.bebop.xyz/pmm/%s/v3/maker/quote"                   // 报价流 URL
	tradesWSURL  = "wss://api.bebop.xyz/pmm/%s/v3/maker/trades"                  // 交易流 URL
)

// Config 配置参数
type Config struct {
	ChainID       uint32
	PrivateKey    string
	MarketMaker   string
	Authorization string
	SelfExecution bool
	PriceSpread   float64
}

// MessageHandler 消息处理器接口
type MessageHandler func(msgType int, data []byte) error

const (
	// 报价相关常量
	defaultQuoteExpiry = 30 * time.Second
	defaultDecimals    = 18
	hexPrefix          = "0x"

	// 消息类型
	msgTypeRequest  = "request"
	msgTypeResponse = "response"
	msgTypeError    = "error"

	// 消息主题
	topicQuoteRequest = "quote_request"
	topicTakerQuote   = "taker_quote"
	topicQuote        = "quote"
	topicTrade        = "trade"
	topicWebSocket    = "websocket"
)

// QuoteRequest 报价请求结构体（对应 JSON 格式）
type QuoteRequest struct {
	ChainID  uint32        `json:"chain_id"`
	Msg      *QuoteMessage `json:"msg"`
	MsgTopic string        `json:"msg_topic"`
	MsgType  string        `json:"msg_type"`
}

// QuoteMessage 报价消息结构体
type QuoteMessage struct {
	Commands           string             `json:"commands"`              // 命令（hex 字符串，如 "0x"）
	EventID            string             `json:"event_id"`              // 事件 ID
	Expiry             int64              `json:"expiry"`                // 过期时间（UNIX 时间戳）
	ExpiryType         string             `json:"expiry_type,omitempty"` // 过期类型（如 "standard"）
	FeeNative          float64            `json:"fee_native"`            // 原生代币手续费
	IsAggregateOrder   bool               `json:"is_aggregate_order"`    // 是否为聚合订单
	MakerAddress       *string            `json:"maker_address"`         // Maker 地址（可为 null）
	MakerNonce         string             `json:"maker_nonce"`           // Maker nonce
	MakerTokens        []string           `json:"maker_tokens"`          // Maker 代币地址列表
	MakerTokensIndices []int32            `json:"maker_tokens_indices"`  // Maker 代币索引列表
	OnchainPartnerID   uint32             `json:"onchain_partner_id"`    // 链上合作伙伴 ID
	OrderSigningType   string             `json:"order_signing_type"`    // 订单签名类型（如 "SingleOrder"）
	OrderType          string             `json:"order_type"`            // 订单类型（如 "121"）
	OriginAddress      string             `json:"origin_address"`        // 原始地址
	PackedCommands     string             `json:"packed_commands"`       // 打包的命令（字符串格式）
	QuoteID            string             `json:"quote_id"`              // 报价 ID
	Quotes             []*Quote           `json:"quotes"`                // 报价列表
	Receiver           string             `json:"receiver"`              // 接收者地址
	Source             string             `json:"source,omitempty"`      // 来源（如 "status"）
	TakerAddress       string             `json:"taker_address"`         // Taker 地址
	TakerTokens        []string           `json:"taker_tokens"`          // Taker 代币地址列表
	TakerTokensIndices []int32            `json:"taker_tokens_indices"`  // Taker 代币索引列表
	Signature          *signatureResponse `json:"signature"`
}

type signatureResponse struct {
	SignScheme string `json:"sign_scheme"`
	Signature  string `json:"signature"`
}

// Quote 单个报价结构体
type Quote struct {
	MakerAmount    *string  `json:"maker_amount,omitempty"`    // Maker 数量（可为 null，需要报价方填写）
	MakerToken     string   `json:"maker_token"`               // Maker 代币地址
	ReferencePrice *float64 `json:"reference_price,omitempty"` // 参考价格（可为 null）
	TakerAmount    *string  `json:"taker_amount,omitempty"`    // Taker 数量（最小单位，如 wei）
	TakerToken     string   `json:"taker_token"`               // Taker 代币地址
}

// 交易请求结构体
type TradeRequest struct {
	MsgTopic string        `json:"msg_topic"`
	MsgType  string        `json:"msg_type"`
	Msg      *TradeMessage `json:"msg"`
}

// TradeMessage 交易消息结构体
type TradeMessage struct {
	Commands           string   `json:"commands"`             // 命令（hex 字符串，如 "0x"）
	EventID            string   `json:"event_id"`             // 事件 ID
	Expiry             int64    `json:"expiry"`               // 过期时间（UNIX 时间戳）
	FeeNative          float64  `json:"fee_native"`           // 原生代币手续费
	IsAggregateOrder   bool     `json:"is_aggregate_order"`   // 是否为聚合订单
	MakerAddress       *string  `json:"maker_address"`        // Maker 地址（可为 null）
	MakerNonce         string   `json:"maker_nonce"`          // Maker nonce
	MakerTokens        []string `json:"maker_tokens"`         // Maker 代币地址列表（可为 null）
	MakerTokensIndices []int32  `json:"maker_tokens_indices"` // Maker 代币索引列表（可为 null）
	OnchainPartnerID   uint32   `json:"onchain_partner_id"`   // 链上合作伙伴 ID
	OrderSigningType   string   `json:"order_signing_type"`   // 订单签名类型
	OrderType          string   `json:"order_type"`           // 订单类型
	OriginAddress      string   `json:"origin_address"`       // 原始地址
	PackedCommands     string   `json:"packed_commands"`      // 打包的命令（字符串格式）
	QuoteID            string   `json:"quote_id"`             // 报价 ID
	Quotes             []*Quote `json:"quotes"`               // 报价列表（可为 null）
	Receiver           string   `json:"receiver"`             // 接收者地址
	TakerAddress       string   `json:"taker_address"`        // Taker 地址
	TakerTokens        []string `json:"taker_tokens"`         // Taker 代币地址列表（可为 null）
	TakerTokensIndices []int32  `json:"taker_tokens_indices"` // Taker 代币索引列表（可为 null）
}

type SingleOrder struct {
	PartnerID      uint64         // partner_id (uint64)
	Expiry         *big.Int       // expiry (uint256)
	TakerAddress   common.Address // taker_address (address)
	MakerAddress   common.Address // maker_address (address)
	MakerNonce     *big.Int       // maker_nonce (uint256)
	TakerToken     common.Address // taker_token (address)
	MakerToken     common.Address // maker_token (address)
	TakerAmount    *big.Int       // taker_amount (uint256)
	MakerAmount    *big.Int       // maker_amount (uint256)
	Receiver       common.Address // receiver (address)
	PackedCommands *big.Int       // packed_commands (uint256)
	ChainID        *big.Int       // 用于 domain
}

type MultiOrder struct {
	PartnerID    uint64           // partner_id (uint64)
	Expiry       *big.Int         // expiry (uint256)
	TakerAddress common.Address   // taker_address (address)
	MakerAddress common.Address   // maker_address (address)
	MakerNonce   *big.Int         // maker_nonce (uint256)
	TakerTokens  []common.Address // taker_tokens (address[])
	MakerTokens  []common.Address // maker_tokens (address[])
	TakerAmounts []*big.Int       // taker_amounts (uint256[])
	MakerAmounts []*big.Int       // maker_amounts (uint256[])
	Receiver     common.Address   // receiver (address)
	Commands     []byte           // commands (bytes)
	ChainID      *big.Int         // 用于 domain
}

// SignatureResponse 签名响应的 msg 字段（根据官方文档）
type SignatureResponse struct {
	QuoteID    string `json:"quote_id"`
	Signature  string `json:"signature"`
	SignScheme string `json:"sign_scheme"`
}

type signedRespond struct {
	ChainID  uint32            `json:"chain_id"`
	MsgTopic string            `json:"msg_topic"` // 必须是 "signature"
	MsgType  string            `json:"msg_type"`  // 必须是 "response"
	Msg      SignatureResponse `json:"msg"`
}

// // TakerQuoteResponse 结构体，用于响应 taker_quote (包含签名)
// type TakerQuoteResponse struct {
// 	ChainID  uint32             `json:"chain_id"`
// 	MsgTopic string             `json:"msg_topic"` // "taker_quote"
// 	MsgType  string             `json:"msg_type"`  // "response"
// 	Msg      *TakerQuoteMessage `json:"msg"`
// }

// type TakerQuoteMessage struct {
// 	QuoteID          string  `json:"quote_id"`
// 	EventID          string  `json:"event_id"`
// 	OrderSigningType string  `json:"order_signing_type"`
// 	OrderType        string  `json:"order_type"`
// 	OnchainPartnerID uint32  `json:"onchain_partner_id"`
// 	Expiry           int64   `json:"expiry"`
// 	TakerAddress     string  `json:"taker_address"`
// 	OriginAddress    string  `json:"origin_address"`
// 	MakerAddress     string  `json:"maker_address"`
// 	MakerNonce       string  `json:"maker_nonce"`
// 	Quotes           []Quote `json:"quotes"` // 复用 Quote 结构体
// 	Receiver         string  `json:"receiver"`
// 	Commands         string  `json:"commands"`
// 	PackedCommands   string  `json:"packed_commands"`
// 	FeeNative        float64 `json:"fee_native"`
// 	IsAggregateOrder bool    `json:"is_aggregate_order"`
// 	Signature        *struct {
// 		Signature  string `json:"signature"`
// 		SignScheme string `json:"sign_scheme"`
// 	} `json:"signature"`
// }
