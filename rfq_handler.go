package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
)

// =============================================================================
// RFQ Handler - 处理 Bebop RFQ (Request for Quote) 请求
// =============================================================================

// RFQHandler 处理 RFQ 报价请求
type RFQHandler struct {
	signer *OrderSigner    // EIP712 订单签名器
	conn   *websocket.Conn // WebSocket 连接
}

// handleJSONQuoteRequest 处理 JSON 格式的报价请求
// 根据 OrderSigningType 分发到 SingleOrder 或 MultiOrder 处理流程
func (h *RFQHandler) handleJSONQuoteRequest(msg QuoteRequest) error {
	switch msg.Msg.OrderSigningType {
	case "SingleOrder":
		return h.processSingleOrder(msg)
	case "MultiOrder":
		return h.processMultiOrder(msg)
	default:
		return nil
	}
}

// =============================================================================
// SingleOrder 处理
// =============================================================================

// processSingleOrder 处理单一订单并发送签名响应
func (h *RFQHandler) processSingleOrder(msg QuoteRequest) error {
	order, response := h.buildSingleOrder(msg)

	signature, err := h.signer.SignOrder(order)
	if err != nil {
		log.Printf("⚠️ SingleOrder 签名失败: %v", err)
		return err
	}

	response.Msg.Signature = &signatureResponse{SignScheme: "EIP712", Signature: signature}
	return h.sendResponse(response)
}

// buildSingleOrder 构建 SingleOrder 和报价响应
func (h *RFQHandler) buildSingleOrder(msg QuoteRequest) (SingleOrder, QuoteRequest) {
	response := h.initResponse(msg)
	quote := msg.Msg.Quotes[0]
	takerToken, makerToken := msg.Msg.TakerTokens[0], msg.Msg.MakerTokens[0]
	feeNative := msg.Msg.FeeNative

	// 计算 taker/maker 金额
	var takerAmount, makerAmount *big.Int

	if quote.TakerAmount != nil {
		// 已知 TakerAmount → 计算 MakerAmount
		takerAmount, _ = new(big.Int).SetString(*quote.TakerAmount, 10)
		if quote.MakerAmount != nil {
			makerAmount, _ = new(big.Int).SetString(*quote.MakerAmount, 10)
		} else {
			makerAmount = CalculateMakerAmount(takerToken, makerToken, takerAmount)
			// 有费用时从 maker_amount 扣除（taker 已通过 native fee 支付）
			if feeNative > 0 {
				makerAmount = subtractFee(makerAmount, feeNative, makerToken)
			}
			response.Msg.Quotes[0].MakerAmount = strPtr(makerAmount.String())
		}
	} else if quote.MakerAmount != nil {
		// 已知 MakerAmount → 计算 TakerAmount
		makerAmount, _ = new(big.Int).SetString(*quote.MakerAmount, 10)
		takerAmount = CalculateTakerAmount(takerToken, makerToken, makerAmount)
		// 有费用时增加 taker_amount（用户需多付以覆盖费用）
		if feeNative > 0 {
			takerAmount = addFee(takerAmount, feeNative, takerToken)
		}
		response.Msg.Quotes[0].TakerAmount = strPtr(takerAmount.String())
	} else {
		takerAmount, makerAmount = big.NewInt(0), big.NewInt(0)
	}

	// 计算参考价格（基础价格，不含费用调整）
	response.Msg.Quotes[0].ReferencePrice = calculateRefPrice(msg.Msg.Quotes[0], takerToken, makerToken, takerAmount, makerAmount)

	// 构建 SingleOrder
	makerNonce, _ := new(big.Int).SetString(msg.Msg.MakerNonce, 10)
	packedCommands, _ := new(big.Int).SetString(msg.Msg.PackedCommands, 10)

	order := SingleOrder{
		PartnerID:      uint64(msg.Msg.OnchainPartnerID),
		Expiry:         big.NewInt(int64(msg.Msg.Expiry)),
		TakerAddress:   common.HexToAddress(msg.Msg.TakerAddress),
		MakerAddress:   common.HexToAddress(h.signer.GetAddress()),
		MakerNonce:     makerNonce,
		TakerToken:     common.HexToAddress(takerToken),
		MakerToken:     common.HexToAddress(makerToken),
		TakerAmount:    takerAmount,
		MakerAmount:    makerAmount,
		Receiver:       common.HexToAddress(msg.Msg.Receiver),
		PackedCommands: packedCommands,
		ChainID:        big.NewInt(int64(msg.ChainID)),
	}

	return order, response
}

// =============================================================================
// MultiOrder 处理
// =============================================================================

