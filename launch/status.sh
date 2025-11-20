#!/bin/bash

# AgentEarthStat 服务状态检查脚本

source "./config.sh"
SCRIPT_DIR="$BIN_DIR"

echo "========================================"
echo "   AgentEarthStat 服务状态"
echo "========================================"
echo "使用环境: $ENV"
echo

# 1. 使用命令查找进程
echo "正在检查服务进程..."
RUNNING_PIDS=$(pgrep -f "$SERVICE_NAME" 2>/dev/null)

if [ -z "$RUNNING_PIDS" ]; then
    echo "✗ 服务未运行 (未找到进程)"
    
    # 检查 PID 文件是否存在但进程不在
    if [ -f "$PID_FILE" ]; then
        echo "  ⚠ PID 文件存在但进程不在运行，正在清理..."
        rm -f "$PID_FILE"
    fi
    
    echo "  使用 $SCRIPT_DIR/start.sh 启动服务"
    echo "========================================"
    exit 0
fi

# 可能有多个进程
PID_COUNT=$(echo "$RUNNING_PIDS" | wc -w)
if [ "$PID_COUNT" -gt 1 ]; then
    echo "⚠ 警告: 发现 $PID_COUNT 个服务进程"
    echo "  进程 PIDs: $RUNNING_PIDS"
else
    echo "✓ 服务正在运行"
    echo "  进程 PID: $RUNNING_PIDS"
fi

# 检查 PID 文件是否与实际进程匹配
if [ -f "$PID_FILE" ]; then
    FILE_PID=$(cat "$PID_FILE")
    if echo "$RUNNING_PIDS" | grep -q "^${FILE_PID}$"; then
        echo "  ✓ PID 文件与实际进程匹配"
    else
        echo "  ⚠ PID 文件 ($FILE_PID) 与实际进程 ($RUNNING_PIDS) 不匹配"
    fi
fi

echo

# 2. 显示进程详细信息
echo "进程详细信息:"
echo "----------------------------------------"
for PID in $RUNNING_PIDS; do
    if ps -p $PID > /dev/null 2>&1; then
        echo "PID: $PID"
        ps -p $PID -o pid,ppid,user,%cpu,%mem,etime,cmd --no-headers 2>/dev/null | sed 's/^/  /' || \
        ps -p $PID -o pid,ppid,user,pcpu,pmem,etime,command 2>/dev/null | sed 's/^/  /'
        echo
    fi
done
echo "----------------------------------------"
echo

# 3. 检查日志目录和文件
echo "日志信息:"
echo "----------------------------------------"
echo "  日志目录: $LOG_DIR"

# 列出日志目录中的所有日志文件
if [ -d "$LOG_DIR" ]; then
    LOG_FILES=$(ls -t "$LOG_DIR"/*.log 2>/dev/null)
    if [ -n "$LOG_FILES" ]; then
        echo "✓ 日志文件列表 (按修改时间倒序):"
        for log_file in $LOG_FILES; do
            SIZE=$(du -h "$log_file" | cut -f1)
            MTIME=$(stat -c %y "$log_file" 2>/dev/null | cut -d'.' -f1 || stat -f "%Sm" -t "%Y-%m-%d %H:%M:%S" "$log_file" 2>/dev/null)
            echo "    $(basename "$log_file") - $SIZE - $MTIME"
        done
        
        # 显示最新日志文件的最后几行
        LATEST_LOG=$(echo "$LOG_FILES" | head -1)
        if [ -f "$LATEST_LOG" ]; then
            echo
            echo "  最新日志文件: $(basename "$LATEST_LOG")"
            echo "  最近日志 (最后 5 行):"
            echo "  ........................................"
            tail -5 "$LATEST_LOG" | sed 's/^/  /'
            echo "  ........................................"
        fi
    else
        echo "⚠ 日志目录中没有日志文件"
    fi
else
    echo "✗ 日志目录不存在: $LOG_DIR"
fi
echo "----------------------------------------"

echo
echo "========================================"
