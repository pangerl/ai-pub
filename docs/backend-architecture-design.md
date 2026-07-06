# Go 后端技术架构设计

## 1. 目标

第一版后端使用 Go 实现一个单体服务，提供 REST API、后台 Worker、执行器调用、审计事件和数据库 migration。

目标：

- 业务逻辑集中在应用服务层，不散落在 API handler 或 Worker 中。
- MySQL 8 是开发、验收和生产的唯一运行路径。
- Mock/Dry-run、SSH 和 K8s 执行器遵守同一 contract。
- 支持发布单创建、preflight、确认、入队、执行、日志、回滚和通知。
- K8s executor 只做既有 Deployment 指定容器镜像发布，不做 Manifest 管理、`apply`、扩缩容或运行参数变更。

## 2. 非目标

- 不拆微服务。
- 不设计多实例 Worker 抢占。
- 不做复杂插件市场。
- 不把 SSH 细节写入发布单领域模型。
- 不实现运行中紧急停止。

## 3. 目录结构

```text
cmd/server
internal/httpapi        # REST API、middleware、auth
internal/app            # 用例服务：发布单、preflight、确认、执行、回滚
internal/domain         # 领域对象、状态机、聚合规则
internal/repository     # SQL、事务与未来数据库方言边界
internal/executor       # Mock/Dry-run、SSH、K8s 执行器
internal/worker         # 队列领取、执行调度、心跳、状态修复
internal/migration      # MySQL migration runner
internal/notification   # 通知事件和渠道发送
internal/auth           # 密码哈希、登录会话和 token
internal/config         # 配置加载和校验
internal/crypto         # 凭据加密、hash、脱敏
```

## 4. 调用链

```text
HTTP request
  -> middleware(auth, request id, logging)
  -> httpapi handler
  -> app service
  -> repository transaction
  -> domain validation/state transition
  -> audit event
  -> response DTO
```

约束：

- handler 只负责认证、鉴权、参数校验和响应转换。
- 发布单、preflight、确认、队列、执行器、审计等业务逻辑放在 `internal/app`。
- 状态合法性放在 `internal/domain`，避免多个入口各自判断。
- SQL 只放在 repository 和 migration 层。

## 5. 应用服务划分

| 服务 | 责任 |
|------|------|
| `ReleaseService` | 创建发布单、查询发布单、取消、回滚 |
| `PreflightService` | 发布前检查、环境保护和风险提示 |
| `ConfirmService` | 本人确认、管理员确认、驳回 |
| `DeployService` | 创建发布记录、生成执行快照、状态聚合 |
| `WorkerService` | 领取任务、调执行器、心跳、状态修复 |
| `InventoryService` | 项目、服务、版本、环境、服务器、K8s 集群、部署目标 |
| `CredentialService` | 凭据加密、脱敏、SSH 配置读取 |
| `NotificationService` | 通知事件生成、发送、失败记录 |
| `AuditService` | 事件写入和查询 |

## 6. 关键事务边界

### 6.1 创建发布单

事务内：

1. 校验服务、环境、版本、部署目标存在且启用。
2. 校验幂等键。
3. 创建 `ReleaseRequest`。
4. 写入 `ReleaseEvent(release_created)`。

事务外：

- 返回发布单详情和下一步动作。

### 6.2 执行 preflight

事务内：

1. 读取发布单或发布意图上下文。
2. 读取环境的生产标记和冻结状态。
3. 检查环境冻结、生产门禁、部署目标完整性、运行中发布。
4. 记录 `ReleaseEvent(preflight_checked)`。

### 6.3 确认并入队

事务内：

1. 锁定或读取当前发布单状态。
2. 再次执行关键 preflight。
3. 校验确认人权限。
4. 更新发布单为 `queued`。
5. 创建 `DeployRecord(queued)`。
6. 构建类型化执行快照，展开执行目标，创建 `DeployTargetLog(queued)`。
7. 冻结部署目标执行快照。
8. 写入确认和入队事件。

约束：

- 发布单进入 `queued` 只能通过该入口。
- 生产发布必须管理员确认。

### 6.4 Worker 领取任务

第一版按单服务实例和内置 Worker 设计。

事务内：

1. 按创建时间选择 `queued` 发布记录。
2. 确认发布单仍处于可执行状态。
3. 确认目标未被其他 running 发布占用；同服务同环境已有 running 发布时阻断新执行。SSH 的服务器目标可按 `target_ref_id` 做更细互斥。
4. 更新发布记录为 `running`，目标日志保持 `queued`。
5. 写入 Worker 标识、心跳和租约时间。
6. 写入 `deploy_started` 事件。

### 6.5 执行结果回写

多目标 executor 按稳定顺序逐个执行。目标真正开始执行时，先将对应 `DeployTargetLog` 从 `queued` 更新为 `running` 并记录开始时间；其余目标继续保持 `queued`。目标执行结束后事务内：

1. 更新 `DeployTargetLog`。
2. 聚合当前 `DeployRecord` 计数。
3. 必要时更新 `DeploymentState`。
4. 写入 `target_finished` 事件。

整单结束时事务内：

1. 聚合发布记录终态。
2. 回写发布单摘要状态。
3. 写入 `deploy_finished` 事件。
4. 触发通知事件。

## 7. Worker 设计

Worker 循环：

```text
tick
  -> repair stale running records
  -> claim queued record
  -> execute deployment target
  -> write result
```

要求：

