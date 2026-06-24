# 服务版本登记与后端 OCI 镜像发布设计

## 1. 结论与范围

本设计定义面向通用发布系统的服务版本登记能力：用户可手动登记版本，外部 CI 也可通过统一接口登记版本；二者最终使用同一套 `ServiceVersion` 供发布选择、执行与审计。

GitLab CI 是本期首个接入示例，后端 OCI 镜像脚本是本期首个部署 profile；它们不构成版本模型的前提。

本期完成的业务闭环是：

```mermaid
flowchart LR
    A[管理员手动登记] --> D[服务版本库]
    B[外部 CI 构建成功] --> C[统一版本登记接口]
    C --> D
    D --> E[用户选择版本并创建发布单]
    E --> F[部署目标按自身能力执行]
    F --> G[后端 OCI 镜像 profile]
```

本期范围：

- 管理员手动登记版本，以及外部 CI 对既有服务的统一版本登记。
- 发布页面从当前服务的版本库选择版本；手动与 CI 版本使用同一模型与审计链路。
- 版本登记接口不绑定 GitLab、Jenkins、GitHub Actions 或特定制品类型。
- GitLab CI 登记后端 OCI 镜像，以及 SSH 调用无位置参数后端镜像脚本，作为本期落地 profile。

本期不做：

- 前端静态资源发布、前端发布脚本或前端 OCI 以外制品的专用部署 profile。
- Jenkins 兼容层、GitLab Webhook 事件解析、自动创建项目或服务。
- CI 成功后自动创建、自动确认或自动执行发布单。
- N9E 告警屏蔽、通用构建状态同步、制品仓库清理策略。
- 为每一种 CI、制品格式或部署方式设计独立的版本表与接口。

## 2. 关键设计决定

### 2.1 采用外部 CI 主动登记 API，而非平台 Webhook

外部 CI 在其构建产物可用后，直接调用发布系统 API 登记版本。这个动作语义明确，不需要接收并过滤某个平台的多种 webhook 事件。

各平台的 Pipeline 字段、构建链接和分支信息作为可选 metadata 保存。未来确有“发布系统主动订阅代码平台事件”的需求时，再增加平台 webhook adapter；本期不需要。

### 2.2 服务必须预先存在，CI 按项目和服务 key 登记版本

CI 不传内部 `service_id`，更不允许自动创建实体。每个 GitLab 仓库在受保护变量中配置一次固定的 `AI_PUB_PROJECT_KEY`；每个单仓多服务 Job 传递自己正在构建的模块 `service_key`。发布系统按 `Project.slug + Service.slug` 找到既有服务。

`service_key` 不要求全局唯一，只需在所属项目内唯一。这样既避免 CI 耦合内部 ID，也不会因不同项目都有 `gateway`、`auth` 等服务而误匹配。服务的创建、启用和部署目标配置继续由管理员在发布系统完成。

第一版不维护 repository 实体或 repository 到服务的映射；外部系统的仓库地址可作为版本 metadata 保存。若未来单个仓库的服务需要归属不同发布项目，再新增 `(repository, service_key) -> service_id` 显式映射。

### 2.3 版本模型通用，制品约束由部署 profile 决定

`version` 是所有版本的必填展示与选择值。`commit_sha`、`artifact_url` 和外部构建信息均为可选补充：手动登记可只填写版本号；外部 CI 可按其能力补充提交、制品和运行链接。

部署目标决定哪些补充信息在执行时必须存在。例如后端 OCI 镜像 profile 要求 `artifact_url` 为已推送镜像的完整不可变引用，优先使用：

```text
harbor.example/team/order-api@sha256:<digest>
```

该 profile 不能只依赖可被覆盖的 `latest` 或普通 tag。CI 可同时把便于阅读的 tag 作为 `version` 保存。包下载、脚本内解析版本等其他部署方式不适用该 OCI 限制。

### 2.4 发布只能选择已登记的版本

