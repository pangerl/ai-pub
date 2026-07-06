# 工程脚手架设计

## 1. 目标

本文定义第一版工程目录、配置、启动、migration、测试和本地 Compose 验收方式。

目标：

- 新开发者可以快速启动项目。
- MySQL 8 Compose 路径可一键跑通。
- 保持脚手架轻量，不引入复杂部署平台。

## 2. 非目标

- 不要求 Kubernetes。
- 不设计多实例部署。
- 不引入重型脚手架。
- 不要求复杂 CI/CD 平台。

## 3. 仓库目录

```text
.
├── cmd/
│   └── server/
├── internal/
│   ├── app/
│   ├── auth/
│   ├── config/
│   ├── crypto/
│   ├── domain/
│   ├── e2e/
│   ├── executor/
│   ├── httpapi/
│   ├── migration/
│   ├── notification/
│   ├── repository/
│   └── worker/
├── migrations/
│   ├── mysql/
│   └── sqlite/        # demo/local 轻量模式与 Go 单测 schema
├── deploy/
│   ├── compose.mysql.yaml
│   ├── compose.sqlite.yaml
│   ├── scripts/
│   └── sql/
├── Dockerfile
├── web/
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── docs/
└── README.md
```

## 4. 后端技术选型

- Go。
- `net/http` + 标准库 `ServeMux`。
- `database/sql`。
- 显式 SQL 和 repository。
- 显式 migration。
- REST API。

## 5. 前端技术选型

- React。
- TypeScript。
- Vite。
- 手写轻量路由状态。
- 直接通过 `fetch` 调用后端 API。
- Ant Design。

## 6. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_ENV` | `dev` | 运行环境 |
| `HTTP_ADDR` | `:8080` | 后端监听地址 |
| `DB_DIALECT` | `mysql` | `mysql` / `sqlite`；`sqlite` 仅用于 demo/local |
| `MYSQL_DSN` | 空 | MySQL DSN |
| `SQLITE_PATH` | `data/ai-pub.db` | SQLite demo/local 数据文件路径 |
| `APP_ENCRYPTION_KEY` | 空 | 凭据加密 key，生产必填 |
| `JWT_SECRET` | dev 默认值 | JWT key，生产必填 |
| `BOOTSTRAP_ADMIN_USERNAME` | `admin` | 首个管理员用户名 |
| `BOOTSTRAP_ADMIN_PASSWORD` | 空 | 首个管理员密码，首次创建或补齐密码时必填 |
| `MIGRATION_AUTO` | `true` | 启动自动 migration |
| `MIGRATION_CHECK_ONLY` | `false` | 只检查 migration |
| `WORKER_ENABLED` | `true` | 是否启动内置 Worker |

## 7. Migration 目录

```text
migrations/
  mysql/
    000001_init.up.sql
    000001_init.down.sql
  sqlite/
    000001_init.up.sql
    000001_init.down.sql
```

要求：

- MySQL migration 是生产和正式集成验收 migration。
- SQLite migration 服务 demo/local 轻量模式和 Go 单测，用于保持核心 schema 同构，不用于生产。
- PostgreSQL 接入时新增独立目录。
- 已发布 migration 不修改。
- 记录 checksum。
- 启动时先执行 migration，再启动 API/Worker。
- `manual` migration 不自动执行。

## 8. 启动命令

```bash
make compose-up
```

Compose 启动 MySQL 和一个 `app` 容器；`app` 同时提供 SPA 静态资源、REST API、启动迁移和内置 Worker。应用访问地址为 `http://127.0.0.1:18080/`，MySQL 仅在 Compose 网络中运行。

部署 YAML 集中在 `deploy/`：

- `deploy/compose.mysql.yaml`：MySQL 正式本地环境和完整验收。
- `deploy/compose.sqlite.yaml`：SQLite demo/local 轻量模式。

## 9. 测试命令

后端单元测试：

```bash
go test ./...
```

前端检查：

```bash
cd web
npm run lint
npm run build
```

代码级检查：

```bash
make verify
```

端到端 Compose 验收：

```bash
make compose-check
```

具体测试包名可在实现阶段调整，但必须保留等价验证命令。

## 10. 验收数据

`make compose-check` 通过一次性验收容器创建临时数据，覆盖：

- 一个管理员。
- 一个普通员工。
- 一个项目。
- 一个服务。
- test 和 prod 环境。
- 一个 Mock 部署目标。
- 成功、失败和部分成功 Mock 部署目标。
- 两个服务版本。
- 发布驳回、取消、确认、服务器组、重试、回滚、日志和事件流。

要求：

- 验收数据只存在于 Compose 验证 profile 的临时数据库中。
- 验收密码和 key 只能用于本地。
- 不写入真实生产凭据。

## 11. 容器化

第一版可提供 Dockerfile 和 docker-compose。

容器启动流程：

```text
加载配置
  -> 检查 migration
  -> 执行 safe migration
  -> 启动 API
  -> 提供 SPA 静态资源
  -> 启动内置 Worker
```

不要求：

- Kubernetes。
- 多副本部署。
- 复杂发布平台。

## 12. 验证要求

- 新开发者按 README 能启动后端和前端。
- MySQL 8 Compose 路径能完成 migration 和 Mock/Dry-run 发布。
- 前端 lint/build 通过。
- 后端核心测试通过。
