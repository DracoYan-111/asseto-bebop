package main

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// =============================================================================
// 代币配置
// =============================================================================

const (
	// 代币价格（USD）
	wbnbUsdtMid     = 879.00 // WBNB 价格
	cashPlusUsdtMid = 106.71 // CASH+ 价格

	// Maker 地址
	makerAddr = "0x95B89a3bB25FCBeD8E30052a8BDf77f106c1B554"
)

// 代币地址和精度
const (
	wbnbAddress      = "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
	wbnbDecimals     = uint32(18)
	usdcAddress      = "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d"
	usdcDecimals     = uint32(18)
	usdtAddress      = "0x55d398326f99059fF775485246999027B3197955"
	usdtDecimals     = uint32(18)
	cashPlusAddress  = "0x1775504c5873e179Ea2f8ABFcE3861EC74D159bc"
	cashPlusDecimals = uint32(18)
)

// =============================================================================
// WebSocket 配置
// =============================================================================

const (
	handshakeTimeout      = 10 * time.Second
	pingInterval          = 10 * time.Second
	readDeadline          = 90 * time.Second
	writeDeadline         = 10 * time.Second
	pricingStreamInterval = 3 * time.Second
	retryDelay            = 5 * time.Second

	pricingWSURL = "wss://api.bebop.xyz/pmm/%s/v3/maker/pricing?format=protobuf"
	quotesWSURL  = "wss://api.bebop.xyz/pmm/%s/v3/maker/quote"
	tradesWSURL  = "wss://api.bebop.xyz/pmm/%s/v3/maker/trades"
)

// =============================================================================
// 消息常量
// =============================================================================

const (
	topicQuoteRequest = "quote_request"
	topicTakerQuote   = "taker_quote"
	topicQuote        = "quote"
)

// =============================================================================
// 配置结构体
// =============================================================================

// Config 应用配置
type Config struct {
	ChainID       uint32
	PrivateKey    string
	MarketMaker   string
	Authorization string
	SelfExecution bool
	PriceSpread   float64
}

// MessageHandler WebSocket 消息处理器
type MessageHandler func(msgType int, data []byte) error

// =============================================================================
// 报价数据结构
// =============================================================================

// QuoteRequest 报价请求/响应
type QuoteRequest struct {
	ChainID  uint32        `json:"chain_id"`
	Msg      *QuoteMessage `json:"msg"`
	MsgTopic string        `json:"msg_topic"`
	MsgType  string        `json:"msg_type"`
}

// QuoteMessage 报价消息
type QuoteMessage struct {
	Commands           string             `json:"commands"`
	EventID            string             `json:"event_id"`
	Expiry             int64              `json:"expiry"`
	ExpiryType         string             `json:"expiry_type,omitempty"`
	FeeNative          float64            `json:"fee_native"`
	IsAggregateOrder   bool               `json:"is_aggregate_order"`
	MakerAddress       *string            `json:"maker_address"`
	MakerNonce         string             `json:"maker_nonce"`
	MakerTokens        []string           `json:"maker_tokens"`
	MakerTokensIndices []int32            `json:"maker_tokens_indices"`
	OnchainPartnerID   uint32             `json:"onchain_partner_id"`
	OrderSigningType   string             `json:"order_signing_type"`
	OrderType          string             `json:"order_type"`
	OriginAddress      string             `json:"origin_address"`
	PackedCommands     string             `json:"packed_commands"`
	QuoteID            string             `json:"quote_id"`
	Quotes             []*Quote           `json:"quotes"`
	Receiver           string             `json:"receiver"`
	Source             string             `json:"source,omitempty"`
	TakerAddress       string             `json:"taker_address"`
	TakerTokens        []string           `json:"taker_tokens"`
	TakerTokensIndices []int32            `json:"taker_tokens_indices"`
	Signature          *signatureResponse `json:"signature"`
}

// Quote 单个报价
type Quote struct {
	MakerAmount    *string  `json:"maker_amount,omitempty"`
	MakerToken     string   `json:"maker_token"`
	ReferencePrice *float64 `json:"reference_price,omitempty"`
	TakerAmount    *string  `json:"taker_amount,omitempty"`
	TakerToken     string   `json:"taker_token"`
}

// signatureResponse 签名响应
type signatureResponse struct {
	SignScheme string `json:"sign_scheme"`
	Signature  string `json:"signature"`
}

// =============================================================================
// 订单数据结构 (EIP712)
// =============================================================================

// SingleOrder 单个订单（用于 EIP712 签名）
type SingleOrder struct {
	PartnerID      uint64
	Expiry         *big.Int
	TakerAddress   common.Address
	MakerAddress   common.Address
	MakerNonce     *big.Int
	TakerToken     common.Address
	MakerToken     common.Address
	TakerAmount    *big.Int
	MakerAmount    *big.Int
	Receiver       common.Address
	PackedCommands *big.Int
	ChainID        *big.Int
}

// MultiOrder 多订单（用于 EIP712 签名）
type MultiOrder struct {
	PartnerID    uint64
	Expiry       *big.Int
	TakerAddress common.Address
	MakerAddress common.Address
	MakerNonce   *big.Int
	TakerTokens  []common.Address
	MakerTokens  []common.Address
	TakerAmounts []*big.Int
	MakerAmounts []*big.Int
	Receiver     common.Address
	Commands     []byte
	ChainID      *big.Int
}
