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

var bebopContractAddress = "0xbbbbbBB520d69a9775E85b458C58c648259FAD5F"

// OrderSigner 订单签名器
//
// 职责：
// - 管理 Market Maker 的私钥和地址
// - 使用 EIP712 标准签名订单
// - 生成唯一的 nonce（确保每个订单的唯一性）
type OrderSigner struct {
	privateKey *ecdsa.PrivateKey // ECDSA 私钥（用于签名订单）
	address    common.Address    // Market Maker 地址（从公钥派生）
	chainID    *big.Int          // 链 ID（用于 EIP712 domain）
	nonce      *big.Int          // 递增的 nonce 计数器（确保每个订单唯一）
}

// GetAddress 获取签名器对应的以太坊地址
func (s *OrderSigner) GetAddress() string {
	return s.address.Hex()
}

// NewOrderSigner 创建订单签名器实例
//
// 功能：
// 1. 解析私钥（从 hex 字符串）
// 2. 从私钥派生公钥和地址
// 3. 初始化 nonce 计数器
//
// 参数：
//   - privateKeyHex: 私钥的 hex 字符串（带或不带 0x 前缀）
//   - chainID: 链 ID（1 = Ethereum mainnet）
//
// 返回：
//   - *OrderSigner: 签名器实例
//   - error: 如果私钥无效返回错误
func NewOrderSigner(privateKeyHex string, chainID int64) (*OrderSigner, error) {
	// 移除 0x 前缀
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	// 解析私钥
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("无效的私钥: %v", err)
	}

	// 获取地址
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("无法获取公钥")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	return &OrderSigner{
		privateKey: privateKey,
		address:    address,
		chainID:    big.NewInt(chainID),
		nonce:      big.NewInt(0), // 初始化 nonce 为 0
	}, nil
}

// SignOrder 使用 EIP712 标准签名 SingleOrder
//
// EIP712 签名流程：
//  1. 计算 domain separator 哈希
//  2. 计算 message 哈希（SingleOrder 结构）
//  3. 组合：\x19\x01 + domain_separator + message_hash
//  4. 对组合结果进行 Keccak256 哈希
//  5. 使用私钥签名哈希
//  6. 调整 v 值（+27）
//
// 参数：
//   - order: SingleOrder 订单结构
//
// 返回：“
//   - string: 签名的 hex 字符串（带 0x 前缀，65 字节）
//   - error: 如果签名失败返回错误
func (s *OrderSigner) SignOrder(order SingleOrder) (string, error) {
	// 计算 domain separator
	domainSeparator := s.computeDomainSeparator(order.ChainID)

	// 计算 SingleOrder 类型哈希
	// keccak256("SingleOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address taker_token,address maker_token,uint256 taker_amount,uint256 maker_amount,address receiver,uint256 packed_commands)")
	singleOrderTypeHash := utils.Keccak256([]byte("SingleOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address taker_token,address maker_token,uint256 taker_amount,uint256 maker_amount,address receiver,uint256 packed_commands)"))

	// 编码 SingleOrder message
	messageHash := utils.Keccak256(utils.Concat(
		singleOrderTypeHash,
		utils.PadLeft(big.NewInt(int64(order.PartnerID)).Bytes(), 32), // partner_id (uint64)
		utils.PadLeft(order.Expiry.Bytes(), 32),                       // expiry (uint256)
		utils.PadLeft(order.TakerAddress.Bytes(), 32),                 // taker_address
		utils.PadLeft(order.MakerAddress.Bytes(), 32),                 // maker_address
		utils.PadLeft(order.MakerNonce.Bytes(), 32),                   // maker_nonce
		utils.PadLeft(order.TakerToken.Bytes(), 32),                   // taker_token
		utils.PadLeft(order.MakerToken.Bytes(), 32),                   // maker_token
		utils.PadLeft(order.TakerAmount.Bytes(), 32),                  // taker_amount
		utils.PadLeft(order.MakerAmount.Bytes(), 32),                  // maker_amount
		utils.PadLeft(order.Receiver.Bytes(), 32),                     // receiver
		utils.PadLeft(order.PackedCommands.Bytes(), 32),               // packed_commands
	))

	// 计算最终签名哈希: \x19\x01 + domainSeparator + messageHash
	signHash := utils.Keccak256(utils.Concat(
		[]byte{0x19, 0x01},
		domainSeparator,
		messageHash,
	))

	// 使用私钥签名
	signature, err := crypto.Sign(signHash, s.privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %v", err)
	}

	// 调整 v 值 (以太坊标准: v = 27 或 28)
	signature[64] += 27

	return "0x" + hex.EncodeToString(signature), nil
}

