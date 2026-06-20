# 开发完成审查

审查日期：2026-06-20

## 结论

本地 MVP 已完成并冻结为基线。运行时统一为 MySQL 8；Docker Compose 会启动 MySQL、Go 后端、Nginx 前端和一次性验证容器；MySQL 不映射宿主机端口，只有前端映射到 `127.0.0.1:18080`。

本轮已通过 `make verify` 与 `make local-check`。后者从空 MySQL 数据库自动迁移并覆盖 Mock 发布的驳回、取消、成功、服务器组、partial/skipped、失败、回滚、日志和事件流。

浏览器人工验收已按 `local-verification.md` 启动：前端容器在 Compose 网络内返回 HTTP 200；但当前 Codex 内置浏览器对 `127.0.0.1:18080` 和 `localhost:18080` 均在请求前报 `ERR_BLOCKED_BY_CLIENT`，没有进入应用页面。该限制属于本次验收自动化运行环境，不是应用运行或接口缺陷；宿主机人工浏览器可按清单补做同一 UI 路径。

## 已完成

| 范围 | 结论 | 验证方式 |
| --- | --- | --- |
| MySQL 8 运行路径 | 已完成 | Compose 空库启动自动执行 7 条 migration |
| Web/API 发布闭环 | 已完成 | `make local-check` |
| 前端容器 | 已完成 | Nginx 提供 SPA 并反向代理 `/api`、`/healthz` |
| Go 单元测试与前端构建 | 已完成 | `make verify` |
| PostgreSQL | 未实现 | 仅保留 repository/migration 扩展边界，不承诺当前支持 |

## 外部集成专项待启动

- 真实 SSH 服务器发布与密码登录。
- 真实企业微信机器人 webhook 发送。
- PostgreSQL 方言与 migration（非外部集成，但也不属于本地 MVP）。
- 高并发、多实例 Worker、复杂 RBAC、多租户和运行中紧急停止。

## 冻结结论

本地 MVP 的代码、文档和自动验收入口在本次基线提交后冻结。后续变更仅处理本地验收缺陷，或由外部集成专项分别启动；每次变更均运行 `make verify` 和 `make local-check`。
