# Kubernetes Deployment 发布需求与技术设计

## 1. 背景与目标

当前系统已支持 Mock/Dry-run 与 SSH 发布。SSH 发布以服务器或服务器组为运行目标，执行时展开目标服务器，按服务器维度记录日志与当前版本。

新增 Kubernetes 发布能力后，目标服务不再关心具体部署在哪台机器，而是关心某个 Kubernetes 集群、命名空间和既有 Deployment 的镜像版本。系统仍应保持统一的发布闭环：选择服务版本、创建发布单、执行 preflight、确认入队、Worker 执行、记录日志、更新当前版本、支持重试和回滚。

本期明确只做“版本发布”：把已登记的 `ServiceVersion.artifact_url` 更新到既有 Deployment 的指定容器。新服务首次部署、Deployment/Service/Ingress 等资源创建、Kubernetes YAML/Manifest 管理、扩缩容和运行参数调整均不在当前版本范围内。

本设计的目标：

- 支持管理员创建 SSH 或 Kubernetes 两类部署目标，创建表单按执行器走不同分支。
- 支持 Kubernetes Deployment 镜像发布，语义等价于 `kubectl set image`：

```bash
kubectl set image deployment/<Deployment名称> <容器名称>=<新镜像> -n <命名空间>
kubectl rollout status deployment/<Deployment名称> -n <命名空间> --timeout=<超时时间>
```

- 保持 `ReleaseRequest`、`DeployRecord`、preflight、确认、队列、审计、通知、重试和回滚的业务一致性。
- 重构执行记录与当前版本模型，避免继续把所有发布目标都强行表达为服务器。
- 保持执行器接口清晰，但不为了未来未知 executor 提前设计通用配置平台。

## 2. 范围

### 2.1 本期必须支持

- Kubernetes executor：`executor_type=k8s`。
- Kubernetes 部署目标：
  - 集群。
  - 命名空间。
  - Deployment 名称。
  - 容器名称。
  - 超时时间。
- 版本镜像来源：`ServiceVersion.artifact_url`。
- K8s 发布目标强制 `artifact_type=oci_image`，要求镜像为不可变 digest 引用。
- preflight 检查集群、凭据、命名空间、Deployment、容器和镜像格式。
- 执行时只更新 Deployment 指定容器镜像并等待 rollout 完成。
- 成功后记录服务在该部署目标上的当前版本。
- 回滚复用现有回滚发布单语义：选择旧 `ServiceVersion` 再执行一次 K8s 发布，不调用 Kubernetes 的历史 revision undo。

### 2.2 本期不做

- 不支持 StatefulSet、DaemonSet、CronJob 等其他 workload。
- 不支持一次发布更新多个 Deployment。
- 不支持按 Pod、Node 或实际调度机器记录版本。
- 不支持业务 HTTP 健康检查；仅等待 Deployment rollout 完成。
- 不支持自动推断容器名；创建 Kubernetes 部署目标时必须填写 `container_name`。
- 不支持新服务首次部署，不创建 Deployment、Service、Ingress、ConfigMap、Secret 等 Kubernetes 资源。
- 不支持在发布系统内管理 Kubernetes YAML/Manifest，不支持 `kubectl apply -f` 模式。
- 不支持调整运行参数，例如副本数 `replicas`、资源限制、环境变量、探针、调度策略、volume、label、annotation、Service/Ingress 配置等。
- 不支持复杂 Kubernetes RBAC 编排；系统只校验凭据是否可完成所需读写动作。

## 3. 产品口径

### 3.1 发布对象

`Service` 仍是业务服务，`ServiceVersion` 仍是可发布版本，`DeploymentTarget` 仍是服务在某环境的发布目标。

Kubernetes 场景下，用户口径可以说“更新某命名空间里的服务”，但技术落点是更新该服务对应的 Kubernetes Deployment 镜像。Kubernetes `Service` 对象只负责流量入口，不承载版本，因此不作为本期发布对象。

ai-pub 的职责是版本发布，不是 Kubernetes 配置管理：

- 版本发布：把一个已登记版本的镜像更新到既有 Deployment 的指定容器，并记录发布单、审计、日志和当前版本。
- 配置变更：创建新服务、调整副本数、修改资源限制、环境变量、探针、调度、Service/Ingress 等运行配置，交由外部 Kubernetes 配置管理流程处理。

若未来需要发布 YAML、Helm、Kustomize 或 GitOps 变更，应作为单独的 Manifest/GitOps 发布目标重新设计，不混入本期 K8s 镜像发布 executor。

### 3.2 创建部署目标

管理员在部署目标表单选择执行器：

