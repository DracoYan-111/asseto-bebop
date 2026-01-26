package main

import (
	"asseto-bebop/utils"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	bebopContractAddress = "0xbbbbbBB520d69a9775E85b458C58c648259FAD5F"
	domainName           = "BebopSettlement"
	domainVersion        = "2"
)

// EIP712 类型哈希（预计算以提高效率）
var (
	domainTypeHash      = utils.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	singleOrderTypeHash = utils.Keccak256([]byte("SingleOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address taker_token,address maker_token,uint256 taker_amount,uint256 maker_amount,address receiver,uint256 packed_commands)"))
	multiOrderTypeHash  = utils.Keccak256([]byte("MultiOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address[] taker_tokens,address[] maker_tokens,uint256[] taker_amounts,uint256[] maker_amounts,address receiver,bytes commands)"))
	nameHash            = utils.Keccak256([]byte(domainName))
	versionHash         = utils.Keccak256([]byte(domainVersion))
	contractAddr        = common.HexToAddress(bebopContractAddress)
)

// =============================================================================
// 数据结构
// =============================================================================

// OrderSigner EIP712 订单签名器
type OrderSigner struct {
	privateKey *ecdsa.PrivateKey // ECDSA 私钥
	address    common.Address    // Market Maker 地址
	chainID    *big.Int          // 链 ID
}

// =============================================================================
// 公共方法
// =============================================================================

// NewOrderSigner 创建订单签名器
func NewOrderSigner(privateKeyHex string, chainID int64) (*OrderSigner, error) {
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("无效的私钥: %v", err)
	}

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("无法获取公钥")
	}

	return &OrderSigner{
		privateKey: privateKey,
		address:    crypto.PubkeyToAddress(*publicKey),
		chainID:    big.NewInt(chainID),
	}, nil
}

// GetAddress 获取签名器的以太坊地址
func (s *OrderSigner) GetAddress() string {
	return s.address.Hex()
}

// SignOrder 使用 EIP712 标准签名 SingleOrder
func (s *OrderSigner) SignOrder(order SingleOrder) (string, error) {
	messageHash := utils.Keccak256(utils.Concat(
		singleOrderTypeHash,
		padUint64(order.PartnerID),
		pad256(order.Expiry),
		padAddr(order.TakerAddress),
		padAddr(order.MakerAddress),
		pad256(order.MakerNonce),
		padAddr(order.TakerToken),
		padAddr(order.MakerToken),
		pad256(order.TakerAmount),
		pad256(order.MakerAmount),
		padAddr(order.Receiver),
		pad256(order.PackedCommands),
	))

	return s.signWithDomain(order.ChainID, messageHash)
}

// SignMultiOrder 使用 EIP712 标准签名 MultiOrder
func (s *OrderSigner) SignMultiOrder(order MultiOrder) (string, error) {
	messageHash := utils.Keccak256(utils.Concat(
		multiOrderTypeHash,
		padUint64(order.PartnerID),
		pad256(order.Expiry),
		padAddr(order.TakerAddress),
		padAddr(order.MakerAddress),
		pad256(order.MakerNonce),
		utils.HashAddressArray(order.TakerTokens),
		utils.HashAddressArray(order.MakerTokens),
		utils.HashBigIntArray(order.TakerAmounts),
		utils.HashBigIntArray(order.MakerAmounts),
		padAddr(order.Receiver),
		utils.Keccak256(order.Commands),
	))

	return s.signWithDomain(order.ChainID, messageHash)
}

// =============================================================================
// 内部方法
// =============================================================================

// signWithDomain 使用 EIP712 domain 签名消息哈希
func (s *OrderSigner) signWithDomain(chainID *big.Int, messageHash []byte) (string, error) {
	domainSeparator := utils.Keccak256(utils.Concat(
		domainTypeHash,
		nameHash,
		versionHash,
		pad256(chainID),
		padAddr(contractAddr),
	))

	signHash := utils.Keccak256(utils.Concat(
		[]byte{0x19, 0x01},
		domainSeparator,
		messageHash,
	))

	signature, err := crypto.Sign(signHash, s.privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %v", err)
	}

	signature[64] += 27 // 调整 v 值 (以太坊标准)
	return "0x" + hex.EncodeToString(signature), nil
}

// =============================================================================
// 辅助函数 - 32 字节填充
// =============================================================================

func pad256(n *big.Int) []byte        { return utils.PadLeft(n.Bytes(), 32) }
func padAddr(a common.Address) []byte { return utils.PadLeft(a.Bytes(), 32) }
func padUint64(n uint64) []byte       { return utils.PadLeft(big.NewInt(int64(n)).Bytes(), 32) }
