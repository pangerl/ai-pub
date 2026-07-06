# AI Pub

AI Pub 是一个轻量发布执行系统，用于管理项目、服务版本、运行环境、部署目标和发布记录。当前实现聚焦 MVP 闭环：通过 Web/API 创建发布单，执行预检、确认、Worker 执行、事件审计和发布记录查询。

## 技术栈

- 后端：Go、`net/http`、`database/sql`、显式 SQL repository。
- 前端：React、TypeScript、Vite、Ant Design。
- 数据库：MySQL 8 是生产和正式集成验收数据库；SQLite 仅用于 demo/local 轻量模式和 Go 单测。
- 部署：前后端打包在同一个 `app` 容器中，部署 YAML 集中在 `deploy/`。

## 快速开始

MySQL 正式本地环境：

```bash
make compose-up
```

访问应用：`http://127.0.0.1:18080/`。

默认管理员仅适合本地体验：

- 用户名：`admin`
- 密码：`ai-pub-dev-admin`

SQLite demo/local 轻量模式：

```bash
make compose-sqlite-up
```

SQLite 模式不启动 MySQL，适合本地快速演示，不得用于生产。

## 验证

```bash
make verify
make compose-check
```

- `make verify`：运行 Go 测试、前端 lint 和生产构建。
- `make compose-check`：从空 MySQL 数据库启动容器并执行端到端发布闭环。
- `make compose-check-sqlite`：SQLite demo/local 轻量验收，不替代 MySQL 正式验收。

清理本地验收环境：

```bash
make compose-down
make compose-sqlite-down
```

## 部署文件

- `deploy/compose.mysql.yaml`：正式部署和完整集成验收使用。
- `deploy/compose.sqlite.yaml`：demo/local 轻量模式使用。
- `deploy/scripts/`：部署 YAML 需要搭配的辅助脚本。
- `deploy/sql/`：部署侧初始化或运维 SQL 说明。

## 生产提醒

生产环境至少应确认：

- 设置 `APP_ENV=prod`。
- 设置强随机 `APP_ENCRYPTION_KEY` 和 `JWT_SECRET`。
- 设置自己的 `BOOTSTRAP_ADMIN_USERNAME` 和 `BOOTSTRAP_ADMIN_PASSWORD`。
- 使用独立 MySQL 8 实例并备份数据库。
- 通过反向代理提供 HTTPS、访问控制和日志留存。
- 不要将 SSH 凭据、企业微信 webhook、API Key 或 `.env` 提交到版本库。

## 文档

- [技术文档索引](docs/README.md)
- [本地功能验证](docs/local-verification.md)
- [当前完成度审查](docs/development-completion-audit.md)
