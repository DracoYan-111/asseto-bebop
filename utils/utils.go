package utils

import (
	pb "asseto-bebop/proto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
)

// HexToBytes 将十六进制地址转换为字节数组
func HexToBytes(hexStr string) []byte {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	bytes, _ := hex.DecodeString(hexStr)
	return bytes
}

// ParseUint32 将字符串解析为 uint32
func ParseUint32(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	bi, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return 0, fmt.Errorf("invalid uint32: %s", s)
	}
	if bi.Sign() < 0 || bi.BitLen() > 32 {
		return 0, fmt.Errorf("out of range uint32: %s", s)
	}
	return uint32(bi.Uint64()), nil
}

// Keccak256 计算 Keccak256 哈希
func Keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// PadLeft 将字节数组左填充到指定长度
func PadLeft(data []byte, size int) []byte {
	if len(data) >= size {
		return data[len(data)-size:]
	}
	result := make([]byte, size)
	copy(result[size-len(data):], data)
	return result
}

// Concat 连接多个字节数组
func Concat(arrays ...[]byte) []byte {
	var result []byte
	for _, arr := range arrays {
		result = append(result, arr...)
	}
	return result
}

// HashAddressArray 对地址数组进行 keccak256 编码
// 用于 EIP712 中的 address[] 类型
func HashAddressArray(addresses []common.Address) []byte {
	var encoded []byte
	for _, addr := range addresses {
		encoded = append(encoded, PadLeft(addr.Bytes(), 32)...)
	}
	return Keccak256(encoded)
}

// hashBigIntArray 对 big.Int 数组进行 keccak256 编码
// 用于 EIP712 中的 uint256[] 类型
func HashBigIntArray(values []*big.Int) []byte {
	var encoded []byte
	for _, val := range values {
		encoded = append(encoded, PadLeft(val.Bytes(), 32)...)
	}
	return Keccak256(encoded)
}

// ParseToJSON 将数据解析/格式化为 JSON 字符串
// 如果数据已经是 JSON，则格式化输出
// 否则返回错误
func ParseToJSON(data []byte) (string, error) {
	// 尝试解析为 JSON
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("不是有效的 JSON: %v", err)
	}

	// 格式化输出
	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON 格式化失败: %v", err)
	}
	return string(formatted), nil
}

// ParseTradeJSON 解析 JSON 格式交易消息
func ParseTradeJSON(data []byte) error {
	var msg struct {
		MsgTopic string `json:"msg_topic"`
		MsgType  string `json:"msg_type"`
		Msg      struct {
			OrderID    string `json:"order_id"`
			QuoteID    string `json:"quote_id"`
			TxHash     string `json:"tx_hash"`
			Status     string `json:"status"`
			BuyToken   string `json:"buy_token"`
			SellToken  string `json:"sell_token"`
			BuyAmount  string `json:"buy_amount"`
			SellAmount string `json:"sell_amount"`
			Timestamp  int64  `json:"timestamp"`
			Code       int    `json:"code"`
			Text       string `json:"text"`
		} `json:"msg"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	switch msg.MsgTopic {
	case "trade":
		m := msg.Msg
		log.Printf("📊 收到交易通知: OrderID=%s, TxHash=%s, Status=%s", m.OrderID, m.TxHash, m.Status)
		if m.Status == "completed" {
			log.Printf("✅ 交易成功完成")
		} else if m.Status == "failed" {
			log.Printf("❌ 交易失败")
		}
	case "websocket":
		log.Printf("✓ trades_client: %s (代码 %d)", msg.Msg.Text, msg.Msg.Code)
	}
	return nil
}

// 将data转为对应的结构体
func ParseToStruct(data []byte, structType interface{}) (interface{}, error) {
	err := json.Unmarshal(data, structType)
	return structType, err
}

// ParseServerResponse 解析服务器的 Protobuf 响应
func ParseServerResponse(data []byte) (*pb.WebSocketResponse, error) {
	resp := &pb.WebSocketResponse{}
	err := proto.Unmarshal(data, resp)
	return resp, err
}
