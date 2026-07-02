# 领域模型设计

## 1. 目标

本文把 `project-requirements.md` 中的发布系统需求落成第一版可实现的数据模型。

第一版模型目标：

- 支持 Web/API 完成发布单创建、preflight、确认、入队、执行、日志查看和回滚。
- MySQL 8 作为唯一运行数据库；未来 PostgreSQL 通过 repository 与独立 migration 接入。
- 审计事件可查询、可追溯，不设计不可篡改账本。
- 支持后续 AI Agent 作为受限调用方复用同一套发布流程。

## 2. 非目标

- 不设计多租户。
- 不设计复杂 RBAC、组织架构、审批流。
- 不设计高并发、多实例 Worker 或复杂锁模型。
- 不设计运行中紧急停止。
- 不设计审计哈希链、外部审计存储或不可篡改账本。

## 3. 实体关系概览

```text
Project 1 -- n Service
Service 1 -- n ServiceVersion
ServiceVersion 1 -- n ServiceVersionEvent
Service 1 -- n DeploymentTarget
Environment 1 -- n DeploymentTarget
DeploymentTarget 1 -- 0..1 SSHDeploymentTarget
DeploymentTarget 1 -- 0..1 KubernetesDeploymentTarget
K8sCluster 1 -- n KubernetesDeploymentTarget

ReleaseRequest 1 -- 0..1 DeployRecord
ReleaseRequest 1 -- n ReleaseEvent
DeployRecord 1 -- n DeployTargetLog
Service + Environment + DeploymentTarget + ExecutionTarget 1 -- 1 DeploymentState

ApiKey belongs to User
NotificationConfig 1 -- n NotificationDelivery
```

## 4. 核心实体

### 4.1 User

用途：表示系统中的人类用户。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `username` | 登录名，唯一 |
| `display_name` | 展示名 |
| `role` | `employee` 或 `admin` |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

约束：

- 第一版只支持普通员工和管理员。
- 不做项目级、服务级 ACL。

### 4.2 ApiKey

用途：自动化调用凭据。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 名称 |
| `prefix` | key 前缀，用于列表展示 |
| `key_hash` | key hash，不保存明文 |
| `owner_user_id` | 归属用户 ID |
| `scopes` | scope 列表，JSON 文本 |
| `expires_at` | 过期时间，可空 |
| `enabled` | 是否启用 |
| `last_used_at` | 最近使用时间 |
| `created_at` / `updated_at` | 时间 |

约束：

- 明文只在创建时返回一次。
- API Key 不因创建者是管理员而自动拥有全部权限。
- API Key 不得绕过生产发布管理员确认。

### 4.3 Project

用途：服务归属分组。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 中文或业务展示名 |
| `slug` | 稳定标识，唯一 |
| `description` | 描述 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

### 4.4 Service

用途：可发布服务。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `project_id` | 所属项目 |
| `name` | 展示名 |
| `slug` | 项目内唯一 |
| `description` | 描述 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

约束：

- `(project_id, slug)` 唯一。
- 服务不保存“当前版本”，当前版本由部署状态聚合。SSH 场景可按服务器展开，K8s 场景按部署目标展示。

### 4.5 ServiceVersion

