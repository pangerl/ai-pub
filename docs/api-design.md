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
- 服务器日志
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

## 4. 鉴权与调用身份

调用身份：

- `user`
- `api_key`
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
| `deploy:read` | 读取执行状态和服务器日志 |
| `inventory:read` | 读取项目、服务、版本、环境、服务器和部署目标 |
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
| `internal_error` | 系统错误 |

## 6. 认证、用户和 API Key

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/auth/login` | 登录 |
| `GET` | `/auth/me` | 当前用户 |
| `POST` | `/auth/logout` | 退出登录 |
| `GET` | `/users` | 用户列表，管理员 |
| `POST` | `/users` | 创建用户，管理员 |
| `PATCH` | `/users/{id}` | 更新用户，管理员 |
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

### 7.3 环境、服务器和部署目标

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/environments` | 环境列表 |
| `POST` | `/environments` | 创建环境 |
| `PATCH` | `/environments/{id}` | 更新环境 |
| `GET` | `/servers` | 服务器列表 |
| `POST` | `/servers` | 创建服务器 |
| `PATCH` | `/servers/{id}` | 更新服务器 |
| `POST` | `/servers/{id}/test` | 测试 SSH 连接 |
| `GET` | `/server-groups` | 服务器组列表 |
| `POST` | `/server-groups` | 创建服务器组 |
| `PATCH` | `/server-groups/{id}` | 更新服务器组 |
| `GET` | `/deployment-targets` | 部署目标列表 |
| `POST` | `/deployment-targets` | 创建部署目标 |
| `PATCH` | `/deployment-targets/{id}` | 更新部署目标 |

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

## 9. 发布记录和日志 API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/deploy-records` | 发布记录列表 |
| `GET` | `/deploy-records/{id}` | 发布记录详情 |
| `GET` | `/deploy-records/{id}/server-logs` | 服务器日志 |
| `GET` | `/server-deployment-states` | 当前运行版本视图 |

## 10. 环境发布保护 API

通过管理员接口 `PATCH /environments/{id}` 更新 `release_frozen`。环境响应同时返回 `is_production` 和 `release_frozen`；不提供独立策略或冻结 API。

## 11. 通知配置 API

### 11.1 凭据

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/credentials` | 凭据列表，不返回 secret |
| `POST` | `/credentials` | 创建凭据 |

### 11.2 通知

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/notification-configs` | 通知配置列表 |
| `POST` | `/notification-configs` | 创建企业微信机器人 webhook |
| `PATCH` | `/notification-configs/{id}` | 更新 |
| `POST` | `/notification-configs/{id}/test` | 测试发送 |
| `GET` | `/notification-deliveries` | 发送记录 |

## 12. 运维接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/healthz`（不带 `/api/v1` 前缀） | 健康检查 |
| `GET` | `/ops/summary` | 运行摘要 |

运维接口不得泄漏密钥和敏感配置。

## 13. 验证要求

- Web 前端可只依赖 API 文档完成发布闭环。
- 每个关键写接口都有鉴权、幂等和事件写入说明。
- API 不包含运行中紧急停止入口。
