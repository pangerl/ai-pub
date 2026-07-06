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
