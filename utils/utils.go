package utils

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// HexToBytes 将十六进制地址转换为字节数组
func HexToBytes(hexStr string) []byte {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	bytes, _ := hex.DecodeString(hexStr)
	return bytes
}

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
