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

// StreamPricing 连接 Pricing WebSocket，发送定价更新并监听服务器响应
func StreamPricing(ctx context.Context, cfg Config, errChan chan error) {
	// 获取链名称
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	// 连接到 Pricing WebSocket
	conn, err := DialWS(ctx, fmt.Sprintf(pricingWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️  pricing_client 连接失败: %v", err)
		return
	}
	defer conn.Close()
	log.Println("✓ pricing_client 连接成功") // 连接成功, 打印日志

	// 写入锁
	var writeMu sync.Mutex

	// 启动发送定价的 goroutine, 每隔一段时间发送一次定价
	go func() {
		// 创建一个定时器, 每隔一段时间发送一次定价
		ticker := time.NewTicker(pricingStreamInterval)
		// 延迟关闭定时器
		defer ticker.Stop()
		log.Printf("	✓ 开始发送定价流 (间隔: %v)", pricingStreamInterval)

		for {
			select {
			case <-ctx.Done():
				return // 上下文取消, 退出循环
			case <-ticker.C:
				// 创建一个定价数据
				pricing := CreateSamplePricing()

				// 构建定价消息
				data, err := BuildPricingMessage(pricing)
				if err != nil {
					log.Printf("✗ 构建定价消息失败: %v", err)
					continue
				}

				// 加锁, 防止并发写入
				writeMu.Lock()
				// 设置写入超时时间
				conn.SetWriteDeadline(time.Now().Add(writeDeadline))
				// 发送定价消息
				err = conn.WriteMessage(websocket.BinaryMessage, data)
				// 解锁
				writeMu.Unlock()

				if err != nil {
					log.Printf("✗ 发送定价失败: %v", err)
					errChan <- err
					return
				}
				log.Printf("→ 已发送定价更新 [%d bytes, %d 交易对]",
					len(data), len(pricing.Levels))
			}
		}
	}()

	// 监听服务器响应

	parseServerResponse := func(msgType int, data []byte) error {
		if msgType == websocket.BinaryMessage {
			resp := &pb.WebSocketResponse{}
			proto.Unmarshal(data, resp)
			log.Printf("✓ 收到服务器响应: %s (代码 %d)", resp.Msg.Text, resp.Msg.Code)
		}
		return nil
	}

	wsLoop(ctx, conn, "pricing_client", parseServerResponse, errChan)
}

// StreamTrades 连接 Trades WebSocket，接收交易通知
func StreamTrades(ctx context.Context, cfg Config, errChan chan error) {
	// 获取链名称
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	conn, err := DialWS(ctx, fmt.Sprintf(tradesWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️  trades_client 连接失败: %v", err)
		return
	}
	defer conn.Close()
	log.Println("✓ trades_client 连接成功")

	parseTradeMessage := func(msgType int, data []byte) error {
		log.Printf("📥 trades_client 收到消息 [大小=%d bytes]", len(data))

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
				return nil
			}
		}
		return nil
	}

	wsLoop(ctx, conn, "trades_client", parseTradeMessage, errChan)
}

