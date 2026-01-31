# Bebop Market Maker (Go 版)

这是一个基于 Go 语言实现的 Bebop Market Maker (做市商) 程序，通过 RFQ (Request for Quote) 协议与 Bebop 交易所进行交互，提供自动化报价和交易执行功能。

## 🚀 项目功能

- **自动化报价**：根据配置的价格曲线自动响应 Bebop 的报价请求。
- **带费用计算**：支持原生代币 (BNB) 费用的自动转换与扣除/添加逻辑。
- **多链支持**：目前配置针对 BSC 网络，可扩展至其他 EVM 链。
- **可视化管理面板**：内置 Web 界面，方便实时查看价格、更新价格和查看交易历史。
- **高可用性**：支持 WebSocket 自动重连和心跳维持。

## 🛠️ 环境准备

- **Go**: 1.21 或更高版本
- **网络**: 能够访问 Bebop API 服务器（可能需要海外网络环境）
- **钱包**: 准备一个带有少量原生代币 (BNB) 的 EVM 钱包私钥

## ⚙️ 配置说明

在项目根目录下创建一个 `.env` 文件（可参考 `.env.example`）：

```env
# 链 ID (56 为 BSC 主网)
CHAIN_ID=56

# 做市商私钥 (必须配置，用于交易签名)
PRIVATE_KEY=your_private_key_here

# Bebop 分配的 Market Maker 名称
MARKET_MAKER=your_name

# Bebop 提供给你的授权 Token
AUTHORIZATION=your_auth_token

# 自执行模式 (推荐设为 true)
SELF_EXECUTION=true

# 默认点差 (例如 0.005 表示 0.5%)
PRICE_SPREAD=0.005

# 内部 API 服务端口
API_PORT=:8080

# 价格管理 API 的鉴权 Key (推荐配置)
PRICE_API_KEY=your_secret_key
```

## 🏗️ 构建与运行

### 1. 编译程序
```bash
go build -o asseto-bebop .
```

### 2. 运行程序
```bash
# 直接运行
./asseto-bebop

# 后台运行并记录日志
nohup ./asseto-bebop > mm.log 2>&1 &
```

## 🌐 API 接口参考

程序启动后会开启一个 HTTP API 服务器，用于动态管理报价。

| 端点 | 方法 | 描述 | 鉴权需求 |
|---|---|---|---|
| `/health` | GET | 检查服务健康状态和价格对数量 | 无 |
| `/api/price` | POST | 批量更新交易对报价 | 需要 `X-API-Key` |
| `/api/prices` | GET | 获取当前所有的报价详情 | 需要 `X-API-Key` |
| `/api/price/history` | GET | 获取价格更新历史记录 | 需要 `X-API-Key` |

### 批量更新报价示例 (POST `/api/price`)

```json
{
  "operator": "admin",
  "levels": [
    {
      "base_token": "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
      "quote_token": "0x55d398326f99059fF775485246999027B3197955",
      "base_decimals": 18,
      "quote_decimals": 18,
      "bids": [[845.39, 0.2], [843.39, 0.3]],
      "asks": [[840.39, 0.2], [843.39, 0.3]]
    }
  ]
}
```

## 📊 可视化管理面板

程序内置了一个简单的可视化 Dashboard，方便手动管理：

**访问地址**: `http://localhost:8080/web/`

**功能说明**:
1. **Health 标签**: 查看服务的连接状态。
2. **Prices 标签**: 查看当前所有正在使用的报价。
3. **History 标签**: 查看过去的价格调整记录。
4. **Update Price 标签**: 图形化表单，填入 Token 地址和价格点即可快捷更新。

## ⚠️ 风险提示

- **资产安全**: 请妥善保管私钥，不要泄露给任何人。建议在专门的做市商钱包中使用。
- **库存风险**: 做市商会根据报价持有不同种类的代币。如果市场发生单边剧烈波动，可能会产生库存贬值风险。
- **报价延迟**: 请确保你的报价服务（或运营手动更新）足够及时，配置的价格与市场真实价格偏差过大可能导致 Bebop 校验失败。

## 📄 许可证

MIT License