- `mock`：保持现状，用于本地和验收。
- `ssh`：展示服务器/服务器组、脚本路径、工作目录、环境变量等 SSH 专属字段。
- `k8s`：展示集群、命名空间、Deployment 名称、容器名称、超时时间等 Kubernetes 专属字段。

同一个服务和环境允许存在多个部署目标，但创建发布单时必须选择明确的部署目标。

### 3.3 发布和回滚

用户创建发布单时选择服务、环境、版本和部署目标。后续确认、排队、执行、失败通知、重试、回滚均复用现有发布流程。

K8s 回滚不依赖 Kubernetes revision 历史。系统应创建一个指向旧 `ServiceVersion` 的回滚发布单，然后由 K8s executor 将 Deployment 镜像更新为旧版本的 digest。这样审计、确认、版本状态和 SSH 发布保持一致。

## 4. 领域模型调整

### 4.1 DeploymentTarget 主表

`deployment_targets` 保留为统一部署目标入口，只放通用字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_id` | 服务 |
| `environment_id` | 环境 |
| `executor_type` | `mock` / `ssh` / `k8s` |
| `artifact_type` | `version_only` / `oci_image` |
| `timeout_seconds` | 执行超时 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

`target_type`、`target_ref_id`、`script_path`、`working_dir`、`env_vars` 不再作为所有 executor 的通用字段。它们迁移到 executor 专属配置中。

### 4.2 SSHDeploymentTarget

新增 `ssh_deployment_targets`：

| 字段 | 说明 |
|------|------|
| `deployment_target_id` | 主键，引用 `deployment_targets.id` |
| `target_type` | `server` / `server_group` |
| `target_ref_id` | 服务器或服务器组 ID |
| `script_path` | SSH 脚本路径 |
| `working_dir` | 工作目录 |
| `env_vars` | JSON 文本 |

存量 SSH 目标应在迁移中落到此表。Mock 是本地和验收 executor，不应绑定 SSH 的脚本、工作目录等语义；实现时可让 Mock 保持最小配置并生成一条 `target_type=mock` 的执行日志。

### 4.3 Kubernetes 集群

新增 `k8s_clusters`：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 集群名称 |
| `credential_ref` | kubeconfig 凭据引用 |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

凭据复用现有 `credentials` 体系，新增 `type=kubeconfig`。凭据列表仍不得返回 secret。

### 4.4 KubernetesDeploymentTarget

新增 `k8s_deployment_targets`：

| 字段 | 说明 |
|------|------|
| `deployment_target_id` | 主键，引用 `deployment_targets.id` |
| `cluster_id` | Kubernetes 集群 |
| `namespace` | 命名空间 |
| `deployment_name` | Deployment 名称 |
| `container_name` | 容器名称 |

第一版仅支持 Deployment，因此不引入 `workload_kind` 字段。后续若支持 StatefulSet，再通过 migration 增加字段或新增专属表。

### 4.5 DeployRecord 通用化

`DeployRecord` 不应继续以服务器数量作为唯一统计口径。建议改为目标维度统计：

| 字段 | 新语义 |
|------|--------|
| `total_targets` | 执行目标总数 |
| `success_targets` | 成功目标数 |
| `failed_targets` | 失败目标数 |
| `skipped_targets` | 跳过目标数 |

可在迁移阶段将旧列重命名，或新增列后兼容读写一段时间。因为当前项目仍处于 MVP，若确认可重建数据，推荐直接重构 schema，减少双写和兼容分支。

### 4.6 DeployTargetLog

新增通用执行明细表 `deploy_target_logs`，替代 `server_deploy_logs`：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `deploy_record_id` | 发布记录 |
| `target_type` | `server` / `server_group_member` / `k8s_deployment` / `mock` |
| `target_ref_id` | 目标引用 |
| `target_name` | 快照名称，用于 UI 展示 |
| `status` | `queued` / `running` / `success` / `failed` / `skipped` |
| `exit_code` | 进程类执行器退出码，可空 |
| `started_at` / `finished_at` | 时间 |
| `duration_ms` | 耗时 |
| `log_output` | 执行输出 |
| `error_code` / `error_message` | 错误信息 |

SSH 多服务器发布仍可生成多条 `deploy_target_logs`，每台服务器一条。K8s Deployment 发布生成一条 `target_type=k8s_deployment` 的日志。

### 4.7 DeploymentState

新增通用当前版本表 `deployment_states`，替代 `server_deployment_states`：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `service_id` | 服务 |
| `environment_id` | 环境 |
| `deployment_target_id` | 部署目标 |
| `target_type` | 当前状态目标类型 |
| `target_ref_id` | 目标引用 |
| `service_version_id` | 当前版本 |
| `deploy_record_id` | 来源执行记录 |
| `updated_at` | 更新时间 |

唯一约束：

```text
(service_id, environment_id, deployment_target_id, target_type, target_ref_id)
```

SSH 场景中 `target_ref_id` 可以是服务器 ID；K8s 场景中可以是 Deployment 的稳定引用，例如 `<cluster_id>/<namespace>/<deployment_name>/<container_name>`。

## 5. 执行器抽象

### 5.1 统一接口

执行器应对 Worker 暴露统一接口，隐藏 SSH 和 K8s 的执行差异：

```go
type Executor interface {
    Execute(ctx context.Context, req ExecuteRequest) ExecuteResult
}
```

建议请求结构：

```go
type ExecuteRequest struct {
    Release domain.ReleaseRequest
    Record  domain.DeployRecord
    Target  domain.DeploymentTarget
    Version domain.ServiceVersion
    Plan    DeployTargetSnapshot
}
```

`DeployTargetSnapshot` 由 repository 在确认入队或 claim 时根据 `deployment_target_id` 和 executor 专属配置构建并快照。Worker 不应在执行过程中重新猜测目标配置。

### 5.2 类型化执行快照

执行快照应使用类型化结构，避免引入 `json.RawMessage` 形式的弱类型配置总线：

```go
type DeployTargetSnapshot struct {
    ExecutorType string
    Targets      []ExecutionTarget
    SSH          *SSHTargetSnapshot
    K8s          *K8sDeploymentSnapshot
}

