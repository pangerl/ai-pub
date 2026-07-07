# 部署文件

本目录放置不同部署场景可直接选择的 Compose YAML，以及部署侧脚本和 SQL 说明。

## 选择部署 YAML

- `compose.mysql.yaml`：镜像版 MySQL 正式本地环境使用，启动 MySQL 8 与 `app` 容器。
- `compose.sqlite.yaml`：镜像版 demo/local 轻量模式使用，只启动 `app` 容器，SQLite 数据文件保存在卷中。
- `compose.demo.yaml`：公网/共享环境轻量部署使用，基于发布镜像与 SQLite，并启用公网加固。
- `compose.local-build.yaml`：开发者本地源码构建与验收 override，供 Makefile 叠加使用。

SQLite 模式仅用于演示和本地快速验证，不用于生产。开源用户如不选择 MySQL，后续应走 PostgreSQL 支持路径；PostgreSQL 不在当前运行时矩阵内。
镜像版 MySQL 和 SQLite 默认使用 `hxjagf/ai-pub:latest`；如需验证指定镜像，可设置 `AI_PUB_IMAGE`。

## 环境变量

真实部署前先准备 `deploy/.env`。本地临时体验可使用 compose 内置默认值；共享环境、公网环境或长期使用时，必须替换 `.env` 中的 `APP_ENCRYPTION_KEY`、`JWT_SECRET` 和 `BOOTSTRAP_ADMIN_PASSWORD`。

```bash
cd deploy
cp ../.env.example .env
# 用 openssl rand -hex 32 生成 APP_ENCRYPTION_KEY / JWT_SECRET，设置强 BOOTSTRAP_ADMIN_PASSWORD
```

部署方式由具体的 `compose.*.yaml` 决定，通常不需要在 `.env` 中配置数据库类型、DSN、Worker 或执行器开关。

## 镜像版启动

```bash
cd deploy
docker compose -f compose.mysql.yaml up -d
docker compose -f compose.sqlite.yaml up -d
```

开发者源码构建：

```bash
make compose-up
make compose-sqlite-up
```

## 目录边界

- `scripts/`：部署项目时需要随 YAML 搭配使用的辅助脚本。
- `sql/`：部署侧初始化或运维 SQL 说明。
- 应用 schema 迁移仍以仓库根目录的 `migrations/{mysql,sqlite}` 为准，不在 `deploy/sql` 复制一份。

## Demo 公网部署

`compose.demo.yaml` 是公网/共享环境轻量部署的自包含 Compose 文件，使用发布镜像 `hxjagf/ai-pub:latest` 与 SQLite，并内置公网加固项。官方 demo 站点也使用这条真实用户部署路径，兼作发布镜像可用性的活体验证。

```bash
# 1. 进入部署目录，让 Compose 默认读取当前目录的 .env
cd deploy

# 2. 首次部署时复制项目环境变量模板，并替换三个必填安全值（已 gitignore）
cp ../.env.example .env
#   用 openssl rand -hex 32 生成 APP_ENCRYPTION_KEY / JWT_SECRET，设强 BOOTSTRAP_ADMIN_PASSWORD

# 3. 启动
docker compose -f compose.demo.yaml up -d

# 4. 反向代理转发 TLS 到 127.0.0.1:18080（参考 examples/Caddyfile，需含 caddy-ratelimit 插件，见 Caddy.Dockerfile）
```

相对 `compose.sqlite.yaml`，`compose.demo.yaml` 额外启用以下公网加固：

- `EXECUTOR_SSH_DISABLED=true` + `EXECUTOR_K8S_DISABLED=true`：worker 只注册 mock，消除 SSH 跳板与 K8s 外联。
- 三个安全值用 `${VAR:?msg}` 强制非空，未设或为空则 compose 报错不启动（dev 模式下空值会回退到公开默认密钥，见 `internal/config/config.go` 与 `internal/crypto/secret.go`）。
- `cap_drop: [ALL]` + `no-new-privileges` + `read_only` + `tmpfs /tmp`：容器提权加固。
- 端口绑 `127.0.0.1:18080`，公网访问走反向代理 + TLS。

详见 `docs/demo-public-hardening.md`。
