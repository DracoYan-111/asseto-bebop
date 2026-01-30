package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// =============================================================================
// 程序入口
// =============================================================================

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ 未找到 .env 文件，使用系统环境变量")
	}

	cfg := loadConfigFromEnv()

	// 验证必要配置
	if cfg.PrivateKey == "" {
		log.Fatal("✗ 错误: PRIVATE_KEY 未配置")
	}
	if cfg.MarketMaker == "" {
		log.Fatal("✗ 错误: MARKET_MAKER 未配置")
	}
	if cfg.Authorization == "" {
		log.Fatal("✗ 错误: AUTHORIZATION 未配置")
	}

	log.Println("========================================")
	log.Printf("🚀 Bebop Market Maker 启动 [%s]", cfg.MarketMaker)
	log.Println("========================================")

	// 初始化价格存储
	globalPriceStore = NewPriceStore("./data")
	log.Printf("✓ 价格存储初始化完成 (已加载 %d 个交易对)", len(globalPriceStore.GetAllPrices()))

	// 启动 HTTP API 服务器
	go StartAPIServer(cfg.APIPort, globalPriceStore, cfg.APIKey)

	// 检查是否有价格配置
	if len(globalPriceStore.GetAllPrices()) == 0 {
		log.Println("⚠️ 警告: 未配置任何价格，请通过 API 上传价格后再连接 Bebop")
		log.Printf("   POST http://localhost%s/api/price", cfg.APIPort)
	}

	for retryCount := 0; ; retryCount++ {
		if runMarketMaker(cfg) {
			break
		}
		log.Printf("🔄 %v 后重连 (#%d)...", retryDelay, retryCount+1)
		time.Sleep(retryDelay)
	}
}

// loadConfigFromEnv 从环境变量加载配置
func loadConfigFromEnv() Config {
	chainID := 56
	if v := os.Getenv("CHAIN_ID"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			chainID = id
		}
	}

	selfExec := true
	if v := os.Getenv("SELF_EXECUTION"); v != "" {
		selfExec = strings.ToLower(v) == "true"
	}

	priceSpread := 0.005
	if v := os.Getenv("PRICE_SPREAD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			priceSpread = f
		}
	}

	apiPort := ":8080"
	if v := os.Getenv("API_PORT"); v != "" {
		apiPort = v
	}

	return Config{
		ChainID:       uint32(chainID),
		PrivateKey:    os.Getenv("PRIVATE_KEY"),
		MarketMaker:   os.Getenv("MARKET_MAKER"),
		Authorization: os.Getenv("AUTHORIZATION"),
		SelfExecution: selfExec,
		PriceSpread:   priceSpread,
		APIPort:       apiPort,
		APIKey:        os.Getenv("PRICE_API_KEY"),
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