创建发布单时，版本列表只显示当前服务的版本，按登记时间倒序，并支持按版本号、短 SHA 搜索。长版本号在列表中截断展示，详情与复制操作保留完整值。

“手动输入版本”表示管理员手动登记一个 `ServiceVersion(source=manual)`，登记后再被选择；不允许通过自由文本绕过版本记录直接创建发布单。

## 3. 版本登记接口

使用统一资源路径：

```text
POST /api/v1/version-registrations
```

管理员通过服务版本管理接口手动登记版本，服务端写入 `source=manual`；外部 CI 调用上述统一接口，服务端写入 `source=ci`、`created_by_type=api_key` 和调用方 ID。来源不能由请求体伪造。

需要新增 `version:write` API Key scope。它只允许登记版本，不可创建服务、修改部署目标或执行发布。即使发布系统仅在内网运行，也不能移除鉴权：该接口决定后续可被部署的制品。GitLab 可使用 Group 级受保护 API Key 作为首个接入示例。

### 3.1 请求体

```json
{
  "project_key": "food-supply",
  "service_key": "order-api",
  "version": "2026.06.23-1842",
  "commit_sha": "7f3c6b5b1b4d...",
  "artifact_url": "https://artifact.example/order-api-20260623.tar.gz",
  "metadata": {
    "provider": "gitlab",
    "run_id": "9281",
    "run_url": "https://gitlab.example/group/order-api/-/pipelines/9281",
    "ref": "main",
    "repository": "group/order-api",
    "built_at": "2026-06-23T10:42:31Z"
  }
}
```

其中 `project_key`、`service_key`、`version` 必填；`commit_sha`、`artifact_url`、`metadata` 可选。服务端按 `project_key + service_key` 定位服务；不接收 `service_id`。

服务端保留 `metadata` 作为 JSON 文本，不为任何 CI 平台的运行号、分支或构建时间新增专属字段。`created_at` 保持为发布系统首次登记时间。

不接收提交说明作为核心字段：它对发布执行没有决定作用，长度和换行也会增加日志及 JSON 处理风险；需要时可通过 `commit_sha` 跳转至 GitLab 查看。

### 3.2 幂等与冲突

CI 必须携带：

```text
Idempotency-Key: {external-provider}:{external-run-id}
```

版本登记的处理规则：

| 情况 | 结果 |
|---|---|
| 首次登记 | 创建版本，返回 `201` |
| 同一服务、同一版本、同一 commit 与制品再次提交 | 返回已有版本，`200` |
| 同一服务、同一版本但 commit 或制品不同 | 返回 `409 version_conflict` |
| 项目或服务不存在、服务已禁用 | 返回 `404` 或 `409`，不自动创建 |
| 可选字段缺失 | 创建版本；具体部署目标在 preflight 时按自身能力判断 |

唯一约束继续使用 `(service_id, version)`；API Key、幂等键、来源 Pipeline 和登记结果要写入可查询审计事件。重试不得产生重复版本。

## 4. GitLab CI 接入示例

本示例用于后端 OCI 镜像：版本登记 Job 排在镜像 push 成功之后。CI 只负责登记制品，不运行服务器部署脚本，也不创建发布单。其他 CI 只需按第 3 节提交同一通用请求即可。

示例片段（`IMAGE_DIGEST_REF` 由镜像构建/推送步骤解析得到）：

