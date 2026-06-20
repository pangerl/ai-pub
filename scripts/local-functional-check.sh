#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
API_URL="${BASE_URL}/api/v1"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

log() {
  printf '[local-check] %s\n' "$*"
}

api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local out="$4"
  local code
  if [[ -z "${body}" ]]; then
    code="$(curl -sS -o "${out}" -w '%{http_code}' -X "${method}" "${API_URL}${path}")"
  else
    code="$(curl -sS -o "${out}" -w '%{http_code}' -X "${method}" "${API_URL}${path}" -H 'Content-Type: application/json' -d "${body}")"
  fi
  if [[ "${code}" -lt 200 || "${code}" -ge 300 ]]; then
    printf 'request failed: %s %s -> HTTP %s\n' "${method}" "${path}" "${code}" >&2
    cat "${out}" >&2
    exit 1
  fi
}

json_get() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    current = json.load(f)
for part in sys.argv[2].split("."):
    if not part:
        continue
    if isinstance(current, list):
        current = current[int(part)]
    else:
        current = current[part]
print("" if current is None else current)
PY
}

json_len() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    current = json.load(f)
for part in sys.argv[2].split("."):
    if not part:
        continue
    if isinstance(current, list):
        current = current[int(part)]
    else:
        current = current[part]
print(len(current))
PY
}

json_count_by_field() {
  python3 - "$1" "$2" "$3" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    items = json.load(f)["data"]
field, value = sys.argv[2], sys.argv[3]
print(sum(1 for item in items if str(item.get(field)) == value))
PY
}

find_by_field() {
  python3 - "$1" "$2" "$3" "$4" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    items = json.load(f)["data"]
field, value, output = sys.argv[2], sys.argv[3], sys.argv[4]
for item in items:
    if str(item.get(field)) == value:
        print(item.get(output, ""))
        break
PY
}

wait_release() {
  local release_id="$1"
  local want="$2"
  local out="${TMP_DIR}/release-${release_id}.json"
  for _ in $(seq 1 30); do
    api GET "/release-requests/${release_id}" "" "${out}"
    local status
    status="$(json_get "${out}" data.status)"
    if [[ "${status}" == "${want}" ]]; then
      log "release ${release_id} reached ${want}"
      return
    fi
    sleep 1
  done
  printf 'release %s did not reach %s\n' "${release_id}" "${want}" >&2
  cat "${out}" >&2
  exit 1
}

assert_non_empty() {
  if [[ -z "$2" ]]; then
    printf 'missing %s\n' "$1" >&2
    exit 1
  fi
}

health_file="${TMP_DIR}/health.json"
curl -sS -o "${health_file}" "${BASE_URL}/healthz"
if [[ "$(json_get "${health_file}" data.status)" != "ok" ]]; then
  cat "${health_file}" >&2
  exit 1
fi
log "backend is healthy"

suffix="$(date +%s)"

project_file="${TMP_DIR}/project.json"
api POST /projects "{\"name\":\"本地验证项目\",\"slug\":\"local-check-${suffix}\"}" "${project_file}"
project_id="$(json_get "${project_file}" data.id)"
assert_non_empty project_id "${project_id}"

service_file="${TMP_DIR}/service.json"
api POST /services "{\"project_id\":\"${project_id}\",\"name\":\"本地验证服务\",\"slug\":\"local-api-${suffix}\"}" "${service_file}"
service_id="$(json_get "${service_file}" data.id)"
assert_non_empty service_id "${service_id}"

version1_file="${TMP_DIR}/version1.json"
api POST "/services/${service_id}/versions" "{\"version\":\"v1.${suffix}\",\"source\":\"api\"}" "${version1_file}"
version1_id="$(json_get "${version1_file}" data.id)"
assert_non_empty version1_id "${version1_id}"

version2_file="${TMP_DIR}/version2.json"
api POST "/services/${service_id}/versions" "{\"version\":\"v2.${suffix}\",\"source\":\"api\"}" "${version2_file}"
version2_id="$(json_get "${version2_file}" data.id)"
assert_non_empty version2_id "${version2_id}"

env_file="${TMP_DIR}/environment.json"
api POST /environments "{\"name\":\"本地验证环境\",\"slug\":\"local-${suffix}\",\"is_production\":false}" "${env_file}"
environment_id="$(json_get "${env_file}" data.id)"
assert_non_empty environment_id "${environment_id}"

server_file="${TMP_DIR}/server.json"
api POST /servers "{\"name\":\"mock-local-${suffix}\",\"host\":\"127.0.0.1\",\"username\":\"deploy\",\"auth_type\":\"none\",\"enabled\":true}" "${server_file}"
server_id="$(json_get "${server_file}" data.id)"
assert_non_empty server_id "${server_id}"

server2_file="${TMP_DIR}/server2.json"
api POST /servers "{\"name\":\"mock-local-2-${suffix}\",\"host\":\"127.0.0.2\",\"username\":\"deploy\",\"auth_type\":\"none\",\"enabled\":true}" "${server2_file}"
server2_id="$(json_get "${server2_file}" data.id)"
assert_non_empty server2_id "${server2_id}"

