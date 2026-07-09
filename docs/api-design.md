# API 设计

## 1. 目标

第一版 API 使用 REST 风格和 `/api/v1` 前缀，为 Web 前端和 CI/CD 提供统一接口基础。

## 2. 非目标

- 不设计运行中紧急停止接口。
- 不设计复杂批量编排接口。
- 不兼容旧系统接口。

## 3. 通用约定

### 3.1 路径前缀

```text
/api/v1
```

下文接口路径均省略 `/api/v1` 前缀；例如 `/projects` 的完整路径是 `/api/v1/projects`。

### 3.2 响应结构

成功：

```json
{
  "data": {},
  "request_id": "req_xxx"
}
```

失败：

```json
{
  "error": {
    "code": "invalid_state",
    "message": "当前发布单状态不允许确认",
    "details": {}
  },
  "request_id": "req_xxx"
}
```

### 3.3 分页

列表接口支持：

- `limit`
- `cursor`
- `sort`

长期增长资源必须支持稳定排序：

- 发布单
- 发布记录
- 执行目标日志
- 审计事件

### 3.4 幂等键

写接口可通过 header 或字段传入幂等键：

```text
Idempotency-Key: xxx
```

要求：

- 创建发布单必须支持幂等键。
- 自动化调用确认、取消、创建回滚等状态变更接口时，也应支持幂等键或等价重复提交保护。
- 幂等键命中时返回首次请求结果。
- 外部版本登记必须使用 `Idempotency-Key: {provider}:{run_id}`；幂等范围为服务。相同 key 与相同请求指纹返回首次版本，不同指纹返回 `409 idempotency_conflict`。

## 4. 鉴权与调用身份

调用身份：

- `user`
- `api_key`
- `ai_agent`
- `system`

认证方式：

- Web 用户：登录会话或 JWT。
- 自动化调用：API Key。

API Key scope：

| scope | 说明 |
|------|------|
| `release:read` | 读取发布单、发布记录、日志 |
| `release:create` | 创建发布单和 preflight |
| `release:confirm` | 确认、驳回、取消发布单 |
| `release:rollback` | 创建回滚发布单 |
| `deploy:read` | 读取执行状态和目标日志 |
| `inventory:read` | 读取项目、服务、版本、环境、服务器、K8s 集群和部署目标 |
| `version:write` | 通过统一外部接口登记服务版本 |
| `admin:write` | 管理基础配置和高风险配置 |

约束：

- API Key 即使具备 `release:confirm`，也不能绕过生产管理员确认。
- scope 必须是上表中的唯一值；不支持 `*` 或未知 scope。
- 只有管理员会话可以创建或授予 `admin:write`；普通用户更新自己的 Key 时只能缩小 scope 集合。
- 所有触发真实发布的 API 必须进入统一发布流程。

## 5. 错误码

| code | 说明 |
|------|------|
| `unauthorized` | 未认证 |
| `forbidden` | 无权限 |
| `not_found` | 资源不存在 |
| `invalid_argument` | 参数错误 |
| `invalid_state` | 状态不允许 |
| `preflight_blocked` | preflight 阻断 |
| `conflict` | 幂等或唯一约束冲突 |
| `executor_error` | 执行器错误 |
| `cluster_not_available` | K8s 集群不存在、禁用或凭据不可用 |
| `namespace_not_found` | K8s 命名空间不存在 |
| `deployment_not_found` | K8s Deployment 不存在 |
| `container_not_found` | K8s 容器名不存在 |
| `image_invalid` | OCI digest 镜像格式不合法 |
| `permission_denied` | 执行凭据权限不足 |
| `rollout_timeout` | K8s rollout 超时 |
| `rollout_failed` | K8s rollout 失败 |
| `internal_error` | 系统错误 |

