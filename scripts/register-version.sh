#!/usr/bin/env bash
# 本地调试用：调用服务版本登记接口 POST /api/v1/version-registrations。
# 仅用于非生产环境的联调与幂等/冲突测试，不用于真实发版链路。
#
# 用法（全部通过环境变量配置，与 CI 接入变量名保持一致）：
#   AI_PUB_API_KEY=<version:write 密钥> \
#   AI_PUB_PROJECT_KEY=<项目 slug> \
#   AI_PUB_SERVICE_KEY=<服务 slug> \
#   AI_PUB_VERSION=<版本号> \
#   AI_PUB_COMMIT_SHA=<可选 commit> \
#   AI_PUB_ARTIFACT_URL=<可选制品引用，OCI 需 digest> \
#   AI_PUB_IDEMPOTENCY_KEY=<可选，默认 local:<时间戳>> \
#   AI_PUB_BASE_URL=<可选，默认 http://127.0.0.1:18080> \
#   ./scripts/register-version.sh
set -euo pipefail

# 本地 compose 默认将单应用容器暴露到 127.0.0.1:18080，与 docs/local-verification.md 一致。
BASE_URL="${AI_PUB_BASE_URL:-http://127.0.0.1:18080}"
ENDPOINT="${BASE_URL}/api/v1/version-registrations"

require() {
  local name="$1"
  local val="${!1:-}"
  if [[ -z "${val}" ]]; then
    printf '缺少必填环境变量: %s\n' "${name}" >&2
    exit 2
  fi
}

require AI_PUB_API_KEY
require AI_PUB_PROJECT_KEY
require AI_PUB_SERVICE_KEY
require AI_PUB_VERSION

COMMIT_SHA="${AI_PUB_COMMIT_SHA:-}"
ARTIFACT_URL="${AI_PUB_ARTIFACT_URL:-}"
IDEMPOTENCY_KEY="${AI_PUB_IDEMPOTENCY_KEY:-local:$(date +%s)}"
PROVIDER="${AI_PUB_PROVIDER:-local}"
REF="${AI_PUB_REF:-}"

# 用 python3 生成 JSON，避免引号/换行破坏负载（不回退到字符串拼接）。
# 通过环境变量传值给 python，不做 shell 插值，杜绝引号注入。
payload="$(AI_PUB_PROJECT_KEY="${AI_PUB_PROJECT_KEY}" \
  AI_PUB_SERVICE_KEY="${AI_PUB_SERVICE_KEY}" \
  AI_PUB_VERSION="${AI_PUB_VERSION}" \
  AI_PUB_COMMIT_SHA="${COMMIT_SHA}" \
  AI_PUB_ARTIFACT_URL="${ARTIFACT_URL}" \
  AI_PUB_PROVIDER="${PROVIDER}" \
  AI_PUB_REF="${REF}" \
  python3 - <<'PY'
import json
import os

def env(key):
    return os.environ.get(key, "")

payload = {
    "project_key": env("AI_PUB_PROJECT_KEY"),
    "service_key": env("AI_PUB_SERVICE_KEY"),
    "version": env("AI_PUB_VERSION"),
    "commit_sha": env("AI_PUB_COMMIT_SHA"),
    "artifact_url": env("AI_PUB_ARTIFACT_URL"),
    "metadata": {
        "provider": env("AI_PUB_PROVIDER"),
        "ref": env("AI_PUB_REF"),
    },
}
# 去掉值为空的键，保持负载干净。
payload = {k: v for k, v in payload.items() if v != ""}
payload["metadata"] = {k: v for k, v in payload["metadata"].items() if v != ""}
print(json.dumps(payload, ensure_ascii=False))
PY
)"

printf '[register] POST %s\n' "${ENDPOINT}"
printf '[register] Idempotency-Key: %s\n' "${IDEMPOTENCY_KEY}"
printf '[register] payload: %s\n' "${payload}"

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

http_code="$(curl -sS -o "${tmp}" -w '%{http_code}' \
  -X POST "${ENDPOINT}" \
  -H "Authorization: Bearer ${AI_PUB_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${IDEMPOTENCY_KEY}" \
  --data "${payload}" || true)"

# 格式化输出响应体，便于阅读 data / error 结构。
body="$(python3 - "${tmp}" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as f:
    raw = f.read()
try:
    print(json.dumps(json.loads(raw), ensure_ascii=False, indent=2))
except Exception:
    print(raw)
PY
)"

printf '[register] HTTP %s\n' "${http_code}"
printf '%s\n' "${body}"

if [[ "${http_code}" -ge 200 && "${http_code}" -lt 300 ]]; then
  exit 0
fi
exit 1
