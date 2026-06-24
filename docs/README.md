# 发布系统技术文档索引

本文档目录记录已完成 MVP 的实现设计、验收方法和当前边界。产品需求基线见根目录的 `project-requirements.md`。

建议阅读顺序：

1. `domain-model-design.md`：领域模型、表结构、状态机和事件模型。
2. `backend-architecture-design.md`：Go 后端分层、事务边界、Worker 和执行器 contract。
3. `api-design.md`：REST API、鉴权和幂等。
4. `frontend-ia-design.md`：React 最小管理界面的页面、状态和交互。
5. `DESIGN.md`：前端视觉语言、颜色、排版、组件和页面布局规范。
6. `engineering-scaffold-design.md`：目录结构、配置、migration、测试和本地 demo。
7. `notification-design.md`：企业微信机器人 webhook 通知和渠道扩展点。
8. `service-version-registration-and-backend-oci-deploy-design.md`：通用服务版本登记，以及后端 OCI 镜像部署 profile 与 GitLab CI 接入示例。
9. `development-completion-audit.md`：当前开发完成审查、已验证范围、验收入口和暂缓项。

第一版实现时应始终遵守：

- MySQL 8 是开发、验收和生产环境的唯一运行数据库。
- 第一版优先功能闭环，不考虑高并发、多实例 Worker、复杂 RBAC、多租户和运行中紧急停止。
- 审计只要求关键动作可追溯、可查询，不设计不可篡改账本。
- 企业微信通知使用机器人 webhook，不做企业微信应用消息。

当前容器化验收范围和命令见 `development-completion-audit.md` 与 `local-verification.md`。