type ExecutionTarget struct {
    Type  string
    RefID string
    Name  string
}

type SSHTargetSnapshot struct {
    TargetType string
    ScriptPath string
    WorkingDir string
    EnvVars    string
}

type K8sDeploymentSnapshot struct {
    ClusterID      string
    Namespace      string
    DeploymentName string
    ContainerName  string
}
```

SSH 的 `Targets` 是展开后的服务器列表。K8s 的 `Targets` 是一个 Deployment 目标。executor 只读取自己对应的 typed snapshot；不通过通用 JSON 字段解析未来未知配置。

### 5.3 Worker 执行规则

Worker 领取发布记录后：

1. 加载发布单、服务版本和执行计划快照。
2. 按 `executor_type` 获取对应 executor。
3. 对多目标 executor，按目标顺序执行并 fail-fast。
4. 对 K8s executor，执行单个 Deployment 目标。
5. 每个目标执行前写 `deploy_target_logs.running`。
6. 每个目标执行后写 `deploy_target_logs.success/failed`。
7. 汇总 `DeployRecord` 状态并更新 `ReleaseRequest`。
8. 成功目标写入 `deployment_states`。

互斥规则仍保持“同服务同环境已有 running 发布时阻断新发布”。SSH 的服务器级互斥可迁移为基于 `deploy_target_logs.target_ref_id` 的目标互斥；K8s 不需要服务器互斥。

## 6. Kubernetes Executor 设计

### 6.1 执行动作

K8s executor 的执行语义等价于：

```bash
kubectl set image deployment/<deployment_name> <container_name>=<artifact_url> -n <namespace>
kubectl rollout status deployment/<deployment_name> -n <namespace> --timeout=<timeout_seconds>s
```

实现优先使用 Kubernetes 官方 client-go，而不是 shell 调用 `kubectl`。原因：

- 无需在运行镜像里安装 kubectl。
- kubeconfig、超时、错误分类和 secret 脱敏更可控。
- 更容易在 Go 单测中用 fake client 覆盖。

本期不采用 `kubectl apply -f deployment.yaml` 语义。`apply` 适合声明式 Manifest 管理，会让 ai-pub 持有或生成完整 Deployment YAML，并可能覆盖副本数、环境变量、资源限制、探针等运行配置；这超出“版本发布”边界。

### 6.2 镜像更新方式

执行器读取 Deployment：

```text
apps/v1.Deployment.spec.template.spec.containers[].image
```

找到 `container_name` 对应容器后，只将该容器 image 更新为 `ServiceVersion.artifact_url`。若找不到容器，应失败并记录 `container_not_found`。

更新方式推荐使用 patch，避免覆盖 Deployment 的其他字段。patch 内容必须只包含容器镜像变化，不修改 replicas、resources、env、probe、volume、label、annotation 或 rollout strategy。patch 后等待 rollout 完成：

- `observedGeneration >= generation`。
- `updatedReplicas == spec.replicas`。
- `availableReplicas == spec.replicas`。
- Deployment condition `Progressing=True` 且 reason 表示新 ReplicaSet 可用，或达到等价的 rollout complete 判断。

这里读取 `spec.replicas` 仅用于判断 rollout 是否完成，不代表 ai-pub 可以修改副本数。

超时返回 `rollout_timeout`。

### 6.3 错误码

建议错误码：

| 错误码 | 场景 |
|--------|------|
| `cluster_not_available` | 集群不存在、禁用或凭据不可用 |
| `namespace_not_found` | 命名空间不存在 |
| `deployment_not_found` | Deployment 不存在 |
| `container_not_found` | 容器名不存在 |
| `image_invalid` | 镜像 digest 格式不合法 |
| `permission_denied` | Kubernetes API 权限不足 |
| `rollout_timeout` | rollout 超时 |
| `rollout_failed` | Deployment 进入失败状态 |
| `executor_error` | 其他执行器错误 |

## 7. Preflight 设计

### 7.1 通用规则

通用 preflight 保持现有规则：

- 版本属于目标服务。
- 部署目标属于目标服务和环境。
- 环境冻结时 block。
- 同服务同环境已有 running 发布时 block。
- 生产环境使用管理员确认，非生产环境使用本人确认。
- `artifact_type=oci_image` 时，`artifact_url` 必须匹配不可变 digest。

### 7.2 SSH 规则

- SSH 专属配置存在。
- `target_type` 为 `server` 或 `server_group`。
- 目标服务器或服务器组存在且启用。
- `script_path` 必填。
- 服务器凭据可用。
- 环境变量不得覆盖系统保留变量。

### 7.3 K8s 规则

- `executor_type=k8s` 必须搭配 `artifact_type=oci_image`。
- K8s 专属配置存在。
- 集群存在且启用。
- kubeconfig 凭据存在、启用且可解密。
- `namespace`、`deployment_name`、`container_name` 均非空。
- Kubernetes API 可访问。
- Deployment 存在。
- 指定容器存在。
- 当前发布镜像与目标镜像相同时给 warning 或 pass；不要因为相同镜像阻断，允许用户重跑 rollout。
- preflight 不接收也不校验 replicas、resources、env、probe 等运行参数；这些参数不属于本期发布单输入。

## 8. API 设计

### 8.1 部署目标

`POST /deployment-targets` 和 `PATCH /deployment-targets/{id}` 按执行器接收不同配置。请求示例：

```json
{
  "service_id": "svc_123",
  "environment_id": "env_test",
  "executor_type": "k8s",
  "artifact_type": "oci_image",
  "timeout_seconds": 300,
  "k8s": {
    "cluster_id": "k8s_123",
    "namespace": "default",
    "deployment_name": "order-api",
    "container_name": "app"
  }
}
```

SSH 示例：

```json
{
  "service_id": "svc_123",
  "environment_id": "env_test",
  "executor_type": "ssh",
  "artifact_type": "version_only",
  "timeout_seconds": 300,
  "ssh": {
    "target_type": "server_group",
    "target_ref_id": "sg_123",
    "script_path": "/opt/deploy/deploy.sh",
    "working_dir": "/opt/app",
    "env_vars": "{}"
  }
}
```

响应应返回通用字段和对应 executor 配置，避免前端再按 `target_ref_id` 猜测。

### 8.2 K8s 集群

新增管理员接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/k8s-clusters` | 列表 |
| `POST` | `/k8s-clusters` | 创建 |
| `PATCH` | `/k8s-clusters/{id}` | 更新名称、凭据、启用状态 |
| `DELETE` | `/k8s-clusters/{id}` | 删除未被部署目标引用的集群 |

