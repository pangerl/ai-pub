# Demo 公网部署安全加固

记录 AI Pub demo 站点公网部署的威胁模型与加固方案。demo 目标：让任意访客试用发布流程（mock 执行器），同时防止 demo 容器被用作 SSH 跳板或被默认凭据接管。demo 使用与用户一致的发布镜像（`hxjagf/ai-pub:latest`），兼作发布镜像可用性的活体验证，故不叠加 `compose.local-build.yaml` 走本地源码构建。

## 威胁模型

demo 站点暴露公网，任意访客可访问登录页与 API。需防御：

1. **默认凭据接管**：compose 默认 `admin/ai-pub-dev-admin` + `dev-encryption-key` + `dev-secret-change-me`，README 还公开了默认密码。攻击者可直接登录或伪造 JWT。
2. **SSH 跳板**：系统支持 SSH 执行器，admin 可配置服务器 + 凭据 + script_path，worker 通过容器内 `ssh` 二进制向远端执行命令。demo 容器可能被用作内网跳板。
3. **K8s 外联**：K8s 预检器在发布单预检阶段访问 K8s API，admin 可配置 kubeconfig 指向任意 URL，构成 SSRF。
4. **容器提权**：容器以 root 运行、无 cap_drop/read_only/no-new-privileges，RCE 后爆炸半径大。
5. **密钥回退陷阱**：dev 模式下 `JWT_SECRET`/`APP_ENCRYPTION_KEY` 为空时回退到公开默认值（`internal/config/config.go` `env()` fallback + `internal/crypto/secret.go`），程序照常启动。

核心判断：**通过 API 在 demo 容器内执行任意命令基本不可行**——worker 在容器内唯一的本地命令调用是 `ssh` 二进制，以 argv 切片方式调用（`internal/executor/executor.go:257`），不经本地 shell；用户可控的 `script_path` 作为 ssh 远端命令参数发往远端执行（`executor.go:182,195`），不在 demo 容器本地执行；版本号等用户可控值经 `shellQuote` 单引号转义（`executor.go:440-471`）。真实风险是默认凭据链 + 执行器无法禁用导致的跳板/外联。

## 加固措施

### 1. 执行器开关（代码层消除跳板/外联）

新增 `EXECUTOR_SSH_DISABLED` / `EXECUTOR_K8S_DISABLED` 配置（默认 `false`，保持现有行为）。demo 设 `true`：

- **worker 注册层**：`internal/worker/worker.go` `NewService` 按开关决定是否注册 ssh/k8s 执行器。禁用后 Registry 只有 mock；即便数据库存在 `executor_type=ssh` 的目标，worker 执行时 `executors.Get("ssh")` 返回 `!ok`，走 `unsupported executor` 失败分支（`worker.go:179-186`），**不调用 ssh 二进制**。
- **server test 端点**：`/api/v1/servers/test`、`/api/v1/servers/{id}/test` 是独立于 worker 的 SSH 调用路径（`SSH.Check`）。`EXECUTOR_SSH_DISABLED=true` 时返回 `400 ssh_test_disabled`（`internal/httpapi/infrastructure.go`）。
- **K8s 预检**：`internal/httpapi/router.go` 按 `ExecutorK8sDisabled` 决定是否注入 K8s preflight checker。为 true 时不注入，`internal/app/release.go` 的 `k8sPreflight != nil` 判断自动跳过 K8s API 外联。

效果：demo 只保留 mock 执行器，发布闭环可完整演示（创建→确认→mock 执行→审计→发布记录），但无法触发任何真实 SSH/K8s 外联。

### 2. 密钥强制非空（防回退陷阱）

`deploy/compose.demo.yaml` 三个密钥用 docker compose `${VAR:?msg}` 语法：

```yaml
APP_ENCRYPTION_KEY: ${APP_ENCRYPTION_KEY:?APP_ENCRYPTION_KEY is required}
JWT_SECRET: ${JWT_SECRET:?JWT_SECRET is required}
BOOTSTRAP_ADMIN_PASSWORD: ${BOOTSTRAP_ADMIN_PASSWORD:?BOOTSTRAP_ADMIN_PASSWORD is required}
```

