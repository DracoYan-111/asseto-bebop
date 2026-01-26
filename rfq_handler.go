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

type RFQHandler struct {
	signer *OrderSigner    // 订单签名器（用于生成 EIP712 签名）
	conn   *websocket.Conn // WebSocket 连接（用于发送响应）
}

func (h *RFQHandler) handleJSONQuoteRequest(msg QuoteRequest) error {
	// // 如果是 taker_quote，返回详细的 TakerQuoteResponse
	// if msg.MsgTopic == "taker_quote" {
	// 	return h.handleTakerQuoteResponse(msg)
	// }

	switch msg.Msg.OrderSigningType {
	case "SingleOrder":
		singleOrder, quoteResponse := h.buildSingleOrder(msg)
		// jsonStr, err := json.MarshalIndent(singleOrder, "", "  ")
		// if err != nil {
		// 	log.Printf("⚠️ JSON 格式化错误: %v", err)
		// 	return err
		// }
		// log.Printf("📋 构建的单个订单: %s", string(jsonStr))

		signature, err := h.signer.SignOrder(singleOrder)
		if err != nil {
			log.Printf("⚠️ SingleOrder 签名失败: %v", err)
			return err
		}
		// log.Printf("📝 生成的 SingleOrder 签名: %s", signature)

		quoteResponse.Msg.Signature = &signatureResponse{
			SignScheme: "EIP712",
			Signature:  signature,
		}

		// 序列化并发送
		respData, err := json.Marshal(quoteResponse)
		if err != nil {
			log.Printf("✗ JSON 序列化失败: %v", err)
			return err
		}
		// Bebop market-maker WS expects messages sent as bytes (binary frames), even for JSON payloads.
		if err := h.conn.WriteMessage(websocket.BinaryMessage, respData); err != nil {
			log.Printf("✗ 发送响应失败: %v", err)
			return err
		}
		return nil
	case "MultiOrder":
		multiOrder, quoteResponse := h.buildMultiOrder(msg)
		// jsonStr, err := json.MarshalIndent(multiOrder, "", "  ")
		// if err != nil {
		// 	log.Printf("⚠️ JSON 格式化错误: %v", err)
		// 	return err
		// }
		// log.Printf("📋 构建的多个订单: %s", string(jsonStr))

		signature, err := h.signer.SignMultiOrder(multiOrder)
		if err != nil {
			log.Printf("⚠️ MultiOrder 签名失败: %v", err)
			return err
		}
		// log.Printf("📝 生成的 MultiOrder 签名: %s", signature)

		quoteResponse.Msg.Signature = &signatureResponse{
			SignScheme: "EIP712",
			Signature:  signature,
		}

		// 序列化并发送
		respData, err := json.Marshal(quoteResponse)
		if err != nil {
			log.Printf("✗ JSON 序列化失败: %v", err)
			return err
		}
		// log.Println("--------------------------------")
		// log.Printf("📤 发送签名响应: %s", string(respData))
		// log.Println("--------------------------------")
		// Bebop market-maker WS expects messages sent as bytes (binary frames), even for JSON payloads.
		if err := h.conn.WriteMessage(websocket.BinaryMessage, respData); err != nil {
			log.Printf("✗ 发送响应失败: %v", err)
			return err
		}
		return nil
	}
	return nil
}