用途：服务可发布版本。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_id` | 所属服务 |
| `version` | 版本号 |
| `commit_sha` | commit，可空 |
| `artifact_url` | 制品地址，可空，展示时脱敏 |
| `source` | 本期仅 `manual` / `ci`；服务端写入 |
| `metadata` | JSON 文本 |
| `created_by_type` / `created_by_id` | 创建来源 |
| `registration_idempotency_key` | 外部登记幂等键，可空 |
| `registration_request_hash` | `version`、`commit_sha`、`artifact_url` 规范化值的指纹，可空 |
| `created_at` | 创建时间 |

约束：

- `(service_id, version)` 唯一。
- `(service_id, registration_idempotency_key)` 在幂等键非空时唯一；指纹排除 metadata、构建时间和运行链接，同 key 同指纹返回已有版本，不同指纹返回冲突。
- 管理员手动登记重复版本返回冲突，不覆盖历史版本。

### 4.6 Environment

用途：发布环境。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 展示名 |
| `slug` | 唯一标识 |
| `is_production` | 是否生产环境 |
| `release_frozen` | 是否冻结该环境的发布 |
| `created_at` / `updated_at` | 时间 |

约束：

- 生产判断必须依赖 `is_production`，不能依赖 tag 文本。
- 生产环境固定要求管理员确认；非生产环境固定由发起人本人确认。
- `release_frozen` 为环境唯一的发布冻结来源，不做系统或服务级覆盖。
- 环境无 `enabled` 字段：发布保护统一由 `release_frozen` 承担，不存在"禁用环境"语义。

### 4.7 Server

用途：SSH 发布目标。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 展示名 |
| `host` | 主机 |
| `port` | 端口 |
| `username` | SSH 用户 |
| `auth_type` | `password` 或 `private_key` |
| `credential_ref` | 凭据引用 |
| `role` | `gateway` 或 `application` |
| `gateway_id` | 应用服务器使用的网关，可空；为空时直连 |
| `enabled` | 是否启用 |
| `last_check_status` | 最近连接测试状态 |
| `last_check_at` | 最近连接测试时间 |
| `created_at` / `updated_at` | 时间 |

约束：

- 密码、私钥不以明文字段保存。
- API、日志、事件不得输出敏感凭据。
- 网关服务器只能由发布服务直连；应用服务器可直连，或通过一个网关建立 SSH 隧道。
- 经网关时，网关使用自身凭据建立隧道，应用服务器仍使用自身凭据建立最终 SSH 会话；不支持多跳。
- 发布目标只能包含应用服务器，不能把网关作为脚本执行目标。

### 4.8 ServerGroup

用途：组织多个服务器作为可复用运行目标。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 展示名 |
| `description` | 描述 |
| `enabled` | 是否启用 |

关联表：

- `server_group_members(server_group_id, server_id)`

### 4.9 DeploymentTarget

用途：连接服务、环境、执行器和发布目标，是发布单选择目标的统一入口。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_id` | 服务 |
| `environment_id` | 环境 |
| `executor_type` | `mock` / `ssh` / `k8s` |
| `artifact_type` | `version_only` 或 `oci_image`，非空，默认 `version_only` |
| `timeout_seconds` | 执行超时 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

约束：

- 第一版内置 `mock`、`ssh` 和 `k8s`。
- SSH 专属字段不放在主表中，避免污染 K8s 和 Mock 语义。
- `artifact_type=version_only` 允许脚本按版本号解析制品；`artifact_type=oci_image` 要求版本制品为 OCI digest，并由 preflight 阻断缺失或格式不符的制品。
- K8s 部署目标强制 `artifact_type=oci_image`，只发布既有 Deployment 的指定容器镜像。
- `(service_id, environment_id)` 可以有多个部署目标，但创建发布单时必须选择明确目标。

### 4.10 SSHDeploymentTarget

用途：保存 SSH executor 专属部署目标配置。

关键字段：

| 字段 | 说明 |
|------|------|
| `deployment_target_id` | 主键，引用 `deployment_targets.id` |
| `target_type` | `server` 或 `server_group` |
| `target_ref_id` | 服务器或服务器组 ID |
| `script_path` | SSH 脚本路径 |
| `working_dir` | 工作目录 |
| `env_vars` | JSON 文本 |

约束：

- `executor_type=ssh` 的部署目标必须有对应 SSH 配置。
- `target_type=server_group` 时执行前展开为稳定顺序的服务器目标。
- 发布目标只能包含应用服务器，不能把网关作为脚本执行目标。

### 4.11 K8sCluster

用途：表示可用于 Kubernetes 发布的集群连接配置。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 集群名称 |
| `credential_ref` | kubeconfig 凭据引用 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

约束：

- 凭据复用 `credentials`，类型为 `kubeconfig`。
- API 列表不得返回 kubeconfig 明文或 secret。
- 集群被部署目标引用时不得删除。

### 4.12 KubernetesDeploymentTarget

用途：保存 K8s executor 的既有 Deployment 镜像发布配置。

关键字段：

| 字段 | 说明 |
|------|------|
| `deployment_target_id` | 主键，引用 `deployment_targets.id` |
| `cluster_id` | Kubernetes 集群 |
| `namespace` | 命名空间 |
| `deployment_name` | Deployment 名称 |
| `container_name` | 容器名称 |

约束：

- 第一版仅支持 Deployment，不支持 StatefulSet、DaemonSet、CronJob。
- 不保存完整 YAML/Manifest，不支持 `kubectl apply -f` 语义。
- 不保存或修改 replicas、resources、env、probe、volume、label、annotation、Service/Ingress 等运行参数。
- 执行时只 patch 指定容器的 image，并等待 Deployment rollout 完成。

### 4.13 发布保护

发布保护直接归属 `Environment`，不单独建策略实体。

