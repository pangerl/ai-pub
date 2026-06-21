# 发布系统技术文档拆分说明

## 1. 目的

本文基于 `project-requirements.md` 拆分后续需要编写的技术设计文档。

这些文档不是开发排期，也不是代码任务清单，而是用于在正式开发前把关键技术边界讲清楚，避免后续实现时反复猜测：

- 模型怎么设计。
- 状态如何流转。
- 后端分层和事务边界在哪里。
- API 如何表达发布流程。
- 前端页面如何围绕用户任务组织。
- MySQL 8 容器化运行路径如何落地。

## 2. 拆分原则

- 每份文档只解决一个清晰问题，避免写成大而全的技术百科。
- 第一版围绕功能闭环，不展开高并发、多实例、复杂 RBAC、多租户、运行中紧急停止等非目标。
- 技术文档应能直接指导实现，必须包含关键约束、接口边界、验收方式和需要避免的方向。
- 先写会影响数据结构、API 和流程的文档，再写页面和工程细节文档。

## 3. 文档清单

| 顺序 | 文档 | 目的 | 优先级 |
|------|------|------|--------|
| 1 | `docs/domain-model-design.md` | 明确领域模型、表结构、状态机和事件模型 | P0 |
| 2 | `docs/backend-architecture-design.md` | 明确 Go 后端分层、事务边界、队列调度和执行器 contract | P0 |
| 3 | `docs/api-design.md` | 明确 Web/API/外部调用接口 | P0 |
| 4 | `docs/frontend-ia-design.md` | 明确前端导航、页面、详情页、状态展示和主要交互 | P0 |
| 5 | `docs/engineering-scaffold-design.md` | 明确目录结构、配置、migration、测试和本地开发方式 | P0 |
| 6 | `docs/notification-design.md` | 明确企业微信机器人 webhook 和通知扩展点 | P1 |
| 7 | `docs/development-plan.md` | 在技术设计稳定后拆分可执行开发任务和验收命令 | P1 |

## 4. 文档一：领域模型设计

文件：

- `docs/domain-model-design.md`

目标：

- 把 PRD 中的核心业务对象落成可实现的数据模型。
- 明确对象关系、状态流转、唯一约束和事件记录。
- 为 migration、repository、API 和前端展示提供统一依据。

必须覆盖：

- 核心实体：
  - `Project`
  - `Service`
  - `ServiceVersion`
  - `Environment`
  - `Server`
  - `ServerGroup`
  - `DeploymentTarget`
  - `ReleaseRequest`
  - `DeployRecord`
  - `ServerDeployLog`
  - `ServerDeploymentState`
  - `ReleaseEvent`
  - `User`
  - `ApiKey`
  - `NotificationConfig`
  - `NotificationDelivery`
- 实体关系图。
- 每个实体的关键字段、可空性、唯一约束和索引建议。
- `ReleaseRequest` 状态机。
- `DeployRecord` 状态机。
- `ServerDeployLog` 状态机。
- `partial`、`skipped`、`failed`、`success` 的聚合规则。
- 环境的生产确认、冻结开关和同服务同环境运行中发布阻断规则。
- 回滚发布单与原发布单的关联方式。
- 审计事件字段和查询维度。
- 用户、角色、API Key 和调用身份的最小模型边界。
- 企业微信机器人 webhook 配置和通知发送记录的最小持久化模型。
- MySQL 8 字段类型和后续 PostgreSQL 适配边界。

明确不做：

- 不设计不可篡改审计账本、哈希链或外部审计存储。
- 不设计复杂组织权限模型。
- 不设计多租户字段。
- 不为高并发场景额外设计复杂锁模型。

完成标准：

- 后续开发可以直接据此编写第一版 migration。
- 状态机图能覆盖发布、确认、取消、执行、失败、部分成功、回滚。
- 每个核心表都有主键、关键外键、唯一约束和必要索引说明。

## 5. 文档二：Go 后端技术架构设计

文件：

- `docs/backend-architecture-design.md`

目标：

- 明确 Go 后端如何组织代码、承载业务流程和隔离外部执行器。
- 明确事务边界、队列领取、Worker、状态修复和执行器 contract。

必须覆盖：

- 后端目录结构：
  - `cmd/server`
  - `internal/httpapi`
  - `internal/app`
  - `internal/domain`
  - `internal/repository`
  - `internal/executor`
  - `internal/worker`
  - `internal/audit`
  - `internal/migration`
  - `internal/config`
- 请求进入 API 后到应用服务、repository、事件写入的调用链。
- 发布单创建、preflight、确认、入队、执行、状态回写的服务边界。
- MySQL 8 下的事务策略。
- PostgreSQL 后续接入的 repository 隔离策略。
- 内置 Worker 的领取、心跳、超时和僵尸任务修复。
- Mock/Dry-run 和 SSH 执行器 contract。
- 错误分类和脱敏规则。
- 配置加载、密钥读取和敏感信息处理。

明确不做：

- 不引入微服务拆分。
- 不设计多实例 Worker 抢占。
- 不设计复杂插件市场。
- 不把 SSH 细节写入发布单领域模型。

完成标准：

- 开发者能据此搭建 Go 后端骨架。
- 每个核心业务流程都有明确所属 app service。
- 每个需要事务保护的写入流程都有事务边界说明。
- 执行器 contract 足够支撑 Mock/Dry-run 和 SSH。

## 6. 文档三：API 设计

文件：

- `docs/api-design.md`

目标：

- 明确第一版 REST API 的资源、请求、响应、错误码和鉴权要求。
- 为 Web 前端和 CI/CD 提供统一接口基础。

必须覆盖：

