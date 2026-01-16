package main

import (
	"asseto-bebop/utils"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
		log.Printf("	→ 开始发送定价流 (间隔: %v)", pricingStreamInterval)

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
			if resp, err := ParseServerResponse(data); err == nil {
				log.Printf("→ 收到服务器响应: %+v", resp)
			}
		}
		return nil
	}

	wsLoop(ctx, conn, "pricing_client", parseServerResponse, errChan)
}