凭据仍通过 `/credentials` 管理，新增 `type=kubeconfig`。

### 8.3 执行日志

保留现有 deploy record 查询入口，新增或替换日志入口：

```text
GET /deploy-records/{id}/target-logs
```

旧的 `/server-logs` 可在实现期同步迁移，前端改为展示“执行目标日志”。

## 9. 前端设计

### 9.1 部署目标表单

执行器选择使用分段控件或下拉：

- Mock。
- SSH。
- Kubernetes。

选择 Kubernetes 后展示：

- 集群。
- 命名空间。
- Deployment 名称。
- 容器名称。
- 制品类型固定或默认 `oci_image`。
- 超时时间。

选择 SSH 后展示现有 SSH 字段。不同分支互不展示无关字段，避免让 K8s 用户看到服务器、脚本路径等概念。K8s 表单不得出现 YAML 上传、Manifest 编辑、副本数或运行参数编辑入口。

### 9.2 发布详情

发布详情将“目标服务器数”“成功/失败/跳过服务器”改为“执行目标数”“成功/失败/跳过目标”。

K8s 详情展示：

- 集群。
- 命名空间。
- Deployment。
- 容器。
- 目标镜像。
- rollout 结果。

### 9.3 当前版本

服务详情和概览里的“服务器当前版本”改为“当前部署版本”。SSH 可继续按服务器展开，K8s 按部署目标展示。