// processMultiOrder 处理多订单并发送签名响应
func (h *RFQHandler) processMultiOrder(msg QuoteRequest) error {
	order, response := h.buildMultiOrder(msg)

	signature, err := h.signer.SignMultiOrder(order)
	if err != nil {
		log.Printf("⚠️ MultiOrder 签名失败: %v", err)
		return err
	}

	response.Msg.Signature = &signatureResponse{SignScheme: "EIP712", Signature: signature}
	return h.sendResponse(response)
}

// buildMultiOrder 构建 MultiOrder 和报价响应
func (h *RFQHandler) buildMultiOrder(msg QuoteRequest) (MultiOrder, QuoteRequest) {
	response := h.initResponse(msg)
	feeNative := msg.Msg.FeeNative
	numQuotes := len(msg.Msg.Quotes)

	// 为每个 quote 设置 token 映射
	h.assignTokensToQuotes(&response, msg)

	// 处理每个 quote 的金额计算
	for i, quote := range msg.Msg.Quotes {
		takerToken := response.Msg.Quotes[i].TakerToken
		makerToken := response.Msg.Quotes[i].MakerToken

		if quote.TakerAmount != nil && quote.MakerAmount == nil {
			takerAmt, _ := new(big.Int).SetString(*quote.TakerAmount, 10)
			makerAmt := CalculateMakerAmount(takerToken, makerToken, takerAmt)
			if feeNative > 0 {
				makerAmt = subtractFee(makerAmt, feeNative/float64(numQuotes), makerToken)
			}
			response.Msg.Quotes[i].MakerAmount = strPtr(makerAmt.String())
		} else if quote.MakerAmount != nil && quote.TakerAmount == nil {
			makerAmt, _ := new(big.Int).SetString(*quote.MakerAmount, 10)
			takerAmt := CalculateTakerAmount(takerToken, makerToken, makerAmt)
			response.Msg.Quotes[i].TakerAmount = strPtr(takerAmt.String())
		}

		// 计算参考价格
		if response.Msg.Quotes[i].MakerAmount != nil && response.Msg.Quotes[i].TakerAmount != nil {
			takerAmt, _ := new(big.Int).SetString(*response.Msg.Quotes[i].TakerAmount, 10)
			baseMakerAmt := CalculateMakerAmount(takerToken, makerToken, takerAmt)
			takerFloat := new(big.Float).SetInt(takerAmt)
			makerFloat := new(big.Float).SetInt(baseMakerAmt)
			if takerFloat.Cmp(big.NewFloat(0)) != 0 {
				refPrice, _ := new(big.Float).Quo(makerFloat, takerFloat).Float64()
				response.Msg.Quotes[i].ReferencePrice = &refPrice
			}
		}
	}

	// 聚合 taker/maker amounts
	takerAmounts := h.aggregateAmounts(response.Msg.Quotes, msg.Msg.TakerTokensIndices, len(msg.Msg.TakerTokens), true)
	makerAmounts := h.aggregateAmounts(response.Msg.Quotes, msg.Msg.MakerTokensIndices, len(msg.Msg.MakerTokens), false)

	// 解析 commands
	var commands []byte
	if strings.HasPrefix(msg.Msg.Commands, "0x") {
		commands, _ = hex.DecodeString(strings.TrimPrefix(msg.Msg.Commands, "0x"))
	} else {
		commands = []byte(msg.Msg.Commands)
	}

	makerNonce, _ := new(big.Int).SetString(msg.Msg.MakerNonce, 10)

	order := MultiOrder{
		PartnerID:    uint64(msg.Msg.OnchainPartnerID),
		Expiry:       big.NewInt(int64(msg.Msg.Expiry)),
		TakerAddress: common.HexToAddress(msg.Msg.TakerAddress),
		MakerAddress: common.HexToAddress(h.signer.GetAddress()),
		MakerNonce:   makerNonce,
		TakerTokens:  toAddresses(msg.Msg.TakerTokens),
		MakerTokens:  toAddresses(msg.Msg.MakerTokens),
		TakerAmounts: takerAmounts,
		MakerAmounts: makerAmounts,
		Receiver:     common.HexToAddress(msg.Msg.Receiver),
		Commands:     commands,
		ChainID:      big.NewInt(int64(msg.ChainID)),
	}

	return order, response
}

// =============================================================================
// 辅助函数
// =============================================================================

// initResponse 初始化响应对象
func (h *RFQHandler) initResponse(msg QuoteRequest) QuoteRequest {
	response := msg
	addr := h.signer.GetAddress()
	response.Msg.MakerAddress = &addr
	response.MsgType = "response"
	return response
}

// sendResponse 序列化并发送响应
func (h *RFQHandler) sendResponse(response QuoteRequest) error {
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("✗ JSON 序列化失败: %v", err)
		return err
	}
	if err := h.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		log.Printf("✗ 发送响应失败: %v", err)
		return err
	}
	return nil
}