server3_file="${TMP_DIR}/server3.json"
api POST /servers "{\"name\":\"mock-local-3-${suffix}\",\"host\":\"127.0.0.3\",\"username\":\"deploy\",\"auth_type\":\"none\",\"enabled\":true}" "${server3_file}"
server3_id="$(json_get "${server3_file}" data.id)"
assert_non_empty server3_id "${server3_id}"

partial_fail_server_id="$(printf '%s\n%s\n%s\n' "${server_id}" "${server2_id}" "${server3_id}" | sort | sed -n '2p')"
assert_non_empty partial_fail_server_id "${partial_fail_server_id}"

server_group_file="${TMP_DIR}/server-group.json"
api POST /server-groups "{\"name\":\"mock-group-${suffix}\",\"server_ids\":[\"${server_id}\",\"${server2_id}\",\"${server3_id}\"]}" "${server_group_file}"
server_group_id="$(json_get "${server_group_file}" data.id)"
assert_non_empty server_group_id "${server_group_id}"

success_target_file="${TMP_DIR}/success-target.json"
api POST /deployment-targets "{\"service_id\":\"${service_id}\",\"environment_id\":\"${environment_id}\",\"executor_type\":\"mock\",\"target_type\":\"server\",\"target_ref_id\":\"${server_id}\",\"timeout_seconds\":60,\"env_vars\":\"{}\",\"enabled\":true}" "${success_target_file}"
success_target_id="$(json_get "${success_target_file}" data.id)"
assert_non_empty success_target_id "${success_target_id}"

group_target_file="${TMP_DIR}/group-target.json"
api POST /deployment-targets "{\"service_id\":\"${service_id}\",\"environment_id\":\"${environment_id}\",\"executor_type\":\"mock\",\"target_type\":\"server_group\",\"target_ref_id\":\"${server_group_id}\",\"timeout_seconds\":60,\"env_vars\":\"{}\",\"enabled\":true}" "${group_target_file}"
group_target_id="$(json_get "${group_target_file}" data.id)"
assert_non_empty group_target_id "${group_target_id}"

partial_target_file="${TMP_DIR}/partial-target.json"
api POST /deployment-targets "{\"service_id\":\"${service_id}\",\"environment_id\":\"${environment_id}\",\"executor_type\":\"mock\",\"target_type\":\"server_group\",\"target_ref_id\":\"${server_group_id}\",\"timeout_seconds\":60,\"env_vars\":\"{\\\"MOCK_FAIL_SERVER_ID\\\":\\\"${partial_fail_server_id}\\\"}\",\"enabled\":true}" "${partial_target_file}"
partial_target_id="$(json_get "${partial_target_file}" data.id)"
assert_non_empty partial_target_id "${partial_target_id}"

failure_target_file="${TMP_DIR}/failure-target.json"
api POST /deployment-targets "{\"service_id\":\"${service_id}\",\"environment_id\":\"${environment_id}\",\"executor_type\":\"mock\",\"target_type\":\"server\",\"target_ref_id\":\"${server_id}\",\"timeout_seconds\":60,\"env_vars\":\"{\\\"MOCK_FAIL\\\":\\\"1\\\"}\",\"enabled\":true}" "${failure_target_file}"
failure_target_id="$(json_get "${failure_target_file}" data.id)"
assert_non_empty failure_target_id "${failure_target_id}"

user_file="${TMP_DIR}/user.json"
api POST /users "{\"username\":\"local-${suffix}\",\"display_name\":\"Local Check\",\"role\":\"employee\",\"enabled\":true}" "${user_file}"
user_id="$(json_get "${user_file}" data.id)"
assert_non_empty user_id "${user_id}"

create_release() {
  local version_id="$1"
  local target_id="$2"
  local key="$3"
  local out="$4"
  api POST /release-requests "{\"service_id\":\"${service_id}\",\"environment_id\":\"${environment_id}\",\"service_version_id\":\"${version_id}\",\"deployment_target_id\":\"${target_id}\",\"created_by_type\":\"user\",\"created_by_id\":\"${user_id}\",\"idempotency_key\":\"${key}\"}" "${out}"
  json_get "${out}" data.release.id
}

confirm_release() {
  local release_id="$1"
  local out="${TMP_DIR}/confirm-${release_id}.json"
  api POST "/release-requests/${release_id}/confirm" "{\"user_id\":\"${user_id}\"}" "${out}"
}

reject_release() {
  local release_id="$1"
  local out="${TMP_DIR}/reject-${release_id}.json"
  api POST "/release-requests/${release_id}/reject" "{\"user_id\":\"${user_id}\",\"reason\":\"local check reject\"}" "${out}"
}

cancel_release() {
  local release_id="$1"
  local out="${TMP_DIR}/cancel-${release_id}.json"
  api POST "/release-requests/${release_id}/cancel" "{\"user_id\":\"${user_id}\"}" "${out}"
}

