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

// DialWS 连接到 Bebop WebSocket 服务器
func DialWS(ctx context.Context, url, marketMaker, auth string, selfExec bool) (*websocket.Conn, error) {
	headers := http.Header{
		"marketmaker":   {marketMaker},
		"authorization": {auth},
	}

	// 如果需要自执行, 则添加自执行参数
	if selfExec {
		if strings.Contains(url, "?") {
			url += "&execution_mode=self"
		} else {
			url += "?execution_mode=self"
		}
	}

	// 连接到 WebSocket 服务器
	conn, resp, err := (&websocket.Dialer{
		HandshakeTimeout: handshakeTimeout,          // 握手超时时间
		Proxy:            http.ProxyFromEnvironment, // 使用系统代理
	}).DialContext(ctx, url, headers) // 返回连接和响应

	if err != nil && resp != nil {
		log.Printf("连接失败: %s", resp.Status) // 连接失败, 打印错误信息
		if body, _ := io.ReadAll(resp.Body); len(body) > 0 {
			log.Printf("错误详情: %s", body) // 读取错误详情
		}
		resp.Body.Close() // 关闭响应体
	}
	return conn, err // 返回连接和错误信息
}

// wsLoop 通用 WebSocket 监听循环（带 keepalive）
func wsLoop(ctx context.Context, conn *websocket.Conn, name string, handler MessageHandler, errChan chan error) {
	// 写入锁
	var writeMu sync.Mutex
	for {
		select {
		case <-ctx.Done(): // 上下文取消
			return
		default: // 默认情况
		}

		conn.SetReadDeadline(time.Now().Add(readDeadline)) // 设置读取超时时间
		msgType, msg, err := conn.ReadMessage()            // 读取消息
		if err != nil {
			log.Printf("✗ %s 读取失败: %v", name, err)
			errChan <- err // 发送错误信息
			return
		}

		// keepalive: 空消息 => 回复空消息
		if len(msg) == 0 {
			writeMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeDeadline))       // 设置写入超时时间
			err = conn.WriteMessage(websocket.TextMessage, []byte("")) // 发送空消息
			writeMu.Unlock()
			if err != nil {
				log.Printf("✗ %s keepalive 失败: %v", name, err)
				errChan <- err // 发送错误信息
				return
			}
			continue // 继续
		}

		if handler != nil {
			if err := handler(msgType, msg); err != nil { // 处理消息
				log.Printf("✗ %s 处理消息失败: %v", name, err)
			}
		}
	}
}

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

// runMarketMaker 运行 Market Maker（单次连接）
func runMarketMaker(cfg Config) bool {

	startTime := time.Now()                                 // 开始时间
	ctx, cancel := context.WithCancel(context.Background()) // 创建上下文
	defer cancel()                                          // 延迟关闭上下文

	// 启动所有连接
	errChan := make(chan error, 3) // 错误通道

	go StreamPricing(ctx, cfg, errChan)
	go StreamQuotes(ctx, cfg, errChan)
	go StreamTrades(ctx, cfg, errChan)

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)                    // 信号通道
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM) // 监听中断信号
	log.Println("按 Ctrl+C 停止...")                         // 打印提示信息

	select {
	case <-sigChan:
		log.Println("✓ 收到中断信号，正在关闭...") // 打印提示信息
		return true                     // 返回 true
	case err := <-errChan:
		log.Printf("✗ 连接异常: %v (持续 %v)", err, time.Since(startTime).Round(time.Second)) // 打印提示信息
		return false                                                                    // 返回 false
	}
}
