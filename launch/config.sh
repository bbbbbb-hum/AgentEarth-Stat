#!/bin/bash
# config.sh - 公共配置常量

# 服务目录名称
APP_DIR="AgentEarthStat"
# 服务名称
SERVICE_NAME="agent-earth-stat"
# 文件路径
BIN_DIR="/opt/xlapps/${APP_DIR}/bin"
LOG_DIR="/opt/xllogs/${APP_DIR}"
TMP_DIR="/tmp/xltmp/${APP_DIR}"
CONFIG_DIR="/opt/xlconfigs/${APP_DIR}"
# 创建目录
mkdir -p "$BIN_DIR"
mkdir -p "$LOG_DIR"
mkdir -p "$TMP_DIR"
mkdir -p "$CONFIG_DIR"

EXEC_FILE="${BIN_DIR}/${SERVICE_NAME}"

LOG_FILE_NAME="${SERVICE_NAME}-$(date +%Y-%m-%d).log"
LOG_FILE="${LOG_DIR}/${LOG_FILE_NAME}"

# 服务进程ID记录文件
PID_FILE_NAME="${SERVICCONFIG_DIRE_NAME}.pid"
PID_FILE="${CONFIG_DIR}/${PID_FILE_NAME}"



# 从环境配置文件读取环境参数
ENV_CONFIG_FILE="/opt/xlconfigs/Env/env.conf"

# 检查环境配置文件是否存在
if [ ! -f "${ENV_CONFIG_FILE}" ]; then
    echo "错误: 环境配置文件不存在: $ENV_CONFIG_FILE"
    exit 1
fi

# 读取环境配置文件中的 ENV 和 MACHINE_NAME
source "${ENV_CONFIG_FILE}"

# 检查 ENV 是否成功读取
if [ -z "$ENV" ]; then
    echo "错误: 未能从配置文件中读取 ENV 变量"
    echo "配置文件: ${ENV_CONFIG_FILE}"
    exit 1
fi

# 验证 ENV 值是否合法（仅允许 test 或 prod）
if [ "$ENV" != "test" ] && [ "$ENV" != "prod" ]; then
    echo "错误: ENV 值不合法: ${ENV}"
    echo "仅允许以下值: test | prod"
    exit 1
fi

# 输出读取到的环境信息（用于调试）
echo "✓ 从配置文件读取环境信息:"
echo "  配置文件: $ENV_CONFIG_FILE"
echo "  ENV: ${ENV}"
if [ -n "${MACHINE_NAME}" ]; then
    echo "  MACHINE_NAME: ${MACHINE_NAME}"
fi
echo
# 配置文件位置
CONFIG_FILE ="${CONFIG_DIR}/cron_${ENV}.yaml"
# 检查配置文件是否存在
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "错误: 环境配置文件不存在: $CONFIG_FILE"
    exit 1
fi