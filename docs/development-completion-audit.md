# 开发完成审查

审查日期：2026-06-20

## 结论

本地 MVP 已完成并冻结为基线。运行时统一为 MySQL 8；Docker Compose 会启动 MySQL、Go 后端、Nginx 前端和一次性验证容器；MySQL 不映射宿主机端口，只有前端映射到 `127.0.0.1:18080`。

本轮已通过 `make verify` 与 `make local-check`。后者从空 MySQL 数据库自动迁移并覆盖 Mock 发布的驳回、取消、成功、服务器组、partial/skipped、失败、回滚、日志和事件流。

浏览器人工验收已完成。验收中发现 `127.0.0.1:18080` 被本项目遗留的宿主机 Go 服务进程占用，导致页面返回后端 404；停止该进程并重建 `web` 容器后，地址恢复为 Nginx `200 OK`。随后在浏览器中完成初始化 Mock 配置、创建发布单、确认入队、等待执行成功、进入发布记录并查看服务器日志；成功发布记录显示 `success`，服务器日志显示对应 Mock 服务器为 `success`。

## 已完成

| 范围 | 结论 | 验证方式 |
| --- | --- | --- |
| MySQL 8 运行路径 | 已完成 | Compose 空库启动自动执行 7 条 migration |
| Web/API 发布闭环 | 已完成 | `make local-check` |
| 前端容器 | 已完成 | Nginx 提供 SPA 并反向代理 `/api`、`/healthz` |
| Go 单元测试与前端构建 | 已完成 | `make verify` |
| 真实 SSH 密码发布 | 已完成 | 非生产测试服务器先验证连接和脚本可执行性，再执行 `/home/dm/service/deploy.sh`；两次发布均为 success，实际脚本退出码为 0 |
| PostgreSQL | 未实现 | 仅保留 repository/migration 扩展边界，不承诺当前支持 |

## 外部集成专项待启动

- 真实企业微信机器人 webhook 发送。
- PostgreSQL 方言与 migration（非外部集成，但也不属于本地 MVP）。
- 高并发、多实例 Worker、复杂 RBAC、多租户和运行中紧急停止。

## 冻结结论

本地 MVP 的代码、文档和自动验收入口在本次基线提交后冻结。后续变更仅处理本地验收缺陷，或由外部集成专项分别启动；每次变更均运行 `make verify` 和 `make local-check`。
