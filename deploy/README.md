# 部署文件

本目录放置不同部署场景可直接选择的 Compose YAML，以及部署侧脚本和 SQL 说明。

## 选择部署 YAML

- `compose.mysql.yaml`：镜像版 MySQL 正式本地环境使用，启动 MySQL 8 与 `app` 容器。
- `compose.sqlite.yaml`：镜像版 demo/local 轻量模式使用，只启动 `app` 容器，SQLite 数据文件保存在卷中。
- `compose.local-build.yaml`：开发者本地源码构建与验收 override，供 Makefile 叠加使用。

SQLite 模式仅用于演示和本地快速验证，不用于生产。开源用户如不选择 MySQL，后续应走 PostgreSQL 支持路径；PostgreSQL 不在当前运行时矩阵内。
镜像版 MySQL 和 SQLite 默认使用 `hxjagf/ai-pub:latest`；如需验证指定镜像，可设置 `AI_PUB_IMAGE`。

镜像版启动：

```bash
docker compose -f deploy/compose.mysql.yaml up -d
docker compose -f deploy/compose.sqlite.yaml up -d
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

`compose.demo.yaml` 是 demo 公网部署的加固 override，叠加在 `compose.sqlite.yaml` 之上，使用发布镜像 `hxjagf/ai-pub:latest`（不叠加 `compose.local-build.yaml`：demo 须与用户最终镜像一致，兼作发布镜像可用性的活体验证）：

```bash
# 1. 生成强随机密钥写入 .env（已 gitignore）
cp .env.example .env
#   用 openssl rand -hex 32 生成 APP_ENCRYPTION_KEY / JWT_SECRET，设强 BOOTSTRAP_ADMIN_PASSWORD

# 2. 启动（--env-file .env 必填：docker compose -f 时 project directory 是 deploy/，不会默认加载项目根 .env）
docker compose --env-file .env -f deploy/compose.sqlite.yaml -f deploy/compose.demo.yaml up -d

# 3. 反向代理转发 TLS 到 127.0.0.1:18080（参考 deploy/examples/Caddyfile，需含 caddy-ratelimit 插件，见 Caddy.Dockerfile）
```

加固要点：

- `EXECUTOR_SSH_DISABLED=true` + `EXECUTOR_K8S_DISABLED=true`：worker 只注册 mock，消除 SSH 跳板与 K8s 外联。
- 三个密钥用 `${VAR:?msg}` 强制非空，未设或为空则 compose 报错不启动（dev 模式下空值会回退到公开默认密钥，见 `internal/config/config.go` 与 `internal/crypto/secret.go`）。
- `cap_drop: [ALL]` + `no-new-privileges` + `read_only` + `tmpfs /tmp`：容器提权加固。
- 端口绑 `127.0.0.1:18080`，公网访问走反向代理 + TLS。

详见 `docs/demo-public-hardening.md`。
