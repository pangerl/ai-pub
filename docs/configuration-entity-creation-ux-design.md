# 配置页实体创建体验优化方案

## 背景

当前配置页（配置中心 → 应用与版本 / 运行环境 / 部署连接）将「已注册实体列表」与「创建新实体表单」平铺在同一视口：左侧 inventory 列表、右侧常驻创建表单，application tab 底部还压一个「服务部署视图」。展示、创建、关联视图三件事挤在一屏，元素过多，对新用户形成混乱。

之前的优化版（在平铺之上加了联动跳转、自动推进、凭据外链入口）没有动「平铺」这个根因，割裂感只是被转移而非消除。本方案重新设计实体创建过程，让新用户能流畅地按业务流程完成首配。

## 目标

- 展示页干净：默认只展示已创建实体，去掉常驻创建表单列。
- 创建按需弹出：新建/编辑统一走 Drawer，不跳出当前页面。
- 业务流连贯：创建 Drawer 之间用预填 + CTA 串成两条平行支线——应用支「项目 → 服务 → 版本」与运行支「环境 → 服务器/服务器组」，在部署目标处汇合，最终到发布单。仅在实体有直接下游依赖时推进，不在支线衔接处硬跳。
- 凭据不外跳：服务器表单内联新建凭据，支持选已有、也支持原地建。

## 核心逻辑

### 1. 列表为主，创建按需弹 Drawer（骨架）

- application / runtime / targeting 三个 tab 去掉右侧 `infrastructure-actions` 常驻表单列，列表 `infrastructure-inventory` 占满宽度。
- 每个 tab 顶部加「新建 XX」主按钮，点击弹创建 Drawer，提交后关闭并刷新列表，不跳出页面。
- 创建 Drawer 是薄壳 `<Drawer>{<XxxForm onDone={...} />}</Drawer>`，复用现有创建表单组件（ProjectForm / ServiceForm / VersionForm / EnvironmentForm / ServerForm / ServerGroupForm / DeploymentTargetForm）。
  - 这些组件自带 `apiPost`，但需最小签名调整（见 §2），不是「表单逻辑零改动」。
  - 现有 `EntityDrawer`（`App.tsx:1330`）是为编辑设计（open 回填 + PATCH），不能直接复用为创建态，但可参照其 Drawer 形态、`footer={null}`、`destroyOnClose` 做法。
- 列表行内能力保持不变：编辑、启用/禁用、冻结入口都在 `EditableInventoryList` / `DeploymentTargetList` 行里，不受布局重排影响。

### 2. 创建状态机（新增，与 openEditor 编辑态并存且互斥）

- `CreatingKind` 枚举（不再用裸 `string`，防拼写错误）：
  ```ts
  type CreatingKind =
    | 'project' | 'service' | 'version'
    | 'environment' | 'server' | 'server-group'
    | 'deployment-target' | 'credential';
  ```
- 新增状态：
  - `creatingKind: CreatingKind | null` —— 当前正在创建的实体类型。
  - `creatingPrefill: { projectID?; serviceID?; environmentID?; targetType?; targetRefID? }` —— 链式推进时的上游预填。
  - `nextCreateAction: { label: string; kind: CreatingKind; prefill: CreatingPrefill; goToCreate?: boolean } | null` —— 创建成功后底部「下一步」CTA，**纯数据**（不在 state 中存函数闭包）。用户点击才推进；关闭 Drawer 即清理。CTA 派发逻辑内联在按钮 `onClick`，不抽命名函数（见 §实施偏差）。
- 方法：
  - `openCreator(kind, prefill?)` 打开创建 Drawer；打开前若 `editingEntity` 非空则先 `closeEditor()`（创建/编辑互斥，避免双抽屉叠加）。
  - `closeCreator()` 关闭并复位 `creatingKind` / `creatingPrefill` / `nextCreateAction`。
- `creatingKind` → Drawer 表单映射集中在一个 switch / 映射表里，不在三处散写。

### 3. onDone 签名调整（链式推进的前提）