## 6. 认证、用户和 API Key

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/auth/login` | 登录 |
| `GET` | `/auth/me` | 当前用户 |
| `POST` | `/auth/logout` | 退出登录 |
| `GET` | `/users` | 用户列表；管理员返回完整字段，普通用户仅返回用户目录（id、username、display_name） |
| `POST` | `/users` | 创建用户，管理员 |
| `PATCH` | `/users/{id}` | 更新用户，管理员 |
| `DELETE` | `/users/{id}` | 删除用户，管理员会话；同步删除该用户归属的 API Key，历史发布记录保留原用户 ID |
| `GET` | `/api-keys` | API Key 列表；普通用户仅返回自己的 Key，管理员返回全部 |
| `POST` | `/api-keys` | 创建 API Key；普通用户的归属固定为当前用户 |
| `PATCH` | `/api-keys/{id}` | 禁用或更新；普通用户只能操作自己的 Key |
| `DELETE` | `/api-keys/{id}` | 删除；普通用户只能操作自己的 Key |

API Key 创建响应只返回一次明文。

## 7. 基础配置 API

### 7.1 项目

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/projects` | 列表 |
| `POST` | `/projects` | 创建 |
| `GET` | `/projects/{id}` | 详情 |
| `PATCH` | `/projects/{id}` | 更新 |

### 7.2 服务和版本

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/services` | 服务列表 |
| `POST` | `/services` | 创建服务 |
| `GET` | `/services/{id}` | 服务详情 |
| `PATCH` | `/services/{id}` | 更新服务 |
| `GET` | `/services/{id}/versions` | 版本列表 |
| `POST` | `/services/{id}/versions` | 注册版本 |

管理员手动登记时服务端强制写入 `source=manual` 与当前用户身份；同一服务重复版本返回 `409 version_conflict`，不覆盖原版本。

### 7.3 外部版本登记

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/version-registrations` | 外部 CI 统一登记版本，需要 `version:write` |

请求体必须包含 `project_key`、`service_key`、`version`；可选包含 `commit_sha`、`artifact_url` 与 JSON 对象 `metadata`。服务端按项目和服务 key 解析内部服务，序列化 metadata，并强制写入 `source=ci` 与 API Key 调用身份。

`artifact_url` 在登记时不按特定制品格式校验。创建发布单时由部署目标 `artifact_type` 决定：`version_only` 缺少制品仅 warning；`oci_image` 缺少制品或不匹配 `^[^@]+@sha256:[0-9a-fA-F]{64}$` 时 block。

