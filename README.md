# ai-pub

轻量发布执行系统。第一版目标是先跑通 Web/API 发布闭环：配置基础对象、创建发布单、preflight、确认入队、Mock/Dry-run 执行、日志和审计查询。

## 当前开发阶段

已完成 M0 工程骨架、M1 基础配置 API、M2 发布单门禁、M3 Mock/Dry-run 发布闭环、M4 SSH 基础能力、M5 前端工作台和 M6 通知能力。运行时统一使用 MySQL 8；开发、验收和生产不再支持 SQLite。

- Go 后端入口和 `/healthz`。
- MySQL 配置加载和 migration runner。
- React/Vite 前端工作台。
- 核心 MySQL 表结构。
- 项目、服务、版本、环境、服务器、服务器组、部署目标、用户和 API Key 基础 API。
- 用户启用/禁用，禁用用户不能确认发布。
- API Key 明文只在创建响应返回一次。
- Bearer API Key 支持读取、创建、确认和回滚发布单，并校验 `release:read`、`release:create`、`release:confirm`、`release:rollback` scope。
- Bearer API Key 支持发布前 preflight、驳回和取消，并分别校验 `release:create`、`release:confirm` scope。
- Bearer API Key 支持读取部署记录和服务器日志，并校验 `deploy:read` scope。
- Bearer API Key 管理基础配置、API Key、凭据、通知和发布策略时校验 `admin:write` scope。
- API Key 调用写入发布事件 `api_key_id` 审计字段。
- 发布单幂等键重复提交返回首次发布单，关键字段变化返回 409 冲突。
- 发布单 preflight、创建、确认入队、驳回和取消。
- 发布单确认和取消重复提交会返回当前状态，不重复创建执行记录或事件。
- Preflight 对缺少制品地址的版本给出 warning，并阻断覆盖 `AI_PUB_*` 系统变量的部署目标。
- 已入队发布取消时同步取消发布记录，避免取消后仍显示为 queued。
- 系统/环境/服务发布策略读取、保存、生效策略查询和冻结开关。
- 非生产本人确认、生产管理员确认、冻结阻断、运行中发布阻断。
- 冻结策略会阻断新发布，并暂停已 queued 发布被 Worker 领取。
- 发布单关键动作审计事件，包括已有发布单预检、确认、驳回、取消、执行、回滚和通知投递结果。
- Mock/Dry-run 执行器。
- Mock/Dry-run 和 SSH 执行器注入标准发布环境变量，包括版本号、commit 和制品地址。
- 内置 Worker 领取 queued 发布记录、执行服务器任务、聚合 success/failed/partial，并在 fail-fast 后标记未执行服务器为 skipped。
- Worker 领取任务时检查目标服务器占用，避免同一服务器同时进入多个 running 发布。
- 发布记录和服务器日志读取 API。
- 发布记录保存部署目标和服务器执行快照，Worker 按入队时快照执行。
- 凭据加密存储和凭据列表 API，列表不返回 secret。
- SSH 私钥执行器基础路径，支持超时、stdout/stderr 采集、错误分类和脱敏。
- 企业微信机器人 webhook 通知配置。
- 通知配置启用/禁用、测试发送和发送记录。
- 生产待管理员确认、发布失败和回滚申请通知，通知发送结果写入发布事件流，通知失败不阻塞发布主流程。
- MySQL 8 容器启动、自动 migration 和完整 Mock 发布闭环验证。
- 回滚候选和回滚发布单。
- 服务器当前版本视图。
- 运维摘要 `/ops/summary`。
- 前端支持初始化 Mock 配置、创建发布单、预检、确认入队、驳回、取消、创建回滚单、模拟失败发布。
- 前端支持发布前预检结果展示。
- 前端支持手动创建项目、服务、版本、环境、服务器、发布目标、确认用户、发布策略、通知配置、凭据、API Key 和发布日志查看。
- 前端支持创建服务器组，并使用 `server_group` 作为发布目标。
- 前端支持通知测试和通知投递查看。
- 前端工作台支持显式选择服务、环境、版本、部署目标和确认用户。
- 前端发布中心和发布记录支持按当前服务/环境、状态筛选。
- 本地功能验证脚本覆盖驳回、取消、成功发布、服务器组发布、partial/skipped、失败发布、回滚、日志和事件。

后续优先做缺陷修复和少量交互打磨。真实 SSH 私钥与密码登录发布、企业微信机器人 webhook 发送均已完成测试验证；凭据能力增强仍可后续扩展。

## 容器启动

需要 OrbStack 或 Docker Desktop。MySQL 不暴露宿主机端口，后端仅在 Compose 网络中运行；只有前端映射到 `127.0.0.1:18080`（可通过 `APP_PORT` 覆盖）。

```bash
docker compose up --build -d
open http://127.0.0.1:18080/
```

## 验证

```bash
make verify        # Go 单元测试与前端构建检查
make local-check   # MySQL 8 + 后端 + 前端 + 容器内发布闭环
```

停止并删除验收数据：

```bash
make compose-down
```

## 已有 API

基础路径为 `/api/v1`：

- `GET/POST /projects`
- `GET/PATCH /projects/{id}`
- `GET/POST /services`
- `GET/PATCH /services/{id}`
- `GET/POST /services/{id}/versions`
- `GET/POST /environments`
- `GET/POST /servers`
- `GET/POST /server-groups`
- `GET/POST /deployment-targets`
- `PATCH /deployment-targets/{id}`
- `GET/POST /users`
- `PATCH /users/{id}`
- `GET/POST /api-keys`
- `PATCH/DELETE /api-keys/{id}`
- `GET/POST /credentials`
- `GET/POST /notification-configs`
- `PATCH /notification-configs/{id}`
- `POST /notification-configs/{id}/test`
- `GET /notification-deliveries`
- `POST /release-requests/preflight`
- `GET/POST /release-requests`
- `GET /release-requests/{id}`
- `POST /release-requests/{id}/preflight`
- `POST /release-requests/{id}/confirm`
- `POST /release-requests/{id}/reject`
- `POST /release-requests/{id}/cancel`
- `GET /release-requests/{id}/events`
- `GET /release-requests/{id}/rollback-candidates`
- `POST /release-requests/{id}/rollback`
- `GET /release-policies`
- `POST /release-policies`
- `GET /release-policies/effective`
- `POST /release-policies/freeze`
- `POST /release-policies/unfreeze`
- `GET /deploy-records`
- `GET /deploy-records/{id}`
- `GET /deploy-records/{id}/server-logs`
- `GET /server-deployment-states`
- `GET /ops/summary`

## 数据库与 PostgreSQL 边界

当前运行时只支持 MySQL 8，所有 migration 由启动中的后端自动执行。业务层通过 repository 访问数据库，不使用 ORM；SQL 必须集中在 repository，禁止将 MySQL 专属语法扩散到 app、worker 或 HTTP 层。

项目开源后如需 PostgreSQL，应新增 `migrations/postgres`、PostgreSQL 连接与 repository 适配，并以同一套容器验收用例验证；不在当前版本维护或宣称 PostgreSQL 支持。

## SSH 能力边界

当前 SSH 执行器不新增 Go SSH 依赖，使用系统 `ssh` 命令执行私钥或密码发布。密码认证使用进程生命周期内的 `SSH_ASKPASS` helper，不出现在命令行、API 响应、审计事件或发布日志；凭据始终从加密存储读取。

## 文档

技术文档索引见 [docs/README.md](docs/README.md)。

开发完成审查见 [docs/development-completion-audit.md](docs/development-completion-audit.md)。

本地功能验证见 [docs/local-verification.md](docs/local-verification.md)。