```yaml
register_release_version:
  stage: register
  needs: [build_and_push_image]
  script:
    - test -n "$AI_PUB_PROJECT_KEY"
    - test -n "$SERVICE_KEY"
    - test -n "$AI_PUB_API_KEY"
    - test -n "$IMAGE_DIGEST_REF"
    - |
      payload=$(jq -n \
        --arg project_key "${AI_PUB_PROJECT_KEY}" \
        --arg service_key "${SERVICE_KEY}" \
        --arg version "${IMAGE_VERSION}" \
        --arg commit_sha "${CI_COMMIT_SHA}" \
        --arg artifact_url "${IMAGE_DIGEST_REF}" \
        --arg provider "gitlab" \
        --arg run_id "${CI_PIPELINE_ID}" \
        --arg run_url "${CI_PIPELINE_URL}" \
        --arg ref "${CI_COMMIT_REF_NAME}" \
        --arg repository "${CI_PROJECT_PATH}" \
        --arg built_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{project_key:$project_key, service_key:$service_key, version:$version, commit_sha:$commit_sha, artifact_url:$artifact_url, metadata:{provider:$provider, run_id:$run_id, run_url:$run_url, ref:$ref, repository:$repository, built_at:$built_at}}')
      curl --fail-with-body --retry 5 --retry-all-errors \
        -X POST "${AI_PUB_BASE_URL}/api/v1/version-registrations" \
        -H "Authorization: Bearer ${AI_PUB_API_KEY}" \
        -H "Content-Type: application/json" \
        -H "Idempotency-Key: gitlab:${CI_PROJECT_ID}:${CI_PIPELINE_ID}" \
        --data "$payload"
```

说明：

- `AI_PUB_PROJECT_KEY`、`AI_PUB_API_KEY`、`AI_PUB_BASE_URL` 是 GitLab Group 或项目级受保护变量；`SERVICE_KEY` 是当前构建模块名。CI 不保存发布系统内部 `service_id`，也不为每个服务维护不同接口。
- `jq -n` 生成 JSON，避免提交信息、分支名或 URL 中的引号、换行破坏 JSON。
- HTTP `200` 与 `201` 都是成功；非 2xx 让 Job 失败，便于构建方修复版本登记而不是静默遗漏。
- 若 GitLab Runner 没有 `jq`，应在构建镜像中提供它；不回退到字符串拼接 JSON。

## 5. 后端 OCI 镜像部署 profile

发布系统 SSH 执行器以环境变量调用部署目标的 `script_path`，脚本不接收位置参数。本期新增一个仅用于后端 OCI 镜像的脚本，例如：

```text
/opt/ai-pub/bin/deploy-backend-image.sh
```

### 5.1 输入边界

发布系统已有且必须保留的注入变量：

| 变量 | 作用 |
|---|---|
| `AI_PUB_VERSION` | 人类可读版本 |
| `AI_PUB_COMMIT_SHA` | 源码追溯 |
| `AI_PUB_ARTIFACT_URL` | 此 profile 必填，完整 OCI digest 引用 |
| `AI_PUB_RELEASE_ID` | 发布单追溯 |
| `AI_PUB_DEPLOY_ID` | 部署记录追溯 |
| `AI_PUB_SERVICE_ID` / `AI_PUB_ENVIRONMENT_ID` | 系统对象追溯 |

部署目标的静态 `env_vars` 配置脚本所需的非敏感业务参数，例如：

| 变量 | 作用 |
|---|---|
| `APP_SERVICE_NAME` | Docker Compose 服务名和容器名，必填 |
| `APP_DEPLOY_DIR` | 每服务部署目录，例如 `/data/service/order-api` |
| `APP_ENV_FILE` | 服务器上的受保护环境文件路径 |
| `APP_HEALTHCHECK_CMD` | 可选的部署后健康检查命令 |

密码、注册表登录凭据、Nacos 配置、监控 token 等不通过 CI 回调、版本 metadata 或部署目标环境变量明文传递。它们应保存在服务器受保护环境文件、Docker 登录状态或后续凭据管理中。

### 5.2 脚本职责

`deploy-backend-image.sh` 必须：

