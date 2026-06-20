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

1. 打开 `http://127.0.0.1:18080/`。
2. 点击 `初始化 Mock 配置`。
3. 在工作台选择服务、环境、版本、部署目标和确认用户。
4. 点击 `创建发布单`。
5. 可选点击 `驳回` 或 `取消` 验证待确认发布单状态变更。
6. 对新的待确认发布单点击 `确认入队`。
7. 进入 `发布记录`，点击对应记录的 `查看日志`，确认服务器日志显示服务器名称且状态为 success。
8. 未点击 `查看日志` 前，服务器日志区应为空；切换发布记录状态筛选或当前服务/环境筛选后，旧日志不应残留。
9. 发布单进入 success、failed、partial、rejected 或 cancelled 后，确认、驳回和取消按钮应按状态禁用，避免重复操作。
10. 回到 `工作台`，点击 `模拟失败发布`。
11. 点击 `确认入队`，再到 `发布记录` 查看失败日志。
12. 在 `发布中心` 使用状态筛选查看 success、failed、partial、cancelled 等发布单，并确认列表显示服务、环境、版本、事件 actor 和部署记录上下文。
13. 切换发布中心状态筛选或当前服务/环境筛选后，旧事件流不应残留；点击某个发布单的 `查看` 后再展示对应事件流。
14. 选择一个成功发布单，确认工作台的服务、环境、版本和部署目标同步到该发布单后，点击 `创建回滚单`，再确认入队；回滚单创建后工作台上下文应同步到新的回滚版本和目标。
15. 到 `基础配置` 查看服务器组、部署目标和当前版本视图，确认服务器组成员名称、目标服务器或服务器组名称、服务、环境、版本和部署记录上下文可读。

如需验证手动配置路径，可不点击 `初始化 Mock 配置`，先进入 `手动创建`：

1. 依次创建项目、服务、版本、环境、服务器、发布目标和确认用户。
2. 发布目标使用 `mock` 执行器、`server` 目标类型即可，不需要真实服务器。
3. 创建服务器后，发布目标表单应自动指向刚创建的服务器；创建发布目标后，工作台部署目标应同步到该目标。
4. 可选创建服务器组，再创建 `server_group` 发布目标验证服务器组路径，发布目标表单应能自动指向刚创建的服务器组。
5. 可选创建 API Key，确认页面只在创建后展示一次明文，列表只展示 prefix；再验证禁用、启用和删除操作。
6. 可选创建通知配置，使用本地不可用 webhook 测试失败投递，确认通知投递列表能看到 `notification_test / failed`，然后禁用该测试配置。
7. 可选保存系统发布策略，勾选冻结后确认发布策略列表显示 `manual_freeze_enabled=true`；验收结束前取消冻结并保存为 `false`。
8. 可选创建凭据，确认凭据列表能看到名称、类型和启用状态，但不展示 secret。
9. 回到 `工作台`，确认刚创建的服务、版本、环境、发布目标和用户已自动带入选择。
10. 按上面的发布、确认、日志和回滚流程验证。

如需验证 API Key 调用发布接口，可使用创建时返回的一次性明文作为 Bearer token：

```bash
curl -X POST http://127.0.0.1:18080/api/v1/release-requests \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{"service_id":"...","environment_id":"...","service_version_id":"...","deployment_target_id":"..."}'
```

API Key 读取发布单、事件和回滚候选需要 `release:read`，发布前 preflight 和创建发布单需要 `release:create`，创建发布单会记录创建时的 `preflight_checked` 事件，已有发布单 preflight 需要 `release:read` 并会再次写入 `preflight_checked` 事件，确认、驳回、取消发布单需要 `release:confirm`，创建回滚单需要 `release:rollback`，读取部署记录和服务器日志需要 `deploy:read`，管理基础配置、API Key、凭据、通知和发布策略需要 `admin:write`。用户禁用后不能确认发布；通过 API Key 创建、确认、驳回、取消或回滚发布单时，事件流会记录 `api_key_id`；禁用、过期或 scope 不足会被拒绝。

通知链路的本地验证以 `go test ./internal/app ./internal/httpapi ./internal/e2e` 为准，覆盖通知配置创建、启用/禁用、发送记录、生产待管理员确认通知、发布失败通知触发入口、回滚申请通知触发入口，以及通知发送成功/失败写入发布事件流。真实企业微信 webhook 发送仍属于外部集成验证。

## 当前不验证

- 真实 SSH 服务器连接和真实发布脚本。
- SSH 密码登录。
- 真实企业微信 webhook 发送。

这些能力属于后续专项验证；MySQL 8 容器下的 Mock 发布闭环必须稳定可重复。