- API 前缀，例如 `/api/v1`。
- 通用响应结构和错误结构。
- 鉴权方式和调用身份。
- API Key scope。
- 幂等键规则。
- 基础配置 API：
  - 项目
  - 服务
  - 版本
  - 环境
  - 服务器
  - 服务器组
  - 部署目标
- 发布流程 API：
  - 创建发布单
  - 执行 preflight
  - 确认
  - 驳回
  - 取消 queued 发布
  - 查询发布单
  - 查询发布记录
  - 查询服务器日志
  - 创建回滚发布单
- 环境发布保护 API。
- 用户、角色和 API Key API。
- 企业微信通知配置 API。
- 运维接口：
  - 健康检查
  - 运行摘要
  - migration 状态

明确不做：

- 不设计运行中紧急停止接口。
- 不设计复杂批量编排接口。

完成标准：

- 前端可以只依赖该文档完成 API client。
- 后端可以据此编写 handler。
- 每个关键写接口都有幂等、鉴权和事件写入要求。

## 7. 文档四：前端信息架构设计

文件：

- `docs/frontend-ia-design.md`

目标：

- 明确 React 最小管理界面如何组织页面和交互。
- 确保前端围绕用户任务，而不是围绕数据库表堆页面。

必须覆盖：

- 导航结构。
- 页面清单和优先级：
  - P0 页面必须支持第一版发布闭环。
  - P1 页面需要在信息架构中预留位置，但可按开发计划后置实现。
- P0 页面：
  - 工作台
  - 发布中心
  - 创建发布单
  - 发布单详情
  - 发布记录
  - 服务与版本
  - 环境与服务器
  - 部署目标
- P1 页面：
  - API Key 管理
  - 通知配置
- P0 工作台只要求覆盖待确认发布、运行中发布、最近失败和发布处理入口；基础 Dashboard 统计卡片和更完整统计视图按 P1 推进。
- 发布单详情页信息组织：
  - 基本信息
  - preflight
  - 环境保护和确认
  - 发布记录
  - 服务器日志
  - 事件流
- 状态标签和视觉规则：
  - `pending_confirm`
  - `queued`
  - `running`
  - `success`
  - `failed`
  - `partial`
  - `skipped`
- 错误、warning、block 的展示方式。
- `partial` 和“混合版本”的展示方式。
- 轮询策略和刷新入口。

明确不做：

- 不做营销页。
- 不做复杂可视化大屏。
- 不按数据库表简单堆导航。
- 不在第一版引入过重前端状态架构。

完成标准：

- 用户能按页面流完成一次发布。
- 管理员能完成第一版必需配置。
- 每个状态都有清晰展示和下一步动作。

## 8. 文档五：工程脚手架设计

文件：

- `docs/engineering-scaffold-design.md`

目标：

- 明确项目目录、开发命令、配置项、migration、测试和本地 demo 方式。
- 让后续开发者可以按文档快速启动项目。

必须覆盖：

- 仓库目录结构。
- 后端启动命令。
- 前端启动命令。
- MySQL 8 Compose 配置。
- 环境变量清单。
- migration 文件组织。
- demo 数据加载方式。
- 测试命令：
  - 后端单元测试
  - repository/migration 测试
  - 前端 lint/build
  - MySQL 8 Compose 端到端 demo
- Docker 或容器化启动方式。

明确不做：

- 不引入复杂部署平台。
- 不要求 Kubernetes。
- 不要求多实例部署。
- 不把开发环境配置做成重型脚手架。

完成标准：

- 新开发者按文档能启动后端和前端。
- MySQL 8 Compose 路径能一键跑通 migration 和 demo。

## 9. 文档六：通知设计

文件：

- `docs/notification-design.md`

目标：

- 明确第一版企业微信机器人 webhook 通知如何实现。
- 保留后续通知渠道扩展点。

必须覆盖：

- 通知事件模型。
- 默认通知事件：
  - 生产待确认
  - 发布失败
  - 回滚申请
- 企业微信机器人 webhook 配置项。
- 消息模板。
- 签名和密钥处理。
- 通知发送、失败记录和重试策略。
- 通知失败不阻塞主发布流程。
- 后续渠道扩展接口。

明确不做：

- 不做企业微信应用消息。
- 不做复杂通知订阅中心。
- 不把通知逻辑散落到发布状态判断里。

完成标准：

- 企业微信机器人 webhook 能用于第一版关键通知。
- 新增后续通知渠道时可以复用通知事件和发送记录。

## 10. 文档七：开发计划

文件：

- `docs/development-plan.md`

目标：

- 在前六份技术设计文档稳定后，再拆分可执行开发任务。
- 每个任务都应有输入文档、实现范围、验收标准和验证命令。

必须覆盖：

- 开发阶段划分。
- 每阶段依赖的技术设计文档。
- 每阶段交付物。
- 每阶段验证命令。
- MySQL 8 Compose 路径验收顺序。
- 第一版最终验收 checklist。

完成标准：

- 开发者可以按该计划逐步实现，而不需要重新解释 PRD。
- 每个任务都能独立验收。
- 不把 P2 能力混入第一版必交付范围。

## 11. 建议编写顺序

1. `docs/domain-model-design.md`
2. `docs/backend-architecture-design.md`
3. `docs/api-design.md`
4. `docs/frontend-ia-design.md`
5. `docs/engineering-scaffold-design.md`
6. `docs/notification-design.md`
7. `docs/development-plan.md`

说明：

- 前三份文档决定数据结构、事务边界和接口，不宜跳过。
- 前端信息架构应在 API 初稿后编写，便于对齐页面所需数据。
- 工程脚手架设计应在后端架构和数据库策略明确后编写。
- 开发计划必须放在技术设计之后，避免任务拆分依赖未决设计。
