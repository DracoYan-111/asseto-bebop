package main

import (
	pb "asseto-bebop/proto"
	"asseto-bebop/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// =============================================================================
// WebSocket 流处理器
// =============================================================================

// StreamPricing 连接 Pricing WebSocket，定期发送定价更新
func StreamPricing(ctx context.Context, cfg Config, errChan chan error) {
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	conn, err := DialWS(ctx, fmt.Sprintf(pricingWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️ pricing_client 连接失败: %v", err)
		return
	}
	defer conn.Close()
	log.Println("✓ pricing_client 连接成功")

	// 启动定价发送协程
	go runPricingSender(ctx, conn, errChan)

	// 监听服务器响应
	wsLoop(ctx, conn, "pricing_client", parsePricingResponse, errChan)
}

// runPricingSender 定期发送定价更新
func runPricingSender(ctx context.Context, conn *websocket.Conn, errChan chan error) {
	var mu sync.Mutex
	ticker := time.NewTicker(pricingStreamInterval)
	defer ticker.Stop()
	log.Printf("\t✓ 开始发送定价流 (间隔: %v)", pricingStreamInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 检查是否有配置价格
			prices := globalPriceStore.GetAllPrices()
			if len(prices) == 0 {
				log.Println("⏸️  跳过定价发送: 未配置任何价格，请先通过 API 上传价格")
				continue
			}

			pricing := CreateSamplePricing()
			data, err := BuildPricingMessage(pricing)
			if err != nil {
				log.Printf("✗ 构建定价消息失败: %v", err)
				continue
			}

			mu.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			err = conn.WriteMessage(websocket.BinaryMessage, data)
			mu.Unlock()

			if err != nil {
				log.Printf("✗ 发送定价失败: %v", err)
				errChan <- err
				return
			}
			log.Printf("→ 已发送定价更新 [%d bytes, %d 交易对]", len(data), len(pricing.Levels))
		}
	}
}

// parsePricingResponse 解析 Pricing 服务器响应
func parsePricingResponse(msgType int, data []byte) error {
	if msgType == websocket.BinaryMessage {
		resp := &pb.WebSocketResponse{}
		if proto.Unmarshal(data, resp) == nil && resp.Msg != nil {
			log.Printf("✓ 收到服务器响应: %s (代码 %d)", resp.Msg.Text, resp.Msg.Code)
		}
	}
	return nil
}

// =============================================================================
// Trades 流处理器
// =============================================================================

// StreamTrades 连接 Trades WebSocket，接收交易通知
func StreamTrades(ctx context.Context, cfg Config, errChan chan error) {
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	conn, err := DialWS(ctx, fmt.Sprintf(tradesWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️ trades_client 连接失败: %v", err)
		return
	}
	defer conn.Close()
	log.Println("✓ trades_client 连接成功")

	wsLoop(ctx, conn, "trades_client", parseTradeMessage, errChan)
}

// parseTradeMessage 解析交易消息
func parseTradeMessage(msgType int, data []byte) error {
	// JSON 消息
	if len(data) > 0 && (data[0] == '{' || msgType == websocket.TextMessage) {
		return utils.ParseTradeJSON(data)
	}

	// Protobuf 消息
	if msgType == websocket.BinaryMessage {
		// 交易通知
		if notif := (&pb.TradeUpdate{}); proto.Unmarshal(data, notif) == nil && notif.MsgTopic == "trade" {
			t := notif.Msg
			log.Printf("📊 收到交易通知: OrderID=%s, TxHash=%s, Status=%s", t.QuoteId, t.TxHash, t.Status)
			if t.Status == "completed" {
				log.Printf("✅ 交易成功完成")
			}
			return nil
		}
		// 服务器响应
		if resp := (&pb.WebSocketResponse{}); proto.Unmarshal(data, resp) == nil && resp.Msg != nil {
			log.Printf("✓ trades_client: %s (代码 %d)", resp.Msg.Text, resp.Msg.Code)
		}
	}
	return nil
}

// =============================================================================
// Quotes 流处理器 (RFQ)
// =============================================================================

// StreamQuotes 连接 Quotes WebSocket，处理 RFQ 请求
func StreamQuotes(ctx context.Context, cfg Config, errChan chan error) {
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	conn, err := DialWS(ctx, fmt.Sprintf(quotesWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️ quotes_client 连接失败: %v", err)
		return
	}
	defer func() {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		conn.Close()
	}()
	log.Println("✓ quotes_client 连接成功")

	// 初始化 RFQ 处理器
	signer, err := NewOrderSigner(cfg.PrivateKey, int64(cfg.ChainID))
	if err != nil {
		log.Printf("⚠️ 签名器初始化失败: %v", err)
		return
	}
	log.Printf("   ✓ 签名器地址: %s", signer.GetAddress())

	handler := &RFQHandler{signer: signer, conn: conn}
	log.Println("   ✓ RFQ 处理器已就绪")

	wsLoop(ctx, conn, "quotes_client", createQuoteMessageParser(handler), errChan)
}

// createQuoteMessageParser 创建报价消息解析器
func createQuoteMessageParser(handler *RFQHandler) func(int, []byte) error {
	return func(msgType int, data []byte) error {
		var msg QuoteRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("⚠️ JSON 解析失败: %v", err)
			return nil
		}

		switch msg.MsgTopic {
		case topicQuoteRequest, topicTakerQuote, topicQuote:
			handler.handleJSONQuoteRequest(msg)
		}
		return nil
	}
}
