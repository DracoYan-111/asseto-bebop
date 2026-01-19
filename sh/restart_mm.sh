#!/bin/bash

# 重启 Market Maker 脚本
# 确保完全停止旧进程并启动新进程

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

echo "========================================"
echo "🔄 重启 Bebop Market Maker"
echo "========================================"
echo ""

# 1. 停止所有相关进程
echo "1️⃣  停止旧进程..."
# 先尝试正常终止 (SIGTERM)，让程序有机会关闭 WS 连接
pkill -f "go run ." 2>/dev/null
pkill -x "bebop" 2>/dev/null
sleep 2

# 强制终止 (SIGKILL)
pkill -9 -f "go run ." 2>/dev/null
pkill -9 -x "bebop" 2>/dev/null

echo "   ⏳ 等待 WebSocket 连接完全关闭..."
sleep 10  # 增加到 10 秒，确保服务器端连接超时清理

# 2. 检查是否还有进程运行
if pgrep -fl "go run ." || pgrep -xl "bebop"; then
    echo "⚠️  警告：仍有进程在运行"
    echo "   请手动停止：pkill -9 -f 'go run .'"
    exit 1
fi

echo "   ✓ 旧进程已停止"
echo ""

# 3. 重新编译（可选）
echo "2️⃣  清理缓存并重新编译..."
go clean -cache 2>/dev/null
sleep 1
echo "   ✓ 缓存已清理"
echo ""

echo "3️⃣  编译..."
go build
echo "   ✓ 编译完成"
echo ""

# 4. 启动新进程
echo "4️⃣  启动 Market Maker..."
echo "========================================"
echo ""

go run .

