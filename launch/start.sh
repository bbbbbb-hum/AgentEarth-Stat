#!/bin/bash

# AgentEarthStat 启动脚本

# 加载配置
source "./config.sh"
SCRIPT_DIR="$BIN_DIR"

echo "========================================"
echo "   AgentEarthStat  启动脚本"
echo "========================================"
echo "使用环境: $ENV"
echo



# 检查服务是否已经运行
echo "正在检查服务状态..."

# 1. 检查是否有同名进程在运行
RUNNING_PIDS=$(pgrep -f "$SERVICE_NAME" 2>/dev/null)
if [ -n "$RUNNING_PIDS" ]; then
    echo "警告: 发现同名服务进程正在运行"
    echo "运行中的进程 PID: $RUNNING_PIDS"
    echo "如需重启服务，请使用: ./restart.sh"
    exit 1
fi

# 2. 清理旧的 PID 文件（如果存在）
if [ -f "$PID_FILE" ]; then
    echo "清理旧的 PID 文件..."
    rm -f "$PID_FILE"
fi

echo "✓ 服务状态检查通过"
echo

# 启动服务
echo "正在启动服务..."
echo "环境: $ENV"
echo "配置文件: $CONFIG_FILE"
echo "可执行文件: $EXEC_FILE"
echo "日志目录: $LOG_DIR"
echo "启动时间: $(date)"
echo "========================================"
echo


# 后台启动服务并保存PID
setsid "$EXEC_FILE" -f "$CONFIG_FILE" > "${LOG_FILE}" 2>&1 &
PID=$!

# 保存PID到文件
echo $PID > "$PID_FILE"

echo "服务进程已启动 (PID: $PID)"
echo

echo "========================================"
echo "✓ 服务启动成功！"
echo "========================================"
echo "  PID: $PID"
echo "  PID文件: $PID_FILE"
echo "  配置文件: $CONFIG_FILE"
echo "  日志目录: $LOG_DIR"
echo

# 等待日志写入并刷新缓冲区
sleep 1
sync 2>/dev/null || true
# 检查启动日志是否有错误
if [ -s "${LOG_FILE}" ]; then
    echo "⚠ 注意: 启动时有以下警告或错误信息:"
    tail -n 20 "${LOG_FILE}"
    echo
fi
echo "使用以下命令管理服务:"
echo "  停止服务: ./stop.sh"
echo "  检查状态: ./status.sh"
echo "  重启服务: ./restart.sh"
echo "========================================"

