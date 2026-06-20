# 开发计划

## 1. 目标

本文记录第一版本地 MVP 的完成边界，以及后续外部集成专项的启动范围。

开发计划不是 PRD，也不是替代技术设计。每个阶段都应引用对应设计文档，并提供可验证的完成标准。

## 2. 范围

本地 MVP 已完成：

- Web/API 发布闭环。
- MySQL 8 容器化运行路径。
- Mock/Dry-run 执行器。
- SSH 执行器的本地实现与 Mock 验证边界。
- 审计事件查询。
- 企业微信机器人 webhook 的配置、事件和本地发送记录能力。
- React 最小管理界面。

外部集成专项待启动：

- 真实 SSH 服务器连接、凭据验证和真实发布。
- 真实企业微信机器人 webhook 发送验证。

仍不纳入本轮范围：

- 高并发性能压测。
- 多实例 Worker。
- 复杂 RBAC。
- 多租户。
- 运行中紧急停止。
- AI Agent 完整接入。

## 3. 阶段划分

### M0 工程骨架

状态：已完成。

依赖文档：

- `engineering-scaffold-design.md`
- `backend-architecture-design.md`

交付：

- Go 后端骨架。
- React 前端骨架。
- 配置加载。
- 健康检查。
- MySQL migration runner。

验证：

- 后端启动成功。
- 前端启动成功。
- MySQL 8 从空库 migration 成功。
- `/healthz` 返回正常。

### M1 模型和基础配置

状态：已完成。

依赖文档：

- `domain-model-design.md`
- `api-design.md`

交付：

- 核心表结构。
- repository 基础。
- 项目、服务、版本、环境、服务器、部署目标 CRUD。
- 用户、角色、API Key 最小能力。

验证：

- 基础 CRUD 在 MySQL 8 下通过。
- API Key 明文只返回一次。

### M2 发布单和 Preflight

状态：已完成。

依赖文档：

- `domain-model-design.md`
- `api-design.md`
- `backend-architecture-design.md`

交付：

- 发布单创建。
- 幂等键。
- 发布策略合并。
- preflight。
- 确认、驳回、queued 前取消。
- 审计事件。

验证：

- 非生产本人确认。
- 生产管理员确认。
- 冻结 block。
- 同服务同环境 running 阻断。
- 关键动作写事件。

### M3 Mock/Dry-run 发布闭环

状态：已完成。

依赖文档：

- `backend-architecture-design.md`
- `frontend-ia-design.md`

交付：

- 执行器 contract。
- Mock/Dry-run 执行器。
- Worker 领取任务。
- 服务器日志。
- 状态聚合。
- 发布单详情页基础闭环。

验证：

- MySQL 8 下 Mock/Dry-run 发布成功。
- Mock/Dry-run 失败能回写错误摘要。
- `partial` 按失败展示。
- 事件流完整。

### M4 SSH 执行器

状态：本地实现已完成；真实 SSH 验证转入外部集成专项。

依赖文档：

- `backend-architecture-design.md`
- `api-design.md`

交付：

- SSH 凭据配置。
- SSH 连接测试。
- SSH 执行器。
- 日志采集和脱敏。

本地验证：

- Mock/Dry-run 发布闭环覆盖 Worker、状态聚合、错误摘要和日志脱敏边界。
- 真实 SSH 连接、认证、脚本和超时场景的验收转入外部集成专项。

### M5 前端完整第一版

状态：已完成。

依赖文档：

- `frontend-ia-design.md`
- `api-design.md`

交付：

- 工作台。
- 发布中心。
- 创建发布单。
- 发布单详情。
- 发布记录。
- 基础配置页面。
- 发布策略页面。

验证：

- 用户通过 Web 完成一次发布。
- 管理员通过 Web 完成部署目标和策略配置。
- 状态标签、warning、block、partial 展示正确。

### M6 通知和运维增强

状态：本地配置、事件和失败不阻塞主流程已完成；真实 webhook 发送验证转入外部集成专项。

依赖文档：

- `notification-design.md`
- `backend-architecture-design.md`

交付：

- 企业微信机器人 webhook 配置。
- 生产待确认通知。
- 发布失败通知。
- 回滚申请通知。
- 运维摘要和基础指标。

本地验证：

- 通知发送记录与发布事件可查询，通知失败不阻塞主流程。
- 运行摘要可定位队列、执行器和通知问题。
- 真实 webhook 发送成功的验收转入外部集成专项。

### M7 MySQL 容器化验证

状态：已完成。

依赖文档：

- `engineering-scaffold-design.md`
- `domain-model-design.md`

交付：

- MySQL migration。
- Docker Compose MySQL 8、后端、前端和验证容器。
- MySQL 8 基础发布闭环验证。

验证：

- `docker compose up --build` 可启动 MySQL、后端和前端。
- MySQL 从空库自动迁移成功。
- `make local-check` 在容器内通过 Mock/Dry-run 发布闭环。

## 4. 最小验收路径

1. 使用 Docker Compose 启动 MySQL、后端和前端。
2. 创建项目、服务、环境、服务版本、服务器和部署目标。
3. 创建非生产发布单。
4. 执行 preflight。
5. 本人确认发布。
6. Mock/Dry-run 执行成功。
7. 查看发布记录、服务器日志和事件流。
8. 制造一次 Mock/Dry-run 失败，确认失败原因展示正确。
9. 创建回滚发布单并完成 Mock/Dry-run 执行。
10. 在本地确认通知配置、发送记录与发布事件可查询；真实 webhook 发送留待外部集成专项。
11. 执行 `make local-check`，验证 MySQL migration 和 Mock/Dry-run 基础发布闭环。

## 5. 验证命令要求

实现阶段至少提供等价命令：

```bash
go test ./...
cd web && npm run lint && npm run build
make verify
make local-check
```

包名可调整，但 README 和本文件必须同步更新。

## 6. 交付检查

- 本地 MVP 验收项通过：MySQL 8 容器路径、Web 发布闭环、Mock/Dry-run、日志、事件和回滚均可重复验证。
- `make verify` 与 `make local-check` 均通过。
- 真实 SSH 发布与真实企业微信 webhook 发送不作为本地 MVP 冻结条件，转入外部集成专项。
- 未混入 P2 能力作为本地 MVP 强依赖。
