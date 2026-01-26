package utils

import (
	pb "asseto-bebop/proto"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
)

// =============================================================================
// 哈希与编码
// =============================================================================

// Keccak256 计算 Keccak256 哈希
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// HexToBytes 将 hex 字符串转换为字节数组
func HexToBytes(hexStr string) []byte {
	bytes, _ := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	return bytes
}

// =============================================================================
// 字节操作
// =============================================================================

// PadLeft 左填充字节数组到指定长度
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
	total := 0
	for _, arr := range arrays {
		total += len(arr)
	}
	result := make([]byte, 0, total)
	for _, arr := range arrays {
		result = append(result, arr...)
	}
	return result
}

// =============================================================================
// EIP712 数组编码
// =============================================================================

// HashAddressArray 对 address[] 进行 keccak256 编码
func HashAddressArray(addresses []common.Address) []byte {
	encoded := make([]byte, 0, len(addresses)*32)
	for _, addr := range addresses {
		encoded = append(encoded, PadLeft(addr.Bytes(), 32)...)
	}
	return Keccak256(encoded)
}

// HashBigIntArray 对 uint256[] 进行 keccak256 编码
func HashBigIntArray(values []*big.Int) []byte {
	encoded := make([]byte, 0, len(values)*32)
	for _, val := range values {
		encoded = append(encoded, PadLeft(val.Bytes(), 32)...)
	}
	return Keccak256(encoded)
}

// =============================================================================
// 解析函数
// =============================================================================

// ParseUint32 解析字符串为 uint32
func ParseUint32(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	bi, ok := new(big.Int).SetString(s, 10)
	if !ok || bi.Sign() < 0 || bi.BitLen() > 32 {
		return 0, nil
	}
	return uint32(bi.Uint64()), nil
}

// ParseTradeJSON 解析交易 JSON 消息
func ParseTradeJSON(data []byte) error {
	var msg struct {
		MsgTopic string `json:"msg_topic"`
		Msg      struct {
			OrderID string `json:"order_id"`
			TxHash  string `json:"tx_hash"`
			Status  string `json:"status"`
			Code    int    `json:"code"`
			Text    string `json:"text"`
		} `json:"msg"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return nil
	}

	switch msg.MsgTopic {
	case "trade":
		log.Printf("📊 收到交易通知: OrderID=%s, TxHash=%s, Status=%s",
			msg.Msg.OrderID, msg.Msg.TxHash, msg.Msg.Status)
		if msg.Msg.Status == "completed" {
			log.Printf("✅ 交易成功完成")
		} else if msg.Msg.Status == "failed" {
			log.Printf("❌ 交易失败")
		}
	case "websocket":
		log.Printf("✓ trades_client: %s (代码 %d)", msg.Msg.Text, msg.Msg.Code)
	}
	return nil
}

// ParseServerResponse 解析服务器 Protobuf 响应
func ParseServerResponse(data []byte) (*pb.WebSocketResponse, error) {
	resp := &pb.WebSocketResponse{}
	return resp, proto.Unmarshal(data, resp)
}
