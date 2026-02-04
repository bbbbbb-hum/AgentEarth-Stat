# 使用 Ubuntu 基础镜像（原生 glibc 支持，避免符号依赖问题）
ARG DOCKER_REP_PATH=""
FROM ${DOCKER_REP_PATH}ubuntu:22.04

# 安装并更新 CA 证书（解决 TLS 证书验证问题）
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/xlapps/AgentEarthStat/bin
RUN mkdir -p /opt/xltmp/AgentEarthStat/tmp
RUN mkdir -p /opt/xldata/AgentEarthStat/data

# 下面的都要mount进来
# /opt/xlconfigs/AgentEarthStat目录下要有cron.yaml文件

# 设置工作目录
WORKDIR /opt/xlapps/AgentEarthStat/bin

# 复制可执行文件
COPY bin/agent-earth-stat /opt/xlapps/AgentEarthStat/bin/agent-earth-stat

# 运行可执行文件
CMD ["/opt/xlapps/AgentEarthStat/bin/agent-earth-stat", "-f", "/opt/xlconfigs/AgentEarthStat/cron.yaml"]