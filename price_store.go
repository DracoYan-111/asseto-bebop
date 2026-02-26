package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// 价格存储模块 - 支持动态 Bid/Ask 价格和文件持久化
// =============================================================================

// PairPrice 交易对价格层级（运营上传）
// Bids/Asks 格式: [[价格, 数量], [价格, 数量], ...]
type PairPrice struct {
	BaseToken     string       `json:"base_token"`     // 基础代币地址
	QuoteToken    string       `json:"quote_token"`    // 报价代币地址
	BaseDecimals  uint32       `json:"base_decimals"`  // 基础代币精度
	QuoteDecimals uint32       `json:"quote_decimals"` // 报价代币精度
	Bids          [][2]float64 `json:"bids"`           // 买单层级 [[价格, 数量], ...]
	Asks          [][2]float64 `json:"asks"`           // 卖单层级 [[价格, 数量], ...]
	UpdatedAt     int64        `json:"updated_at"`     // 更新时间戳（Unix 秒）
}

// PriceHistory 历史价格记录
type PriceHistory struct {
	PairPrice
	Operator  string `json:"operator,omitempty"` // 操作人（可选）
	Timestamp int64  `json:"timestamp"`          // 记录时间戳
}

// PriceStore 价格存储（线程安全）
type PriceStore struct {
	sync.RWMutex
	prices   map[string]PairPrice // key: "base_quote" 小写
	history  []PriceHistory       // 历史记录
	dataDir  string               // 数据目录
	dataFile string               // 当前价格文件路径
	histFile string               // 历史记录文件路径
}

// 全局价格存储实例
var globalPriceStore *PriceStore

// =============================================================================
// 初始化与加载
// =============================================================================

// NewPriceStore 创建价格存储实例并加载历史数据
func NewPriceStore(dataDir string) *PriceStore {
	store := &PriceStore{
		prices:   make(map[string]PairPrice),
		history:  make([]PriceHistory, 0),
		dataDir:  dataDir,
		dataFile: filepath.Join(dataDir, "prices.json"),
		histFile: filepath.Join(dataDir, "price_history.json"),
	}

	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("⚠️ 创建数据目录失败: %v", err)
	}

	// 加载现有数据
	store.loadPrices()
	store.loadHistory()

	return store
}

// loadPrices 从文件加载当前价格
func (s *PriceStore) loadPrices() {
	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("⚠️ 读取价格文件失败: %v", err)
		}
		return
	}

	var prices []PairPrice
	if err := json.Unmarshal(data, &prices); err != nil {
		log.Printf("⚠️ 解析价格文件失败: %v", err)
		return
	}

	for _, p := range prices {
		key := s.makeKey(p.BaseToken, p.QuoteToken)
		s.prices[key] = p
	}
	log.Printf("✓ 加载 %d 个交易对价格", len(prices))
}