## 10. Migration 与兼容策略

当前项目处于 MVP，且 MySQL 8 是唯一受支持运行时。若确认数据可重建，推荐进行直接 schema 重构：

1. 新增 `ssh_deployment_targets`、`k8s_clusters`、`k8s_deployment_targets`。
2. 新增 `deploy_target_logs`、`deployment_states`。
3. 重构 `deployment_targets` 通用字段。
4. 将 `deploy_records.total_servers/success_servers/failed_servers/skipped_servers` 改为 target 语义字段。
5. 删除或停止使用 `server_deploy_logs` 与 `server_deployment_states`。
6. 更新 MySQL migration；SQLite migration 仅用于 Go 单测同构 schema。

若仍需要保留历史数据，应单独设计数据迁移脚本，将旧服务器日志迁移为 `deploy_target_logs`，将旧服务器状态迁移为 `deployment_states`。本期不建议为了历史兼容引入长期双写。

实现时应避免把 schema 重构扩大为 Kubernetes 配置平台重构。数据库只表达发布目标、执行日志和当前版本，不保存完整 Deployment YAML。

## 11. 实施步骤

1. 文档同步：
   - 更新领域模型、后端架构、API 和前端 IA 文档。
   - 明确 SSH 与 K8s 两条部署目标配置分支。
2. 数据模型：
   - 编写 MySQL/SQLite migration。
   - 更新 `domain` model。
   - 更新 repository 的部署目标、执行计划、日志和状态读写。
3. 应用服务：
   - 拆分通用 preflight 和 executor-specific preflight。
   - 更新确认入队逻辑，生成通用执行计划和目标日志。
4. 执行器：
   - 抽象 executor registry。
   - 迁移 Mock/SSH 到统一接口。
   - 新增 K8s executor。
5. API：
   - 更新部署目标创建/编辑响应结构。
   - 新增 K8s 集群管理接口。
   - 更新执行日志接口。
6. 前端：
   - 部署目标表单按 executor 分支展示。
   - 发布详情和服务详情改为目标维度措辞。
   - 增加 K8s 集群管理入口。
7. 验证：
   - Go 单测覆盖 preflight、repository migration、executor 选择和 K8s executor fake client。
   - 前端 lint/build。
   - `make verify`。
   - 涉及 migration、Worker 和发布执行链路，最终必须执行 `make compose-check`。

## 12. 验收标准

- 管理员能创建 kubeconfig 凭据和 K8s 集群。
- 管理员能为服务和环境创建 K8s Deployment 部署目标。
- K8s 目标创建时必须填写 namespace、Deployment 名称和容器名称。
- 对 K8s 目标创建发布单时，缺少或非法 OCI digest 会被 preflight block。
- Deployment 或容器不存在时 preflight block。
- 确认发布后，Worker 只更新 Deployment 指定容器镜像并等待 rollout 完成。
- 发布前后 Deployment 的副本数、资源限制、环境变量、探针、调度、Service/Ingress 等运行配置不应被 ai-pub 修改。
- 发布成功后，`deployment_states` 记录该部署目标的当前版本。
- 发布失败时，`deploy_target_logs` 记录错误码、错误信息和有限执行输出。
- SSH 发布仍保持现有服务器/服务器组语义，顺序 fail-fast 行为不回退。
- 发布详情不再把 K8s 发布显示为“目标服务器数”。
- 回滚 K8s 发布时，系统创建回滚发布单并将 Deployment 镜像更新为旧版本 digest。

## 13. 风险与注意事项

- 不要把 K8s 配置塞进 `env_vars`；它们是部署目标结构化配置。
- 不要让 K8s 发布复用伪服务器；这会污染 UI、冲突检测和当前版本语义。
- 不要默认更新 Deployment 的所有容器；必须明确 `container_name`。
- 不要使用 Kubernetes `Service` 对象作为版本承载对象。
- 不要使用 `kubectl apply -f` 或保存完整 YAML 来实现本期镜像发布。
- 不要在发布单里加入 replicas、resources、env、probe 等运行参数。
- 不要把新服务部署、配置变更、扩缩容和版本发布合并为一个流程。
- 不要绕过发布单直接执行 K8s 更新，否则审计、确认和回滚链路会断裂。
- 不要在日志中输出 kubeconfig、token 或完整敏感错误上下文。