- `is_production=true` 时固定管理员确认，其他环境固定本人确认。
- `release_frozen=true` 时发布 preflight 返回 block，待确认发布不得确认通过。
- 已 queued 的发布在冻结期间暂停领取，running 发布继续执行。
- 同服务同环境已有 running 发布时，默认阻断新的真实执行。

### 4.14 ReleaseRequest

用途：发布执行意图和门禁状态。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `project_id` | 项目 |
| `service_id` | 服务 |
| `environment_id` | 环境 |
| `service_version_id` | 版本 |
| `deployment_target_id` | 部署目标 |
| `status` | 发布单状态 |
| `source` | `web` / `api` |
| `idempotency_key` | 幂等键，可空 |
| `created_by_type` / `created_by_id` | 创建主体 |
| `authorized_by_user_id` | 授权用户，可空 |
| `confirmed_by_user_id` | 确认用户，可空 |
| `confirmed_at` | 确认时间，可空 |
| `rejected_by_user_id` | 驳回用户，可空 |
| `rejected_reason` | 驳回原因，可空 |
| `rollback_of_id` | 原发布单，可空 |
| `summary_status` | 摘要状态 |
| `summary_message` | 摘要说明 |
| `metadata` | JSON 文本 |
| `created_at` / `updated_at` | 时间 |

状态：

- `pending_confirm`
- `rejected`
- `cancelled`
- `queued`
- `running`
- `success`
- `failed`

状态流转：

```text
pending_confirm -> rejected
pending_confirm -> cancelled
pending_confirm -> queued
queued -> cancelled
queued -> running
running -> success
running -> failed
```

约束：

- 进入执行后，终态由 `DeployRecord` 和执行目标日志聚合回写。
- `partial` 在发布单摘要中按 `failed` 处理，并保留部分成功计数。
- queued 前允许取消；running 后不提供系统级紧急停止入口。
- 发布单不得绕过发布记录直接进入 `running/success/failed`。

### 4.15 DeployRecord

用途：真实执行记录。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `release_request_id` | 发布单 |
| `status` | 执行状态 |
| `executor_type` | 执行器 |
| `target_snapshot` | 部署目标执行快照 JSON 文本 |
| `total_targets` | 执行目标总数 |
| `success_targets` | 成功目标数 |
| `failed_targets` | 失败目标数 |
| `skipped_targets` | 跳过目标数 |
| `worker_id` | 当前 Worker |
| `lease_expires_at` | 租约过期时间 |
| `heartbeat_at` | 心跳时间 |
| `started_at` / `finished_at` | 开始和结束时间 |
| `error_summary` | 错误摘要 |
| `created_at` / `updated_at` | 时间 |

状态：

- `queued`
- `running`
- `success`
- `failed`
- `partial`

聚合规则：

- 全部目标成功：`success`。
- 全部失败或 skipped，且没有成功目标：`failed`。
- 至少一台成功，且有 failed 或 skipped：`partial`。
- SSH 服务器组按服务器展开多个目标；K8s Deployment 发布生成一个目标。

### 4.16 DeployTargetLog

用途：单个执行目标的状态和日志。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `deploy_record_id` | 发布记录 |
| `target_type` | `server` / `server_group_member` / `k8s_deployment` / `mock` |
| `target_ref_id` | 目标引用 ID |
| `target_name` | 执行目标快照名称 |
| `status` | `queued` / `running` / `success` / `failed` / `skipped` |
| `exit_code` | 进程类执行器退出码，可空 |
| `started_at` / `finished_at` | 时间 |
| `duration_ms` | 耗时 |
| `log_output` | 日志文本或引用 |
| `error_code` | 错误码 |
| `error_message` | 脱敏错误信息 |

约束：

- 日志不得包含未脱敏密码、私钥、token。
- 发布记录被 Worker 领取后，未开始执行的目标仍为 `queued`；仅在实际开始该目标时进入 `running` 并写入 `started_at`。
- SSH 多服务器 fail-fast 后未执行目标标记为 `skipped`。
- K8s 日志只记录发布目标、rollout 结果和脱敏错误，不输出 kubeconfig、token 或完整敏感错误上下文。

### 4.17 DeploymentState

