# MVP 完成审查

审查日期：2026-06-26

## 结论

本地 MVP 已完成，可作为上线准备基线。运行时只支持 MySQL 8；Docker Compose 从空数据库启动 MySQL、Go 后端、Nginx 前端和一次性验证容器。MySQL 与后端不映射宿主机端口，前端地址为 `http://127.0.0.1:18080/`。

本次审查中 `make verify` 已通过；人工测试由项目维护者确认已通过。`make compose-check` 本次因本机 Docker/OrbStack API 无响应未完成，应在上线前恢复 Docker 后补跑。因此“已完成”仅指下表列出的本地 MVP 闭环；不把尚未纳入本地自动验收的扩展能力算入完成范围。

## 本次验证

| 命令 | 结果 | 覆盖范围 |
| --- | --- | --- |
| `make verify` | 通过 | Go 测试、前端 lint 与生产构建 |
| `make compose-check` | 本次未完成 | 本机 Docker/OrbStack API 在 `docker compose --profile verify down -v --remove-orphans` 阶段无响应；上线前必须补跑 |
| 人工测试 | 通过 | 由项目维护者确认 MVP 阶段人工测试已通过 |

容器化检查覆盖管理员登录、个人访问密钥归属、驳回、取消、成功发布、服务器组、`partial`/`skipped`、失败、重新发布、回滚、发布记录、服务器日志和事件流。前端构建仍会提示单个 JavaScript chunk 超过 500 kB；这是非阻断的性能优化项，不影响本次验收结果。

## 已完成范围

| 范围 | 状态 | 证据 |
| --- | --- | --- |
| MySQL 8 容器运行与 migration | 已完成，需上线前复验 | 历史 Compose 基线已通过；本次因 Docker/OrbStack API 无响应未完成复验 |
| 登录、会话与 API Key 权限边界 | 已完成 | Go 测试与 Compose 登录/访问密钥检查通过 |
| 基础配置与发布保护 | 已完成 | 项目、服务、版本、环境、服务器、服务器组、部署目标 CRUD；生产管理员确认、非生产发起人确认、环境冻结均已实现 |
| Web/API 发布闭环 | 已完成，需上线前复验 | preflight、确认、队列、Mock/Dry-run、状态聚合、日志、事件、重试与回滚均由 Compose 检查覆盖；本次复验被本机 Docker 状态阻塞 |
| 前端容器与日常操作界面 | 已完成 | Nginx 提供 SPA 和 `/api` 反向代理；发布、配置、系统与个人访问密钥入口已实现 |
| 通知与 SSH 基础能力 | 已完成 | 企业微信机器人配置/投递记录和 SSH 密钥、密码执行器均已有单元测试与实现 |

## 专项验证记录

真实非生产 SSH 密码发布和真实企业微信机器人 webhook 曾完成专项验证，详见 `local-verification.md`。这两项依赖外部系统，未包含在 2026-06-22 的离线 Compose 自动验收中；后续修改相关代码时应重新执行对应专项。

## 不在 MVP 范围

- PostgreSQL 方言、migration 与 repository 适配。
- 高并发压测、多实例 Worker、复杂 RBAC、多租户。
- 运行中紧急停止和更复杂的发布编排。

## 维护要求

每次改变本地 MVP 行为后，运行：

```bash
make verify
make compose-check
```

如调整初始 migration，先执行 `make compose-down` 删除现有 Compose 数据卷，再从空库复验。

当前上线前阻塞项：恢复本机 Docker/OrbStack API 后补跑 `make compose-check`，确认从空 MySQL 数据库启动和发布闭环仍通过。