reject_file="${TMP_DIR}/reject-release.json"
reject_id="$(create_release "${version1_id}" "${success_target_id}" "local-reject-${suffix}" "${reject_file}")"
reject_release "${reject_id}"
wait_release "${reject_id}" rejected

cancel_file="${TMP_DIR}/cancel-release.json"
cancel_id="$(create_release "${version1_id}" "${success_target_id}" "local-cancel-${suffix}" "${cancel_file}")"
cancel_release "${cancel_id}"
wait_release "${cancel_id}" cancelled

release1_file="${TMP_DIR}/release1.json"
release1_id="$(create_release "${version1_id}" "${success_target_id}" "local-success-${suffix}" "${release1_file}")"
confirm_release "${release1_id}"
wait_release "${release1_id}" success

group_release_file="${TMP_DIR}/group-release.json"
group_release_id="$(create_release "${version1_id}" "${group_target_id}" "local-group-success-${suffix}" "${group_release_file}")"
confirm_release "${group_release_id}"
wait_release "${group_release_id}" success

partial_release_file="${TMP_DIR}/partial-release.json"
partial_release_id="$(create_release "${version1_id}" "${partial_target_id}" "local-partial-${suffix}" "${partial_release_file}")"
confirm_release "${partial_release_id}"
wait_release "${partial_release_id}" failed

partial_deploys_file="${TMP_DIR}/partial-deploys.json"
api GET /deploy-records "" "${partial_deploys_file}"
partial_deploy_id="$(find_by_field "${partial_deploys_file}" release_request_id "${partial_release_id}" id)"
assert_non_empty partial_deploy_id "${partial_deploy_id}"

partial_deploy_file="${TMP_DIR}/partial-deploy.json"
api GET "/deploy-records/${partial_deploy_id}" "" "${partial_deploy_file}"
if [[ "$(json_get "${partial_deploy_file}" data.status)" != "partial" ]]; then
  cat "${partial_deploy_file}" >&2
  exit 1
fi
if [[ "$(json_get "${partial_deploy_file}" data.success_servers)" != "1" || "$(json_get "${partial_deploy_file}" data.failed_servers)" != "1" || "$(json_get "${partial_deploy_file}" data.skipped_servers)" != "1" ]]; then
  cat "${partial_deploy_file}" >&2
  exit 1
fi

partial_logs_file="${TMP_DIR}/partial-logs.json"
api GET "/deploy-records/${partial_deploy_id}/server-logs" "" "${partial_logs_file}"
if [[ "$(json_count_by_field "${partial_logs_file}" status skipped)" != "1" ]]; then
  cat "${partial_logs_file}" >&2
  exit 1
fi

release2_file="${TMP_DIR}/release2.json"
release2_id="$(create_release "${version2_id}" "${success_target_id}" "local-success-v2-${suffix}" "${release2_file}")"
confirm_release "${release2_id}"
wait_release "${release2_id}" success

failure_release_file="${TMP_DIR}/failure-release.json"
failure_release_id="$(create_release "${version2_id}" "${failure_target_id}" "local-failure-${suffix}" "${failure_release_file}")"
confirm_release "${failure_release_id}"
wait_release "${failure_release_id}" failed

candidates_file="${TMP_DIR}/rollback-candidates.json"
api GET "/release-requests/${release2_id}/rollback-candidates" "" "${candidates_file}"
if [[ "$(json_len "${candidates_file}" data)" -lt 1 ]]; then
  cat "${candidates_file}" >&2
  exit 1
fi
log "rollback candidates are available"

rollback_file="${TMP_DIR}/rollback.json"
api POST "/release-requests/${release2_id}/rollback" "{\"service_version_id\":\"${version1_id}\",\"created_by_type\":\"user\",\"created_by_id\":\"${user_id}\",\"idempotency_key\":\"local-rollback-${suffix}\"}" "${rollback_file}"
rollback_id="$(json_get "${rollback_file}" data.release.id)"
confirm_release "${rollback_id}"
wait_release "${rollback_id}" success

deploys_file="${TMP_DIR}/deploys.json"
api GET /deploy-records "" "${deploys_file}"
deploy_id="$(find_by_field "${deploys_file}" release_request_id "${rollback_id}" id)"
assert_non_empty deploy_id "${deploy_id}"

logs_file="${TMP_DIR}/logs.json"
api GET "/deploy-records/${deploy_id}/server-logs" "" "${logs_file}"
if [[ "$(json_len "${logs_file}" data)" -lt 1 ]]; then
  cat "${logs_file}" >&2
  exit 1
fi

events_file="${TMP_DIR}/events.json"
api GET "/release-requests/${rollback_id}/events" "" "${events_file}"
if [[ "$(json_len "${events_file}" data)" -lt 4 ]]; then
  cat "${events_file}" >&2
  exit 1
fi

log "success release: ${release1_id}"
log "server group release: ${group_release_id}"
log "partial release: ${partial_release_id}"
log "failure release: ${failure_release_id}"
log "rollback release: ${rollback_id}"
log "rejected release: ${reject_id}"
log "cancelled release: ${cancel_id}"
log "local functional check passed"