// loadHistory 从文件加载历史记录
func (s *PriceStore) loadHistory() {
	data, err := os.ReadFile(s.histFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("⚠️ 读取历史文件失败: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, &s.history); err != nil {
		log.Printf("⚠️ 解析历史文件失败: %v", err)
		return
	}
	log.Printf("✓ 加载 %d 条历史记录", len(s.history))
}

// =============================================================================
// 价格操作
// =============================================================================

// SetPrice 设置交易对价格并持久化
func (s *PriceStore) SetPrice(price PairPrice, operator string) error {
	s.Lock()
	defer s.Unlock()

	// 设置更新时间
	price.UpdatedAt = time.Now().Unix()

	// 存储价格
	key := s.makeKey(price.BaseToken, price.QuoteToken)
	s.prices[key] = price

	// 添加历史记录
	s.history = append(s.history, PriceHistory{
		PairPrice: price,
		Operator:  operator,
		Timestamp: price.UpdatedAt,
	})

	// 持久化
	if err := s.savePrices(); err != nil {
		return err
	}
	return s.saveHistory()
}

// SetPrices 批量设置交易对价格并持久化
func (s *PriceStore) SetPrices(prices []PairPrice, operator string) error {
	s.Lock()
	defer s.Unlock()

	now := time.Now().Unix()

	for i := range prices {
		prices[i].UpdatedAt = now

		// 存储价格
		key := s.makeKey(prices[i].BaseToken, prices[i].QuoteToken)
		s.prices[key] = prices[i]

		// 添加历史记录
		s.history = append(s.history, PriceHistory{
			PairPrice: prices[i],
			Operator:  operator,
			Timestamp: now,
		})
	}

	// 持久化
	if err := s.savePrices(); err != nil {
		return err
	}
	return s.saveHistory()
}

// GetPrice 获取交易对价格
func (s *PriceStore) GetPrice(baseToken, quoteToken string) (PairPrice, bool) {
	s.RLock()
	defer s.RUnlock()

	key := s.makeKey(baseToken, quoteToken)
	price, ok := s.prices[key]
	return price, ok
}

// GetBidPrice 获取最佳买入价（你买入 base 代币的价格）
// 返回 Bids 数组第一层的价格
// 场景: 用户卖出 base 给你，你以 bid 价格买入
func (s *PriceStore) GetBidPrice(baseToken, quoteToken string) (float64, bool) {
	price, ok := s.GetPrice(baseToken, quoteToken)
	if !ok || len(price.Bids) == 0 {
		return 0, false
	}
	return price.Bids[0][0], true
}

// GetAskPrice 获取最佳卖出价（你卖出 base 代币的价格）
// 返回 Asks 数组第一层的价格
// 场景: 用户买入 base 从你这里，你以 ask 价格卖出
func (s *PriceStore) GetAskPrice(baseToken, quoteToken string) (float64, bool) {
	price, ok := s.GetPrice(baseToken, quoteToken)
	if !ok || len(price.Asks) == 0 {
		return 0, false
	}
	return price.Asks[0][0], true
}

// GetAllPrices 获取所有当前价格
func (s *PriceStore) GetAllPrices() []PairPrice {
	s.RLock()
	defer s.RUnlock()

	prices := make([]PairPrice, 0, len(s.prices))
	for _, p := range s.prices {
		prices = append(prices, p)
	}
	return prices
}

// GetHistory 获取历史记录（最近 limit 条）
func (s *PriceStore) GetHistory(limit int) []PriceHistory {
	s.RLock()
	defer s.RUnlock()

	if limit <= 0 || limit > len(s.history) {
		limit = len(s.history)
	}

	// 返回最近的记录（倒序）
	result := make([]PriceHistory, limit)
	for i := 0; i < limit; i++ {
		result[i] = s.history[len(s.history)-1-i]
	}
	return result
}

// ClearPrices 清空所有价格数据并持久化
func (s *PriceStore) ClearPrices() error {
	s.Lock()
	defer s.Unlock()

	s.prices = make(map[string]PairPrice)
	return s.savePrices()
}

// =============================================================================
// 持久化
// =============================================================================

// savePrices 保存当前价格到文件
func (s *PriceStore) savePrices() error {
	prices := make([]PairPrice, 0, len(s.prices))
	for _, p := range s.prices {
		prices = append(prices, p)
	}

	data, err := json.MarshalIndent(prices, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.dataFile, data, 0644)
}

// saveHistory 保存历史记录到文件
func (s *PriceStore) saveHistory() error {
	data, err := json.MarshalIndent(s.history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.histFile, data, 0644)
}

// =============================================================================
// 辅助函数
// =============================================================================

// makeKey 生成交易对唯一键
func (s *PriceStore) makeKey(baseToken, quoteToken string) string {
	return strings.ToLower(baseToken) + "_" + strings.ToLower(quoteToken)
}

// GetTokenDecimals 获取代币精度（保持与原有逻辑兼容）
func GetTokenDecimals(tokenAddress string) uint32 {
	addr := strings.ToLower(tokenAddress)
	switch addr {
	case strings.ToLower(wbnbAddress):
		return wbnbDecimals
	case strings.ToLower(usdcAddress):
		return usdcDecimals
	case strings.ToLower(usdtAddress):
		return usdtDecimals
	case strings.ToLower(cashPlusAddress):
		return cashPlusDecimals
	default:
		return 18 // 默认精度
	}
}
