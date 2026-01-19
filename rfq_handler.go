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

		jsonStr1, err := json.MarshalIndent(quoteResponse, "", "  ")
		if err != nil {
			log.Printf("⚠️ JSON 格式化错误: %v", err)
			return err
		}
		log.Printf("📋 构建的单个订单: %s", string(jsonStr1))

		// // 根据官方文档构建签名响应
		// signedRespond := &signedRespond{
		// 	ChainID:  msg.ChainID,
		// 	MsgTopic: "signature",
		// 	MsgType:  "response",
		// 	Msg: SignatureResponse{
		// 		QuoteID:    msg.Msg.QuoteID,
		// 		Signature:  signature,
		// 		SignScheme: "EIP712",
		// 	},
		// }

		// respData, err := json.Marshal(signedRespond)
		// if err != nil {
		// 	log.Printf("✗ JSON 序列化失败: %v", err)
		// 	return err
		// }
		// log.Println("--------------------------------")
		// log.Printf("📤 发送签名响应: %s", string(respData))
		// log.Println("--------------------------------")
		// // Bebop market-maker WS expects messages sent as bytes (binary frames), even for JSON payloads.
		// if err := h.conn.WriteMessage(websocket.BinaryMessage, respData); err != nil {
		// 	log.Printf("✗ 发送响应失败: %v", err)
		// 	return err
		// }
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

		jsonStr1, err := json.MarshalIndent(quoteResponse, "", "  ")
		if err != nil {
			log.Printf("⚠️ JSON 格式化错误: %v", err)
			return err
		}
		log.Printf("📋 构建的 MultiOrder 响应: %s", string(jsonStr1))

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

	if msg.Msg.Quotes[0].TakerAmount != nil {
		// 已知 TakerAmount，计算 MakerAmount
		takerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].TakerAmount, 10)
		if msg.Msg.Quotes[0].MakerAmount != nil {
			makerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].MakerAmount, 10)
		} else {
			makerAmount = CalculateMakerAmount(takerToken, makerToken, takerAmount)
			quoteResponse.Msg.Quotes[0].MakerAmount = new(string)
			*quoteResponse.Msg.Quotes[0].MakerAmount = makerAmount.String()
		}
	} else if msg.Msg.Quotes[0].MakerAmount != nil {
		// 已知 MakerAmount，计算 TakerAmount
		makerAmount, _ = new(big.Int).SetString(*msg.Msg.Quotes[0].MakerAmount, 10)
		takerAmount = CalculateTakerAmount(takerToken, makerToken, makerAmount)
		quoteResponse.Msg.Quotes[0].TakerAmount = new(string)
		*quoteResponse.Msg.Quotes[0].TakerAmount = takerAmount.String()
	} else {
		// 两者都为空，使用默认值（不应该发生）
		takerAmount = big.NewInt(0)
		makerAmount = big.NewInt(0)
	}

	packedCommands, _ := new(big.Int).SetString(msg.Msg.PackedCommands, 10)
	expiry := new(big.Int).SetInt64(int64(msg.Msg.Expiry))

	// 参考价格（可选），需要将字符串转换为浮点数进行计算
	if len(quoteResponse.Msg.Quotes) > 0 && quoteResponse.Msg.Quotes[0] != nil &&
		quoteResponse.Msg.Quotes[0].MakerAmount != nil && quoteResponse.Msg.Quotes[0].TakerAmount != nil {
		makerAmtFloat, _ := new(big.Float).SetString(*quoteResponse.Msg.Quotes[0].MakerAmount)
		takerAmtFloat, _ := new(big.Float).SetString(*quoteResponse.Msg.Quotes[0].TakerAmount)
		if takerAmtFloat.Cmp(big.NewFloat(0)) != 0 { // 避免除以零
			refPriceFloat := new(big.Float).Quo(makerAmtFloat, takerAmtFloat)
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
		quoteResponse.Msg.Quotes[i].TakerToken = token
	}
	makerTokens := make([]common.Address, len(msg.Msg.MakerTokens))
	for i, token := range msg.Msg.MakerTokens {
		makerTokens[i] = common.HexToAddress(token)
		quoteResponse.Msg.Quotes[i].MakerToken = token
	}
	// 根据价格计算 TakerAmounts 和 MakerAmounts
	takerAmounts := make([]*big.Int, len(msg.Msg.Quotes))
	makerAmounts := make([]*big.Int, len(msg.Msg.Quotes))

	for i, quote := range msg.Msg.Quotes {
		takerToken := quote.TakerToken
		makerToken := quote.MakerToken

		if quote.TakerAmount != nil {
			// 已知 TakerAmount，计算 MakerAmount
			takerAmounts[i], _ = new(big.Int).SetString(*quote.TakerAmount, 10)
			if quote.MakerAmount != nil {
				makerAmounts[i], _ = new(big.Int).SetString(*quote.MakerAmount, 10)
			} else {
				makerAmounts[i] = CalculateMakerAmount(takerToken, makerToken, takerAmounts[i])
				quoteResponse.Msg.Quotes[i].MakerAmount = new(string)
				*quoteResponse.Msg.Quotes[i].MakerAmount = makerAmounts[i].String()
			}
		} else if quote.MakerAmount != nil {
			// 已知 MakerAmount，计算 TakerAmount
			makerAmounts[i], _ = new(big.Int).SetString(*quote.MakerAmount, 10)
			takerAmounts[i] = CalculateTakerAmount(takerToken, makerToken, makerAmounts[i])
			quoteResponse.Msg.Quotes[i].TakerAmount = new(string)
			*quoteResponse.Msg.Quotes[i].TakerAmount = takerAmounts[i].String()
		} else {
			// 两者都为空，使用默认值（不应该发生）
			takerAmounts[i] = big.NewInt(0)
			makerAmounts[i] = big.NewInt(0)
		}

		if quote.MakerAmount != nil && quote.TakerAmount != nil {
			makerAmtFloat, _ := new(big.Float).SetString(*quote.MakerAmount)
			takerAmtFloat, _ := new(big.Float).SetString(*quote.TakerAmount)
			if takerAmtFloat.Cmp(big.NewFloat(0)) != 0 { // 避免除以零
				refPriceFloat := new(big.Float).Quo(makerAmtFloat, takerAmtFloat)
				refPriceVal, _ := refPriceFloat.Float64()
				quoteResponse.Msg.Quotes[i].ReferencePrice = &refPriceVal
			}
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
	log.Println("onchain_partner_id:", uint64(msg.Msg.OnchainPartnerID))
	log.Println("expiry:", expiry.String())

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
