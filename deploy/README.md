# 部署文件

本目录放置不同部署场景可直接选择的 Compose YAML，以及部署侧脚本和 SQL 说明。

## 选择部署 YAML

- `compose.mysql.yaml`：正式部署和完整集成验收使用，启动 MySQL 8 与 `app` 容器。
- `compose.sqlite.yaml`：demo/local 轻量模式使用，只启动 `app` 容器，SQLite 数据文件保存在卷中。

SQLite 模式仅用于演示和本地快速验证，不用于生产。开源用户如不选择 MySQL，后续应走 PostgreSQL 支持路径；PostgreSQL 不在当前运行时矩阵内。

常用命令：

```bash
docker compose -f deploy/compose.mysql.yaml up --build -d
docker compose -f deploy/compose.sqlite.yaml up --build -d
```

## 目录边界

- `scripts/`：部署项目时需要随 YAML 搭配使用的辅助脚本。
- `sql/`：部署侧初始化或运维 SQL 说明。
- 应用 schema 迁移仍以仓库根目录的 `migrations/{mysql,sqlite}` 为准，不在 `deploy/sql` 复制一份。
