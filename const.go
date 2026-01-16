package main

import "time"

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