### 7.4 环境、服务器和部署目标

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/environments` | 环境列表 |
| `POST` | `/environments` | 创建环境 |
| `PATCH` | `/environments/{id}` | 更新环境 |
| `GET` | `/servers` | 服务器列表 |
| `POST` | `/servers` | 创建服务器 |
| `PATCH` | `/servers/{id}` | 更新服务器 |
| `POST` | `/servers/test` | 校验未落库的服务器配置，不更新最近测试状态 |
| `POST` | `/servers/{id}/test` | 测试 SSH 连接 |
| `GET` | `/server-groups` | 服务器组列表 |
| `POST` | `/server-groups` | 创建服务器组 |
| `PATCH` | `/server-groups/{id}` | 更新服务器组 |
| `GET` | `/k8s-clusters` | Kubernetes 集群列表 |
| `POST` | `/k8s-clusters` | 创建 Kubernetes 集群 |
| `PATCH` | `/k8s-clusters/{id}` | 更新 Kubernetes 集群名称、凭据和启用状态 |
| `DELETE` | `/k8s-clusters/{id}` | 删除未被部署目标引用的 Kubernetes 集群 |
| `GET` | `/deployment-targets` | 部署目标列表 |
| `POST` | `/deployment-targets` | 创建部署目标 |
| `PATCH` | `/deployment-targets/{id}` | 更新部署目标 |

部署目标请求和响应包含通用字段和 executor 专属配置。`artifact_type` 本期仅支持 `version_only` 与 `oci_image`，缺省时为 `version_only`。该字段必须由管理员显式配置，不能根据 `script_path` 推断。

SSH 部署目标请求示例：

```json
{
  "service_id": "svc_1",
  "environment_id": "env_test",
  "executor_type": "ssh",
  "artifact_type": "version_only",
  "timeout_seconds": 300,
  "ssh": {
    "target_type": "server_group",
    "target_ref_id": "sg_1",
    "script_path": "/opt/deploy/deploy.sh",
    "working_dir": "/opt/app",
    "env_vars": "{}"
  }
}
```

K8s Deployment 部署目标请求示例：

```json
{
  "service_id": "svc_1",
  "environment_id": "env_test",
  "executor_type": "k8s",
  "artifact_type": "oci_image",
  "timeout_seconds": 300,
  "k8s": {
    "cluster_id": "k8s_1",
    "namespace": "default",
    "deployment_name": "order-api",
    "container_name": "app"
  }
}
```

K8s 目标只表达版本发布配置，不接收 YAML、Manifest、副本数、资源限制、环境变量、探针、调度、volume、label、annotation、Service/Ingress 等运行参数。

## 8. 发布流程 API

### 8.1 创建发布单

```text
POST /release-requests
```

约束：

- 创建发布单前，客户端应先调用 preflight。
- 创建发布单时，服务端必须基于环境发布保护和关键 preflight 规则再次校验。
- preflight 为 `block` 时不得创建真实发布单。

请求：

```json
{
  "service_id": "svc_1",
  "environment_id": "env_test",
  "service_version_id": "ver_1",
  "deployment_target_id": "target_1",
  "reason": "",
  "risk_note": "",
  "rollback_note": "",
  "metadata": {}
}
```

响应：

```json
{
  "data": {
    "id": "rel_1",
    "status": "pending_confirm",
    "next_action": "self_confirm"
  }
}
```

### 8.2 Preflight

```text
POST /release-requests/preflight
POST /release-requests/{id}/preflight
```

响应：

```json
{
  "data": {
    "result": "pass",
    "items": [
      {
        "code": "target_ready",
        "level": "pass",
        "message": "部署目标配置完整"
      }
    ]
  }
}
```

结果：

- `pass`
- `warning`
- `block`

K8s preflight 额外检查：

- 部署目标 `executor_type=k8s` 必须搭配 `artifact_type=oci_image`。
- `artifact_url` 必须是不可变 OCI digest。
- 集群存在、启用，kubeconfig 凭据存在、启用且可解密。
- 命名空间、Deployment 和容器存在。
- 当前镜像与目标镜像相同时可 warning 或 pass，不阻断用户重跑 rollout。

### 8.3 查询发布单

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/release-requests` | 列表 |
| `GET` | `/release-requests/{id}` | 详情 |

列表筛选：

- `status`
- `service_id`
- `environment_id`
- `source`
- `created_by`

### 8.4 确认、驳回、取消

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/release-requests/{id}/confirm` | 确认并入队 |
| `POST` | `/release-requests/{id}/reject` | 驳回 |
| `POST` | `/release-requests/{id}/cancel` | 取消 queued 前发布 |
| `POST` | `/release-requests/{id}/retry` | 创建重新发布单 |

约束：

- 确认时必须再次执行关键 preflight。
- 生产发布必须管理员确认。
- running 后取消返回当前运行状态，不提供紧急停止。

### 8.5 回滚

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/release-requests/{id}/rollback-candidates` | 推荐回滚版本 |
| `POST` | `/release-requests/{id}/rollback` | 创建回滚发布单 |
| `GET` | `/release-requests/{id}/events` | 发布事件 |

回滚发布单复用同一套 preflight、环境发布保护、确认、审计流程。

## 9. Agent API

Agent API 是业务 API 的薄封装，用于降低 AI 调用复杂度，不是第二套发布流程。

