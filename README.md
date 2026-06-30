# ai-pub

ai-pub 是一个轻量发布执行系统，面向中小研发团队的服务发布闭环：配置发布对象，创建发布单，执行 preflight，确认入队，执行 Mock/Dry-run 或 SSH 发布，查看服务器日志、审计事件、重新发布和回滚。

它不是完整 DevOps 平台、审批流系统或 GitOps 编排器。第一版聚焦一件事：让“发布什么版本、发布到哪个环境、谁确认、执行到哪些服务器、结果如何、失败如何追溯”进入同一条可查询、可审计的执行链路。

## 功能概览

- Web 管理界面：工作台、发布中心、发布详情、发布记录、配置、系统管理和个人访问密钥。
- 发布闭环：发布单创建、preflight、本人/管理员确认、排队、执行、状态聚合、驳回、取消、重试和回滚。
- 环境保护：生产环境固定需要管理员确认，非生产环境由发起人确认；环境可冻结发布。
- 执行器：内置 Mock/Dry-run 和 SSH 执行器，SSH 支持私钥、密码认证和单跳网关。
- 多服务器发布：支持服务器组，按顺序执行并 fail-fast，未执行服务器标记为 `skipped`。
- 版本登记：外部 CI 可通过 `version:write` API Key 登记服务版本和 OCI 制品地址。
- 审计与通知：关键发布动作写入事件流，支持企业微信机器人 webhook 通知和投递记录。
- API Key：按 scope 授权 `inventory:read`、`release:*`、`deploy:read`、`version:write`、`admin:write` 等能力。

## 技术栈

- 后端：Go、`net/http`、`database/sql`、显式 SQL repository。
- 数据库：MySQL 8 是唯一受支持的运行时数据库。
- 前端：React、TypeScript、Vite、Ant Design。
- 本地与验收：Docker Compose 启动 MySQL、后端、前端和一次性验收容器。

SQLite 仅用于 Go 单元测试中的内存 schema，不是开发、验收或生产运行时。

## 快速开始

需要 Docker Desktop 或 OrbStack。

```bash
docker compose up --build -d
open http://127.0.0.1:18080/
```

本地 Compose 默认只暴露前端端口 `127.0.0.1:18080`，前端会反向代理 `/api` 到容器内后端。首次启动会创建管理员：

- 用户名：`admin`
- 密码：`ai-pub-dev-admin`

这些默认值只适合本地体验。共享环境或生产环境请先复制并调整 `.env.example`：

```bash
cp .env.example .env
docker compose --env-file .env up --build -d
```

## 验证

```bash
make verify
make compose-check
```

- `make verify`：运行 Go 测试、前端 lint 和生产构建。
- `make compose-check`：清理验证 profile 的数据卷，从空 MySQL 数据库启动容器并执行端到端发布闭环。

清理本地验收环境：

```bash
make compose-down
```

## 生产部署提醒

当前仓库提供的是最小可运行容器化部署基线。生产上线前至少应确认：

- 设置 `APP_ENV=prod`。
- 设置强随机 `APP_ENCRYPTION_KEY` 和 `JWT_SECRET`，不要使用开发默认值。
- 设置自己的 `BOOTSTRAP_ADMIN_USERNAME` 和 `BOOTSTRAP_ADMIN_PASSWORD`，并在首次登录后妥善轮换管理员密码。
- 使用独立 MySQL 8 实例并备份数据库。
- 通过反向代理提供 HTTPS、访问控制和日志留存。
- 保护 SSH 凭据、企业微信 webhook、API Key 和 `.env` 文件，不要提交到版本库。
- 发布前运行 `make verify` 和 `make compose-check`。

## 常用环境变量

| 变量 | 说明 |
| --- | --- |
| `APP_PORT` | Compose 暴露的前端端口，默认 `18080` |
| `APP_ENV` | 运行环境，生产使用 `prod` |
| `MYSQL_DSN` | 后端 MySQL DSN，Compose 内部已预置 |
| `APP_ENCRYPTION_KEY` | 凭据和 webhook 加密 key，生产必填 |
| `JWT_SECRET` | 登录会话签名 key，生产必填 |
| `BOOTSTRAP_ADMIN_USERNAME` | 首个管理员用户名 |
| `BOOTSTRAP_ADMIN_PASSWORD` | 首个管理员密码，首次创建或补齐密码时必填 |
| `MIGRATION_AUTO` | 是否启动时自动执行 migration，默认 `true` |
| `MIGRATION_CHECK_ONLY` | 只检查待执行 migration 后退出，默认 `false` |
| `WORKER_ENABLED` | 是否启动内置 Worker，默认 `true` |

## API

基础路径为 `/api/v1`，健康检查为 `/healthz`。主要资源包括：

- 认证：`/auth/login`、`/auth/me`、`/auth/logout`
- 基础配置：`/projects`、`/services`、`/services/{id}/versions`、`/environments`、`/servers`、`/server-groups`、`/deployment-targets`
- 管理对象：`/users`、`/api-keys`、`/credentials`、`/notification-configs`、`/notification-deliveries`
- 版本登记：`/version-registrations`
- 发布：`/release-requests`、`/release-requests/{id}/preflight`、`/confirm`、`/reject`、`/cancel`、`/rollback`、`/retry`、`/events`
- Agent 发版：`/agent/services`、`/agent/environments`、`/agent/release-intents/preflight`、`/agent/release-requests`、`/summary`
- 执行记录：`/deploy-records`、`/deploy-records/{id}/server-logs`、`/server-deployment-states`
- 运维摘要：`/ops/summary`

详细接口、状态机和权限边界见 [docs/api-design.md](docs/api-design.md)。

仓库内置 Codex skill：[skills/ai-pub-release](skills/ai-pub-release)，用于通过 Agent API 创建、确认和跟踪发布单。

## 文档

- [技术文档索引](docs/README.md)
- [本地功能验证](docs/local-verification.md)
- [MVP 完成审查](docs/development-completion-audit.md)
- [产品需求文档](project-requirements.md)

## 当前边界

- 仅支持 MySQL 8 运行时；PostgreSQL 需作为独立方言扩展，不在当前版本宣称支持。
- 不提供多租户、复杂 RBAC、审批流、运行中紧急停止、高并发或多实例 Worker 能力。
- 审计目标是关键动作可追溯、可查询，不设计不可篡改账本。
- Jenkins、Kubernetes、ArgoCD、Webhook 等执行器是后续扩展，不是第一版内置能力。

## License

MIT License. See [LICENSE](LICENSE).