用途：部署目标当前版本视图。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_id` | 服务 |
| `environment_id` | 环境 |
| `deployment_target_id` | 部署目标 |
| `target_type` | 当前状态目标类型 |
| `target_ref_id` | 目标引用 ID |
| `service_version_id` | 当前版本 |
| `deploy_record_id` | 来源发布记录 |
| `updated_at` | 更新时间 |

约束：

- `(service_id, environment_id, deployment_target_id, target_type, target_ref_id)` 唯一。
- SSH 场景 `target_ref_id` 为服务器 ID；K8s 场景可使用 `<cluster_id>/<namespace>/<deployment_name>/<container_name>`。
- 环境级当前版本由部署状态聚合得出；不一致时显示“混合版本”。

### 4.18 ReleaseEvent

用途：关键动作追溯。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `release_request_id` | 发布单，可空 |
| `deploy_record_id` | 发布记录，可空 |
| `event_type` | 事件类型 |
| `actor_type` | `user` / `api_key` / `system` |
| `actor_id` | 主体 ID |
| `authorized_user_id` | 授权用户，可空 |
| `api_key_id` | API Key，可空 |
| `source_ip` | 来源 IP，可空 |
| `message` | 可读说明 |
| `metadata` | JSON 文本 |
| `created_at` | 时间 |

事件类型至少包含：

- `release_created`
- `preflight_checked`
- `release_confirmed`
- `release_rejected`
- `release_cancelled`
- `deploy_started`
- `target_finished`
- `deploy_finished`
- `rollback_requested`
- `notification_sent`
- `notification_failed`

### 4.19 ServiceVersionEvent

用途：服务版本登记与后续版本动作的独立审计；不关联发布单，不复用 `ReleaseEvent`。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_version_id` | 服务版本 |
| `event_type` | `version_registered` 等 |
| `actor_type` | `user` / `api_key` / `system` |
| `actor_id` | 主体 ID |
| `api_key_id` | API Key，可空 |
| `registration_idempotency_key` | 外部登记幂等键，可空 |
| `message` | 可读说明 |
| `metadata` | JSON 文本 |
| `created_at` | 创建时间 |

### 4.20 NotificationConfig

用途：通知渠道配置。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `channel` | `wecom_robot` |
| `name` | 名称 |
| `webhook_url_enc` | 加密后的 webhook |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

### 4.21 NotificationDelivery

用途：通知发送记录。

关键字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `config_id` | 通知配置 |
| `event_type` | 触发事件 |
| `release_request_id` | 发布单，可空 |
| `deploy_record_id` | 发布记录，可空 |
| `status` | `sent` / `failed` |
| `last_error` | 错误摘要 |
| `sent_at` | 发送时间 |
| `created_at` / `updated_at` | 时间 |

## 5. MySQL 类型约定

| 语义 | MySQL |
|------|-------|
| 主键 | `VARCHAR(64)` |
| 布尔值 | `TINYINT(1)` |
| 时间 | UTC 文本 |
| JSON | `TEXT`，由应用层解析 |
| 大日志 | `MEDIUMTEXT` |

第一版避免依赖 JSON 函数、存储过程和复杂视图。PostgreSQL 如需接入，应在 repository 和独立 migration 中解决类型差异。

## 6. 索引和唯一约束

最低约束：

- `projects.slug` 唯一。
- `(services.project_id, services.slug)` 唯一。
- `(service_versions.service_id, service_versions.version)` 唯一。
- `environments.slug` 唯一。
- `(deployment_states.service_id, deployment_states.environment_id, deployment_states.deployment_target_id, deployment_states.target_type, deployment_states.target_ref_id)` 唯一。
- `ssh_deployment_targets.deployment_target_id` 唯一。
- `k8s_clusters.name` 唯一。
- `k8s_deployment_targets.deployment_target_id` 唯一。
- API Key `prefix` 可建索引，`key_hash` 唯一。
- 发布单按 `status`、`service_id`、`environment_id`、`created_at` 建查询索引。
- 发布记录按 `status`、`service_id`、`environment_id`、`created_at` 建查询索引。
- 执行目标日志按 `deploy_record_id`、`target_type`、`target_ref_id` 建查询索引。
- 事件按 `release_request_id`、`deploy_record_id`、`created_at` 建查询索引。

## 7. 验证要求

- MySQL 8 migration 从空库执行成功。
- 发布单、发布记录、执行目标日志状态机单元测试通过。
- 基础 CRUD、审计事件写入和查询在 MySQL 8 下通过。
- Mock/Dry-run 发布闭环能写入发布单、发布记录、执行目标日志、部署状态和事件。
- SSH 发布保持服务器/服务器组顺序 fail-fast 语义。
- K8s 发布只更新既有 Deployment 指定容器镜像，不修改运行参数。
