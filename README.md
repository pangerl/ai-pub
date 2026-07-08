# AI Pub

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)
![React](https://img.shields.io/badge/React-19-61DAFB.svg)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED.svg)

AI Pub 是一个面向中小团队的轻量发布执行系统，用 Web/API 串起版本登记、发布确认、部署执行、事件审计和发布记录查询。

它不是完整 CI/CD 平台的替代品，而是把“谁发布了什么、发布到哪里、结果如何、是否可追溯”这条链路收口成一个简单、可落地的内部工具。

## Demo

推荐先体验在线演示，再决定是否本地运行。

[体验地址：https://pub-demo.lanpang.top/](https://pub-demo.lanpang.top/)

- 体验账号：`demo`
- 体验密码：`ai-pub-demo-2026`
- 演示说明：执行器为 mock，可体验主要配置与发布流程，不会触发真实 SSH/K8s 外联。
- 数据说明：演示环境会定期重置，请勿存放敏感信息。

## 界面预览

| 工作台 | 发布配置中心 |
| --- | --- |
| ![AI Pub 工作台](docs/images/demo-workbench.png) | ![AI Pub 发布配置中心](docs/images/demo-release.png) |

## Why AI Pub

很多团队已经有 CI、镜像仓库、SSH 脚本或 K8s 部署方式，但缺少一个轻量的发布执行面板：

- 发布对象分散在项目、服务、环境、服务器和脚本里，缺少统一视图。
- 发布动作靠聊天确认或手工命令推进，事后难以追踪。
- CI 负责构建，但“什么时候发、谁确认、发到哪里、结果如何”仍需要业务侧记录。
- 引入完整流水线平台又太重，第一步只需要清晰、可审计、可复用的发布闭环。

AI Pub 的目标是用尽量少的系统心智，补上这段发布执行和审计链路。

## 核心能力

- **发布对象管理**：项目、服务、版本、环境、服务器、K8s 集群和部署目标。
- **发布执行闭环**：发布单创建、预检、确认、执行、状态追踪、重试和回滚入口。
- **可观测与审计**：发布事件、部署记录、目标日志和当前版本状态。
- **自动化接入**：REST API 与 Agent 入口，可由 CI 或 AI Agent 发起受控发布。
- **轻量运行**：Go 后端 + React 前端打包为单个应用镜像，MySQL 作为正式数据库。

## 适合场景

- 中小团队希望把发布执行、确认和记录从聊天/表格中收口出来。
- 已有构建流程，只缺少版本登记、环境选择、发布确认和部署记录。
- 内部系统需要保留“谁在什么时候把哪个版本发到哪个环境”的审计线索。
- 希望优先落地 MVP，而不是一次性建设复杂流水线平台。

## 不适合场景

- 替代 Jenkins、GitLab CI、Argo CD 等完整 CI/CD 或 GitOps 平台。
- 面向多租户商业 SaaS 的权限、计费、隔离和组织管理。
- 高并发任务调度、复杂编排、跨区域容灾等平台级能力。

## 技术栈

- 后端：Go、`net/http`、`database/sql`、显式 SQL repository。
- 前端：React、TypeScript、Vite、Ant Design。
- 数据库：MySQL 8 是生产和正式集成验收数据库；SQLite 仅用于 demo/local 轻量模式和 Go 单测。
- 打包：前后端构建到同一个 `app` 容器镜像。

## Quick Start

使用 Docker Compose 启动 MySQL 本地环境：

```bash
cd deploy
docker compose -f compose.mysql.yaml up -d
```

访问应用：[http://127.0.0.1:18080/](http://127.0.0.1:18080/)

默认管理员仅适合本地体验：

- 用户名：`admin`
- 密码：`ai-pub-dev-admin`

基于当前源码构建并启动：

```bash
make compose-up
```

更多本地运行、SQLite 轻量模式和验收说明见 [本地功能验证](docs/local-verification.md)。

## 验证

代码级检查：

```bash
make verify
```

MySQL Compose 端到端验收：

```bash
make compose-check
```

SQLite demo/local 轻量验收：

```bash
make compose-check-sqlite
```

- `make verify`：运行 Go 测试、前端 lint 和生产构建。
- `make compose-check`：从空 MySQL 数据库启动容器并执行端到端发布闭环。
- `make compose-check-sqlite`：SQLite demo/local 轻量验收，不替代 MySQL 正式验收。

## 文档

- [技术文档索引](docs/README.md)
- [API 设计](docs/api-design.md)
- [后端架构设计](docs/backend-architecture-design.md)
- [前端信息架构](docs/frontend-ia-design.md)
- [本地功能验证](docs/local-verification.md)

## Roadmap

- 完善 K8s Deployment 执行器体验与验收覆盖。
- 补齐更多 CI/Agent 接入示例。
- 增强发布记录筛选、检索和审计视图。
- 评估 PostgreSQL 支持边界。

## Contributing

欢迎通过 [GitHub Issues](https://github.com/pangerl/ai-pub/issues) 反馈问题、讨论需求或提交 PR。提交问题时请尽量包含复现步骤、期望行为和相关日志。

## License

AI Pub 使用 [MIT License](LICENSE)。
