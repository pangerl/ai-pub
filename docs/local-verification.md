# 本地功能验证

本文档用于验证第一版轻量发布执行闭环。验证范围使用 Docker Compose、MySQL 8、内置 Worker 和 Mock 执行器，不依赖真实服务器或外部通知服务。

## 启动服务

```bash
docker compose up --build -d
```

访问前端：`http://127.0.0.1:18080/`。MySQL 和后端仅在 Compose 网络中暴露，不占用宿主机数据库或后端端口。

## 自动验证

执行：

```bash
make local-check
```

脚本会自动完成：

- 创建项目、服务、两个版本、环境、Mock 服务器、服务器组、成功/partial/失败部署目标和用户。
- 创建待确认发布单并验证驳回。
- 创建待确认发布单并验证取消。
- 创建并确认成功发布单，等待 Worker 执行成功；版本未配置制品地址时 preflight 会保留 warning。
- 创建并确认服务器组发布单，验证 `server_group` 目标可展开执行。
- 创建并确认三台服务器的部分失败发布单，验证发布记录为 `partial` 且未执行服务器为 `skipped`。
- 创建并确认第二个成功发布单，用于产生回滚候选。
- 创建并确认失败发布单，验证失败聚合。
- 创建并确认回滚发布单，验证回滚到上一版本。
- 检查发布记录、服务器日志和审计事件。

成功时最后会输出：

```text
[local-check] local functional check passed
```

`make compose-check` 保留为与 `make local-check` 等价的兼容入口。可选直接执行 Compose 验收容器：

```bash
docker compose --profile verify up --build --abort-on-container-exit --exit-code-from verify verify
```

## 前端人工验证

推荐顺序：

1. 打开 `http://127.0.0.1:18080/` 并以管理员登录。
2. 首次没有可发布配置时，工作台应显示“定义应用、准备运行环境、建立部署连接”三步清单；每一步进入“配置”中的对应工作区。
3. 创建项目、服务、版本、环境、服务器和部署目标。发布目标使用 `mock` 执行器、`server` 目标类型即可，不需要真实服务器。
4. 从“发布”进入发布中心，点击“创建发布单”，执行 preflight 并创建发布单。
5. 对待确认发布单验证驳回、取消或确认入队；进入发布记录查看服务器日志。
6. 对失败或部分成功发布单创建重新发布单或回滚单，确认它们都是新发布单并重新走预检与确认。
7. 在“配置”检查服务器组、部署目标和当前版本视图；在“策略”检查最终生效策略和冻结提示；在“系统”验证用户、通知和凭据管理。
8. 在右上角“访问密钥”创建个人访问密钥，确认明文只显示一次，随后可禁用、启用或删除。
9. 打开或刷新 `/releases`、`/releases/new`、`/releases/{id}`、`/deploys`，确认均能回到相应页面；普通用户访问管理员路径应返回工作台。

如需验证 API Key 调用发布接口，可使用创建时返回的一次性明文作为 Bearer token：

```bash
curl -X POST http://127.0.0.1:18080/api/v1/release-requests \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{"service_id":"...","environment_id":"...","service_version_id":"...","deployment_target_id":"..."}'
```

API Key 读取发布单、事件和回滚候选需要 `release:read`，发布前 preflight 和创建发布单需要 `release:create`，创建发布单会记录创建时的 `preflight_checked` 事件，已有发布单 preflight 需要 `release:read` 并会再次写入 `preflight_checked` 事件，确认、驳回、取消发布单需要 `release:confirm`，创建回滚单需要 `release:rollback`，读取部署记录和服务器日志需要 `deploy:read`，管理基础配置、API Key、凭据、通知和发布策略需要 `admin:write`。用户禁用后不能确认发布；通过 API Key 创建、确认、驳回、取消或回滚发布单时，事件流会记录 `api_key_id`；禁用、过期或 scope 不足会被拒绝。

通知链路的本地验证以 `go test ./internal/app ./internal/httpapi ./internal/e2e` 为准，覆盖通知配置创建、启用/禁用、发送记录、生产待管理员确认通知、发布失败通知触发入口、回滚申请通知触发入口，以及通知发送成功/失败写入发布事件流。

## 真实企业微信机器人专项验收

已使用真实企业微信机器人 webhook 创建加密通知配置并执行测试发送。企业微信响应被校验为 `errcode=0`，本地 `notification_test` 投递记录为 `sent`。运行镜像包含 `ca-certificates`，TLS 校验失败时错误文本会脱敏 webhook URL。

## 真实 SSH 专项验收

已在一台非生产测试服务器完成密码认证专项：先使用 SSH 发布目标执行 `test -x /home/dm/service/deploy.sh` 验证连接和脚本可执行性，再执行 `/home/dm/service/deploy.sh`。两次发布均为 `success`，实际脚本发布退出码为 `0`，服务器日志未包含凭据。

专项配置使用 `password` 类型凭据并由凭据服务加密保存；不得将真实密码写入版本库、发布目标环境变量、日志或审计事件。

MySQL 8 容器下的 Mock 发布闭环必须稳定可重复。
