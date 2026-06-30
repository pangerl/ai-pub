---
name: ai-pub-release
description: 通过 AI Pub Agent API 创建、预检、确认和查询服务发布单。Use when the user asks an agent to publish or deploy a specific service version, create a release request, run AI Pub preflight, confirm a non-production release when permitted, wait for production admin confirmation, or check release status/log summary through AI Pub.
---

# AI Pub Release

## 核心原则

使用 AI Pub 的 Agent API 作为薄封装入口。不要绕过发布单、preflight、确认、队列、执行记录和审计事件；不要直接操作执行器或服务器。

生产发布必须等待管理员确认。即使 API Key 具备 `release:confirm`，也不要尝试让 Agent 独立确认生产发布。

多候选时不要替用户猜测。服务、环境、版本或部署目标返回多个候选时，先把候选列给用户并要求明确选择。

## 准备

要求环境变量：

```bash
export AI_PUB_BASE_URL="http://127.0.0.1:18080"
export AI_PUB_API_KEY="..."
```

推荐 API Key scopes：

- 查询和创建发布：`inventory:read`、`release:create`、`release:read`
- 允许非生产确认：再加 `release:confirm`
- 回滚发布单：再加 `release:rollback`

详细接口和响应字段见 `references/api.md`。执行 API 调用时优先使用 `scripts/ai_pub_release.py`，避免手写请求。

## 发版流程

1. 从用户意图提取服务、环境、版本和可选发布说明。
2. 查询服务候选：

```bash
python3 skills/ai-pub-release/scripts/ai_pub_release.py services --query "订单服务"
```

3. 查询环境、版本和部署目标候选；任何一步出现多个候选都先让用户选择。
4. 执行发布意图 preflight：

```bash
python3 skills/ai-pub-release/scripts/ai_pub_release.py preflight \
  --service-id svc_x --environment-id env_x --version-id ver_x --target-id target_x
```

5. 如果 preflight 返回 `block`，停止创建发布单并向用户解释阻断项。
6. 创建发布单。必须提供幂等键和用户确认后的意图摘要：

```bash
python3 skills/ai-pub-release/scripts/ai_pub_release.py create \
  --service-id svc_x --environment-id env_x --version-id ver_x --target-id target_x \
  --idempotency-key "agent:order-api:test:v1.2.3:20260630" \
  --intent-summary "发布订单服务 v1.2.3 到测试环境"
```

7. 创建后根据 `next_action` 处理：

- `self_confirm`：只有用户明确要求“创建并发布/确认发布”时，且权限允许，才调用 `confirm`。
- `admin_confirm`：告诉用户发布单已创建，等待管理员确认。

8. 查询 summary 并汇报状态：

```bash
python3 skills/ai-pub-release/scripts/ai_pub_release.py summary --release-id rel_x
```

## 安全边界

不得在发布单 metadata 中保存完整对话、prompt、模型推理过程、token 明细或敏感凭据。只使用最小审计元数据：`agent_name`、`skill_version`、`intent_summary`、`client_request_id`、`conversation_ref`。

不要把 Agent 视为人类角色；权限来自当前用户会话、受限 API Key 或 service account scope。

不要创建独立工单、审批流、Agent 注册中心或 Agent 专用角色体系。