// 构建 SingleOrder
func (h *RFQHandler) buildSingleOrder(msg QuoteRequest) (SingleOrder, QuoteRequest) {
	quoteResponse := QuoteRequest{}
	quoteResponse = msg
	makerAddr := h.signer.GetAddress()
	quoteResponse.Msg.MakerAddress = &makerAddr
	quoteResponse.MsgType = "response"

	makerNonce, _ := new(big.Int).SetString(msg.Msg.MakerNonce, 10)
	takerToken := msg.Msg.TakerTokens[0]
	makerToken := msg.Msg.MakerTokens[0]

	// 根据价格计算 TakerAmount 和 MakerAmount
	var takerAmount, makerAmount *big.Int

	// 检查是否有费用需要处理
	feeNative := msg.Msg.FeeNative

	if msg.Msg.Quotes[0].TakerAmount != nil {
		// 已知 TakerAmount，计算 MakerAmount
		takerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].TakerAmount, 10)
		if msg.Msg.Quotes[0].MakerAmount != nil {
			makerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].MakerAmount, 10)
		} else {
			makerAmount = CalculateMakerAmount(takerToken, makerToken, takerAmount)
			// 如果有费用，需要从 maker_amount 中减去费用
			// 因为 taker 已经通过 native fee 支付了这部分，所以 maker 给的代币更少
			if feeNative > 0 {
				feeInMakerToken := ConvertNativeFeeToToken(feeNative, makerToken)
				if makerAmount.Cmp(feeInMakerToken) > 0 {
					makerAmount = new(big.Int).Sub(makerAmount, feeInMakerToken)
				}
			}
			quoteResponse.Msg.Quotes[0].MakerAmount = new(string)
			*quoteResponse.Msg.Quotes[0].MakerAmount = makerAmount.String()
		}
	} else if msg.Msg.Quotes[0].MakerAmount != nil {
		// 已知 MakerAmount，计算 TakerAmount
		makerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].MakerAmount, 10)
		takerAmount = CalculateTakerAmount(takerToken, makerToken, makerAmount)
		// 如果有费用，需要增加 taker_amount
		// 因为用户想要固定的 maker_amount，他们需要支付更多（费用 + 兑换金额）
		if feeNative > 0 {
			feeInTakerToken := ConvertNativeFeeToToken(feeNative, takerToken)
			takerAmount = new(big.Int).Add(takerAmount, feeInTakerToken)
		}
		quoteResponse.Msg.Quotes[0].TakerAmount = new(string)
		*quoteResponse.Msg.Quotes[0].TakerAmount = takerAmount.String()
	} else {
		// 两者都为空，使用默认值（不应该发生）
		takerAmount = big.NewInt(0)
		makerAmount = big.NewInt(0)
	}

	packedCommands, _ := new(big.Int).SetString(msg.Msg.PackedCommands, 10)
	expiry := new(big.Int).SetInt64(int64(msg.Msg.Expiry))

	// 参考价格（可选）
	// ref_price 应该是基础价格（不含费用调整）
	// 例如：taker_amount=1 ETH, fee=0.5 ETH, price=4000 => maker_amount=2000, ref_price=4000
	if len(quoteResponse.Msg.Quotes) > 0 && quoteResponse.Msg.Quotes[0] != nil &&
		quoteResponse.Msg.Quotes[0].MakerAmount != nil && quoteResponse.Msg.Quotes[0].TakerAmount != nil {

		// 使用基础价格计算 ref_price（不含任何费用调整）
		// 需要使用未扣除费用的基础金额
		var baseMakerFloat, baseTakerFloat *big.Float

		if msg.Msg.Quotes[0].TakerAmount != nil {
			// TakerAmount 请求：使用原始 taker_amount
			baseTakerFloat, _ = new(big.Float).SetString(*msg.Msg.Quotes[0].TakerAmount)
			baseMakerAmount := CalculateMakerAmount(takerToken, makerToken, takerAmount)
			baseMakerFloat = new(big.Float).SetInt(baseMakerAmount)
		} else if msg.Msg.Quotes[0].MakerAmount != nil {
			// MakerAmount 请求：使用原始 maker_amount，计算基础 taker_amount
			baseMakerFloat, _ = new(big.Float).SetString(*msg.Msg.Quotes[0].MakerAmount)
			baseTakerAmount := CalculateTakerAmount(takerToken, makerToken, makerAmount)
			baseTakerFloat = new(big.Float).SetInt(baseTakerAmount)
		} else {
			baseTakerFloat, _ = new(big.Float).SetString(*quoteResponse.Msg.Quotes[0].TakerAmount)
			baseMakerFloat, _ = new(big.Float).SetString(*quoteResponse.Msg.Quotes[0].MakerAmount)
		}

		if baseTakerFloat != nil && baseTakerFloat.Cmp(big.NewFloat(0)) != 0 {
			refPriceFloat := new(big.Float).Quo(baseMakerFloat, baseTakerFloat)
			refPriceVal, _ := refPriceFloat.Float64()
			quoteResponse.Msg.Quotes[0].ReferencePrice = &refPriceVal
		}
	}
	singleOrder := SingleOrder{
		PartnerID:      uint64(msg.Msg.OnchainPartnerID),
		Expiry:         expiry,
		TakerAddress:   common.HexToAddress(msg.Msg.TakerAddress),
		MakerAddress:   common.HexToAddress(h.signer.GetAddress()),
		MakerNonce:     makerNonce,
		TakerToken:     common.HexToAddress(msg.Msg.TakerTokens[0]),
		MakerToken:     common.HexToAddress(msg.Msg.MakerTokens[0]),
		TakerAmount:    takerAmount,
		MakerAmount:    makerAmount,
		Receiver:       common.HexToAddress(msg.Msg.Receiver),
		PackedCommands: packedCommands,
		ChainID:        big.NewInt(int64(msg.ChainID)),
	}

	return singleOrder, quoteResponse
}

