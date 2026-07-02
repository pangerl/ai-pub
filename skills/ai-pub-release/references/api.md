# AI Pub Agent API 参考

基础地址来自 `AI_PUB_BASE_URL`，认证使用 `Authorization: Bearer $AI_PUB_API_KEY`。

## 候选查询

```text
GET /api/v1/agent/services?q={query}&project_id={project_id}
GET /api/v1/agent/environments?q={query}
GET /api/v1/agent/services/{service_id}/versions?q={query}
GET /api/v1/agent/deployment-targets?service_id={service_id}&environment_id={environment_id}
```

响应结构：

```json
{
  "data": {
    "items": [],
    "count": 0,
    "query": "order"
  }
}
```

`count > 1` 时不要猜测，必须让用户选择。

部署目标候选会返回通用字段和 executor 专属配置：

- `executor_type=ssh`：读取 `ssh.target_type`、`ssh.target_ref_id`、`ssh.script_path`、`ssh.working_dir`、`ssh.env_vars`。
- `executor_type=k8s`：读取 `k8s.cluster_id`、`k8s.namespace`、`k8s.deployment_name`、`k8s.container_name`。

K8s 目标只用于既有 Kubernetes Deployment 的镜像版本发布；Agent 不得生成或提交 YAML、Manifest、副本数、资源限制、环境变量、探针、volume、label、annotation、Service/Ingress 等运行参数。

## Preflight

```text
POST /api/v1/agent/release-intents/preflight
```

请求：

```json
{
  "service_id": "svc_1",
  "environment_id": "env_test",
  "service_version_id": "ver_1",
  "deployment_target_id": "target_1"
}
```

`result=block` 时不得创建发布单。`warning` 可以继续，但要向用户展示警告。

K8s 发布要求目标版本的 `artifact_url` 是不可变 OCI digest，例如 `repo/name@sha256:<64位十六进制>`。Deployment 不存在、容器不存在、集群或 kubeconfig 不可用时，preflight 会返回 `block`。

## 创建发布单

```text
POST /api/v1/agent/release-requests
```

请求：

```json
{
  "service_id": "svc_1",
  "environment_id": "env_test",
  "service_version_id": "ver_1",
  "deployment_target_id": "target_1",
  "idempotency_key": "agent:svc_1:env_test:ver_1:20260630",
  "agent_name": "codex",
  "skill_version": "0.1.0",
  "intent_summary": "发布订单服务 v1.2.3 到测试环境",
  "client_request_id": "req_123",
  "conversation_ref": "thread_123"
}
```

服务端固定写入 `source=ai_agent`。客户端不要传 `source`、`created_by_type` 或 `created_by_id`；即使传入也不应被信任。

Agent metadata 只保存最小审计上下文：

- `agent_name`
- `skill_version`
- `intent_summary`
- `client_request_id`
- `conversation_ref`

不要保存完整对话、prompt、模型推理过程、token 明细或敏感凭据。

## 确认发布

```text
POST /api/v1/agent/release-requests/{release_id}/confirm
```

只在用户明确要求“确认发布/创建并发布”时调用。生产发布必须等待管理员确认；API Key 不能独立确认生产发布。

## 查询 Summary

```text
GET /api/v1/agent/release-requests/{release_id}/summary
```

响应包含：

- `release`
- `events`
- `deploy_records`
- `next_action`

`next_action` 常见值：

- `confirm_required`
- `wait`
- `done`
- `inspect`

## 最小 Scope

- 候选查询：`inventory:read`
- preflight 和创建发布单：`release:create`
- summary：`release:read`
- 非生产确认：`release:confirm`