// SignMultiOrder 使用 EIP712 标准签名 MultiOrder
//
// 参数：
//   - order: MultiOrder 订单结构
//
// 返回：
//   - string: 签名的 hex 字符串（带 0x 前缀，65 字节）
//   - error: 如果签名失败返回错误
func (s *OrderSigner) SignMultiOrder(order MultiOrder) (string, error) {
	// 计算 domain separator
	domainSeparator := s.computeDomainSeparator(order.ChainID)

	// 计算 MultiOrder 类型哈希
	// keccak256("MultiOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address[] taker_tokens,address[] maker_tokens,uint256[] taker_amounts,uint256[] maker_amounts,address receiver,bytes commands)")
	multiOrderTypeHash := utils.Keccak256([]byte("MultiOrder(uint64 partner_id,uint256 expiry,address taker_address,address maker_address,uint256 maker_nonce,address[] taker_tokens,address[] maker_tokens,uint256[] taker_amounts,uint256[] maker_amounts,address receiver,bytes commands)"))

	// 对数组进行 keccak256 编码
	takerTokensHash := utils.HashAddressArray(order.TakerTokens)
	makerTokensHash := utils.HashAddressArray(order.MakerTokens)
	takerAmountsHash := utils.HashBigIntArray(order.TakerAmounts)
	makerAmountsHash := utils.HashBigIntArray(order.MakerAmounts)
	commandsHash := utils.Keccak256(order.Commands)

	// 编码 MultiOrder message
	messageHash := utils.Keccak256(utils.Concat(
		multiOrderTypeHash,
		utils.PadLeft(big.NewInt(int64(order.PartnerID)).Bytes(), 32), // partner_id (uint64)
		utils.PadLeft(order.Expiry.Bytes(), 32),                       // expiry (uint256)
		utils.PadLeft(order.TakerAddress.Bytes(), 32),                 // taker_address
		utils.PadLeft(order.MakerAddress.Bytes(), 32),                 // maker_address
		utils.PadLeft(order.MakerNonce.Bytes(), 32),                   // maker_nonce
		takerTokensHash,  // keccak256(taker_tokens)
		makerTokensHash,  // keccak256(maker_tokens)
		takerAmountsHash, // keccak256(taker_amounts)
		makerAmountsHash, // keccak256(maker_amounts)
		utils.PadLeft(order.Receiver.Bytes(), 32), // receiver
		commandsHash, // keccak256(commands)
	))

	// 计算最终签名哈希: \x19\x01 + domainSeparator + messageHash
	signHash := utils.Keccak256(utils.Concat(
		[]byte{0x19, 0x01},
		domainSeparator,
		messageHash,
	))

	// 使用私钥签名
	signature, err := crypto.Sign(signHash, s.privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %v", err)
	}

	// 调整 v 值 (以太坊标准: v = 27 或 28)
	signature[64] += 27

	return "0x" + hex.EncodeToString(signature), nil
}

// computeDomainSeparator 计算 EIP712 domain separator
//
//	domain = {
//			name: "BebopSettlement",
//			version: "2",
//			chainId: chainID,
//			verifyingContract: bebopContractAddress
//		}
func (s *OrderSigner) computeDomainSeparator(chainID *big.Int) []byte {
	// keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")
	domainTypeHash := utils.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))

	// 编码 domain 数据
	nameHash := utils.Keccak256([]byte("BebopSettlement"))
	versionHash := utils.Keccak256([]byte("2"))
	contractAddr := common.HexToAddress(bebopContractAddress)

	return utils.Keccak256(utils.Concat(
		domainTypeHash,
		nameHash,
		versionHash,
		utils.PadLeft(chainID.Bytes(), 32),
		utils.PadLeft(contractAddr.Bytes(), 32),
	))
}