// 构建 MultiOrder
func (h *RFQHandler) buildMultiOrder(msg QuoteRequest) (MultiOrder, QuoteRequest) {
	quoteResponse := QuoteRequest{}
	quoteResponse = msg
	makerAddr := h.signer.GetAddress()
	quoteResponse.Msg.MakerAddress = &makerAddr
	quoteResponse.MsgType = "response"

	takerAddress := common.HexToAddress(msg.Msg.TakerAddress)
	makerAddress := common.HexToAddress(h.signer.GetAddress())
	makerNonce, _ := new(big.Int).SetString(msg.Msg.MakerNonce, 10)
	takerTokens := make([]common.Address, len(msg.Msg.TakerTokens))
	for i, token := range msg.Msg.TakerTokens {
		takerTokens[i] = common.HexToAddress(token)
	}
	makerTokens := make([]common.Address, len(msg.Msg.MakerTokens))
	for i, token := range msg.Msg.MakerTokens {
		makerTokens[i] = common.HexToAddress(token)
	}

	// 使用索引映射正确设置每个 Quote 的 TakerToken 和 MakerToken
	// 对于 MANY_TO_ONE: 多个 taker tokens -> 1 个 maker token
	// 对于 ONE_TO_MANY: 1 个 taker token -> 多个 maker tokens
	for i := range msg.Msg.Quotes {
		// 使用 TakerTokensIndices 获取对应的 taker token
		if i < len(msg.Msg.TakerTokensIndices) {
			idx := msg.Msg.TakerTokensIndices[i]
			if int(idx) < len(msg.Msg.TakerTokens) {
				quoteResponse.Msg.Quotes[i].TakerToken = msg.Msg.TakerTokens[idx]
			}
		} else if i < len(msg.Msg.TakerTokens) {
			quoteResponse.Msg.Quotes[i].TakerToken = msg.Msg.TakerTokens[i]
		} else if len(msg.Msg.TakerTokens) == 1 {
			// ONE_TO_MANY: 所有 quotes 使用同一个 taker token
			quoteResponse.Msg.Quotes[i].TakerToken = msg.Msg.TakerTokens[0]
		}

		// 使用 MakerTokensIndices 获取对应的 maker token
		if i < len(msg.Msg.MakerTokensIndices) {
			idx := msg.Msg.MakerTokensIndices[i]
			if int(idx) < len(msg.Msg.MakerTokens) {
				quoteResponse.Msg.Quotes[i].MakerToken = msg.Msg.MakerTokens[idx]
			}
		} else if i < len(msg.Msg.MakerTokens) {
			quoteResponse.Msg.Quotes[i].MakerToken = msg.Msg.MakerTokens[i]
		} else if len(msg.Msg.MakerTokens) == 1 {
			// MANY_TO_ONE: 所有 quotes 使用同一个 maker token
			quoteResponse.Msg.Quotes[i].MakerToken = msg.Msg.MakerTokens[0]
		}
	}
	// 首先处理每个 Quote，计算缺失的 TakerAmount 或 MakerAmount
	// 检查是否有费用
	feeNative := msg.Msg.FeeNative
	numQuotes := len(msg.Msg.Quotes)

	for i, quote := range msg.Msg.Quotes {
		takerToken := quoteResponse.Msg.Quotes[i].TakerToken
		makerToken := quoteResponse.Msg.Quotes[i].MakerToken

		if quote.TakerAmount != nil && quote.MakerAmount == nil {
			// 已知 TakerAmount，计算 MakerAmount
			takerAmt, _ := new(big.Int).SetString(*quote.TakerAmount, 10)
			makerAmt := CalculateMakerAmount(takerToken, makerToken, takerAmt)
			// 如果有费用，从 maker_amount 中扣除（按比例分配）
			if feeNative > 0 && numQuotes > 0 {
				feePerQuote := feeNative / float64(numQuotes)
				feeInMakerToken := ConvertNativeFeeToToken(feePerQuote, makerToken)
				if makerAmt.Cmp(feeInMakerToken) > 0 {
					makerAmt = new(big.Int).Sub(makerAmt, feeInMakerToken)
				}
			}
			quoteResponse.Msg.Quotes[i].MakerAmount = new(string)
			*quoteResponse.Msg.Quotes[i].MakerAmount = makerAmt.String()
		} else if quote.MakerAmount != nil && quote.TakerAmount == nil {
			// 已知 MakerAmount，计算 TakerAmount
			makerAmt, _ := new(big.Int).SetString(*quote.MakerAmount, 10)
			takerAmt := CalculateTakerAmount(takerToken, makerToken, makerAmt)
			quoteResponse.Msg.Quotes[i].TakerAmount = new(string)
			*quoteResponse.Msg.Quotes[i].TakerAmount = takerAmt.String()
		}

		// 计算参考价格
		// ref_price 应该是基础价格（不含费用调整）
		if quoteResponse.Msg.Quotes[i].MakerAmount != nil && quoteResponse.Msg.Quotes[i].TakerAmount != nil {
			takerAmtFloat, _ := new(big.Float).SetString(*quoteResponse.Msg.Quotes[i].TakerAmount)
			// 使用基础 maker_amount 计算 ref_price（不含费用扣减）
			takerAmt, _ := new(big.Int).SetString(*quoteResponse.Msg.Quotes[i].TakerAmount, 10)
			baseMakerAmt := CalculateMakerAmount(takerToken, makerToken, takerAmt)
			baseMakerFloat := new(big.Float).SetInt(baseMakerAmt)

			if takerAmtFloat.Cmp(big.NewFloat(0)) != 0 {
				refPriceFloat := new(big.Float).Quo(baseMakerFloat, takerAmtFloat)
				refPriceVal, _ := refPriceFloat.Float64()
				quoteResponse.Msg.Quotes[i].ReferencePrice = &refPriceVal
			}
		}
	}

	// 构建 TakerAmounts：长度等于 TakerTokens 数量
	// 使用 TakerTokensIndices 聚合对应的 taker amounts
	takerAmounts := make([]*big.Int, len(msg.Msg.TakerTokens))
	for i := range takerAmounts {
		takerAmounts[i] = big.NewInt(0)
	}
	for i, quote := range quoteResponse.Msg.Quotes {
		if quote.TakerAmount == nil {
			continue
		}
		amt, _ := new(big.Int).SetString(*quote.TakerAmount, 10)
		// 确定这个 quote 对应哪个 taker token
		var tokenIdx int
		if i < len(msg.Msg.TakerTokensIndices) {
			tokenIdx = int(msg.Msg.TakerTokensIndices[i])
		} else if len(msg.Msg.TakerTokens) == 1 {
			tokenIdx = 0 // ONE_TO_MANY: 所有 quotes 对应同一个 taker token
		} else {
			tokenIdx = i
		}
		if tokenIdx < len(takerAmounts) {
			takerAmounts[tokenIdx] = new(big.Int).Add(takerAmounts[tokenIdx], amt)
		}
	}

	// 构建 MakerAmounts：长度等于 MakerTokens 数量
	// 使用 MakerTokensIndices 聚合对应的 maker amounts
	makerAmounts := make([]*big.Int, len(msg.Msg.MakerTokens))
	for i := range makerAmounts {
		makerAmounts[i] = big.NewInt(0)
	}
	for i, quote := range quoteResponse.Msg.Quotes {
		if quote.MakerAmount == nil {
			continue
		}
		amt, _ := new(big.Int).SetString(*quote.MakerAmount, 10)
		// 确定这个 quote 对应哪个 maker token
		var tokenIdx int
		if i < len(msg.Msg.MakerTokensIndices) {
			tokenIdx = int(msg.Msg.MakerTokensIndices[i])
		} else if len(msg.Msg.MakerTokens) == 1 {
			tokenIdx = 0 // MANY_TO_ONE: 所有 quotes 对应同一个 maker token
		} else {
			tokenIdx = i
		}
		if tokenIdx < len(makerAmounts) {
			makerAmounts[tokenIdx] = new(big.Int).Add(makerAmounts[tokenIdx], amt)
		}
	}
	receiver := common.HexToAddress(msg.Msg.Receiver)
	var commands []byte
	if strings.HasPrefix(msg.Msg.Commands, "0x") {
		commands, _ = hex.DecodeString(strings.TrimPrefix(msg.Msg.Commands, "0x"))
	} else {
		commands = []byte(msg.Msg.Commands)
	}

	expiry := new(big.Int).SetInt64(int64(msg.Msg.Expiry))

	multiOrder := MultiOrder{
		PartnerID:    uint64(msg.Msg.OnchainPartnerID),
		Expiry:       expiry,
		TakerAddress: takerAddress,
		MakerAddress: makerAddress,
		MakerNonce:   makerNonce,
		TakerTokens:  takerTokens,
		MakerTokens:  makerTokens,
		TakerAmounts: takerAmounts,
		MakerAmounts: makerAmounts,
		Receiver:     receiver,
		Commands:     commands,
		ChainID:      big.NewInt(int64(msg.ChainID)),
	}

	return multiOrder, quoteResponse
}