现有创建表单的 `onDone` 多为 `() => void`，丢弃了 `apiPost` 返回的新建实体，链式推进拿不到 ID。统一改为返回新建实体：

```ts
type CreatedHandler = (created: Entity) => void;
```

需改签名（仅透传已有返回值，不重写提交逻辑）：`ProjectForm` / `ServiceForm` / `VersionForm` / `EnvironmentForm` / `ServerForm` / `ServerGroupForm` / `DeploymentTargetForm` / `CredentialForm`。

### 4. 链式推进（onDone 设 CTA，不自动强跳）

每个创建成功后，在 Drawer 底部置 `nextCreateAction`，用户点击才推进、预填上游：

| 当前创建 | CTA label | 点击动作 |
|---|---|---|
| 项目 | 为该项目创建服务 | `openCreator('service', { projectID })` |
| 服务 | 为该服务创建版本 | `openCreator('version', { serviceID })` |
| 版本 | （无 CTA） | 应用支自然终点，与运行支无直接依赖，不引导 |
| 环境 | 登记服务器 | `openCreator('server')` |
| 服务器 | 建立部署连接 | `openCreator('deployment-target', { serviceID, environmentID, targetType: 'server', targetRefID })` |
| 服务器组（分支） | 建立部署连接 | `openCreator('deployment-target', { serviceID, environmentID, targetType: 'server_group', targetRefID })` |
| 部署目标 | 创建发布单 | `setPage('create')` |

> 说明：链式推进仅在实体有直接下游依赖时推进，不在两条平行支线（应用支 项目→服务→版本 / 运行支 环境→服务器）的衔接处硬跳。版本是应用支终点，不引导到环境；服务器/服务器组因部署目标依赖运行目标，仍推进到部署连接。

- **不自动强跳**：用户可关掉 Drawer 继续建同类（多个版本、多台服务器），推进权交还用户。
- **服务器组不进主链路，但补分支 CTA**：首配最小路径走单台服务器；服务器组创建完后同样给「建立部署连接」CTA，预填 `targetType: 'server_group'`，避免建完服务器组还要手动找入口。

### 5. creatingPrefill 为唯一预填来源

- `creatingPrefill` 是**唯一**上游预填来源，**不 fallback 到全局 `selection`**。数据流可预测、不受用户在别处点选影响。
- 非链式入口（列表页直接点「新建部署目标」，`creatingPrefill` 为空）时，`DeploymentTargetForm` 的服务/环境字段为空，由用户手动从下拉选——这是为换取链式推进纯粹性所做的取舍。
- 链式入口一定带 prefill：打开部署目标 Drawer 前，由调用方显式写入 `serviceID` / `environmentID` / `targetType` / `targetRefID`。
- CTA 禁用条件：推进前检查上游实体是否仍存在且启用；若已被禁用/删除，CTA 禁用并提示「上游 XX 已不可用」。

### 6. 凭据 inline 新建（二级小 Drawer）

- `ServerForm` 的凭据 `Select` 改用 antd **`popupRender`**（`dropdownRender` 在 antd@6 已 deprecated，见 `web/node_modules/antd/es/select/index.d.ts`），下拉底部加「+ 新建凭据」项。
- 点击「+ 新建凭据」关掉下拉，弹一个 **360px 二级小 Drawer**（不是塞进下拉里），填 name + secret。
- 提交契约（Codex 审查要求明确）：
  1. `CredentialForm.onDone` 改为返回 created credential（见 §3）。
  2. 成功后重新拉取 `credentials` 列表（`refreshAll`），再 `ServerForm` 内 `form.setFieldValue('credential_ref', created.id)` 自动选中。
  3. 失败时保持小 Drawer 输入不丢、给 message；服务器主表单全程不动，其它字段不丢。
- 后端强校验：非 `none` 认证必须引用存在且启用的凭据（`internal/httpapi/inventory.go:345`），inline 建完即回填可满足。
- 已有凭据仍可下拉选（哪怕不是本机凭据）。

