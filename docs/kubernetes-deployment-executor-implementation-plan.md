# Kubernetes Deployment Executor 实施计划

## 1. 当前基线

本计划承接 `kubernetes-deployment-executor-design.md`，用于把 Kubernetes Deployment 发布能力拆成可执行的小阶段。当前需求边界如下：

- ai-pub 只做版本发布：把 `ServiceVersion.artifact_url` 更新到既有 Kubernetes Deployment 的指定容器。
- K8s executor 的语义等价于 `kubectl set image deployment/<deployment_name> <container_name>=<artifact_url> -n <namespace>`，随后等待 rollout 完成。
- 实现优先使用 Kubernetes 官方 client-go 和 patch，不 shell 调 `kubectl`。
- 不做新服务部署、Kubernetes YAML/Manifest 管理、`apply` 模式、扩缩容、运行参数变更或复杂 RBAC 编排。
- K8s 发布目标必须使用 `artifact_type=oci_image`，镜像要求不可变 digest 引用。
- 回滚复用现有回滚发布单语义：选择旧 `ServiceVersion` 再执行一次镜像发布，不调用 Kubernetes revision undo。

本计划默认项目仍处于 MVP，数据可重建；因此 schema 改造优先直接重构，避免长期兼容分支和双写。

## 2. 总体成功标准

- SSH、Mock 发布在新模型下保持可用；SSH 的服务器组顺序执行、fail-fast、重试和回滚语义不回退。
- K8s 发布能从发布单进入统一 preflight、确认、队列、Worker、日志、当前版本和回滚链路。
- Worker 只修改既有 Deployment 指定容器的 image，不修改 replicas、resources、env、probe、volume、label、annotation、Service/Ingress 或其他运行配置。
- 发布详情、发布记录和服务当前版本都改为“目标”口径，不再把 K8s 显示为服务器数。
- 代码级验证通过 `make verify`；涉及 migration、Worker 和发布执行链路的最终验收必须通过 `make compose-check`。

## 3. 阶段 0：长期文档基线同步

目标：先让长期文档与已确认边界一致，避免代码实现时被旧的“服务器唯一模型”牵引。

涉及文件：

- `docs/domain-model-design.md`
- `docs/backend-architecture-design.md`
- `docs/api-design.md`
- `docs/frontend-ia-design.md`
- `docs/README.md`

任务：

1. 将 `DeploymentTarget` 从服务器专属配置改为通用入口 + executor 专属配置。
2. 将 `DeployRecord`、执行日志、当前版本从 server 口径改为 target 口径。
3. 补充 `k8s_clusters`、`k8s_deployment_targets`、K8s executor、K8s preflight 和 target logs API。
4. 明确前端部署目标表单按 `mock`、`ssh`、`k8s` 分支展示，K8s 分支不出现 YAML、Manifest、副本数或运行参数入口。

验证点：

- 文档中不再把发布执行模型描述为只能展开服务器。
- K8s 相关文档持续强调只更新既有 Deployment 的指定容器镜像。
- `docs/README.md` 的阅读顺序能引导后续开发从设计到实施计划。

验收标准：

- 文档 diff 只覆盖上述长期设计文档和索引。
- 不引入和 Kubernetes 配置管理、Manifest 管理、扩缩容相关的新范围。

## 4. 阶段 1：Schema 与领域模型重构

目标：建立 executor 通用模型和 K8s 专属模型，先让数据结构能表达 SSH、Mock 和 K8s 三类目标。

涉及文件：

- `migrations/mysql/*.sql`
- `migrations/sqlite/*.sql`
- `internal/domain/models.go`
- `internal/migration/runner_test.go`
- `internal/repository/repository_test.go`

数据结构：

- 新增 `ssh_deployment_targets`。
- 新增 `k8s_clusters`。
- 新增 `k8s_deployment_targets`。
- 新增 `deploy_target_logs`。
- 新增 `deployment_states`。
- 将 `deploy_records.total_servers/success_servers/failed_servers/skipped_servers` 改为 `total_targets/success_targets/failed_targets/skipped_targets`。
- 将 `deployment_targets` 保留为通用入口，仅保留 `service_id`、`environment_id`、`executor_type`、`artifact_type`、`timeout_seconds`、`enabled` 和时间字段。

接口/类型：

- `domain.DeploymentTarget`
- `domain.SSHDeploymentTarget`
- `domain.K8sCluster`
- `domain.K8sDeploymentTarget`
- `domain.DeployRecord`
- `domain.DeployTargetLog`
- `domain.DeploymentState`

验证点：