// func (h *RFQHandler) handleTakerQuoteResponse(msg QuoteRequest) error {
// 	singleOrder := h.buildSingleOrder(msg)

// 	// 2. 签名
// 	signature, err := h.signer.SignOrder(singleOrder)
// 	if err != nil {
// 		log.Printf("⚠️ 签名失败: %v", err)
// 		return err
// 	}
// 	makerAmountStr := singleOrder.MakerAmount.String()
// 	takerAmountStr := singleOrder.TakerAmount.String()

// 	respQuotes := make([]Quote, len(msg.Msg.Quotes))
// 	for i, q := range msg.Msg.Quotes {
// 		// 复制原有字段
// 		rq := Quote{
// 			TakerToken: q.TakerToken,
// 			MakerToken: q.MakerToken,
// 		}
// 		// 填入金额
// 		// 这里只处理了 index 0 的计算结果，如果是多 quote 需改进 buildSingleOrder 或由 buildMultiOrder 处理
// 		if i == 0 {
// 			rq.MakerAmount = &makerAmountStr
// 			rq.TakerAmount = &takerAmountStr
// 		}
// 		// 参考价格（可选），需要将字符串转换为浮点数进行计算
// 		if rq.MakerAmount != nil && rq.TakerAmount != nil {
// 			makerAmtFloat, _ := new(big.Float).SetString(*rq.MakerAmount)
// 			takerAmtFloat, _ := new(big.Float).SetString(*rq.TakerAmount)
// 			if takerAmtFloat.Cmp(big.NewFloat(0)) != 0 { // 避免除以零
// 				refPriceFloat := new(big.Float).Quo(makerAmtFloat, takerAmtFloat)
// 				refPriceStr := refPriceFloat.String()
// 				rq.ReferencePrice = &refPriceStr
// 			}
// 		}