### 7. 关联视图降噪

- application tab 底部 `service-detail`「服务部署视图」保留（有业务价值），但从平铺改为默认折叠 / 可展开，减少首屏噪音。

## 涉及文件

- `web/src/App.tsx`：
  - 新增创建状态机（`creatingKind` / `creatingPrefill` / `nextCreateAction` / `openCreator` / `closeCreator`）+ `CreatingKind` 映射。
  - 实现创建 Drawer 薄壳组件（包现有 Form，`footer={null}`，onDone 关闭+刷新+设 CTA）。
  - 改写 application / runtime / targeting 三个 tab 为「列表为主」布局 + 顶部「新建」按钮。
  - 链式 onDone 串联 + `nextCreateAction`。
  - 8 个创建表单 `onDone` 签名改为 `(created) => void`。
  - ServerForm 凭据 `popupRender` + 二级小 Drawer inline 新建。
  - service-detail 折叠化。
- `web/src/styles.css`：
  - `infrastructure-actions` 相关样式作废或调整为单列列表布局。
  - 新增创建 CTA（Drawer 底部）样式。

## 潜在风险

1. **结构性改动**：三个 tab 布局重排，是本次改动面最大的部分。需逐 tab 验证不丢功能——启用/禁用、冻结、编辑入口都在列表行里，不受影响（见 `App.tsx:1290` `EditableInventoryList`、`App.tsx:1234` `DeploymentTargetList`）。
2. **薄壳嵌套**：现有 Form 组件自带提交按钮和 `apiPost`，包进 Drawer 后用 `footer={null}` 避免双重渲染，沿用编辑 Drawer 做法。
3. **链式中途关闭**：用户随时关 Drawer，`closeCreator()` 要干净复位 `creatingKind` / `creatingPrefill` / `nextCreateAction`，不残留半开的下一环。
4. **凭据 inline 是 Secret 表单**：建完即用、小 Drawer `destroyOnClose` 销毁，secret 不回显（产品既定规则）；失败保输入。
5. **失败处理**：创建失败不 reset 表单、保输入 + message；刷新失败给 message；CTA 点击时上游被禁用/删除则禁用 CTA 提示。
6. **权限边界**：配置页本身已 admin-only；创建接口均走 `withAdmin`（`internal/httpapi/router.go:34/38/44/47/55/65`），无需额外前端判断。

## 取舍说明

- **不做独立创建向导页**：当前单文件 `App.tsx` + 手写 `pushState`（`App.tsx:100/125/226`）无路由库，独立页架构成本高；且创建下游依赖看上游现状，跳页丢上下文，与「连贯」诉求冲突。编辑实体早已是 Drawer 模式，创建复用同一心智更优。
- **链式推进用 CTA 而非自动跳转**：避免管理员连续建同类实体（多个版本、多台服务器）时被打断。推进权交还用户。
- **`creatingPrefill` 唯一来源、不 fallback `selection`**：牺牲非链式入口的便利（需手动选服务/环境），换链式推进数据流完全可预测。
- **服务器组不进主链路、补分支 CTA**：首配最小路径走单台服务器，但服务器组建完也给推进 CTA，不断流。
- **凭据用二级小 Drawer 而非嵌入下拉**：secret 敏感输入需稳定交互，避免下拉内误触丢输入。
- **`service-detail` 折叠**：保留关联视图价值，但降首屏噪音。

## 实施偏差（落地与方案的差异）

实施中因 React Compiler 严格约束与业务边界，对方案做了以下调整：

