# 工程脚手架设计

## 1. 目标

本文定义第一版工程目录、配置、启动、migration、测试和本地 demo 方式。

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
│   ├── audit/
│   ├── config/
│   ├── crypto/
│   ├── domain/
│   ├── executor/
│   ├── httpapi/
│   ├── migration/
│   ├── notification/
│   ├── repository/
│   └── worker/
├── migrations/
│   └── mysql/
├── compose.yaml
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
- `net/http` + 轻量路由框架，例如 `chi`。
- `database/sql`。
- 显式 SQL 和 repository。
- 显式 migration。
- REST API + OpenAPI。

## 5. 前端技术选型

- React。
- TypeScript。
- Vite。
- React Router。
- TanStack Query。
- Ant Design。

## 6. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_ENV` | `dev` | 运行环境 |
| `HTTP_ADDR` | `:8080` | 后端监听地址 |
| `MYSQL_DSN` | 空 | MySQL DSN |
| `APP_ENCRYPTION_KEY` | 必填 | 凭据加密 key |
| `JWT_SECRET` | dev 默认值 | JWT key，生产必填 |
| `MIGRATION_AUTO` | `true` | 启动自动 migration |
| `MIGRATION_CHECK_ONLY` | `false` | 只检查 migration |
| `WORKER_ENABLED` | `true` | 是否启动内置 Worker |

## 7. Migration 目录

```text
migrations/
  mysql/
    000001_init.up.sql
    000001_init.down.sql
```

要求：

- 当前仅维护 MySQL migration；PostgreSQL 接入时新增独立目录。
- 已发布 migration 不修改。
- 记录 checksum。
- 启动时先执行 migration，再启动 API/Worker。
- `manual` migration 不自动执行。

## 8. 启动命令

```bash
docker compose up --build -d
```

MySQL 与后端仅在 Compose 网络中运行，前端访问地址为 `http://127.0.0.1:18080/`。

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

Migration 检查：

```bash
make verify
```

端到端 demo：

```bash
make local-check
```

具体测试包名可在实现阶段调整，但必须保留等价验证命令。

## 10. Demo 数据

MySQL Compose demo 数据应包含：

- 一个管理员。
- 一个普通员工。
- 一个项目。
- 一个服务。
- test 和 prod 环境。
- 一个 Mock 部署目标。
- 一个 SSH 示例部署目标。
- 两个服务版本。

要求：

- demo 数据加载可重复执行。
- demo 密码和 key 只能用于本地。
- demo 不写入真实生产凭据。

## 11. 容器化

第一版可提供 Dockerfile 和 docker-compose。

容器启动流程：

```text
加载配置
  -> 检查 migration
  -> 执行 safe migration
  -> 启动 API
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
