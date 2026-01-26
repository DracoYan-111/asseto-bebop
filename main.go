package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// =============================================================================
// 程序入口
// =============================================================================

func main() {
	cfg := Config{
		ChainID:       56,
		PrivateKey:    "0xe1c494380f48f3792d098ccd0276ba98803545fa796a2fbd4ecbcd890822fda5",
		MarketMaker:   "asseto-rfqt-test",
		Authorization: "24b95b00-bcaa-4cc4-834a-9dfa7e8c0571",
		SelfExecution: true,
		PriceSpread:   0.005,
	}

	log.Println("========================================")
	log.Printf("🚀 Bebop Market Maker 启动 [%s]", cfg.MarketMaker)
	log.Println("========================================")

	for retryCount := 0; ; retryCount++ {
		if runMarketMaker(cfg) {
			break
		}
		log.Printf("🔄 %v 后重连 (#%d)...", retryDelay, retryCount+1)
		time.Sleep(retryDelay)
	}
}

// =============================================================================
// Market Maker 运行
// =============================================================================

// runMarketMaker 运行单次 Market Maker 连接
// 返回 true 表示正常退出，false 表示需要重连
func runMarketMaker(cfg Config) bool {
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 3)

	// 启动 WebSocket 流
	go StreamPricing(ctx, cfg, errChan)
	go StreamQuotes(ctx, cfg, errChan)
	go StreamTrades(ctx, cfg, errChan)

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.Println("按 Ctrl+C 停止...")

	select {
	case <-sigChan:
		log.Println("✓ 收到中断信号，正在关闭...")
		return true
	case err := <-errChan:
		log.Printf("✗ 连接异常: %v (持续 %v)", err, time.Since(startTime).Round(time.Second))
		return false
	}
}

// =============================================================================
// WebSocket 工具函数
// =============================================================================

// DialWS 连接 Bebop WebSocket 服务器
func DialWS(ctx context.Context, url, marketMaker, auth string, selfExec bool) (*websocket.Conn, error) {
	headers := http.Header{
		"marketmaker":   {marketMaker},
		"authorization": {auth},
	}

	if selfExec {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += sep + "execution_mode=self"
	}

	conn, resp, err := (&websocket.Dialer{
		HandshakeTimeout: handshakeTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}).DialContext(ctx, url, headers)

	if err != nil && resp != nil {
		log.Printf("连接失败: %s", resp.Status)
		if body, _ := io.ReadAll(resp.Body); len(body) > 0 {
			log.Printf("错误详情: %s", body)
		}
		resp.Body.Close()
	}
	return conn, err
}

// wsLoop WebSocket 监听循环（带 keepalive）
func wsLoop(ctx context.Context, conn *websocket.Conn, name string, handler MessageHandler, errChan chan error) {
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(readDeadline))
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("✗ %s 读取失败: %v", name, err)
			errChan <- err
			return
		}

		// Keepalive: 空消息回复空消息
		if len(msg) == 0 {
			mu.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			err = conn.WriteMessage(websocket.TextMessage, []byte(""))
			mu.Unlock()
			if err != nil {
				log.Printf("✗ %s keepalive 失败: %v", name, err)
				errChan <- err
				return
			}
			continue
		}

		if handler != nil {
			if err := handler(msgType, msg); err != nil {
				log.Printf("✗ %s 处理消息失败: %v", name, err)
			}
		}
	}
}