| 方法 | 路径 | 说明 | scope |
|------|------|------|------|
| `GET` | `/agent/services` | 服务候选，支持 `q`/`query` 和 `project_id` | `inventory:read` |
| `GET` | `/agent/environments` | 环境候选，支持 `q`/`query` | `inventory:read` |
| `GET` | `/agent/services/{id}/versions` | 服务版本候选，支持 `q`/`query` | `inventory:read` |
| `GET` | `/agent/deployment-targets` | 部署目标候选，支持 `service_id` 和 `environment_id` | `inventory:read` |
| `POST` | `/agent/release-intents/preflight` | 发布意图 preflight | `release:create` |
| `POST` | `/agent/release-requests` | Agent 创建发布单 | `release:create` |
| `POST` | `/agent/release-requests/{id}/confirm` | Agent 确认发布单 | `release:confirm` |
| `GET` | `/agent/release-requests/{id}/summary` | 终端友好的发布单摘要 | `release:read` |

约束：

- 候选接口返回多个结果时，Agent 不应替用户猜测。
- 创建发布单时服务端固定写入 `source=ai_agent`；普通客户端不能通过请求体伪造来源。
- Agent metadata 只保存最小审计上下文：`agent_name`、`skill_version`、`intent_summary`、`client_request_id`、`conversation_ref`。
- Agent API 仍复用同一个发布单应用服务、preflight、确认、队列、审计和权限校验。
- Agent 不能独立确认生产发布；生产发布必须等待管理员确认。

## 10. 发布记录和日志 API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/deploy-records` | 发布记录列表 |
| `GET` | `/deploy-records/{id}` | 发布记录详情 |
| `GET` | `/deploy-records/{id}/target-logs` | 执行目标日志 |
| `GET` | `/deployment-states` | 当前部署版本视图 |

## 11. 环境发布保护 API

通过管理员接口 `PATCH /environments/{id}` 更新 `release_frozen`。环境响应同时返回 `is_production` 和 `release_frozen`；不提供独立策略或冻结 API。

## 12. 通知配置 API

### 12.1 凭据

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/credentials` | 凭据列表，不返回 secret |
| `POST` | `/credentials` | 创建凭据 |
| `PATCH` | `/credentials/{id}` | 更新 name/description/enabled；不允许改 type 与 secret |
| `DELETE` | `/credentials/{id}` | 删除；仍被服务器或 K8s 集群引用（含已禁用）时返回 `409 credential_in_use` |

凭据类型包含 SSH 凭据和 `kubeconfig`。凭据校验：服务器创建/更新时，`auth_type != none` 必须引用存在且启用的凭据，否则返回 `400`；K8s 集群创建/更新时必须引用存在且启用的 `kubeconfig` 凭据。删除凭据的引用检查与删除在单事务内完成，避免并发先查后删。凭据列表和 K8s 集群响应不得返回 secret。

### 12.2 通知

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/notification-configs` | 通知配置列表 |
| `POST` | `/notification-configs` | 创建企业微信机器人 webhook |
| `PATCH` | `/notification-configs/{id}` | 更新 |
| `DELETE` | `/notification-configs/{id}` | 删除；历史投递记录保留 config_id 悬空引用，前端显示"已删除配置" |
| `POST` | `/notification-configs/{id}/test` | 测试发送 |
| `GET` | `/notification-deliveries` | 发送记录 |

### 12.3 删除边界

已被发布链路引用的实体（项目、服务、版本、环境、服务器、服务器组、部署目标、用户）只能禁用，不能删除，以保护审计完整性。未被引用的辅助实体（凭据、通知配置、访问密钥）支持删除；凭据删除需通过服务器引用校验。

## 13. 运维接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/healthz`（不带 `/api/v1` 前缀） | 健康检查 |
| `GET` | `/ops/summary` | 运行摘要 |

运维接口不得泄漏密钥和敏感配置。

## 14. 验证要求

- Web 前端可只依赖 API 文档完成发布闭环。
- 每个关键写接口都有鉴权、幂等和事件写入说明。
- API 不包含运行中紧急停止入口。