// assignTokensToQuotes 为 MultiOrder 的每个 quote 分配 taker/maker token
func (h *RFQHandler) assignTokensToQuotes(response *QuoteRequest, msg QuoteRequest) {
	for i := range msg.Msg.Quotes {
		// TakerToken
		if i < len(msg.Msg.TakerTokensIndices) && int(msg.Msg.TakerTokensIndices[i]) < len(msg.Msg.TakerTokens) {
			response.Msg.Quotes[i].TakerToken = msg.Msg.TakerTokens[msg.Msg.TakerTokensIndices[i]]
		} else if len(msg.Msg.TakerTokens) == 1 {
			response.Msg.Quotes[i].TakerToken = msg.Msg.TakerTokens[0]
		}
		// MakerToken
		if i < len(msg.Msg.MakerTokensIndices) && int(msg.Msg.MakerTokensIndices[i]) < len(msg.Msg.MakerTokens) {
			response.Msg.Quotes[i].MakerToken = msg.Msg.MakerTokens[msg.Msg.MakerTokensIndices[i]]
		} else if len(msg.Msg.MakerTokens) == 1 {
			response.Msg.Quotes[i].MakerToken = msg.Msg.MakerTokens[0]
		}
	}
}

// aggregateAmounts 聚合 quotes 中的金额到 token 数组
func (h *RFQHandler) aggregateAmounts(quotes []*Quote, indices []int32, tokenCount int, isTaker bool) []*big.Int {
	amounts := make([]*big.Int, tokenCount)
	for i := range amounts {
		amounts[i] = big.NewInt(0)
	}
	for i, quote := range quotes {
		var amtStr *string
		if isTaker {
			amtStr = quote.TakerAmount
		} else {
			amtStr = quote.MakerAmount
		}
		if amtStr == nil {
			continue
		}
		amt, _ := new(big.Int).SetString(*amtStr, 10)
		idx := h.getTokenIndex(i, indices, tokenCount)
		if idx < len(amounts) {
			amounts[idx] = new(big.Int).Add(amounts[idx], amt)
		}
	}
	return amounts
}

// getTokenIndex 获取 quote 对应的 token 索引
func (h *RFQHandler) getTokenIndex(quoteIdx int, indices []int32, tokenCount int) int {
	if quoteIdx < len(indices) {
		return int(indices[quoteIdx])
	}
	if tokenCount == 1 {
		return 0
	}
	return quoteIdx
}

// calculateRefPrice 计算参考价格（基础价格，不含费用）
func calculateRefPrice(quote *Quote, takerToken, makerToken string, takerAmt, makerAmt *big.Int) *float64 {
	var baseMaker, baseTaker *big.Float
	if quote.TakerAmount != nil {
		baseTaker, _ = new(big.Float).SetString(*quote.TakerAmount)
		baseMaker = new(big.Float).SetInt(CalculateMakerAmount(takerToken, makerToken, takerAmt))
	} else if quote.MakerAmount != nil {
		baseMaker, _ = new(big.Float).SetString(*quote.MakerAmount)
		baseTaker = new(big.Float).SetInt(CalculateTakerAmount(takerToken, makerToken, makerAmt))
	} else {
		return nil
	}
	if baseTaker == nil || baseTaker.Cmp(big.NewFloat(0)) == 0 {
		return nil
	}
	refPrice, _ := new(big.Float).Quo(baseMaker, baseTaker).Float64()
	return &refPrice
}

// subtractFee 从金额中扣除费用
// 使用 useBidPrice=true，让扣除的费用更大（对 MM 有利）
func subtractFee(amount *big.Int, feeNative float64, tokenAddr string) *big.Int {
	fee := ConvertNativeFeeToToken(feeNative, tokenAddr, true)
	if amount.Cmp(fee) > 0 {
		return new(big.Int).Sub(amount, fee)
	}
	return amount
}

// addFee 向金额添加费用
// 使用 useBidPrice=false，让添加的费用更小（对用户稍有利）
func addFee(amount *big.Int, feeNative float64, tokenAddr string) *big.Int {
	fee := ConvertNativeFeeToToken(feeNative, tokenAddr, false)
	return new(big.Int).Add(amount, fee)
}

// toAddresses 将字符串地址数组转换为 common.Address 数组
func toAddresses(addrs []string) []common.Address {
	result := make([]common.Address, len(addrs))
	for i, addr := range addrs {
		result[i] = common.HexToAddress(addr)
	}
	return result
}

// strPtr 返回字符串指针
func strPtr(s string) *string {
	return &s
}
