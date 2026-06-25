#!/usr/bin/env bash
# 后端 OCI 镜像部署脚本（deploy-backend-image.sh）
#
# 由发布系统 SSH 执行器以环境变量调用，不接收位置参数。
# 注入变量见 docs/service-version-registration-and-backend-oci-deploy-design.md 第 5.1 节。
#
# 部署目标须配置 artifact_type=oci_image；预检已保证 AI_PUB_ARTIFACT_URL 为
# 完整不可变 digest 引用（repo@sha256:<digest>），本脚本直接使用，不重建镜像地址。

set -euo pipefail

# ===== 输入校验 =====
: "${AI_PUB_ARTIFACT_URL:?AI_PUB_ARTIFACT_URL 必填，须为完整 OCI digest 引用}"
: "${APP_SERVICE_NAME:?APP_SERVICE_NAME 必填，Docker Compose 服务名与容器名}"
: "${APP_DEPLOY_DIR:?APP_DEPLOY_DIR 必填，每服务部署目录}"

# 系统注入的可追溯变量（可选展示，缺失不阻断）。
AI_PUB_VERSION="${AI_PUB_VERSION:-}"
AI_PUB_COMMIT_SHA="${AI_PUB_COMMIT_SHA:-}"
AI_PUB_RELEASE_ID="${AI_PUB_RELEASE_ID:-}"
AI_PUB_DEPLOY_ID="${AI_PUB_DEPLOY_ID:-}"

# 可选的部署后健康检查命令。
APP_HEALTHCHECK_CMD="${APP_HEALTHCHECK_CMD:-}"
# 可选的服务器受保护环境文件，供 docker compose --env-file 加载凭据。
APP_ENV_FILE="${APP_ENV_FILE:-}"

COMPOSE_FILE="${APP_DEPLOY_DIR}/docker-compose.yml"

# 校验 artifact_url 为 digest 引用，避免误用可覆盖的 tag。
if ! printf '%s' "${AI_PUB_ARTIFACT_URL}" | grep -Eq '^[^@]+@sha256:[0-9a-fA-F]{64}$'; then
  echo "[deploy] artifact_url 不是 OCI digest 引用: ${AI_PUB_ARTIFACT_URL}" >&2
  exit 2
fi

echo "[deploy] 服务=${APP_SERVICE_NAME} 版本=${AI_PUB_VERSION} commit=${AI_PUB_COMMIT_SHA:0:8} release=${AI_PUB_RELEASE_ID} deploy=${AI_PUB_DEPLOY_ID}"
echo "[deploy] 镜像=${AI_PUB_ARTIFACT_URL}"

# ===== 写入 Compose 配置 =====
# 直接以 digest 引用写入镜像字段，不根据服务名与 tag 重建地址。
mkdir -p "${APP_DEPLOY_DIR}"
cat > "${COMPOSE_FILE}" <<EOF
services:
  ${APP_SERVICE_NAME}:
    image: ${AI_PUB_ARTIFACT_URL}
    container_name: ${APP_SERVICE_NAME}
    restart: unless-stopped
EOF

COMPOSE_ARGS=(--file "${COMPOSE_FILE}")
if [[ -n "${APP_ENV_FILE}" ]]; then
  COMPOSE_ARGS+=(--env-file "${APP_ENV_FILE}")
fi

# ===== 拉取并启动 =====
echo "[deploy] 拉取镜像"
docker compose "${COMPOSE_ARGS[@]}" pull

echo "[deploy] 启动容器"
docker compose "${COMPOSE_ARGS[@]}" up -d

# ===== 容器存活检查 =====
if ! docker inspect --format '{{.State.Running}}' "${APP_SERVICE_NAME}" | grep -q true; then
  echo "[deploy] 容器未运行: ${APP_SERVICE_NAME}" >&2
  exit 3
fi

# ===== 健康检查 =====
if [[ -n "${APP_HEALTHCHECK_CMD}" ]]; then
  echo "[deploy] 执行健康检查"
  if ! eval "${APP_HEALTHCHECK_CMD}"; then
    echo "[deploy] 健康检查失败" >&2
    exit 4
  fi
fi

# ===== 执行摘要（不含凭据）=====
echo "[deploy] 完成: 服务=${APP_SERVICE_NAME} 版本=${AI_PUB_VERSION} commit=${AI_PUB_COMMIT_SHA:0:8} release=${AI_PUB_RELEASE_ID} deploy=${AI_PUB_DEPLOY_ID} 镜像仓库=${AI_PUB_ARTIFACT_URL%@*}"
