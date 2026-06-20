# AI Agent 接入方案

## 1. 目标

AI Agent 接入必须在 Web/API + React 最小管理界面跑通基础发布执行闭环之后进行。

目标：

- Agent 作为受限调用方复用同一套发布流程。
- Agent 可帮助用户查询候选、执行 preflight、创建发布单、查询状态和总结结果。
- 生产发布必须等待管理员确认。

## 2. 非目标

- Agent 不绕过发布执行流程。
- Agent 不独立确认生产发布。
- Agent 不直接操作执行器。
- Agent 不使用第二套发布状态机。

## 3. 调用身份

Agent 调用必须记录：

- `actor_type=ai_agent`。
- 代表的授权用户。
- 使用的服务账号或 API Key。
- 来源上下文。
- 幂等键。

所有关键动作写入 `ReleaseEvent`。

## 4. 可用能力

Agent 可调用：

- 查询服务候选。
- 查询环境候选。
- 查询服务版本。
- 查询部署目标。
- 发布意图 preflight。
- 创建发布单。
- 查询发布单 summary。
- 查询发布记录和服务器日志摘要。
- 创建回滚发布单。

Agent 不可调用：

- 生产发布管理员确认。
- 运行中紧急停止。
- 直接执行 SSH。
- 直接修改服务器部署状态。

## 5. 推荐流程

```text
用户提出发布意图
  -> Agent 查询服务候选
  -> Agent 查询环境和版本
  -> Agent 查询部署目标
  -> Agent 执行 release intent preflight
  -> Agent 汇报摘要和风险
  -> 用户确认创建发布单
  -> Agent 创建发布单
  -> 非生产本人确认或生产等待管理员确认
  -> Agent 轮询 summary
  -> Agent 汇报结果
```

## 6. Agent API 薄封装

Agent API 是业务 API 的薄封装，不是第二套流程。

建议接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/agent/services:search` | 查询服务候选 |
| `GET` | `/agent/environments:search` | 查询环境候选 |
| `GET` | `/agent/services/{id}/versions` | 查询版本 |
| `GET` | `/agent/deployment-targets` | 查询部署目标 |
| `POST` | `/agent/release-intents/preflight` | 发布意图预检 |
| `POST` | `/agent/release-requests` | 创建发布单 |
| `GET` | `/agent/release-requests/{id}/summary` | 查询摘要 |
| `POST` | `/agent/release-requests/{id}/rollback` | 创建回滚发布单 |

约束：

- 所有写接口必须支持幂等键。
- 多候选时必须返回候选，不替用户猜测。
- 创建发布单前必须 preflight。
- Agent API 内部复用业务 API 和应用服务。

## 7. Summary 响应

Agent summary 应面向终端友好。

字段：

| 字段 | 说明 |
|------|------|
| `release_request_id` | 发布单 |
| `status` | 当前状态 |
| `service` | 服务 |
| `environment` | 环境 |
| `version` | 版本 |
| `next_action` | 下一步 |
| `summary` | 一句话摘要 |
| `errors` | 错误摘要 |
| `server_results` | 服务器结果概览 |

示例：

```json
{
  "release_request_id": "rel_1",
  "status": "pending_confirm",
  "next_action": "wait_admin_confirm",
  "summary": "生产发布已创建，等待管理员确认。"
}
```

## 8. 权限和门禁

- Agent 不能绕过 API Key scope。
- Agent 不能绕过生产管理员确认。
- Agent 不能绕过冻结。
- Agent 不能忽略 preflight block。
- Agent 创建回滚发布单也必须走同一套策略和确认。

## 9. Skill 或工具封装建议

后续 skill 应提供：

- 查询服务。
- 查询版本。
- 查询环境。
- 发布 preflight。
- 创建发布单。
- 查询发布结果。
- 创建回滚发布单。

Skill 文案应强调：

- 遇到多个候选必须让用户确认。
- 生产发布需要管理员确认。
- Agent 不能直接执行真实发布。

## 10. 验证要求

- Agent 可发起非生产发布单。
- Agent 创建发布单前执行 preflight。
- Agent 不能确认生产发布。
- Agent 查询 summary 能返回状态、下一步和失败原因。
- Agent 回滚发布单复用同一套回滚流程。
