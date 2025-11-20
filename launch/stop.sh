#!/bin/bash

# AgentEarthStat 停止服务脚本


source "./config.sh"
SCRIPT_DIR="$BIN_DIR"

echo "========================================"
echo "   AgentEarthStat 停止服务"
echo "========================================"

# 检查PID文件是否存在
if [ ! -f "$PID_FILE" ]; then
    echo "PID文件不存在: $PID_FILE"
    echo "服务可能未启动或PID文件丢失"
    exit 1
fi

# 读取PID
PID=$(cat "$PID_FILE")
if [ -z "$PID" ]; then
    echo "PID文件为空: $PID_FILE"
    exit 1
fi

echo "从PID文件读取到进程ID: $PID"

# 检查进程是否存在
if ! kill -0 "$PID" 2>/dev/null; then
    echo "进程 $PID 不存在，可能已经停止"
    rm -f "$PID_FILE"
    echo "已清理PID文件"
    exit 0
fi

echo "正在停止服务进程 $PID..."

# 优雅停止
kill -TERM "$PID"

# 等待进程优雅退出（最多60秒）
echo "等待进程优雅退出（包括清理子进程）..."
echo "最长等待时间: 60秒"
for i in {1..60}; do
    if ! kill -0 "$PID" 2>/dev/null; then
        echo
        echo "✓ 进程已优雅停止（耗时: ${i}秒）"
        rm -f "$PID_FILE"
        echo "✓ 已清理PID文件"
        echo "========================================"
        echo "停止服务完成"
        exit 0
    fi
    # 每5秒显示一次进度
    if [ $((i % 5)) -eq 0 ]; then
        echo "  等待中... (${i}/60秒)"
    fi
    sleep 1
done

echo
echo "⚠ 进程在60秒内未能优雅退出，准备强制停止..."

# 强制停止
kill -KILL "$PID"

# 再次检查
sleep 1
if ! kill -0 "$PID" 2>/dev/null; then
    echo "进程已强制停止"
    rm -f "$PID_FILE"
    echo "已清理PID文件"
else
    echo "警告: 无法停止进程 $PID"
    exit 1
fi

echo "========================================"
echo "停止服务完成"