- **`nextCreateAction` 改纯数据**：原方案 CTA 含 `run: () => void` 闭包。实际把 state 中存函数闭包会让 React Compiler 放弃保留 `refreshWithSelection`/`changeSelection` 等手写 `useCallback` 的 memo（`react-hooks/preserve-manual-memoization` 报错）。改为 `{ label, kind, prefill, goToCreate? }` 纯数据，CTA 派发逻辑内联在按钮 `onClick`（与原有 `onDone` 闭包同性质，Compiler 接受）。**不抽命名派发函数**——命名函数读 `nextCreateAction` state 会触发同一问题。
- **CreateDrawer 内联**：原方案抽 `CreateDrawer` 薄壳组件。实际内联进配置页 JSX（直接用 App 的 `state`/`creatingPrefill`/`handleCreated`），减少跨组件闭包传递。`createDrawerTitles` 提为模块常量。
- **server/server-group → 部署目标 CTA 不预填 service/env**：原方案 CTA 带完整 `serviceID/environmentID`。实际 server 创建时 `created` 不携带 service/env，且 `handleCreated` 读 `creatingPrefill`（state）同样触发 Compiler 报错。改为 CTA 只预填 `targetType/targetRefID`，service/env 留给用户在部署目标表单内手选（符合 §5「非链式入口手选」取向）。
- **`handleCreated` 不读 `creatingPrefill`**：所有上游 ID 尽量从 `created` 推导（project→service 用 `created.id`，service→version 用 `created.id`，version→environment 无需上游，deployment-target→create 用 `created.service_id/environment_id`）。
- **`"use no memo"` 弃用**：React Compiler 的退出指令对该 eslint 规则无效，最终靠「纯数据 CTA + 内联派发」根治。
- **移除 `manualTargetRef` state**：原 runtime/server 的 `manualTargetRef` 联动被 `creatingPrefill` 取代，useState 声明删除（`ManualTargetRef` 类型保留，供 `DeploymentTargetForm.preferredTargetRef` 使用）。

## 验收

- `make verify`（Go 测试 + 前端 lint + build）作为基础门槛；前端无测试脚本（`web/package.json` 仅 dev/build/lint），UI 正确性靠手工回归。
- **手工回归清单**：
  1. application / runtime / targeting 三 tab 列表为主布局，常驻表单已移除。
  2. 各 tab「新建」按钮弹创建 Drawer，提交后关闭并刷新列表，不跳页。
  3. 列表行编辑 / 启用-禁用 / 冻结均正常。
  4. 链式：项目 → 服务（CTA）→ 版本（版本无 CTA，应用支终点）；环境 → 服务器/服务器组 → 部署目标（CTA 校验 service/env 存在性后预填）→ 发布单。
  5. 服务器组创建后分支 CTA「建立部署连接」预填 `targetType=server_group`。
  6. 服务器凭据：选已有凭据可用；点「+新建凭据」弹小 Drawer，建完自动回填 `credential_ref` 并刷新；失败保输入。
  7. 非链式入口（列表页直接新建部署目标）服务/环境为空、可手动选。
  8. 部署目标 CTA「创建发布单」跳 `page=create`。
  9. 创建/编辑 Drawer 互斥，不同时开。
  10. CTA 上游被禁用/删除时禁用并提示。

## 实施任务清单

> 审核通过后按此分步实施。

1. 8 个创建表单 `onDone` 签名改为 `(created: Entity) => void`（透传已有 `apiPost` 返回值）。
2. 新增创建状态机：`CreatingKind` 枚举 + `creatingKind` / `creatingPrefill` / `nextCreateAction` / `openCreator` / `closeCreator` + kind→表单映射。
3. 实现创建 Drawer 薄壳组件（包现有 Form，`footer={null}`，onDone 关闭+刷新+设 `nextCreateAction`）。
4. application tab 改为列表为主 + 顶部「新建」按钮。
5. runtime tab 同上。
6. targeting tab 同上 + 保留 DependencyChecklist。
7. 链式推进：各创建 onDone 设 `nextCreateAction` + 预填上游；服务器组补分支 CTA。
8. `creatingPrefill` 唯一来源落地，移除对 `selection` 的隐式依赖；CTA 上游校验。
9. ServerForm 凭据 `popupRender` + 二级小 Drawer inline 新建。
10. service-detail 折叠化。
11. styles.css 清理 `infrastructure-actions` + 新增 Drawer 底部 CTA 样式。
12. `make verify` + 手工回归清单逐项验收。
