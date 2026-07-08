# AI Pub

AI Pub 是一个轻量发布执行系统，用于管理项目、服务版本、运行环境、部署目标和发布记录。当前实现聚焦 MVP 闭环：通过 Web/API 创建发布单，执行预检、确认、Worker 执行、事件审计和发布记录查询。

## 技术栈

- 后端：Go、`net/http`、`database/sql`、显式 SQL repository。
- 前端：React、TypeScript、Vite、Ant Design。
- 数据库：MySQL 8 是生产和正式集成验收数据库；SQLite 仅用于 demo/local 轻量模式和 Go 单测。
- 部署：前后端打包在同一个 `app` 容器中，部署 YAML 集中在 `deploy/`。

## 快速开始

MySQL 镜像正式本地环境：

```bash
cd deploy
docker compose -f compose.mysql.yaml up -d
```

访问应用：`http://127.0.0.1:18080/`。
镜像版 MySQL 和 SQLite 默认使用 `hxjagf/ai-pub:latest`；如需验证指定镜像，可设置 `AI_PUB_IMAGE`。
本地临时体验可使用内置默认值；共享环境、公网环境或长期使用前，应先复制 [.env.example](.env.example) 为 `deploy/.env`，并替换 `APP_ENCRYPTION_KEY`、`JWT_SECRET` 和 `BOOTSTRAP_ADMIN_PASSWORD`。

开发者如需基于当前源码构建 MySQL 本地环境：

```bash
make compose-up
```

默认管理员仅适合本地体验：

- 用户名：`admin`
- 密码：`ai-pub-dev-admin`

SQLite 镜像轻量体验：

```bash
cd deploy
docker compose -f compose.sqlite.yaml up -d
```

SQLite 模式不启动 MySQL，适合本地快速演示，不得用于生产。

开发者如需基于当前源码构建 SQLite 轻量环境：

```bash
make compose-sqlite-up
```

## 在线 demo

提供一个公网 demo 站点供试用体验（执行器为 mock，不会触发真实 SSH/K8s 外联）：

- 访问：`https://pub-demo.lanpang.top`
- 体验账号：`demo`
- 体验密码：`ai-pub-demo-2026`
- 账号权限：管理员权限，可体验项目、服务、环境、部署目标和发布流程配置
- 账号保护：demo 入口账号和内置 admin 账号不可被访客禁用、降级或重置密码
- 数据定期重置，请勿存放敏感信息

自建 demo 部署见 [deploy/README.md](deploy/README.md) 的「Demo 公网部署」小节；安全加固方案见 [docs/demo-public-hardening.md](docs/demo-public-hardening.md)。

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

如使用镜像版环境，可直接执行：

```bash
cd deploy
docker compose -f compose.mysql.yaml down -v --remove-orphans
docker compose -f compose.sqlite.yaml down -v --remove-orphans
```

## 部署文件

- `deploy/compose.mysql.yaml`：镜像版 MySQL 正式本地环境使用。
- `deploy/compose.sqlite.yaml`：镜像版 demo/local 轻量模式使用。
- `deploy/compose.local-build.yaml`：开发者本地源码构建与验收 override，供 Makefile 叠加使用。
- `deploy/compose.demo.yaml`：公网/共享环境轻量部署文件，使用发布镜像与 SQLite，并启用公网加固（`make demo-up`）。
- `deploy/examples/`：demo 反向代理配置示例（Nginx/Caddy）。
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