- SQLite migration 仍与 Go 单测同构。
- MySQL migration 能从空库建立完整 schema。
- `credential.type` 支持 `kubeconfig`，列表接口仍不返回 secret。

验收标准：

- 定向运行 `go test ./internal/migration ./internal/repository`。
- 现有 repository fixture 不再依赖旧的 `server_deploy_logs` 和 `server_deployment_states` 表名。

## 5. 阶段 2：Repository 与应用服务迁移

目标：先不接 K8s API，优先让 SSH 和 Mock 在新 target 模型下跑通。

涉及文件：

- `internal/repository/release.go`
- `internal/repository/deploy.go`
- `internal/repository/repository.go`
- `internal/app/release.go`
- `internal/app/preflight_artifact_type_test.go`
- `internal/app/release_test.go`
- `internal/e2e/mock_deploy_test.go`

任务：

1. 将确认入队逻辑改为生成类型化 `DeployTargetSnapshot`。
2. 将旧的 `ServerDeployLog` 操作迁移为 `DeployTargetLog` 操作。
3. 将当前版本写入迁移为 `DeploymentState`。
4. 将 SSH 目标展开保留为稳定顺序的多个 execution targets。
5. Mock 生成最小的单 target 日志，不复用 SSH 配置语义。
6. 保持同服务同环境 running 发布阻断；SSH 服务器级互斥迁移为 target 互斥。

接口/类型：

- `DeployTargetSnapshot`
- `ExecutionTarget`
- `SSHTargetSnapshot`
- `K8sDeploymentSnapshot`
- `ClaimedDeploy`
- `MarkTargetRunning`
- `MarkTargetFinished`
- `MarkQueuedTargetsSkipped`
- `ListDeployTargetLogs`
- `ListDeploymentStates`

验证点：

- SSH 单服务器发布仍成功更新目标日志和当前版本。
- SSH 服务器组仍按稳定顺序执行，并在失败后跳过后续目标。
- Mock 发布仍支撑本地 e2e 和 Compose 验收。
- 重试和回滚仍使用原发布单链路，不回退成直接执行。

验收标准：

- 定向运行 `go test ./internal/app ./internal/repository ./internal/e2e`。
- 发布记录计数已使用 target 口径。

## 6. 阶段 3：执行器抽象与 Registry

目标：把 Worker 对 Mock、SSH、K8s 的分发收敛到统一 executor contract。

涉及文件：

- `internal/executor/executor.go`
- `internal/executor/ssh_test.go`
- `internal/worker/worker.go`
- `internal/repository/deploy.go`

任务：

1. 定义统一 `Executor` 接口和 `ExecuteRequest` / `ExecuteResult`。
2. 增加 executor registry，按 `executor_type` 选择 executor。
3. 将 Mock 和 SSH 适配到统一接口。
4. Worker 只读取 typed snapshot，不解析 `json.RawMessage` 配置总线。

验证点：

- 不支持的 `executor_type` 返回明确 `executor_error`。
- SSH gateway 凭据解析和执行路径不回退。
- Worker 对多目标 executor 保持顺序执行和 fail-fast。

验收标准：

- 定向运行 `go test ./internal/executor ./internal/worker ./internal/repository`。
- 无新增弱类型配置总线或未来 executor 的空泛抽象。

## 7. 阶段 4：K8s 集群、Preflight 与 Executor

目标：接入 Kubernetes Deployment 镜像发布的后端核心能力。

涉及文件：

- `go.mod`
- `go.sum`
- `internal/app/credential.go`
- `internal/app/infrastructure.go`
- `internal/app/release.go`
- `internal/executor/k8s.go`
- `internal/executor/k8s_test.go`
- `internal/repository/credential.go`
- `internal/repository/deploy.go`
- `internal/repository/repository.go`

任务：

1. 新增 `kubeconfig` 凭据类型。
2. 新增 K8s 集群 repository 与 app service。
3. K8s preflight 校验集群、凭据、命名空间、Deployment、容器和 OCI digest。
4. K8s executor 使用 client-go patch 指定容器 image。
5. 等待 rollout 完成并映射错误码：`cluster_not_available`、`namespace_not_found`、`deployment_not_found`、`container_not_found`、`image_invalid`、`permission_denied`、`rollout_timeout`、`rollout_failed`、`executor_error`。

验证点：

- fake client 覆盖 Deployment 不存在、容器不存在、权限错误、rollout 成功和 rollout 超时。
- patch 内容只包含目标容器 image 变化。
- 日志不输出 kubeconfig、token 或完整敏感错误上下文。

验收标准：

