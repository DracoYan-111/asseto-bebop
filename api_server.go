package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

// =============================================================================
// HTTP API 服务器 - 价格管理接口（带 API Key 认证）
// =============================================================================

// APIServer HTTP API 服务器
type APIServer struct {
	priceStore *PriceStore
	apiKey     string
	addr       string
}

// =============================================================================
// 服务器启动
// =============================================================================

// StartAPIServer 启动 HTTP API 服务器
func StartAPIServer(addr string, priceStore *PriceStore, apiKey string) {
	server := &APIServer{
		priceStore: priceStore,
		apiKey:     apiKey,
		addr:       addr,
	}

	mux := http.NewServeMux()

	// 静态文件服务（web 目录）
	fs := http.FileServer(http.Dir("./web"))
	mux.Handle("/web/", http.StripPrefix("/web/", fs))

	// 健康检查（无需认证）
	mux.HandleFunc("/health", server.handleHealth)

	// 价格 API（需要认证）
	mux.HandleFunc("/api/price", server.withAuth(server.handlePrice))
	mux.HandleFunc("/api/prices", server.withAuth(server.handlePrices))
	mux.HandleFunc("/api/price/history", server.withAuth(server.handleHistory))

	log.Printf("🌐 HTTP API 服务器启动: %s", addr)
	log.Printf("📄 Web Dashboard: http://localhost%s/web/", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Printf("✗ HTTP 服务器错误: %v", err)
	}
}

// =============================================================================
// 中间件
// =============================================================================

// withCORS 添加 CORS 头
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuth API Key 认证中间件
func (s *APIServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 如果未配置 API Key，跳过认证
		if s.apiKey == "" {
			next(w, r)
			return
		}

		// 验证 API Key
		key := r.Header.Get("X-API-Key")
		if key != s.apiKey {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// =============================================================================
// 处理器
// =============================================================================

// handleHealth 健康检查
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"prices": len(s.priceStore.GetAllPrices()),
	})
}

// handlePrice POST 更新价格（批量上传）
// 请求格式: []PairPrice 数组
func (s *APIServer) handlePrice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Levels   []PairPrice `json:"levels"`
		Operator string      `json:"operator,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// 验证 levels 数组
	if len(req.Levels) == 0 {
		http.Error(w, `{"error": "levels array is required"}`, http.StatusBadRequest)
		return
	}

	// 验证每个 level
	for idx, level := range req.Levels {
		if level.BaseToken == "" || level.QuoteToken == "" {
			http.Error(w, fmt.Sprintf(`{"error": "level %d: base_token and quote_token are required"}`, idx), http.StatusBadRequest)
			return
		}
		if level.BaseDecimals == 0 || level.QuoteDecimals == 0 {
			http.Error(w, fmt.Sprintf(`{"error": "level %d: base_decimals and quote_decimals are required"}`, idx), http.StatusBadRequest)
			return
		}
		if len(level.Bids) == 0 || len(level.Asks) == 0 {
			http.Error(w, fmt.Sprintf(`{"error": "level %d: bids and asks arrays are required"}`, idx), http.StatusBadRequest)
			return
		}

		// 验证每层的格式：[价格, 数量]，价格和数量都必须 > 0
		for i, bid := range level.Bids {
			if bid[0] <= 0 || bid[1] <= 0 {
				http.Error(w, fmt.Sprintf(`{"error": "level %d: invalid bid at index %d: price and amount must be positive"}`, idx, i), http.StatusBadRequest)
				return
			}
		}
		for i, ask := range level.Asks {
			if ask[0] <= 0 || ask[1] <= 0 {
				http.Error(w, fmt.Sprintf(`{"error": "level %d: invalid ask at index %d: price and amount must be positive"}`, idx, i), http.StatusBadRequest)
				return
			}
		}
	}

	// 批量设置价格
	if err := s.priceStore.SetPrices(req.Levels, req.Operator); err != nil {
		log.Printf("✗ 保存价格失败: %v", err)
		http.Error(w, `{"error": "Failed to save prices"}`, http.StatusInternalServerError)
		return
	}

	log.Printf("✓ 批量价格更新: %d 个交易对 (by %s)", len(req.Levels), req.Operator)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(req.Levels),
		"levels":  req.Levels,
	})
}

// handlePrices GET 获取所有价格
func (s *APIServer) handlePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	prices := s.priceStore.GetAllPrices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(prices),
		"prices": prices,
	})
}

// handleHistory GET 获取历史记录
func (s *APIServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 解析 limit 参数
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history := s.priceStore.GetHistory(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(history),
		"history": history,
	})
}

// =============================================================================
// 辅助函数
// =============================================================================

// getAPIKeyFromEnv 从环境变量获取 API Key
func getAPIKeyFromEnv() string {
	return os.Getenv("PRICE_API_KEY")
}