- Worker 必须等待 migration 成功后启动。
- Worker 执行期间定期心跳。
- 租约 TTL 不得小于命令执行超时时间。
- 僵尸任务通过心跳超时识别，并标记失败或待人工处理。
- SSH 多服务器发布按顺序执行，某台失败后 fail-fast，未执行目标标记 `skipped`。
- K8s Deployment 发布是单目标执行。
- 发布记录进入 `running` 不代表所有目标已开始；目标日志以 `queued -> running -> 终态` 表示实际进度。

## 8. 执行器 contract

输入：

| 字段 | 说明 |
|------|------|
| `release` | 发布单快照 |
| `deploy_record` | 发布记录快照 |
| `project` / `service` / `environment` | 基础上下文 |
| `version` | 服务版本 |
| `deployment_target` | 部署目标通用快照 |
| `plan` | 类型化执行快照 |
| `target` | 当前执行目标 |
| `env_vars` | 系统变量和部署目标变量 |
| `timeout_seconds` | 超时 |
| `attempt` | 尝试次数 |

输出：

| 字段 | 说明 |
|------|------|
| `status` | `success` / `failed` |
| `exit_code` | 退出码，可空 |
| `started_at` / `finished_at` | 时间 |
| `duration_ms` | 耗时 |
| `log_output` | 日志或日志引用 |
| `error_code` | 错误分类 |
| `error_message` | 脱敏错误 |
| `external_task_id` | 外部任务 ID，可空 |

错误分类：

- `connect_failed`
- `auth_failed`
- `script_not_found`
- `permission_denied`
- `command_timeout`
- `exit_non_zero`
- `internal_error`

K8s executor 额外使用：

- `cluster_not_available`
- `namespace_not_found`
- `deployment_not_found`
- `container_not_found`
- `image_invalid`
- `permission_denied`
- `rollout_timeout`
- `rollout_failed`
- `executor_error`

执行快照必须是类型化结构，包含 `ExecutionTarget`、SSH 快照和 K8s Deployment 快照；Worker 不通过 `json.RawMessage` 解析未知 executor 配置。

## 9. Mock/Dry-run 执行器

用途：

- 本地体验。
- 自动化测试。
- 端到端 demo。

能力：

- 模拟成功。
- 模拟失败。
- 生成最小执行目标日志。
- 使用同一套变量注入和结果回写路径。

## 10. SSH 执行器

能力：

- 支持密码登录和密钥登录。
- 支持端口、网关、超时。
- 支持脚本路径和工作目录。
- 注入标准变量。
- 采集 stdout/stderr、退出码和耗时。

安全要求：

- 凭据从加密存储读取。
- 日志和错误必须脱敏。
- 私钥、密码、token 不得进入 API 响应、事件和日志。
- 网关通过 `ProxyCommand` 建立单跳隧道：网关认证与应用服务器认证分开读取，最终脚本始终在应用服务器上执行。

## 11. K8s executor

能力：

- 读取 kubeconfig 凭据并连接 Kubernetes API。
- 读取既有 Deployment，检查命名空间、Deployment 和容器存在。
- 使用 patch 只更新指定容器的 `image` 为 `ServiceVersion.artifact_url`。
- 等待 Deployment rollout 完成。

边界：

- 不 shell 调 `kubectl`。
- 不保存完整 Deployment YAML，不支持 `kubectl apply -f`。
- 不创建 Deployment、Service、Ingress、ConfigMap、Secret 等资源。
- 不修改 replicas、resources、env、probe、volume、label、annotation、Service/Ingress 或调度策略。
- 回滚通过创建指向旧 `ServiceVersion` 的回滚发布单完成，不调用 Kubernetes revision undo。

安全要求：

- kubeconfig 使用加密凭据存储。
- 日志和错误不得输出 kubeconfig、token 或完整敏感错误上下文。

## 12. 数据库运行边界

- 使用 `database/sql`。
- 业务 SQL 集中在 repository；MySQL 专属 SQL 只出现在 repository 或 migration。
- JSON 数据按文本保存，由应用层解析。
- 布尔值按 `0/1` 保存。
- 时间由应用层生成。
- MySQL 8 是生产和正式集成验收数据库。
- SQLite 仅用于 demo/local 轻量模式，不用于生产，不承诺多实例或高并发语义。
- PostgreSQL 是后续开源扩展，不在当前运行时配置或测试矩阵中维护；接入时新增 `migrations/postgres` 和 repository 适配。
- 当前不做高并发或多实例专项设计。

## 13. 配置

关键环境变量：

| 变量 | 说明 |
|------|------|
| `APP_ENV` | `dev` / `test` / `prod` |
| `HTTP_ADDR` | 监听地址 |
| `DB_DIALECT` | `mysql` / `sqlite`，默认 `mysql`；`sqlite` 仅限 demo/local |
| `MYSQL_DSN` | MySQL DSN |
| `SQLITE_PATH` | SQLite demo/local 数据文件路径 |
| `APP_ENCRYPTION_KEY` | 凭据和 webhook 加密 key，生产必填 |
| `JWT_SECRET` | 登录 token key，生产必填 |
| `BOOTSTRAP_ADMIN_USERNAME` | 首个管理员用户名 |
| `BOOTSTRAP_ADMIN_PASSWORD` | 首个管理员密码，首次创建或补齐密码时必填 |
| `MIGRATION_AUTO` | 是否自动 migration |
| `MIGRATION_CHECK_ONLY` | 只检查待执行 migration 后退出 |
| `WORKER_ENABLED` | 是否启动内置 Worker |

## 13. 验证要求

- 后端可启动并通过健康检查。
- MySQL migration 成功后 Worker 才启动。
- Mock/Dry-run 发布闭环通过。
- SSH 最小真实发布通过。
- MySQL 8 Compose 路径能完成 migration 和 Mock/Dry-run 发布。
- 关键写操作都写入审计事件。