- 定向运行 `go test ./internal/executor ./internal/app ./internal/repository`。
- 单测断言 replicas、resources、env、probe、volume、label、annotation 不被 executor 修改。

## 8. 阶段 5：HTTP API 与 Agent Surface 更新

目标：对外暴露 K8s 集群和 executor 分支后的部署目标、日志、当前版本接口。

涉及文件：

- `internal/httpapi/infrastructure.go`
- `internal/httpapi/deploy.go`
- `internal/httpapi/release.go`
- `internal/httpapi/agent.go`
- `internal/httpapi/router.go`
- `internal/httpapi/*_test.go`
- `docs/api-design.md`
- `skills/ai-pub-release/references/api.md`

任务：

1. `POST/PATCH /deployment-targets` 按 `executor_type` 接收 `ssh` 或 `k8s` 配置。
2. 新增 `/k8s-clusters` 管理接口。
3. 将 `/deploy-records/{id}/server-logs` 迁移为 `/deploy-records/{id}/target-logs`。
4. 将 `/server-deployment-states` 迁移为 target 口径的当前版本视图。
5. 更新 Agent inventory 和 release summary，避免继续暴露服务器唯一口径。

验证点：

- K8s 目标缺少 namespace、Deployment 名称或容器名称时返回 400。
- K8s 目标强制 `artifact_type=oci_image`。
- 删除被部署目标引用的 K8s 集群会被阻断。
- 凭据列表和集群响应不泄露 secret。

验收标准：

- 定向运行 `go test ./internal/httpapi ./internal/app ./internal/repository`。
- API 文档和 bundled skill API reference 与实现一致。

## 9. 阶段 6：前端交互迁移

目标：让管理界面能创建和查看 K8s 发布目标，同时去掉服务器唯一措辞。

涉及文件：

- `web/src/App.tsx`
- `web/src/styles.css`
- `docs/frontend-ia-design.md`
- `docs/DESIGN.md`，仅当新增 UI 模式需要补充规范时修改。

任务：

1. 部署目标表单增加 `mock`、`ssh`、`k8s` 分支。
2. 增加 K8s 集群管理入口。
3. 发布详情和发布记录将服务器计数改为目标计数。
4. 服务详情和概览将“服务器当前版本”改为“当前部署版本”。
5. K8s 详情展示集群、命名空间、Deployment、容器、目标镜像和 rollout 结果。

验证点：

- K8s 表单不展示服务器、脚本路径、工作目录、env vars、YAML、Manifest、副本数或运行参数字段。
- SSH 表单仍保留现有字段和编辑能力。
- 移动端与桌面端文本不重叠，目标计数字段不撑破列表行。

验收标准：

- 在 `web/` 下运行前端 lint/build。
- 如启动本地页面检查，使用截图确认主要表单分支和发布详情无布局错乱。

## 10. 阶段 7：端到端验证与收口

目标：确认跨 migration、API、Worker、前端和 Compose 的发布闭环。

涉及文件：

- `docs/development-completion-audit.md`
- `docs/local-verification.md`
- `README.md`，仅当公开入口或验证命令说明需要同步时修改。

验证矩阵：

- `go test ./internal/migration ./internal/repository ./internal/app ./internal/executor ./internal/httpapi ./internal/worker ./internal/e2e`
- `make verify`
- `make compose-check`
- 手工/API 验证：
  - Mock 发布成功。
  - SSH 单服务器发布成功。
  - SSH 服务器组失败后 fail-fast。
  - K8s preflight 对非法 digest、Deployment 不存在、容器不存在进行 block。
  - K8s 发布成功后写入 `deployment_states`。
  - K8s 发布失败后写入 `deploy_target_logs` 错误码和有限输出。
  - K8s 回滚发布单将 Deployment 镜像更新到旧 digest。

验收标准：

- `make verify` 和 `make compose-check` 均通过，或明确记录环境性阻塞和已通过的定向检查。
- `development-completion-audit.md` 和 `local-verification.md` 记录最新验证结论。
- 提交为任务范围内的 scoped commit，不纳入无关未跟踪文件。

## 11. 实施注意事项

- 不把 K8s 配置塞进 `env_vars`。
- 不用伪服务器表达 K8s Deployment。
- 不默认更新所有容器；必须使用明确的 `container_name`。
- 不把 Kubernetes `Service` 对象作为版本承载对象。
- 不保存完整 Deployment YAML，不使用 `kubectl apply -f`。
- 不在发布单里加入 replicas、resources、env、probe、volume、label、annotation 等运行参数。
- 不绕过发布单直接执行 K8s 更新。
- 不在日志中输出 kubeconfig、token 或完整敏感错误上下文。
