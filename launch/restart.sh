#!/bin/bash

# AgentEarthStat 重启服务脚本

source "./config.sh"
SCRIPT_DIR="$BIN_DIR"

echo "========================================"
echo "   AgentEarthStat 重启服务"
echo "========================================"
echo "使用环境: $ENV"
echo





# 停止服务
echo "正在停止现有服务..."
"$SCRIPT_DIR/stop.sh"

echo
echo "等待服务完全停止..."
sleep 2


echo "正在重启服务..."
"$SCRIPT_DIR/start.sh"