1. 使用 `set -euo pipefail`，校验 `AI_PUB_ARTIFACT_URL`、`APP_SERVICE_NAME`、`APP_DEPLOY_DIR`。
2. 直接以 `AI_PUB_ARTIFACT_URL` 写入 Compose 配置并拉取镜像；不得根据服务名与 tag 重建镜像地址。
3. `docker compose pull` 成功后再执行 `docker compose up -d`。
4. 检查容器仍在运行；配置了 `APP_HEALTHCHECK_CMD` 时，健康检查失败必须退出非零。
5. 输出不含凭据的执行摘要（服务、版本、commit 短 SHA、release ID、deploy ID、镜像仓库路径）。
6. 所有失败都返回非零，让 Worker 将失败原因写入服务器部署日志。

脚本不应：

- 依赖 `$1`、`$2` 等位置参数。
- 包含前端 tar 包下载、Nginx 文件替换或根据服务名后缀判断类型。
- 创建或删除 N9E 告警屏蔽。
- 使用 `docker image prune -a -f` 清理共享主机的全部未运行镜像。
- 内嵌密码、API token 或其他长期凭据。

当前 `scripts/deploy.sh` 仍属于待替换的用户脚本：它要求位置参数，而执行器注入的是 `AI_PUB_*` 环境变量；同时它自行拼接镜像地址。因此不作为本设计的后端脚本基线，也不在本次文档工作中修改。

## 6. 版本选择与按目标预检

版本登记成功后，发布页应：

1. 切换服务时只加载或筛选该服务版本。
2. 默认展示最近登记的版本，并显示 `version`、短 SHA（如有）、来源 `CI/手动`、登记时间和外部运行链接（如有）。
3. 隐藏完整 `artifact_url`；详情页按现有脱敏规则展示。
4. 保留管理员“手动登记版本”入口，其字段遵循同一 `ServiceVersion` 模型。

发布前预检新增或收紧以下判断：

- 后端 OCI 镜像 profile 缺少 `artifact_url` 时阻断，不只给 warning。
- 后端 OCI 镜像 profile 的 `artifact_url` 不是 OCI digest 引用时阻断。
- 其他部署目标可按自身契约使用 `version` 或其他制品形式，不继承 OCI 限制。
- 服务、版本、部署目标三者不匹配时继续沿用现有阻断。
- 环境冻结、生产管理员确认、运行中发布冲突继续沿用现有发布门禁。

## 7. 审计与故障处理

需要新增 `version_registered` 事件，至少记录：服务、版本 ID、来源 `ci/manual`、API Key ID（CI 时）、外部运行摘要（如有）、commit SHA（如有）和脱敏制品摘要（如有）。

故障边界：

| 环节 | 行为 |
|---|---|
| 外部制品准备失败 | 不调用版本登记 |
| 登记 API 暂时失败 | 外部 CI 重试；同一请求幂等 |
| 同版本制品不一致 | 明确 `409`，由构建人员修复版本规则，不覆盖历史 |
| CI 已登记但未发布 | 正常保留为可选版本 |
| SSH 拉取或健康检查失败 | 发布失败，保留 Worker 日志与发布审计；不修改已登记版本 |

## 8. 实施顺序与验收

1. 后端：新增 `version:write` scope，实现通用版本登记接口、来源写入、字段校验、幂等与冲突处理，写入 `version_registered` 事件。
2. 前端：版本列表按服务筛选、搜索与长版本展示；保留手动登记入口。
3. 服务器：部署专用 `deploy-backend-image.sh` 和受保护环境文件，部署目标改为该脚本和静态 `env_vars`。
4. GitLab：在镜像 push 之后加入版本登记 Job，配置受保护变量，作为通用接口的首个接入示例。
5. 验收：覆盖手动版本登记、外部 CI 版本登记、幂等重试和版本冲突；再执行一次真实非生产 OCI 镜像登记、选择、SSH 发布与健康检查。

代码级验证至少包含受影响的 Go 单测、前端 lint/build；涉及 migration、API、Worker 和 SSH 执行路径时，再执行 `make compose-check`。真实 GitLab、Harbor、SSH 服务器属于外部集成，需在非生产环境进行一次专项验收并记录结果。