// StreamQuotes 连接 Quotes WebSocket，处理 RFQ 请求
func StreamQuotes(ctx context.Context, cfg Config, errChan chan error) {
	// 获取链名称
	chainName, err := utils.GetChainName(cfg.ChainID)
	if err != nil {
		log.Printf("⚠️ 获取链名称失败: %v", err)
		return
	}

	conn, err := DialWS(ctx, fmt.Sprintf(quotesWSURL, chainName), cfg.MarketMaker, cfg.Authorization, cfg.SelfExecution)
	if err != nil {
		log.Printf("⚠️  quotes_client 连接失败: %v", err)
		return
	}
	defer func() {
		// 发送关闭帧，优雅关闭连接
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		conn.Close()
	}()
	log.Println("✓ quotes_client 连接成功")

	// 初始化 RFQ 处理器
	signer, err := NewOrderSigner(cfg.PrivateKey, int64(cfg.ChainID))
	if err != nil {
		log.Printf("⚠️  签名器初始化失败: %v", err)
		return
	}
	log.Printf("   ✓ 签名器地址: %s", signer.GetAddress())

	rfqHandler := &RFQHandler{
		signer: signer,
		conn:   conn,
	}

	log.Println("   ✓ RFQ 处理器已就绪")
	// 接收bebop服务端的消息转换为json并打印
	parseIncomingMessage := func(msgType int, data []byte) error {
		// 打印收到的消息基本信息
		log.Printf("📥 quotes_client 收到消息 [type=%d, size=%d bytes]", msgType, len(data))
		// 转换为JSON
		var msg QuoteRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("⚠️  JSON 解析失败: %v", err)
			return nil
		}

		// log.Printf("📋 收到 RFQ 报价请求: %s", msg.MsgTopic)

		switch msg.MsgTopic {
		case topicQuoteRequest, topicTakerQuote, topicQuote:
			quoteReq := &QuoteRequest{}
			if err := json.Unmarshal(data, quoteReq); err != nil {
				log.Printf("⚠️ JSON 解析错误: %v", err)
				log.Printf("📝 原始数据: %s", string(data))
				return nil
			}

			//打印json
			jsonStr, err := json.MarshalIndent(quoteReq, "", "  ")
			if err != nil {
				log.Printf("⚠️ JSON 格式化错误: %v", err)
				return nil
			}
			log.Printf("📋 收到 RFQ 报价请求: %s", string(jsonStr))
			rfqHandler.handleJSONQuoteRequest(*quoteReq)
			// case topicWebSocket:
			// 	// 将请求转为可读的json并打印
			// 	jsonStr, err := json.MarshalIndent(msg, "", "  ")
			// 	if err != nil {
			// 		log.Printf("⚠️ JSON 格式化错误: %v", err)
			// 		return nil
			// 	}
			// 	log.Printf("📋 收到 topicWebSocket请求: %s", string(jsonStr))

			// case topicTrade:
			// 	log.Printf("📋 收到 topicTrade请求: %+v", msg.Msg)

			// case topicWebSocket:
			// 	if msg.MsgType == msgTypeError {
			// 		var errMsg struct {
			// 			Code   int    `json:"code"`
			// 			Text   string `json:"text"`
			// 			Reason string `json:"reason,omitempty"`
			// 		}
			// 		if err := json.Unmarshal(data, &errMsg); err == nil {
			// 			log.Printf("⚠️  WebSocket 错误: %s (代码 %d)", errMsg.Text, errMsg.Code)
			// 		}
			// 	}
			// case topicTrade:
			// 	// 处理 JSON 格式的交易通知
			// 	var tradeNotif struct {
			// 		OrderID    string `json:"order_id"`
			// 		RequestID  string `json:"request_id"`
			// 		BuyToken   string `json:"buy_token"`
			// 		SellToken  string `json:"sell_token"`
			// 		BuyAmount  string `json:"buy_amount"`
			// 		SellAmount string `json:"sell_amount"`
			// 		TxHash     string `json:"tx_hash"`
			// 		Timestamp  int64  `json:"timestamp,omitempty"`
			// 		Status     string `json:"status"`
			// 	}
			// 	if err := json.Unmarshal(data, &tradeNotif); err == nil {
			// 		log.Printf("📊 收到交易通知 (JSON):")
			// 		log.Printf("   订单 ID: %s", tradeNotif.OrderID)
			// 		log.Printf("   请求 ID: %s", tradeNotif.RequestID)
			// 		log.Printf("   买入: %s (%s)", tradeNotif.BuyAmount, tradeNotif.BuyToken[:10])
			// 		log.Printf("   卖出: %s (%s)", tradeNotif.SellAmount, tradeNotif.SellToken[:10])
			// 		log.Printf("   交易哈希: %s", tradeNotif.TxHash)
			// 		log.Printf("   状态: %s", tradeNotif.Status)
			// 		if tradeNotif.Timestamp > 0 {
			// 			timestamp := time.Unix(tradeNotif.Timestamp, 0)
			// 			log.Printf("   时间: %s", timestamp.Format("2006-01-02 15:04:05"))
			// 		}
			// 		if tradeNotif.Status == "completed" {
			// 			log.Printf("✅ 交易成功完成")
			// 		}
			// 	} else {
			// 		log.Printf("⚠️  解析交易通知失败: %v", err)
			// 	}
			// default:
			// 	log.Printf("📩 收到 JSON 消息: topic=%s, type=%s", msg.MsgTopic, msg.MsgType)
		}

		return nil
	}

	wsLoop(ctx, conn, "quotes_client", parseIncomingMessage, errChan)
}

//
// // StreamQuotes 连接 Quotes WebSocket，处理 RFQ 请求
// func StreamQuotes(ctx context.Context, cfg Config, errChan chan error) {

// }