// 		respQuotes[i] = rq
// 	}

// 	// 4. 构建 TakerQuoteResponse
// 	response := TakerQuoteResponse{
// 		ChainID:  msg.ChainID,
// 		MsgTopic: "taker_quote",
// 		MsgType:  "response",
// 		Msg: &TakerQuoteMessage{
// 			QuoteID:          msg.Msg.QuoteID,
// 			EventID:          msg.Msg.EventID,
// 			OrderSigningType: msg.Msg.OrderSigningType,
// 			OrderType:        msg.Msg.OrderType,
// 			OnchainPartnerID: msg.Msg.OnchainPartnerID,
// 			Expiry:           singleOrder.Expiry.Int64(),
// 			TakerAddress:     singleOrder.TakerAddress.Hex(),
// 			OriginAddress:    msg.Msg.OriginAddress,
// 			MakerAddress:     h.signer.GetAddress(), // 使用签名器的地址
// 			MakerNonce:       singleOrder.MakerNonce.String(),
// 			Quotes:           respQuotes,
// 			Receiver:         singleOrder.Receiver.Hex(),
// 			Commands:         msg.Msg.Commands,
// 			PackedCommands:   singleOrder.PackedCommands.String(),
// 			FeeNative:        msg.Msg.FeeNative,
// 			IsAggregateOrder: msg.Msg.IsAggregateOrder,
// 			Signature: &struct {
// 				Signature  string `json:"signature"`
// 				SignScheme string `json:"sign_scheme"`
// 			}{
// 				Signature:  signature,
// 				SignScheme: "EIP712",
// 			},
// 		},
// 	}

// 	// 5. 序列化并发送
// 	respData, err := json.Marshal(response)
// 	if err != nil {
// 		log.Printf("✗ JSON 序列化失败: %v", err)
// 		return err
// 	}

// 	log.Println("--------------------------------")
// 	log.Printf("📤 发送 TakerQuote 响应: %s", string(respData))
// 	log.Println("--------------------------------")

// 	if err := h.conn.WriteMessage(websocket.BinaryMessage, respData); err != nil {
// 		log.Printf("✗ 发送响应失败: %v", err)
// 		return err
// 	}

// 	return nil
// }