未设或为空时 compose 直接报错不启动，避免 dev 模式回退到公开默认密钥。密钥值由 `.env`（已 gitignore）提供，用 `openssl rand -hex 32` 生成。

**注意**：`docker compose -f` 时 project directory 是 `deploy/`，不会默认加载项目根的 `.env`，故部署命令必须显式 `--env-file .env`（或 `make demo-up`）。

### 3. 容器加固

`deploy/compose.demo.yaml`：

- `cap_drop: [ALL]`：移除所有 Linux capabilities。
- `security_opt: [no-new-privileges:true]`：禁止 setuid 提权。
- `read_only: true`：容器根文件系统只读（sqlite 写 `/data` named volume，不受影响；`/tmp` 用 tmpfs）。
- `tmpfs: [/tmp:rw,noexec,nosuid,size=64m]`：临时目录可写不可执行。
- 端口绑 `127.0.0.1:18080`：公网访问走反向代理。

### 4. 反向代理 + TLS

`deploy/examples/Caddyfile`（主推荐）：Caddy 自动 TLS + `rate_limit`（caddy-ratelimit 插件）保护登录接口每 IP 每分钟 5 次（补偿应用层无速率限制的 F2 风险）+ `request_body max_size 1MB` 防超大 body DoS。需含插件的自定义 build（见 `deploy/examples/Caddy.Dockerfile`，`xcaddy build --with github.com/mholt/caddy-ratelimit`）。

`deploy/examples/nginx-demo.conf`（备选）：Nginx `limit_req` 内置开箱即用，`client_max_body_size 1m`，适合不愿自定义 build Caddy 的场景。

## 残余风险（demo 可接受）

| 风险 | 说明 | 缓解 |
|---|---|---|
| 发布执行接口无 admin 检查 | 任意已登录用户可创建/确认/回滚发布单（`internal/httpapi/release.go` 的 `createRelease`、`confirmRelease`、`createRollbackRelease`、`retryRelease` 等 handler） | demo 希望任意用户体验发布；mock 执行器无真实副作用 |
| 登录无速率限制 | 应用层无 rate limit（`auth.go:14-37`） | Caddy `rate_limit`（caddy-ratelimit 插件）或 Nginx `limit_req` 补偿 |
| `JWT_SECRET=""` 免鉴权后门 | `middleware.go:32-34` 空值绕过 | compose `:?` 强制非空，未改代码 |
| Cookie Secure 反代后失效 | `session.go:34` `r.TLS!=nil` | demo 接受，后续可固定 `Secure: true` |
| IDOR | 发布单/部署记录无 ownership | demo 数据可重置 |
| 无密码修改接口 | `SetUserPassword` 无 HTTP handler | demo 数据可重置 |

## 数据重置策略

demo 数据定期重置：

```bash
make demo-down  # 删除容器与 sqlite 卷
make demo-up    # 重新启动（拉取发布镜像），数据归零
```

访客请勿存放敏感信息。bootstrap admin 密码由 `.env` 的 `BOOTSTRAP_ADMIN_PASSWORD` 决定，重置后不变（除非改 `.env`）。

## 验证清单

- `EXECUTOR_SSH_DISABLED=true` 时创建 ssh 目标并执行 → worker 返回 `unsupported executor`，无 ssh 进程。
- `POST /api/v1/servers/test` → 返回 `400 ssh_test_disabled`。
- `EXECUTOR_K8S_DISABLED=true` 时 k8s 目标预检 → 不发起 K8s API 外联。
- 删除 `.env` 的 `JWT_SECRET` → `make demo-up` 报错不启动。
- `docker inspect` 确认 `CapDrop=[ALL]`、`ReadonlyRootfs=true`。
- mock 目标发布闭环完整：创建→确认→执行→审计日志→发布记录。
