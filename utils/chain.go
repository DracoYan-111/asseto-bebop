package utils

import "fmt"

var chainNames = map[uint32]string{
	1:     "ethereum",
	137:   "polygon",
	42161: "arbitrum",
	10:    "optimism",
	8453:  "base",
	56:    "bsc",
	999:   "hyperevm",
	43114: "avalanche",
}

// GetChainName 根据 ChainID 获取链名称, 如果链ID不存在, 返回错误
func GetChainName(chainId uint32) (string, error) {
	if name, ok := chainNames[chainId]; ok {
		return name, nil
	}
	return "", fmt.Errorf("chain id %d not found", chainId)
}
