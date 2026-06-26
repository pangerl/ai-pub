import {
  Alert,
  Button,
  Checkbox,
  ConfigProvider,
  Drawer,
  Form,
  Input,
  Pagination,
  Popconfirm,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';

type ScalarValue = string | number | boolean | null | undefined;
type Entity = Record<string, ScalarValue | string[]> & {
  id?: ScalarValue;
  name?: ScalarValue;
  project_id?: ScalarValue;
  service_id?: ScalarValue;
  environment_id?: ScalarValue;
  service_version_id?: ScalarValue;
  deployment_target_id?: ScalarValue;
  release_request_id?: ScalarValue;
  target_ref_id?: ScalarValue;
  target_type?: ScalarValue;
  executor_type?: ScalarValue;
  status?: ScalarValue;
  display_name?: ScalarValue;
  version?: ScalarValue;
  _kind?: string;
};

type ReleaseResponse = {
  release: Entity;
  next_action: string;
};

type APIKeyCreateResponse = {
  key: Entity;
  plaintext: string;
};

type PreflightItem = {
  code: string;
  level: string;
  message: string;
};

type PreflightResult = {
  result: string;
  next_action: string;
  confirm_mode: string;
  items: PreflightItem[];
};

type AppState = {
  projects: Entity[];
  services: Entity[];
  versions: Entity[];
  environments: Entity[];
  servers: Entity[];
  serverGroups: Entity[];
  targets: Entity[];
  users: Entity[];
  apiKeys: Entity[];
  credentials: Entity[];
  events: Entity[];
  serverLogs: Entity[];
  states: Entity[];
  notificationConfigs: Entity[];
  notificationDeliveries: Entity[];
  ops: Entity | null;
};

// 服务端分页列表响应结构（与后端 {items,total,page,page_size} 对齐）。
type PagedList<T> = { items: T[]; total: number; page: number; page_size: number };

// 创建发布单 / 发布详情展示上下文：允许 fallback 到第一个可用对象。
// user 不进此结构，统一来自登录态 currentUser。
type Selection = {
  serviceID: string;
  environmentID: string;
  versionID: string;
  targetID: string;
};

// 发布中心列表筛选：空 serviceID/environmentID/projectID 表示全部，禁止 fallback。
type ReleaseListFilters = {
  view: ReleaseView;
  projectID: string;
  serviceID: string;
  environmentID: string;
  status: string;
  source: string;
  timeRange: string;
  query: string;
  page: number;
};

// 发布记录列表筛选：空字段表示全部；releaseRequestID 优先级高于 service/environment。
type DeployListFilters = {
  releaseRequestID: string;
  serviceID: string;
  environmentID: string;
  status: string;
  page: number;
};

type ManualTargetRef = {
  targetType: string;
  targetRefID: string;
};

type Page = 'workbench' | 'create' | 'releases' | 'release-detail' | 'deploys' | 'configuration' | 'management' | 'api-keys';
type ReleaseView = 'pending' | 'mine' | 'all';
type InfrastructureView = 'overview' | 'application' | 'runtime' | 'targeting' | 'state';
type ManagementView = 'overview' | 'users' | 'access' | 'notifications' | 'credentials';

// 配置页创建抽屉：正在创建的实体类型（枚举防拼写错误）。
type CreatingKind =
  | 'project' | 'service' | 'version'
  | 'environment' | 'server' | 'server-group'
  | 'deployment-target';

// 创建链式推进时的上游预填；creatingPrefill 为唯一预填来源，不 fallback 到 selection。
type CreatingPrefill = {
  projectID?: string;
  serviceID?: string;
  environmentID?: string;
  targetType?: string;
  targetRefID?: string;
};

// 创建成功后底部「下一步」CTA；纯数据，由渲染层派发（不在 state 中存函数闭包，避免 React Compiler 放弃 memo）。
// goToCreate 场景下 targetID 为刚创建的部署目标 id（与 prefill.targetRefID「运行目标 id」语义区分）。
type NextCreateAction = { label: string; kind: CreatingKind; prefill: CreatingPrefill; goToCreate?: boolean; targetID?: string } | null;

// 系统页创建抽屉：与配置页独立，实体间无业务流依赖，不做链式推进。
type ManagementCreatingKind = 'user' | 'api-key' | 'notification' | 'credential';

type AppRoute = {
  page: Page;
  releaseID?: string;
  // 当前 URL 的 query 参数；发布中心与发布记录页都从这里读取筛选。
  search: URLSearchParams;
};

function routeFromLocation(): AppRoute {
  const path = window.location.pathname.replace(/\/+$/, '') || '/';
  const search = new URLSearchParams(window.location.search);
  const releaseMatch = path.match(/^\/releases\/([^/]+)$/);
  if (releaseMatch && releaseMatch[1] !== 'new') return { page: 'release-detail', releaseID: decodeURIComponent(releaseMatch[1]), search };
  switch (path) {
    case '/releases/new': return { page: 'create', search };
    case '/releases': return { page: 'releases', search };
    case '/deploys': return { page: 'deploys', search };
    case '/configuration': return { page: 'configuration', search };
    case '/management': return { page: 'management', search };
    case '/access-keys': return { page: 'api-keys', search };
    default: return { page: 'workbench', search };
  }
}

function pathForPage(page: Page, releaseID?: string) {
  if (page === 'release-detail' && releaseID) return `/releases/${encodeURIComponent(releaseID)}`;
  return ({ workbench: '/', create: '/releases/new', releases: '/releases', deploys: '/deploys', configuration: '/configuration', management: '/management', 'api-keys': '/access-keys', 'release-detail': '/releases' } as Record<Page, string>)[page];
}

// 构造带 query 的 URL；query 中空值会被忽略，避免产生噪声参数。
function buildPath(page: Page, releaseID?: string, query?: Record<string, string>) {
  const base = pathForPage(page, releaseID);
  if (!query) {
    return base;
  }
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value) {
      params.set(key, value);
    }
  }
  const qs = params.toString();
  return qs ? `${base}?${qs}` : base;
}

// 发布中心筛选 <-> URL query 互转。空/默认值不写回 URL，保持链接干净。
const RELEASE_FILTER_DEFAULTS: ReleaseListFilters = { view: 'all', projectID: '', serviceID: '', environmentID: '', status: 'all', source: 'all', timeRange: 'all', query: '', page: 1 };

function releaseListFiltersFromSearch(search: URLSearchParams): ReleaseListFilters {
  return {
    view: releaseViewFromValue(search.get('view')) ?? RELEASE_FILTER_DEFAULTS.view,
    projectID: search.get('project_id') ?? '',
    serviceID: search.get('service_id') ?? '',
    environmentID: search.get('environment_id') ?? '',
    status: search.get('status') ?? 'all',
    source: search.get('source') ?? 'all',
    timeRange: search.get('time_range') ?? 'all',
    query: search.get('q') ?? '',
    page: parseIntDefault(search.get('page'), 1),
  };
}

function releaseListFiltersToQuery(filters: ReleaseListFilters): Record<string, string> {
  const query: Record<string, string> = {};
  if (filters.view !== RELEASE_FILTER_DEFAULTS.view) query['view'] = filters.view;
  if (filters.projectID) query['project_id'] = filters.projectID;
  if (filters.serviceID) query['service_id'] = filters.serviceID;
  if (filters.environmentID) query['environment_id'] = filters.environmentID;
  if (filters.status !== 'all') query['status'] = filters.status;
  if (filters.source !== 'all') query['source'] = filters.source;
  if (filters.timeRange !== 'all') query['time_range'] = filters.timeRange;
  if (filters.query) query['q'] = filters.query;
  if (filters.page > 1) query['page'] = String(filters.page);
  return query;
}

function releaseListFiltersEqual(a: ReleaseListFilters, b: ReleaseListFilters) {
  return (
    a.view === b.view &&
    a.projectID === b.projectID &&
    a.serviceID === b.serviceID &&
    a.environmentID === b.environmentID &&
    a.status === b.status &&
    a.source === b.source &&
    a.timeRange === b.timeRange &&
    a.query === b.query &&
    a.page === b.page
  );
}

// 发布记录筛选 <-> URL query 互转。

function deployListFiltersFromSearch(search: URLSearchParams): DeployListFilters {
  return {
    releaseRequestID: search.get('release_request_id') ?? '',
    serviceID: search.get('service_id') ?? '',
    environmentID: search.get('environment_id') ?? '',
    status: search.get('status') ?? 'all',
    page: parseIntDefault(search.get('page'), 1),
  };
}

function deployListFiltersToQuery(filters: DeployListFilters): Record<string, string> {
  const query: Record<string, string> = {};
  if (filters.releaseRequestID) query['release_request_id'] = filters.releaseRequestID;
  if (filters.serviceID) query['service_id'] = filters.serviceID;
  if (filters.environmentID) query['environment_id'] = filters.environmentID;
  if (filters.status !== 'all') query['status'] = filters.status;
  if (filters.page > 1) query['page'] = String(filters.page);
  return query;
}

function deployListFiltersEqual(a: DeployListFilters, b: DeployListFilters) {
  return a.releaseRequestID === b.releaseRequestID && a.serviceID === b.serviceID && a.environmentID === b.environmentID && a.status === b.status && a.page === b.page;
}

// URL query 整数解析：空/非法回退默认值，用于 page 等分页参数。
function parseIntDefault(value: string | null, fallback: number) {
  if (!value) {
    return fallback;
  }
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallback;
}

// 校验 view 参数合法性，非法值回退到默认（避免脏 URL 破坏类型）。
function releaseViewFromValue(value: string | null): ReleaseView | undefined {
  if (value === 'pending' || value === 'mine' || value === 'all') {
    return value;
  }
  return undefined;
}

const emptyState: AppState = {
  projects: [],
  services: [],
  versions: [],
  environments: [],
  servers: [],
  serverGroups: [],
  targets: [],
  users: [],
  apiKeys: [],
  credentials: [],
  events: [],
  serverLogs: [],
  states: [],
  notificationConfigs: [],
  notificationDeliveries: [],
  ops: null,
};

const releaseStatusOptions = [
  { label: '全部状态', value: 'all' },
  { label: 'pending_confirm', value: 'pending_confirm' },
  { label: 'queued', value: 'queued' },
  { label: 'running', value: 'running' },
  { label: 'success', value: 'success' },
  { label: 'failed', value: 'failed' },
  { label: 'partial', value: 'partial' },
  { label: 'rejected', value: 'rejected' },
  { label: 'cancelled', value: 'cancelled' },
];

const deployStatusOptions = [
  { label: '全部状态', value: 'all' },
  { label: 'queued', value: 'queued' },
  { label: 'running', value: 'running' },
  { label: 'success', value: 'success' },
  { label: 'failed', value: 'failed' },
  { label: 'partial', value: 'partial' },
  { label: 'cancelled', value: 'cancelled' },
];

const releaseTerminalStatuses = new Set(['success', 'failed', 'partial', 'rejected', 'cancelled']);

// 发布来源下拉：对齐 release_requests 表 CHECK 约束（web/api/ci/ai_agent），不再依赖列表数据动态提取。
const releaseSourceOptions = [
  { label: '全部来源', value: 'all' },
  { label: 'web', value: 'web' },
  { label: 'api', value: 'api' },
  { label: 'ci', value: 'ci' },
  { label: 'ai_agent', value: 'ai_agent' },
];

export function App() {
  const [api, contextHolder] = message.useMessage();
  const [state, setState] = useState<AppState>(emptyState);
  const [health, setHealth] = useState<Entity | null>(null);
  const [activeRelease, setActiveRelease] = useState<Entity | null>(null);
  const [activeDeployID, setActiveDeployID] = useState('');
  const [selection, setSelection] = useState<Selection>({ serviceID: '', environmentID: '', versionID: '', targetID: '' });
  const [releaseListFilters, setReleaseListFilters] = useState<ReleaseListFilters>(() => releaseListFiltersFromSearch(routeFromLocation().search));
  const [deployListFilters, setDeployListFilters] = useState<DeployListFilters>(() => deployListFiltersFromSearch(routeFromLocation().search));
  // 发布中心/发布记录列表分页数据，独立于全量 AppState，随 page/filter 请求服务端。
  const [releaseListData, setReleaseListData] = useState<PagedList<Entity>>({ items: [], total: 0, page: 1, page_size: 50 });
  const [deployListData, setDeployListData] = useState<PagedList<Entity>>({ items: [], total: 0, page: 1, page_size: 50 });
  // 工作台与详情页的上下文数据：定向查询替代全量列表，避免与分页冲突。
  const [workbenchSlice, setWorkbenchSlice] = useState<Entity[]>([]);
  const [activeReleaseDeploys, setActiveReleaseDeploys] = useState<Entity[]>([]);
  const [activeDeploy, setActiveDeploy] = useState<Entity | null>(null);
  const [preflight, setPreflight] = useState<PreflightResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [page, setPageState] = useState<Page>(() => routeFromLocation().page);
  const [routeReleaseID, setRouteReleaseID] = useState<string | undefined>(() => routeFromLocation().releaseID);
  // 当前 URL 的 query 快照；发布中心与发布记录页的筛选都从这里还原。
  const [routeSearch, setRouteSearch] = useState<URLSearchParams>(() => routeFromLocation().search);
  const [infrastructureView, setInfrastructureView] = useState<InfrastructureView>('overview');
  const [managementView, setManagementView] = useState<ManagementView>('overview');
  // 系统页创建抽屉：mgmtCreatingKind 标记正在创建的实体类型，独立于配置页状态机。
  const [mgmtCreatingKind, setMgmtCreatingKind] = useState<ManagementCreatingKind | null>(null);
  // 个人访问密钥页创建抽屉（ownKey 变体，独立于系统页集成密钥）。
  const [apiKeyCreating, setApiKeyCreating] = useState(false);
  // 配置页编辑抽屉：统一用一个 selectedEditor 记录正在编辑的实体（带 _kind 标记）。
  const [editingEntity, setEditingEntity] = useState<Entity | null>(null);
  function openEditor(item: Entity, kind: string) {
    setEditingEntity({ ...item, _kind: kind });
  }
  function closeEditor() {
    setEditingEntity(null);
  }
  // 配置页创建抽屉：creatingKind 标记正在创建的实体类型；creatingPrefill 为链式推进的上游预填（唯一来源）。
  const [creatingKind, setCreatingKind] = useState<CreatingKind | null>(null);
  const [creatingPrefill, setCreatingPrefill] = useState<CreatingPrefill>({});
  const [nextCreateAction, setNextCreateAction] = useState<NextCreateAction>(null);
  function openCreator(kind: CreatingKind, prefill: CreatingPrefill = {}) {
    // 创建/编辑互斥：打开创建前先关闭可能存在的编辑抽屉。
    closeEditor();
    setCreatingPrefill(prefill);
    setNextCreateAction(null);
    setCreatingKind(kind);
  }
  function closeCreator() {
    setCreatingKind(null);
    setCreatingPrefill({});
    setNextCreateAction(null);
  }
  // 创建成功回调：刷新数据，并按业务流设定「下一步」CTA（纯数据，点击才推进，不自动强跳）。
  // 上游预填尽量从 created 推导；server/server-group → 部署目标缺 service/env 时由用户在表单内手选。
  function handleCreated(kind: CreatingKind, created: Entity) {
    void refreshAll();
    const id = String(created.id ?? '');
    if (kind === 'project') {
      setNextCreateAction({ label: '为该项目创建服务', kind: 'service', prefill: { projectID: id } });
    } else if (kind === 'service') {
      setNextCreateAction({ label: '为该服务创建版本', kind: 'version', prefill: { serviceID: id } });
    } else if (kind === 'version') {
      // 版本是应用支（项目→服务→版本）的自然终点，与运行支无直接依赖，不强行引导到环境。
      setNextCreateAction(null);
    } else if (kind === 'environment') {
      setNextCreateAction({ label: '登记服务器', kind: 'server', prefill: {} });
    } else if (kind === 'server') {
      // 部署目标还需 service+environment；此处仅预填运行目标，service/env 留给用户在表单内选。
      setNextCreateAction({ label: '建立部署连接', kind: 'deployment-target', prefill: { targetType: 'server', targetRefID: id } });
    } else if (kind === 'server-group') {
      setNextCreateAction({ label: '建立部署连接', kind: 'deployment-target', prefill: { targetType: 'server_group', targetRefID: id } });
    } else if (kind === 'deployment-target') {
      setNextCreateAction({ label: '创建发布单', kind: 'deployment-target', prefill: { serviceID: String(created.service_id ?? ''), environmentID: String(created.environment_id ?? '') }, goToCreate: true, targetID: id });
    }
  }
  // 行内启用/禁用：调用 PATCH { enabled }。
  async function toggleEntityEnabled(item: Entity, kind: string, enabled: boolean) {
    try {
      await apiPatch<Entity>(`${entityEndpoint({ ...item, _kind: kind })}/${item.id}`, { enabled });
      await refreshAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败');
    }
  }
  // 行内冻结/解冻环境：调用 PATCH { release_frozen }。冻结是真正生效的发布保护。
  async function toggleEnvironmentFrozen(item: Entity, frozen: boolean) {
    try {
      await apiPatch<Entity>(`/api/v1/environments/${item.id}`, { release_frozen: frozen });
      await refreshAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败');
    }
  }
  const [currentUser, setCurrentUser] = useState<Entity | null>(null);
  const [authReady, setAuthReady] = useState(false);
  const activeReleaseID = activeRelease?.id as string | undefined;

  const setPage = useCallback((next: Page, releaseID?: string, query?: Record<string, string>) => {
    const nextReleaseID = next === 'release-detail' ? releaseID : undefined;
    setPageState(next);
    setRouteReleaseID(nextReleaseID);
    setRouteSearch(new URLSearchParams(query ?? {}));
    window.history.pushState(null, '', buildPath(next, nextReleaseID, query));
  }, []);

  const selected = useMemo(() => {
    const service = findByID(state.services, selection.serviceID) ?? state.services[0];
    const environment = findByID(state.environments, selection.environmentID) ?? state.environments[0];
    const targetOptions = filterTargets(state.targets, service?.id, environment?.id);
    const target = findByID(targetOptions, selection.targetID) ?? targetOptions[0] ?? state.targets[0];
    const targetRef =
      target?.target_type === 'server_group'
        ? findByID(state.serverGroups, target?.target_ref_id)
        : findByID(state.servers, target?.target_ref_id);
    return {
      project: findByID(state.projects, service?.project_id) ?? state.projects[0],
      service,
      version: findByID(state.versions, selection.versionID) ?? state.versions[0],
      environment,
      server: findByID(state.servers, target?.target_ref_id) ?? state.servers[0],
      targetRef,
      target,
      targetOptions,
      user: currentUser ?? state.users[0],
      release: activeRelease,
    };
  }, [activeRelease, currentUser, selection, state]);

  // 发布记录页筛选提示：仅在存在上下文（发布单/服务/环境）时生成文案，用于提示条展示。
  const deployFilterSummary = useMemo(() => {
    const parts: string[] = [];
    if (deployListFilters.releaseRequestID) {
      parts.push(`发布单 ${shortID(deployListFilters.releaseRequestID)}`);
    }
    if (deployListFilters.serviceID) {
      parts.push(`服务 ${namedRef(findByID(state.services, deployListFilters.serviceID), deployListFilters.serviceID, 'name')}`);
    }
    if (deployListFilters.environmentID) {
      parts.push(`环境 ${namedRef(findByID(state.environments, deployListFilters.environmentID), deployListFilters.environmentID, 'name')}`);
    }
    return parts.length > 0 ? `正在查看按 ${parts.join(' / ')} 筛选的记录` : '';
  }, [deployListFilters.environmentID, deployListFilters.releaseRequestID, deployListFilters.serviceID, state.environments, state.services]);

  // 工作台分桶：从定向查询拿到的 workbenchSlice 按状态/归属分到 pending/inProgress/failed。
  // workbenchSlice 由 refreshAll 中发起的定向请求填充（见 fetchWorkbenchSlice）。
  const workbenchReleases = useMemo(() => {
    const currentUserID = String(currentUser?.id ?? '');
    return {
      pending: workbenchSlice.filter(
        (item) => item.status === 'pending_confirm' && (currentUser?.role === 'admin' || (currentUserID !== '' && String(item.created_by_id) === currentUserID)),
      ),
      inProgress: workbenchSlice.filter(
        (item) => (item.status === 'queued' || item.status === 'running') && currentUserID !== '' && String(item.created_by_id) === currentUserID,
      ),
      failed: workbenchSlice.filter(
        (item) => currentUserID !== '' && String(item.created_by_id) === currentUserID && (item.status === 'failed' || item.status === 'partial'),
      ).slice(0, 5),
    };
  }, [currentUser?.id, currentUser?.role, workbenchSlice]);

  const setupSteps = useMemo(() => [
    { key: 'application', label: '定义应用', detail: '创建项目、服务和至少一个版本。', complete: state.projects.length > 0 && state.services.length > 0 && state.versions.length > 0 },
    { key: 'runtime', label: '准备运行环境', detail: '登记环境和至少一台服务器。', complete: state.environments.length > 0 && state.servers.length > 0 },
    { key: 'targeting', label: '建立部署连接', detail: '创建可启用的部署目标。', complete: state.targets.some((item) => item.enabled !== false) },
  ] as const, [state.environments.length, state.projects.length, state.servers.length, state.services.length, state.targets, state.versions.length]);
  const needsSetup = currentUser?.role === 'admin' && setupSteps.some((step) => !step.complete);

  // 工作台上下文用定向查询替代全量列表：admin 多看全量待确认，其余只看自己的进行中/失败。
  // 后端 status 支持逗号分隔多值；前端拿到后按状态分桶到 pending/inProgress/failed。
  const fetchWorkbenchSlice = useCallback(async (user: Entity | null): Promise<Entity[]> => {
    if (!user?.id) {
      return [];
    }
    const me = String(user.id);
    try {
      if (user.role === 'admin') {
        const [pending, mine] = await Promise.all([
          apiGet<PagedList<Entity>>('/api/v1/release-requests?status=pending_confirm&page_size=20'),
          apiGet<PagedList<Entity>>(`/api/v1/release-requests?created_by_id=${encodeURIComponent(me)}&status=queued,running,failed,partial&page_size=10`),
        ]);
        return [...pending.items, ...mine.items];
      }
      const result = await apiGet<PagedList<Entity>>(`/api/v1/release-requests?created_by_id=${encodeURIComponent(me)}&status=pending_confirm,queued,running,failed,partial&page_size=30`);
      return result.items;
    } catch {
      return [];
    }
  }, []);

  const refreshAll = useCallback(async (preferredReleaseID?: string | null, preferredSelection?: Partial<Selection>) => {
    setError('');
    try {
      const [
        healthBody,
        projects,
        services,
        environments,
        servers,
        serverGroups,
        targets,
        users,
        apiKeys,
        credentials,
        states,
        notificationConfigs,
        notificationDeliveries,
        ops,
      ] = await Promise.all([
        apiGet<Entity>('/healthz'),
        apiGet<Entity[]>('/api/v1/projects'),
        apiGet<Entity[]>('/api/v1/services'),
        apiGet<Entity[]>('/api/v1/environments'),
        apiGet<Entity[]>('/api/v1/servers'),
        apiGet<Entity[]>('/api/v1/server-groups'),
        apiGet<Entity[]>('/api/v1/deployment-targets'),
        apiGet<Entity[]>('/api/v1/users'),
        apiGet<Entity[]>('/api/v1/api-keys'),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/credentials') : Promise.resolve([]),
        apiGet<Entity[]>('/api/v1/server-deployment-states'),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/notification-configs') : Promise.resolve([]),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/notification-deliveries') : Promise.resolve([]),
        apiGet<Entity>('/api/v1/ops/summary'),
      ]);
      const serviceID = preferredSelection?.serviceID || selection.serviceID || (services[0]?.id as string | undefined);
      const versions = serviceID ? await apiGet<Entity[]>(`/api/v1/services/${serviceID}/versions`) : [];
      // 单条 release 与其关联 deploy 记录改用定向查询，不再依赖全量列表。
      const releaseID = preferredReleaseID === null ? undefined : ((preferredReleaseID ?? activeReleaseID) as string | undefined);
      const [refreshedActiveRelease, activeDeploys, workbenchItems] = await Promise.all([
        releaseID ? apiGet<Entity>(`/api/v1/release-requests/${releaseID}`) : Promise.resolve(null),
        releaseID ? apiGet<PagedList<Entity>>(`/api/v1/deploy-records?release_request_id=${encodeURIComponent(releaseID)}&page_size=50`).then((p) => p.items) : Promise.resolve([] as Entity[]),
        fetchWorkbenchSlice(currentUser),
      ]);
      const events = releaseID ? await apiGet<Entity[]>(`/api/v1/release-requests/${releaseID}/events`) : [];
      const currentDeployID = activeDeployID || undefined;
      const serverLogs = currentDeployID ? await apiGet<Entity[]>(`/api/v1/deploy-records/${currentDeployID}/server-logs`) : [];
      setHealth(healthBody);
      if (releaseID) {
        setActiveRelease(refreshedActiveRelease);
        setActiveReleaseDeploys(activeDeploys);
      }
      setWorkbenchSlice(workbenchItems);
      setState({
        projects,
        services,
        versions,
        environments,
        servers,
        serverGroups,
        targets,
        users,
        apiKeys,
        credentials,
        events,
        serverLogs,
        states,
        notificationConfigs,
        notificationDeliveries,
        ops,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败');
    }
  }, [activeDeployID, activeReleaseID, currentUser, fetchWorkbenchSlice, selection.serviceID]);

  const refreshWithSelection = useCallback(
    (patch: Partial<Selection>) => {
      setActiveRelease(null);
      setActiveDeployID('');
      setState((current) => ({ ...current, events: [], serverLogs: [] }));
      setSelection((current) => ({ ...current, ...patch }));
      void refreshAll(null, { ...selection, ...patch });
    },
    [refreshAll, selection],
  );

  const changeSelection = useCallback((patch: Partial<Selection>) => {
    setActiveRelease(null);
    setActiveDeployID('');
    setState((current) => ({ ...current, events: [], serverLogs: [] }));
    setSelection((current) => ({ ...current, ...patch }));
  }, []);

  const changeReleaseFilters = useCallback((patch: Partial<ReleaseListFilters>) => {
    setActiveRelease(null);
    setState((current) => ({ ...current, events: [] }));
    setReleaseListFilters((current) => {
      const next = { ...current, ...patch, page: 1 };
      const query = releaseListFiltersToQuery(next);
      setRouteSearch(new URLSearchParams(query));
      window.history.replaceState(null, '', buildPath('releases', undefined, query));
      return next;
    });
  }, []);

  const changeDeployFilters = useCallback((patch: Partial<DeployListFilters>) => {
    setActiveDeployID('');
    setState((current) => ({ ...current, serverLogs: [] }));
    setDeployListFilters((current) => {
      const next = { ...current, ...patch, page: 1 };
      const query = deployListFiltersToQuery(next);
      setRouteSearch(new URLSearchParams(query));
      window.history.replaceState(null, '', buildPath('deploys', undefined, query));
      return next;
    });
  }, []);

  // 进入发布记录页：从发布中心进入时看全部记录；从发布详情进入时按当前发布单过滤。
  // 重置筛选为默认全部，再按上下文填入 release_request_id；URL 同步由 setPage 处理。
  const goToDeploys = useCallback((releaseRequestID?: string) => {
    setDeployListFilters({ releaseRequestID: releaseRequestID ?? '', serviceID: '', environmentID: '', status: 'all', page: 1 });
    setPage('deploys', undefined, releaseRequestID ? { release_request_id: releaseRequestID } : undefined);
  }, [setPage]);

  // 切页：只改 page、写 URL、触发请求；不重置其他筛选。
  const changeReleasePage = useCallback((p: number) => {
    setReleaseListFilters((current) => {
      const next = { ...current, page: p };
      const query = releaseListFiltersToQuery(next);
      setRouteSearch(new URLSearchParams(query));
      window.history.replaceState(null, '', buildPath('releases', undefined, query));
      return next;
    });
  }, []);

  const changeDeployPage = useCallback((p: number) => {
    setDeployListFilters((current) => {
      const next = { ...current, page: p };
      const query = deployListFiltersToQuery(next);
      setRouteSearch(new URLSearchParams(query));
      window.history.replaceState(null, '', buildPath('deploys', undefined, query));
      return next;
    });
  }, []);

  // 发布中心列表请求：page/filter → URLSearchParams → apiGet。
  // view=mine 翻译为 created_by_id=currentUser.id；view=pending 覆盖 status。
  const refreshReleaseList = useCallback(async () => {
    if (page !== 'releases') {
      return;
    }
    try {
      const params = new URLSearchParams();
      params.set('page', String(releaseListFilters.page));
      params.set('page_size', '50');
      if (releaseListFilters.view === 'mine') {
        params.set('created_by_id', String(currentUser?.id ?? ''));
      }
      if (releaseListFilters.view === 'pending') {
        params.set('status', 'pending_confirm');
      } else if (releaseListFilters.status !== 'all') {
        params.set('status', releaseListFilters.status);
      }
      if (releaseListFilters.projectID) {
        params.set('project_id', releaseListFilters.projectID);
      }
      if (releaseListFilters.serviceID) {
        params.set('service_id', releaseListFilters.serviceID);
      }
      if (releaseListFilters.environmentID) {
        params.set('environment_id', releaseListFilters.environmentID);
      }
      if (releaseListFilters.source !== 'all') {
        params.set('source', releaseListFilters.source);
      }
      if (releaseListFilters.timeRange !== 'all') {
        params.set('time_range', releaseListFilters.timeRange);
      }
      if (releaseListFilters.query.trim()) {
        params.set('q', releaseListFilters.query.trim());
      }
      const result = await apiGet<PagedList<Entity>>(`/api/v1/release-requests?${params}`);
      setReleaseListData(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败');
    }
  }, [page, releaseListFilters, currentUser?.id]);

  // 发布记录列表请求。
  const refreshDeployList = useCallback(async () => {
    if (page !== 'deploys') {
      return;
    }
    try {
      const params = new URLSearchParams();
      params.set('page', String(deployListFilters.page));
      params.set('page_size', '50');
      if (deployListFilters.releaseRequestID) {
        params.set('release_request_id', deployListFilters.releaseRequestID);
      }
      if (deployListFilters.serviceID) {
        params.set('service_id', deployListFilters.serviceID);
      }
      if (deployListFilters.environmentID) {
        params.set('environment_id', deployListFilters.environmentID);
      }
      if (deployListFilters.status !== 'all') {
        params.set('status', deployListFilters.status);
      }
      const result = await apiGet<PagedList<Entity>>(`/api/v1/deploy-records?${params}`);
      setDeployListData(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败');
    }
  }, [page, deployListFilters]);

  // 列表请求 effect：filter/page 变化时自动拉取，复用 routeSearch 同步机制防循环。
  useEffect(() => { void refreshReleaseList(); }, [refreshReleaseList]);
  useEffect(() => { void refreshDeployList(); }, [refreshDeployList]);

  useEffect(() => {
    void apiGet<Entity>('/api/v1/auth/me')
      .then((user) => {
        setCurrentUser(user);
      })
      .catch(() => setCurrentUser(null))
      .finally(() => setAuthReady(true));
  }, []);

  useEffect(() => {
    const onPopState = () => {
      const route = routeFromLocation();
      setPageState(route.page);
      setRouteReleaseID(route.releaseID);
      setRouteSearch(route.search);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  // 发布中心：从 URL 还原筛选状态（刷新、浏览器返回/前进、直接访问链接时）。
  // 仅在路由 query 变化时同步；页面内改筛选会反向写回 URL，避免循环。
  useEffect(() => {
    if (page !== 'releases') {
      return;
    }
    const next = releaseListFiltersFromSearch(routeSearch);
    setReleaseListFilters((current) => (releaseListFiltersEqual(current, next) ? current : next));
  }, [page, routeSearch]);

  // 发布记录页：从 URL 还原筛选状态（同上）。
  useEffect(() => {
    if (page !== 'deploys') {
      return;
    }
    const next = deployListFiltersFromSearch(routeSearch);
    setDeployListFilters((current) => (deployListFiltersEqual(current, next) ? current : next));
  }, [page, routeSearch]);

  useEffect(() => {
    if (currentUser) {
      void refreshAll();
    }
  }, [currentUser, refreshAll]);

  useEffect(() => {
    if (currentUser && currentUser.role !== 'admin' && ['configuration', 'policy', 'management'].includes(page)) {
      setPage('workbench');
    }
  }, [currentUser, page, setPage]);

  useEffect(() => {
    if (page !== 'release-detail' || !routeReleaseID || !currentUser || activeReleaseID === routeReleaseID) {
      return;
    }
    // URL 直达详情：直接 GET 单条 release，不再依赖全量列表里命中。
    void apiGet<Entity>(`/api/v1/release-requests/${routeReleaseID}`)
      .then((release) => {
        const nextSelection = selectionFromRelease(release, selection);
        setActiveRelease(release);
        setSelection(nextSelection);
        void refreshAll(routeReleaseID, nextSelection);
        void apiPost<PreflightResult>(`/api/v1/release-requests/${routeReleaseID}/preflight`, {}).then(setPreflight).catch(() => setPreflight(null));
      })
      .catch(() => { /* 单条拉取失败时交由 refreshAll 的 error 提示，这里静默 */ });
  }, [activeReleaseID, currentUser, page, refreshAll, routeReleaseID, selection]);

  // 归一化只作用于 selection（创建/详情编辑上下文），允许 fallback 到第一个可用对象。
  // 发布中心、发布记录的列表筛选不进入此 effect，空值必须保持空值（即“全部”）。
  useEffect(() => {
    setSelection((current) => {
      const serviceID = keepOrFirst(current.serviceID, state.services);
      const environmentID = keepOrFirst(current.environmentID, state.environments);
      const versionID = keepOrFirst(current.versionID, state.versions);
      const targets = filterTargets(state.targets, serviceID, environmentID);
      const targetID = keepOrFirst(current.targetID, targets.length > 0 ? targets : state.targets);
      if (
        serviceID === current.serviceID &&
        environmentID === current.environmentID &&
        versionID === current.versionID &&
        targetID === current.targetID
      ) {
        return current;
      }
      return { serviceID, environmentID, versionID, targetID };
    });
  }, [state.environments, state.services, state.targets, state.versions]);

  useEffect(() => {
    setPreflight(null);
  }, [selection.environmentID, selection.serviceID, selection.targetID, selection.versionID]);

  useEffect(() => {
    if (page !== 'release-detail' || !activeReleaseID || !['queued', 'running'].includes(String(activeRelease?.status))) {
      return;
    }
    const timer = window.setInterval(() => void refreshAll(activeReleaseID), 5000);
    return () => window.clearInterval(timer);
  }, [activeRelease?.status, activeReleaseID, page, refreshAll]);

  async function createRelease() {
    if (!selected.service || !selected.environment || !selected.version || !selected.target || !selected.user) {
      setError('请先创建 Demo 配置');
      return;
    }
    setLoading(true);
    try {
      const body = await apiPost<ReleaseResponse>('/api/v1/release-requests', {
        service_id: selected.service.id,
        environment_id: selected.environment.id,
        service_version_id: selected.version.id,
        deployment_target_id: selected.target.id,
        created_by_type: 'user',
        created_by_id: selected.user.id,
        idempotency_key: `web-${Date.now()}`,
      });
      await selectRelease(body.release);
      api.success(`发布单已创建：${body.next_action}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '发布单创建失败');
    } finally {
      setLoading(false);
    }
  }

  async function runPreflight() {
    if (!selected.service || !selected.environment || !selected.version || !selected.target) {
      setError('请先选择服务、环境、版本和部署目标');
      return;
    }
    setLoading(true);
    try {
      const result = await apiPost<PreflightResult>('/api/v1/release-requests/preflight', {
        service_id: selected.service.id,
        environment_id: selected.environment.id,
        service_version_id: selected.version.id,
        deployment_target_id: selected.target.id,
      });
      setPreflight(result);
      api.success(result.result === 'block' ? '预检完成：存在阻断项' : '预检通过');
    } catch (err) {
      setError(err instanceof Error ? err.message : '预检失败');
    } finally {
      setLoading(false);
    }
  }

  async function confirmRelease() {
    const releaseID = selected.release?.id as string | undefined;
    const userID = selected.user?.id as string | undefined;
    if (!releaseID || !userID) {
      setError('没有可确认的发布单');
      return;
    }
    setLoading(true);
    try {
      const release = await apiPost<Entity>(`/api/v1/release-requests/${releaseID}/confirm`, { user_id: userID });
      setActiveRelease(release);
      await refreshAfterWorker(release.id as string);
      api.success('发布单已确认入队');
    } catch (err) {
      setError(err instanceof Error ? err.message : '确认失败');
    } finally {
      setLoading(false);
    }
  }

  async function rejectRelease() {
    const releaseID = selected.release?.id as string | undefined;
    const userID = selected.user?.id as string | undefined;
    if (!releaseID || !userID) {
      setError('没有可驳回的发布单');
      return;
    }
    setLoading(true);
    try {
      const release = await apiPost<Entity>(`/api/v1/release-requests/${releaseID}/reject`, {
        user_id: userID,
        reason: '本地验证驳回',
      });
      setActiveRelease(release);
      await refreshAll(release.id as string);
      api.success('发布单已驳回');
    } catch (err) {
      setError(err instanceof Error ? err.message : '驳回失败');
    } finally {
      setLoading(false);
    }
  }

  async function cancelRelease() {
    const releaseID = selected.release?.id as string | undefined;
    const userID = selected.user?.id as string | undefined;
    if (!releaseID || !userID) {
      setError('没有可取消的发布单');
      return;
    }
    setLoading(true);
    try {
      const release = await apiPost<Entity>(`/api/v1/release-requests/${releaseID}/cancel`, { user_id: userID });
      setActiveRelease(release);
      await refreshAll(release.id as string);
      api.success('发布单已取消');
    } catch (err) {
      setError(err instanceof Error ? err.message : '取消失败');
    } finally {
      setLoading(false);
    }
  }

  async function rollbackRelease() {
    const releaseID = selected.release?.id as string | undefined;
    const userID = selected.user?.id as string | undefined;
    if (!releaseID || !userID) {
      setError('没有可回滚的发布单');
      return;
    }
    setLoading(true);
    try {
      const body = await apiPost<ReleaseResponse>(`/api/v1/release-requests/${releaseID}/rollback`, {
        created_by_type: 'user',
        created_by_id: userID,
      });
      await selectRelease(body.release);
      api.success('回滚发布单已创建');
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建回滚失败');
    } finally {
      setLoading(false);
    }
  }

  async function retryRelease() {
    const releaseID = selected.release?.id as string | undefined;
    if (!releaseID) {
      setError('没有可重新发布的发布单');
      return;
    }
    setLoading(true);
    try {
      const body = await apiPost<ReleaseResponse>(`/api/v1/release-requests/${releaseID}/retry`, { idempotency_key: `web-retry-${Date.now()}` });
      await selectRelease(body.release);
      api.success('重新发布单已创建，请按预检结果继续确认');
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建重新发布单失败');
    } finally {
      setLoading(false);
    }
  }

  async function selectRelease(item: Entity) {
    const nextSelection = selectionFromRelease(item, selection);
    setActiveRelease(item);
    setActiveDeployID('');
    setPreflight(null);
    setSelection(nextSelection);
    await refreshAll(item.id as string, nextSelection);
    try {
      setPreflight(await apiPost<PreflightResult>(`/api/v1/release-requests/${item.id}/preflight`, {}));
    } catch {
      // The release remains viewable even when a fresh preflight cannot be read.
    }
    setPage('release-detail', String(item.id));
  }

  async function selectDeploy(item: Entity) {
    setActiveDeploy(item);
    setActiveDeployID(String(item.id));
    await refreshServerLogs(String(item.id));
  }

  async function refreshServerLogs(deployID: string) {
    const serverLogs = await apiGet<Entity[]>(`/api/v1/deploy-records/${deployID}/server-logs`);
    setState((current) => ({ ...current, serverLogs }));
  }

  async function refreshAfterWorker(releaseID: string) {
    for (let attempt = 0; attempt < 10; attempt++) {
      await refreshAll(releaseID);
      const release = await apiGet<Entity>(`/api/v1/release-requests/${releaseID}`);
      setActiveRelease(release);
      if (releaseTerminalStatuses.has(String(release.status))) {
        return;
      }
      await sleep(600);
    }
    await refreshAll(releaseID);
  }

  const status = (health?.status as string | undefined) ?? 'unknown';
  const activeReleaseStatus = String(selected.release?.status ?? '');
  const canConfirmRelease = activeReleaseStatus === 'pending_confirm';
  const canRejectRelease = activeReleaseStatus === 'pending_confirm';
  const canCancelRelease = activeReleaseStatus === 'pending_confirm' || activeReleaseStatus === 'queued';
  const canRollbackRelease = activeReleaseStatus === 'success' || selected.release?.summary_status === 'partial';
  const canRetryRelease = activeReleaseStatus === 'failed' || selected.release?.summary_status === 'partial';

  async function signIn(username: string, password: string) {
    setLoading(true);
    setError('');
    try {
      const user = await apiPost<Entity>('/api/v1/auth/login', { username, password });
      setCurrentUser(user);
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败');
    } finally {
      setLoading(false);
    }
  }

  async function signOut() {
    await apiPost<Entity>('/api/v1/auth/logout', {});
    setCurrentUser(null);
    setState(emptyState);
    setActiveRelease(null);
    setActiveDeployID('');
    setPage('workbench');
  }

  if (!authReady) {
    return <div className="auth-loading">正在恢复会话…</div>;
  }

  if (!currentUser) {
    return <ConfigProvider theme={{ token: { borderRadius: 6, colorPrimary: '#171717' } }}>{contextHolder}<LoginScreen loading={loading} error={error} onSubmit={signIn} /></ConfigProvider>;
  }

  return (
    <ConfigProvider theme={{ token: { borderRadius: 6, colorPrimary: '#171717', colorText: '#171717', colorBorder: '#ebebeb' } }}>
      {contextHolder}
      <div className="app-shell">
        <header className="app-header">
          <button className="brand" onClick={() => setPage('workbench')} aria-label="返回工作台">
            <span className="brand-mark" />
            <span>ai-pub</span>
          </button>
          <nav className="main-nav" aria-label="主导航">
            <NavButton active={page === 'workbench'} onClick={() => setPage('workbench')}>工作台</NavButton>
            <NavButton active={page === 'create' || page === 'releases' || page === 'release-detail' || page === 'deploys'} onClick={() => setPage('releases')}>发布</NavButton>
            {currentUser.role === 'admin' ? <NavButton active={page === 'configuration'} onClick={() => setPage('configuration')}>配置</NavButton> : null}
            {currentUser.role === 'admin' ? <NavButton active={page === 'management'} onClick={() => setPage('management')}>系统</NavButton> : null}
          </nav>
          <div className="header-actions">
            <span className={`health-dot ${status === 'ok' ? 'ok' : ''}`} title={`服务状态：${status}`} />
            <span className="current-user">{currentUser.display_name ?? currentUser.username} <small>{roleLabel(currentUser.role)}</small></span>
            <Button className="quiet-button" onClick={() => setPage('api-keys')}>访问密钥</Button>
            <Button className="quiet-button" onClick={() => void refreshAll()}>刷新</Button>
            <Button className="quiet-button" onClick={() => void signOut()}>退出</Button>
          </div>
        </header>
        <main className="app-main">
          {error ? <Alert className="notice" type="error" message="操作未完成" description={error} showIcon closable onClose={() => setError('')} /> : null}

          {page === 'workbench' ? (
            <>
              <PageHeading eyebrow="RELEASE OPERATIONS" title={needsSetup ? '准备首个发布' : '发布工作台'} description={needsSetup ? '完成以下最小配置后，即可创建并执行发布。' : '从需要你处理的发布开始。'} action={<Button type="primary" onClick={() => setPage(needsSetup ? 'configuration' : 'releases')}>{needsSetup ? '进入配置中心' : '进入发布中心'}</Button>} />
              {needsSetup ? <SetupChecklist steps={setupSteps} onOpen={(key) => { setInfrastructureView(key); setPage('configuration'); }} /> : null}
              <section className="workbench-tasks">
                <SectionTitle title="需要处理的发布" meta="MY RELEASE WORK" />
                <div className="content-grid three-up">
                <TaskList title="待我确认" data={workbenchReleases.pending} state={state} empty="暂无待确认发布" onOpen={(item) => void selectRelease(item)} />
                <TaskList title="我发起的进行中" data={workbenchReleases.inProgress} state={state} empty="暂无进行中发布" onOpen={(item) => void selectRelease(item)} />
                <TaskList title="我发起的最近失败" data={workbenchReleases.failed} state={state} empty="暂无失败发布" onOpen={(item) => void selectRelease(item)} />
                </div>
              </section>
            </>
          ) : null}

          {page === 'create' ? (
            <>
              <PageHeading eyebrow="NEW RELEASE" title="创建发布单" description="选择一次明确的服务、环境、版本与部署目标。" />
              <section className="release-layout">
                <div className="surface form-surface">
                  <div className="section-heading"><span>01</span><div><h2>发布范围</h2><p>所有必填项都会在创建前再次校验。</p></div></div>
                  <div className="selector-grid create-grid">
                    <LabeledSelect label="服务" value={selected.service?.id} options={state.services} nameField="name" onChange={(value) => changeSelection({ serviceID: value, versionID: '', targetID: '' })} />
                    <LabeledSelect label="环境" value={selected.environment?.id} options={state.environments} nameField="name" onChange={(value) => changeSelection({ environmentID: value, targetID: '' })} />
                    <label className="field-select">
                      <Typography.Text type="secondary">版本</Typography.Text>
                      <Select
                        value={selected.version?.id ? String(selected.version.id) : undefined}
                        options={state.versions.map((item) => ({ label: String(item.version ?? item.id), value: String(item.id), item }))}
                        optionFilterProp="label"
                        showSearch
                        placeholder="版本"
                        optionRender={(option) => {
                          const item = option.data.item as Entity;
                          return (
                            <span className="version-option">
                              <span className="version-option-label">{String(item.version)}</span>
                              {item.commit_sha ? <small className="version-option-sha">{String(item.commit_sha).slice(0, 8)}</small> : null}
                              <Tag color={item.source === 'ci' ? 'blue' : 'default'}>{item.source === 'ci' ? 'CI' : '手动'}</Tag>
                            </span>
                          );
                        }}
                        onChange={(value) => changeSelection({ versionID: String(value) })}
                      />
                    </label>
                    <LabeledSelect label="部署目标" value={selected.target?.id} options={selected.targetOptions} nameField="executor_type" onChange={(value) => changeSelection({ targetID: value })} />
                  </div>
                  <div className="release-context">
                    <span>项目 <strong>{selected.project?.name ?? '-'}</strong></span>
                    <span>版本来源 <strong>{selected.version?.source === 'ci' ? 'CI' : selected.version?.source === 'manual' ? '手动' : '-'}</strong></span>
                    <span>登记时间 <strong>{selected.version?.created_at ? formatDateTime(selected.version.created_at) : '-'}</strong></span>
                    <span>执行器 <strong>{selected.target?.executor_type ?? '-'}</strong></span>
                    <span>操作身份 <strong>{selected.user?.display_name ?? selected.user?.username ?? '未选择'}</strong></span>
                  </div>
                  <div className="form-actions">
                    <Button loading={loading} onClick={() => void runPreflight()}>执行预检</Button>
                    <Button type="primary" loading={loading} disabled={!preflight || preflight.result === 'block'} onClick={() => void createRelease()}>创建发布单</Button>
                  </div>
                  {!preflight ? <div className="hint-box">先执行预检。系统会在创建时再次检查，避免配置或策略在提交期间发生变化。</div> : <PreflightPanel result={preflight} />}
                </div>
                <aside className="surface guide-surface">
                  <span className="mono-label">发布说明</span><h2>最小、明确、可追溯。</h2>
                  <p>普通发布不强制填写冗长说明；生产环境会按生效策略进入管理员确认。</p>
                  <div className="guide-rule"><span>1</span>预检 block 时不能创建或确认。</div>
                  <div className="guide-rule"><span>2</span>warning 会保留在发布单和事件流中。</div>
                  <div className="guide-rule"><span>3</span>回滚与重新发布都会新建发布单。</div>
                </aside>
              </section>
            </>
          ) : null}

          {page === 'releases' ? (
            <>
              <PageHeading eyebrow="RELEASE CENTER" title="发布中心" description="查看、确认和追踪每一次发布。" action={<Button type="primary" onClick={() => setPage('create')}>创建发布单</Button>} />
              <section className="surface list-surface">
                <div className="list-toolbar-v2">
                  <div className="segmented-control">
                    <button className={releaseListFilters.view === 'pending' ? 'active' : ''} onClick={() => changeReleaseFilters({ view: 'pending' })}>待我确认</button>
                    <button className={releaseListFilters.view === 'mine' ? 'active' : ''} onClick={() => changeReleaseFilters({ view: 'mine' })}>我发起的</button>
                    <button className={releaseListFilters.view === 'all' ? 'active' : ''} onClick={() => changeReleaseFilters({ view: 'all' })}>全部发布</button>
                  </div>
                  <div className="filter-row">
                    <Select value={releaseListFilters.projectID || undefined} placeholder="全部项目" allowClear options={state.projects.map(entityOption)} onChange={(value) => changeReleaseFilters({ projectID: value ?? '', serviceID: '' })} />
                    <Select value={releaseListFilters.serviceID || undefined} placeholder="全部服务" allowClear options={state.services.filter((s) => !releaseListFilters.projectID || String(s.project_id) === releaseListFilters.projectID).map(entityOption)} onChange={(value) => changeReleaseFilters({ serviceID: value ?? '' })} />
                    <Select value={releaseListFilters.environmentID || undefined} placeholder="全部环境" allowClear options={state.environments.map(entityOption)} onChange={(value) => changeReleaseFilters({ environmentID: value ?? '' })} />
                    <Select value={releaseListFilters.status} options={releaseStatusOptions} onChange={(value) => changeReleaseFilters({ status: value })} />
                    <Select value={releaseListFilters.source} options={releaseSourceOptions} onChange={(value) => changeReleaseFilters({ source: value })} />
                    <Select value={releaseListFilters.timeRange} options={[{ label: '全部时间', value: 'all' }, { label: '近 24 小时', value: '24' }, { label: '近 7 天', value: '168' }]} onChange={(value) => changeReleaseFilters({ timeRange: value })} />
                    <Input className="release-search" value={releaseListFilters.query} allowClear placeholder="搜索发布单、申请人或来源" onChange={(event) => changeReleaseFilters({ query: event.target.value })} />
                    {!releaseListFiltersEqual(releaseListFilters, RELEASE_FILTER_DEFAULTS) ? <Button className="quiet-button" onClick={() => changeReleaseFilters({ ...RELEASE_FILTER_DEFAULTS })}>清除筛选</Button> : null}
                  </div>
                </div>
                <ReleaseRows data={releaseListData.items} state={state} onOpen={(item) => void selectRelease(item)} />
                {releaseListData.total > releaseListData.page_size ? (
                  <Pagination className="list-pagination" current={releaseListFilters.page} total={releaseListData.total} pageSize={releaseListData.page_size} onChange={(p) => changeReleasePage(p)} showTotal={(t) => `共 ${t} 条`} />
                ) : null}
              </section>
            </>
          ) : null}

          {page === 'release-detail' ? (
            <>
              <PageHeading eyebrow="RELEASE DETAIL" title={activeRelease?.id ? `发布单 ${shortID(activeRelease.id)}` : '发布单详情'} description={activeRelease ? formatReleaseContext(activeRelease, state) : '从发布中心选择一条发布单。'} action={<Button onClick={() => setPage('releases')}>返回发布中心</Button>} />
              {!activeRelease ? <EmptyPanel text="尚未选择发布单。" action="前往发布中心" onAction={() => setPage('releases')} /> : (
                <div className="detail-layout">
                  <div className="detail-main">
                    <section className="surface detail-hero">
                      <div><span className="mono-label">当前状态</span><div className="status-line"><StatusTag value={releaseStatusValue(activeRelease)} /><strong>{releaseActionLabel(activeRelease, selected.user)}</strong></div></div>
                      <div className="detail-actions">
                        <Button type="primary" loading={loading} disabled={!canConfirmRelease} onClick={() => void confirmRelease()}>确认并入队</Button>
                        <Button danger loading={loading} disabled={!canRejectRelease} onClick={() => void rejectRelease()}>驳回</Button>
                        <Button loading={loading} disabled={!canCancelRelease} onClick={() => void cancelRelease()}>取消发布</Button>
                        <Button loading={loading} disabled={!canRetryRelease} onClick={() => void retryRelease()}>重新发布</Button>
                        <Button loading={loading} disabled={!canRollbackRelease} onClick={() => void rollbackRelease()}>创建回滚单</Button>
                      </div>
                    </section>
                    <section className="surface"><SectionTitle title="发布信息" meta="REQUEST" /><KeyValueGrid values={[
                      ['服务', selected.service?.name], ['环境', selected.environment?.name], ['版本', selected.version?.version], ['部署目标', formatTarget(selected.target, selected.targetRef)], ['来源', activeRelease.source], ['申请人', formatActor(activeRelease.created_by_type, activeRelease.created_by_id, state)], ['授权人', formatActor('user', activeRelease.authorized_by_user_id, state)], ['确认人', activeRelease.confirmed_by_user_id ? `${formatActor('user', activeRelease.confirmed_by_user_id, state)}${activeRelease.confirmed_at ? ` · ${formatDateTime(activeRelease.confirmed_at)}` : ''}` : ''], ['驳回人', activeRelease.rejected_by_user_id ? `${formatActor('user', activeRelease.rejected_by_user_id, state)}${activeRelease.rejected_reason ? ` · ${activeRelease.rejected_reason}` : ''}` : ''], ['创建时间', formatDateTime(activeRelease.created_at)], ['更新时间', formatDateTime(activeRelease.updated_at)],
                    ]} /></section>
                    <section className="surface"><SectionTitle title="预检与门禁" meta="PREFLIGHT" /><PreflightPanel result={preflight} /></section>
                    <section className="surface"><SectionTitle title="关联发布记录" meta="DEPLOY RECORDS" /><DeployRows data={activeReleaseDeploys} state={state} onOpen={(item) => { void selectDeploy(item); goToDeploys(String(activeReleaseID ?? '')); }} /></section>
                    <section className="surface"><SectionTitle title="事件流" meta="EVENTS" /><EventRows data={state.events} state={state} /></section>
                  </div>
                  <aside className="detail-side surface"><span className="mono-label">恢复路径</span><h3>{releaseStatusValue(activeRelease) === 'partial' ? '部分成功按失败处理' : '发布恢复'}</h3><p>{activeRelease.status === 'running' ? '运行中的发布不可在系统内紧急停止，请结合执行器、服务器与超时机制人工处理。' : '失败或部分成功后，请创建新的发布单重新发布或回滚。'}</p><Button onClick={() => void refreshAll(String(activeRelease.id))}>刷新当前状态</Button></aside>
                </div>
              )}
            </>
          ) : null}

          {page === 'deploys' ? (
            <>
              <PageHeading eyebrow="DEPLOY RECORDS" title="发布记录" description="以服务器结果为准，定位真实执行情况。" action={<Button onClick={() => setPage('releases')}>返回发布中心</Button>} />
              <section className="surface list-surface">
                {deployFilterSummary ? (
                  <Alert
                    className="filter-notice"
                    type="info"
                    showIcon
                    message={deployFilterSummary}
                    action={<Button size="small" onClick={() => changeDeployFilters({ releaseRequestID: '', serviceID: '', environmentID: '', status: 'all' })}>查看全部记录</Button>}
                  />
                ) : null}
                <div className="list-toolbar-v2"><div className="filter-row"><Select value={deployListFilters.status} options={deployStatusOptions} onChange={(value) => changeDeployFilters({ status: value })} /></div></div>
                <DeployRows data={deployListData.items} state={state} onOpen={(item) => void selectDeploy(item)} />
                {deployListData.total > deployListData.page_size ? (
                  <Pagination className="list-pagination" current={deployListFilters.page} total={deployListData.total} pageSize={deployListData.page_size} onChange={(p) => changeDeployPage(p)} showTotal={(t) => `共 ${t} 条`} />
                ) : null}
              </section>
              {activeDeploy ? <section className="surface deploy-detail"><SectionTitle title="执行快照" meta={`DEPLOY ${shortID(activeDeploy.id)}`} /><KeyValueGrid values={[
                ['状态', activeDeploy.status], ['执行器', activeDeploy.executor_type], ['创建时间', formatDateTime(activeDeploy.created_at)], ['更新时间', formatDateTime(activeDeploy.updated_at)], ['目标服务器数', activeDeploy.total_servers], ['成功 / 失败 / 跳过', `${activeDeploy.success_servers ?? 0} / ${activeDeploy.failed_servers ?? 0} / ${activeDeploy.skipped_servers ?? 0}`],
              ]} /><JsonPreview value={activeDeploy.target_snapshot} /></section> : null}
              <section className="surface logs-surface"><SectionTitle title="服务器日志" meta={activeDeployID ? `DEPLOY ${shortID(activeDeployID)}` : 'SELECT A DEPLOY RECORD'} /><ServerLogRows data={state.serverLogs} state={state} /></section>
            </>
          ) : null}

          {page === 'configuration' ? (
            <>
              <PageHeading eyebrow="INFRASTRUCTURE" title="发布配置中心" description="按发布链路组织对象：先定义应用，再配置运行环境和部署连接。" />
              <section className="surface infrastructure-map">
                <div><span className="mono-label">配置关系</span><h2>项目 → 服务 → 版本</h2><p>服务与环境、运行目标组合成可执行的部署目标。</p></div>
                <div className="infrastructure-flow" aria-label="发布配置关系">
                  <button onClick={() => setInfrastructureView('application')}>项目与服务 <strong>{state.services.length}</strong></button><i>→</i>
                  <button onClick={() => setInfrastructureView('application')}>版本 <strong>{state.versions.length}</strong></button><i>＋</i>
                  <button onClick={() => setInfrastructureView('runtime')}>环境与服务器 <strong>{state.servers.length}</strong></button><i>→</i>
                  <button onClick={() => setInfrastructureView('targeting')}>部署目标 <strong>{state.targets.length}</strong></button>
                </div>
              </section>
              <section className="infrastructure-layout">
                <nav className="infrastructure-nav" aria-label="配置模块">
                  <InfrastructureNavButton active={infrastructureView === 'overview'} label="配置概览" note="查看准备情况" count={state.targets.length} onClick={() => setInfrastructureView('overview')} />
                  <InfrastructureNavButton active={infrastructureView === 'application'} label="应用与版本" note="项目、服务、版本" count={state.services.length} onClick={() => setInfrastructureView('application')} />
                  <InfrastructureNavButton active={infrastructureView === 'runtime'} label="运行环境" note="环境、服务器、服务器组" count={state.servers.length + state.serverGroups.length} onClick={() => setInfrastructureView('runtime')} />
                  <InfrastructureNavButton active={infrastructureView === 'targeting'} label="部署连接" note="服务、环境与运行目标" count={state.targets.length} onClick={() => setInfrastructureView('targeting')} />
                  <InfrastructureNavButton active={infrastructureView === 'state'} label="当前状态" note="已部署版本视图" count={state.states.length} onClick={() => setInfrastructureView('state')} />
                </nav>
                <div className="infrastructure-workspace">
                  {infrastructureView === 'overview' ? <>
                    <section className="surface infrastructure-summary"><SectionTitle title="配置准备情况" meta="RELEASE READINESS" /><div className="infrastructure-stat-grid">
                      <InfrastructureStat label="项目" value={state.projects.length} action="定义业务边界" onClick={() => setInfrastructureView('application')} />
                      <InfrastructureStat label="服务" value={state.services.length} action="注册发布对象" onClick={() => setInfrastructureView('application')} />
                      <InfrastructureStat label="运行目标" value={state.servers.length + state.serverGroups.length} action="配置服务器或分组" onClick={() => setInfrastructureView('runtime')} />
                      <InfrastructureStat label="可发布连接" value={state.targets.filter((item) => item.enabled !== false).length} action="绑定服务与环境" onClick={() => setInfrastructureView('targeting')} />
                    </div></section>
                    <section className="surface infrastructure-guide"><span className="mono-label">推荐顺序</span><h2>从首个对象到发布单。</h2><p>按发布实际依赖继续创建；SSH 服务器需要凭据时，会在服务器表单中直接提示创建。</p><div className="infrastructure-guide-actions"><button onClick={() => setInfrastructureView('application')}><strong>1. 定义应用</strong><small>{`${state.projects.length} 项目 / ${state.services.length} 服务 / ${state.versions.length} 版本`}</small><span>{state.projects.length > 0 && state.services.length > 0 && state.versions.length > 0 ? '查看应用与版本' : '创建项目、服务和版本'} →</span></button><button onClick={() => setInfrastructureView('runtime')}><strong>2. 准备运行环境</strong><small>{`${state.environments.length} 环境 / ${state.servers.length + state.serverGroups.length} 运行目标`}</small><span>{state.environments.length > 0 && state.servers.length + state.serverGroups.length > 0 ? '查看运行环境' : '创建环境和服务器'} →</span></button><button onClick={() => state.targets.some((item) => item.enabled !== false) ? setPage('create') : setInfrastructureView('targeting')}><strong>3. 建立部署连接</strong><small>{`${state.targets.filter((item) => item.enabled !== false).length} 个可发布部署目标`}</small><span>{state.targets.some((item) => item.enabled !== false) ? '创建发布单' : '连接服务、环境与目标'} →</span></button></div></section>
                  </> : null}
                  {infrastructureView === 'application' ? <>
                    <InfrastructureSectionHeading eyebrow="APPLICATION" title="应用与版本" description="这是发布内容的来源。先创建项目和服务，再为服务登记可部署版本。" />
                    <div className="infrastructure-create-bar">
                      <Button type="primary" onClick={() => openCreator('project')}>新建项目</Button>
                      <Button onClick={() => openCreator('service')}>新建服务</Button>
                      <Button onClick={() => openCreator('version')}>新建版本</Button>
                    </div>
                    <section className="surface infrastructure-inventory"><SectionTitle title="已注册对象" meta="INVENTORY" /><div className="infrastructure-list-stack"><EditableInventoryList title="项目" data={state.projects} nameField="name" subFields={['slug', 'description']} onOpen={(item) => openEditor(item, 'project')} onToggleEnabled={(item, enabled) => void toggleEntityEnabled(item, 'project', enabled)} /><EditableInventoryList title="服务" data={state.services} nameField="name" subFields={['slug']} onOpen={(item) => openEditor(item, 'service')} onToggleEnabled={(item, enabled) => void toggleEntityEnabled(item, 'service', enabled)} /><EntityList title="当前服务版本（不可编辑）" data={state.versions} fields={['version', 'source']} /></div></section>
                    <details className="surface service-detail">
                      <summary><SectionTitle title="服务部署视图" meta="SERVICE" /></summary>
                      <LabeledSelect label="服务" value={selected.service?.id} options={state.services} nameField="name" onChange={(value) => refreshWithSelection({ serviceID: value, versionID: '', targetID: '' })} />
                      <ServiceDetail service={selected.service} versions={state.versions} targets={state.targets} environments={state.environments} states={state.states} />
                    </details>
                  </> : null}
                  {infrastructureView === 'runtime' ? <>
                    <InfrastructureSectionHeading eyebrow="RUNTIME" title="运行环境" description="管理发布到哪里，以及哪些服务器作为一个批次共同执行。" />
                    <div className="infrastructure-create-bar">
                      <Button type="primary" onClick={() => openCreator('environment')}>新建环境</Button>
                      <Button onClick={() => openCreator('server')}>新建服务器</Button>
                      <Button onClick={() => openCreator('server-group')}>新建服务器组</Button>
                    </div>
                    <section className="surface infrastructure-inventory"><SectionTitle title="运行资源" meta="INVENTORY" /><div className="infrastructure-list-stack"><EditableInventoryList title="环境" data={state.environments} nameField="name" subFields={['slug', 'is_production']} frozenField="release_frozen" onOpen={(item) => openEditor(item, 'environment')} onToggleFrozen={(item, frozen) => void toggleEnvironmentFrozen(item, frozen)} /><EditableInventoryList title="服务器" data={state.servers} nameField="name" subFields={['host', 'role', 'username', 'last_check_status']} onOpen={(item) => openEditor(item, 'server')} onToggleEnabled={(item, enabled) => void toggleEntityEnabled(item, 'server', enabled)} /><EditableInventoryList title="服务器组" data={state.serverGroups} nameField="name" subFields={['description']} onOpen={(item) => openEditor(item, 'server-group')} onToggleEnabled={(item, enabled) => void toggleEntityEnabled(item, 'server-group', enabled)} /></div></section>
                  </> : null}
                  {infrastructureView === 'targeting' ? <>
                    <InfrastructureSectionHeading eyebrow="DEPLOYMENT TARGET" title="部署连接" description="把服务、环境与服务器或服务器组组合为发布时可选择的部署目标。" />
                    <DependencyChecklist state={state} onOpenApplication={() => setInfrastructureView('application')} onOpenRuntime={() => setInfrastructureView('runtime')} />
                    <div className="infrastructure-create-bar">
                      <Button type="primary" disabled={state.services.length === 0 || state.environments.length === 0} onClick={() => openCreator('deployment-target')}>新建部署目标</Button>
                    </div>
                    <section className="surface infrastructure-inventory"><SectionTitle title="现有部署目标" meta="INVENTORY" /><DeploymentTargetList data={state.targets} state={state} onOpen={(item) => openEditor(item, 'deployment-target')} onToggleEnabled={(item, enabled) => void toggleEntityEnabled(item, 'deployment-target', enabled)} /><div className="targeting-note"><strong>当前选择</strong><span>{selected.service?.name ?? '未选择服务'} / {selected.environment?.name ?? '未选择环境'}</span></div></section>
                  </> : null}
                  {infrastructureView === 'state' ? <>
                    <InfrastructureSectionHeading eyebrow="RUNTIME STATE" title="当前部署状态" description="查看每台服务器最后一次成功部署后的版本状态；它是运行视图，不是配置入口。" />
                    <section className="surface infrastructure-state"><DeploymentStateList data={state.states} state={state} /></section>
                  </> : null}
                </div>
                {/* 配置页编辑抽屉：根据 editingEntity._kind 渲染对应表单 */}
                <ProjectEditorDrawer open={!!editingEntity && editingEntity._kind === 'project'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} />
                <ServiceEditorDrawer open={!!editingEntity && editingEntity._kind === 'service'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} />
                <EnvironmentEditorDrawer open={!!editingEntity && editingEntity._kind === 'environment'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} />
                <ServerEditorDrawer open={!!editingEntity && editingEntity._kind === 'server'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} servers={state.servers} credentials={state.credentials} />
                <ServerGroupEditorDrawer open={!!editingEntity && editingEntity._kind === 'server-group'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} servers={state.servers} />
                <DeploymentTargetEditorDrawer open={!!editingEntity && editingEntity._kind === 'deployment-target'} selected={editingEntity ?? undefined} onClose={closeEditor} onDone={() => { void refreshAll(); closeEditor(); }} servers={state.servers} serverGroups={state.serverGroups} />
                <Drawer title={creatingKind ? createDrawerTitles[creatingKind] : ''} open={creatingKind !== null} onClose={closeCreator} width={520} footer={null} destroyOnClose>
                  {creatingKind === 'project' ? <ProjectForm onDone={(project) => handleCreated('project', project)} /> : null}
                  {creatingKind === 'service' ? <ServiceForm projects={state.projects} selectedProjectID={creatingPrefill.projectID} onDone={(service) => handleCreated('service', service)} /> : null}
                  {creatingKind === 'version' ? <VersionForm services={state.services} selectedServiceID={creatingPrefill.serviceID} onDone={(version) => handleCreated('version', version)} /> : null}
                  {creatingKind === 'environment' ? <EnvironmentForm onDone={(environment) => handleCreated('environment', environment)} /> : null}
                  {creatingKind === 'server' ? <ServerForm servers={state.servers} credentials={state.credentials} onCredentialsChanged={() => void refreshAll()} onDone={(server) => handleCreated('server', server)} /> : null}
                  {creatingKind === 'server-group' ? <ServerGroupForm servers={state.servers} onDone={(group) => handleCreated('server-group', group)} /> : null}
                  {creatingKind === 'deployment-target' ? (
                    <DeploymentTargetForm
                      services={state.services}
                      environments={state.environments}
                      servers={state.servers}
                      serverGroups={state.serverGroups}
                      selectedServiceID={creatingPrefill.serviceID ?? ''}
                      selectedEnvironmentID={creatingPrefill.environmentID ?? ''}
                      preferredTargetRef={{ targetType: creatingPrefill.targetType ?? '', targetRefID: creatingPrefill.targetRefID ?? '' }}
                      onDone={(target) => handleCreated('deployment-target', target)}
                    />
                  ) : null}
                  {nextCreateAction ? (
                    <div className="create-next-action">
                      <Button
                        type="primary"
                        onClick={() => {
                          const action = nextCreateAction;
                          if (!action) return;
                          if (action.goToCreate) {
                            changeSelection({ serviceID: action.prefill.serviceID ?? '', environmentID: action.prefill.environmentID ?? '', versionID: '', targetID: action.targetID ?? '' });
                            void refreshAll();
                            closeCreator();
                            setPage('create');
                            return;
                          }
                          if (action.kind === 'deployment-target') {
                            // server/server-group → 部署目标：service/env 不在 prefill，从现有实体补全；缺则引导先创建。
                            const sid = action.prefill.serviceID || (state.services.length > 0 ? String(state.services[0].id ?? '') : '');
                            const eid = action.prefill.environmentID || (state.environments.length > 0 ? String(state.environments[0].id ?? '') : '');
                            if (!action.prefill.targetRefID) {
                              message.warning('请先创建运行目标，再建立部署连接');
                              return;
                            }
                            if (!sid) {
                              message.warning('请先创建服务，再建立部署连接');
                              setInfrastructureView('application');
                              return;
                            }
                            if (!eid) {
                              message.warning('请先创建环境，再建立部署连接');
                              setInfrastructureView('runtime');
                              return;
                            }
                            openCreator(action.kind, { ...action.prefill, serviceID: sid, environmentID: eid });
                            return;
                          }
                          openCreator(action.kind, action.prefill);
                        }}
                      >
                        {nextCreateAction.label} →
                      </Button>
                    </div>
                  ) : null}
                </Drawer>
              </section>
            </>
          ) : null}


          {page === 'management' ? <>
            <PageHeading eyebrow="ADMINISTRATION" title="管理控制台" description="管理人员、访问凭据和通知集成。高风险配置按职责单独处理。" />
            <section className="surface management-brief"><div><span className="mono-label">管理边界</span><h2>谁可以发布，系统如何连接与通知。</h2><p>用户与访问密钥控制访问；凭据仅供服务器连接引用；通知投递失败不会阻塞发布。</p></div><div className="management-risk-notes"><span><b>01</b> 用户角色决定生产发布确认权限</span><span><b>02</b> 访问密钥明文只在创建时出现</span><span><b>03</b> 凭据与 Webhook 不会在列表中回显</span></div></section>
            <section className="management-layout"><nav className="management-nav" aria-label="管理模块"><ManagementNavButton active={managementView === 'overview'} label="管理概览" note="查看关键状态" count={state.users.length} onClick={() => setManagementView('overview')} /><ManagementNavButton active={managementView === 'users'} label="用户与权限" note="确认发布身份" count={state.users.length} onClick={() => setManagementView('users')} /><ManagementNavButton active={managementView === 'access'} label="集成访问密钥" note="访问密钥与 scopes" count={state.apiKeys.length} onClick={() => setManagementView('access')} /><ManagementNavButton active={managementView === 'notifications'} label="通知与投递" note="机器人与发送记录" count={state.notificationConfigs.length} onClick={() => setManagementView('notifications')} /><ManagementNavButton active={managementView === 'credentials'} label="连接凭据" note="SSH 认证材料" count={state.credentials.length} onClick={() => setManagementView('credentials')} /></nav>
              <div className="management-workspace">
                {managementView === 'overview' ? <><section className="surface management-summary"><SectionTitle title="管理状态" meta="CONTROL PLANE" /><div className="management-stat-grid"><ManagementStat label="可用用户" value={state.users.filter((item) => item.enabled !== false).length} note="可登录并参与发布" onClick={() => setManagementView('users')} /><ManagementStat label="启用访问密钥" value={state.apiKeys.filter((item) => item.enabled !== false).length} note="供 CI/CD 和脚本调用" onClick={() => setManagementView('access')} /><ManagementStat label="启用通知" value={state.notificationConfigs.filter((item) => item.enabled !== false).length} note="企业微信机器人" onClick={() => setManagementView('notifications')} /><ManagementStat label="投递异常" value={state.notificationDeliveries.filter((item) => item.status !== 'sent').length} note="查看最近失败原因" onClick={() => setManagementView('notifications')} /></div></section><section className="surface management-guide"><span className="mono-label">日常管理</span><h2>只在需要时打开对应的管理面板。</h2><div><button onClick={() => setManagementView('users')}>新增发布用户 <span>用户与权限 →</span></button><button onClick={() => setManagementView('access')}>创建 CI/CD 访问密钥 <span>集成访问密钥 →</span></button><button onClick={() => setManagementView('notifications')}>测试通知机器人 <span>通知与投递 →</span></button></div></section></> : null}
                {managementView === 'users' ? <><ManagementSectionHeading eyebrow="IDENTITY" title="用户与权限" description="用户承担发布创建与确认身份。生产环境固定由管理员确认。" /><div className="infrastructure-create-bar"><Button type="primary" onClick={() => setMgmtCreatingKind('user')}>新建用户</Button></div><section className="surface management-inventory"><SectionTitle title="现有用户" meta="INVENTORY" /><UserList data={state.users} onDone={() => void refreshAll()} /></section></> : null}
                {managementView === 'access' ? <><ManagementSectionHeading eyebrow="ACCESS" title="集成访问密钥" description="管理员可管理全部访问密钥；普通用户在个人访问密钥页面仅管理自己的密钥。" /><div className="infrastructure-create-bar"><Button type="primary" onClick={() => setMgmtCreatingKind('api-key')}>新建访问密钥</Button></div><section className="surface management-inventory"><SectionTitle title="现有访问密钥" meta="INVENTORY" /><APIKeyList data={state.apiKeys} users={state.users} onDone={() => void refreshAll()} /></section></> : null}
                {managementView === 'notifications' ? <><ManagementSectionHeading eyebrow="NOTIFICATION" title="通知与投递" description="配置企业微信机器人、发送测试消息，并从投递记录定位失败原因。" /><div className="infrastructure-create-bar"><Button type="primary" onClick={() => setMgmtCreatingKind('notification')}>新建通知配置</Button></div><section className="surface management-inventory"><SectionTitle title="通知配置" meta="INVENTORY" /><NotificationList data={state.notificationConfigs} onTest={() => void refreshAll()} /></section><section className="surface management-deliveries"><SectionTitle title="通知投递记录" meta="DELIVERIES" /><NotificationDeliveryList data={state.notificationDeliveries} configs={state.notificationConfigs} /></section></> : null}
                {managementView === 'credentials' ? <><ManagementSectionHeading eyebrow="CREDENTIAL" title="连接凭据" description="凭据只供服务器 SSH 连接引用；Secret 不会在创建后再次展示。" /><div className="infrastructure-create-bar"><Button type="primary" onClick={() => setMgmtCreatingKind('credential')}>新建凭据</Button></div><section className="surface management-inventory"><SectionTitle title="已保存凭据" meta="INVENTORY" /><CredentialList data={state.credentials} servers={state.servers} onDone={() => void refreshAll()} /></section></> : null}
              </div>
              <Drawer title={mgmtCreatingKind ? mgmtCreateTitles[mgmtCreatingKind] : ''} open={mgmtCreatingKind !== null} onClose={() => setMgmtCreatingKind(null)} width={520} footer={null} destroyOnClose>
                {mgmtCreatingKind === 'user' ? <UserForm onDone={() => { void refreshAll(); setMgmtCreatingKind(null); }} /> : null}
                {mgmtCreatingKind === 'api-key' ? <APIKeyForm users={state.users} onCreated={() => { void refreshAll(); }} /> : null}
                {mgmtCreatingKind === 'notification' ? <NotificationForm onDone={() => { void refreshAll(); setMgmtCreatingKind(null); }} /> : null}
                {mgmtCreatingKind === 'credential' ? <CredentialForm onDone={() => { void refreshAll(); setMgmtCreatingKind(null); }} /> : null}
              </Drawer>
            </section>
          </> : null}
          {page === 'api-keys' ? <><PageHeading eyebrow="PERSONAL ACCESS" title="个人访问密钥" description="为 CI/CD 或本地脚本创建受 scope 限制的访问凭证。" /><section className="surface access-key-brief"><div><span className="mono-label">使用边界</span><h2>密钥只在创建时显示一次。</h2><p>请立即保存到受保护的 CI/CD 变量中。禁用或删除后，使用它的调用会立刻失效。</p></div><div className="access-key-facts"><span><b>{state.apiKeys.length}</b> 已创建</span><span><b>{state.apiKeys.filter((item) => item.enabled !== false).length}</b> 已启用</span><span>密钥归属当前登录用户</span></div></section><section className="access-key-layout"><div className="access-key-workspace"><div className="infrastructure-create-bar"><Button type="primary" onClick={() => setApiKeyCreating(true)}>新建访问密钥</Button></div><section className="surface access-key-inventory"><SectionTitle title="我的访问密钥" meta="INVENTORY" /><APIKeyList data={state.apiKeys} users={state.users} onDone={() => void refreshAll()} /></section><section className="surface access-key-guide"><span className="mono-label">最小权限</span><h2>只授予调用真正需要的 scopes。</h2><p>发布创建、确认、回滚和读取日志分别对应不同 scope；生产发布依然受管理员确认约束。</p></section></div></section><Drawer title="新建访问密钥" open={apiKeyCreating} onClose={() => setApiKeyCreating(false)} width={520} footer={null} destroyOnClose><APIKeyForm users={[]} ownKey onCreated={() => { void refreshAll(); }} /></Drawer></> : null}
        </main>
      </div>
    </ConfigProvider>
  );
}

function LoginScreen({ loading, error, onSubmit }: { loading: boolean; error: string; onSubmit: (username: string, password: string) => void }) {
  const [form] = Form.useForm<{ username: string; password: string }>();
  return (
    <main className="login-page">
      <section className="login-panel">
        <div className="login-brand"><span className="brand-mark" /> ai-pub</div>
        <span className="mono-label">RELEASE OPERATIONS</span>
        <h1>登录后开始发布。</h1>
        <p>发布、确认和基础设施管理均使用当前登录身份执行。</p>
        {error ? <Alert type="error" message="登录失败" description={error} showIcon /> : null}
        <Form form={form} layout="vertical" onFinish={(values) => onSubmit(values.username, values.password)}>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}><Input autoComplete="username" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}><Input.Password autoComplete="current-password" /></Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>登录</Button>
        </Form>
      </section>
    </main>
  );
}

function NavButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return <button className={`nav-button ${active ? 'active' : ''}`} onClick={onClick}>{children}</button>;
}

function PageHeading({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="page-heading">
      <div><span className="mono-label">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>
      {action ? <div className="page-heading-action">{action}</div> : null}
    </div>
  );
}

function TaskList({ title, data, state, empty, onOpen }: { title: string; data: Entity[]; state: AppState; empty: string; onOpen: (item: Entity) => void }) {
  return (
    <section className="surface task-list"><div className="surface-title"><h2>{title}</h2><span>{data.length}</span></div>
      {data.length === 0 ? <div className="inline-empty">{empty}</div> : data.slice(0, 5).map((item) => <button className="task-row" key={String(item.id)} onClick={() => onOpen(item)}><span><strong>{shortID(item.id)}</strong><small>{formatReleaseContext(item, state)}</small></span><StatusTag value={releaseStatusValue(item)} /></button>)}
    </section>
  );
}

function SetupChecklist({
  steps,
  onOpen,
}: {
  steps: readonly { key: 'application' | 'runtime' | 'targeting'; label: string; detail: string; complete: boolean }[];
  onOpen: (key: 'application' | 'runtime' | 'targeting') => void;
}) {
  return <section className="surface setup-checklist"><div><span className="mono-label">FIRST RELEASE SETUP</span><h2>先把发布所需的最小对象准备好。</h2><p>不需要填写额外流程或说明；完成以下三步后即可创建发布单。</p></div><ol>{steps.map((step, index) => <li key={step.key} className={step.complete ? 'complete' : ''}><span>{step.complete ? '✓' : `0${index + 1}`}</span><div><strong>{step.label}</strong><small>{step.detail}</small></div><Button type={step.complete ? 'default' : 'primary'} onClick={() => onOpen(step.key)}>{step.complete ? '查看' : '去完成'}</Button></li>)}</ol></section>;
}

function DependencyChecklist({ state, onOpenApplication, onOpenRuntime }: { state: AppState; onOpenApplication: () => void; onOpenRuntime: () => void }) {
  const items = [
    { label: '服务与版本', complete: state.services.length > 0 && state.versions.length > 0, onClick: onOpenApplication },
    { label: '环境', complete: state.environments.length > 0, onClick: onOpenRuntime },
    { label: '服务器或服务器组', complete: state.servers.length + state.serverGroups.length > 0, onClick: onOpenRuntime },
  ];
  if (items.every((item) => item.complete)) return null;
  return (
    <div className="dependency-checklist">
      {items.map((item) => (
        <button className={item.complete ? 'complete' : ''} key={item.label} onClick={item.onClick}>
          <span>{item.complete ? '✓' : '!'}</span>{item.label}
        </button>
      ))}
    </div>
  );
}

function ReleaseRows({ data, state, onOpen }: { data: Entity[]; state: AppState; onOpen: (item: Entity) => void }) {
  if (data.length === 0) return <div className="empty-state">没有匹配的发布单。</div>;
  return <div className="release-table">{data.map((item) => <button className="release-row" key={String(item.id)} onClick={() => onOpen(item)}><span className="release-id">{shortID(item.id)}</span><span><strong>{formatReleaseContext(item, state)}</strong><small>{`申请人：${formatActor(item.created_by_type, item.created_by_id, state)} · 来源：${item.source ?? '-'}`}</small></span><StatusTag value={releaseStatusValue(item)} /><span className="next-action">{releaseActionLabel(item)}</span><span aria-hidden="true">→</span></button>)}</div>;
}

function DeployRows({ data, state, onOpen }: { data: Entity[]; state: AppState; onOpen: (item: Entity) => void }) {
  if (data.length === 0) return <div className="inline-empty">暂无发布记录。</div>;
  return <div className="deploy-list">{data.map((item) => <button className="deploy-row" key={String(item.id)} onClick={() => onOpen(item)}><span><strong>{shortID(item.id)}</strong><small>{formatDeployContext(item, state)}</small></span><span className="server-counts">成功 {item.success_servers ?? 0} / 失败 {item.failed_servers ?? 0} / 跳过 {item.skipped_servers ?? 0}</span><StatusTag value={String(item.status)} /><span aria-hidden="true">→</span></button>)}</div>;
}

function EventRows({ data, state }: { data: Entity[]; state: AppState }) {
  if (data.length === 0) return <div className="inline-empty">暂无事件。</div>;
  return <div className="event-list">{data.map((item) => <div className="event-row" key={String(item.id)}><span className="event-dot" /><div><strong>{item.event_type}</strong><p>{item.message ?? '—'}</p><small>{formatEventContext(item, state)}</small></div></div>)}</div>;
}

function ServerLogRows({ data, state }: { data: Entity[]; state: AppState }) {
  if (data.length === 0) return <div className="inline-empty">选择一条发布记录查看服务器日志。</div>;
  return <div className="server-log-list">{data.map((item) => {
    const status = String(item.status ?? '');
    // queued/running 阶段尚无输出属正常，避免误判为故障。
    const pending = status === 'queued' || status === 'running';
    const output = item.error_message || item.log_output || (pending ? '执行中，暂无输出' : '暂无输出');
    return <article className="server-log-row" key={String(item.id)}><div><strong>{formatServerRef(item.server_id, state)}</strong><StatusTag value={status} /></div><small>{`开始：${formatDateTime(item.started_at)} · 结束：${formatDateTime(item.finished_at)} · 耗时：${item.duration_ms ?? 0}ms`}</small>{item.error_code ? <code>{item.error_code}</code> : null}<pre>{String(output)}</pre></article>;
  })}</div>;
}

function SectionTitle({ title, meta }: { title: string; meta: string }) {
  return <div className="section-title"><h2>{title}</h2><span className="mono-label">{meta}</span></div>;
}

function KeyValueGrid({ values }: { values: Array<[string, unknown]> }) {
  return <div className="key-value-grid">{values.map(([label, value]) => <div key={label}><span>{label}</span><strong>{value === undefined || value === null || value === '' ? '—' : String(value)}</strong></div>)}</div>;
}

function JsonPreview({ value }: { value: Entity[string] }) {
  if (!value) return null;
  let output = String(value);
  try {
    output = JSON.stringify(JSON.parse(output), null, 2);
  } catch {
    // Keep the original payload visible when a legacy snapshot is not JSON.
  }
  return <pre className="json-preview">{output}</pre>;
}

function EmptyPanel({ text, action, onAction }: { text: string; action: string; onAction: () => void }) {
  return <section className="surface empty-panel"><p>{text}</p><Button type="primary" onClick={onAction}>{action}</Button></section>;
}

function releaseActionLabel(item: Entity, user?: Entity) {
  if (releaseStatusValue(item) === 'partial') return '需要恢复处理';
  switch (item.status) {
    case 'pending_confirm': return user?.role === 'admin' ? '等待管理员确认' : '等待确认';
    case 'queued': return '等待执行';
    case 'running': return '执行中';
    case 'partial': return '需要恢复处理';
    case 'failed': return '查看失败原因';
    case 'success': return '已完成';
    default: return '查看详情';
  }
}

function releaseStatusValue(item: Entity) {
  return item.summary_status === 'partial' ? 'partial' : String(item.status ?? '-');
}

function DataList({ data, renderItem }: { data: Entity[]; renderItem: (item: Entity) => ReactNode }) {
  if (data.length === 0) {
    return <div className="empty-state">No data</div>;
  }
  return <div className="data-list">{data.map((item, index) => <div key={String(item.id ?? index)}>{renderItem(item)}</div>)}</div>;
}

function PreflightPanel({ result }: { result: PreflightResult | null }) {
  if (!result) {
    return null;
  }
  const type = result.result === 'block' ? 'error' : 'success';
  return (
    <div className="preflight-panel">
      <Alert
        type={type}
        showIcon
        message={`预检：${result.result}`}
        description={`确认方式：${result.confirm_mode || '-'} / 下一步：${result.next_action || '-'}`}
      />
      <div className="preflight-items">
        {result.items.map((item) => (
          <div className="preflight-item" key={`${item.code}-${item.message}`}>
            <StatusTag value={item.level} />
            <Typography.Text strong>{item.code}</Typography.Text>
            <Typography.Text type={item.level === 'block' ? 'danger' : 'secondary'}>{item.message}</Typography.Text>
          </div>
        ))}
      </div>
    </div>
  );
}

function LabeledSelect({
  label,
  value,
  options,
  nameField,
  onChange,
}: {
  label: string;
  value?: string | number | boolean | null;
  options: Entity[];
  nameField: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="field-select">
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Select
        value={value ? String(value) : undefined}
        options={options.map((item) => ({ label: selectLabel(item, nameField), value: String(item.id) }))}
        onChange={onChange}
        placeholder={label}
      />
    </label>
  );
}

function StatusTag({ value }: { value?: string }) {
  const color =
    value === 'success' || value === 'pass' || value === 'active'
      ? 'green'
      : value === 'failed' || value === 'block' || value === 'rejected'
        ? 'red'
        : value === 'partial' || value === 'warning' || value === 'frozen'
          ? 'orange'
        : value === 'queued' || value === 'running'
          ? 'blue'
            : value === 'cancelled'
              ? 'default'
            : 'default';
  return <Tag color={color}>{value ?? '-'}</Tag>;
}

function InfrastructureNavButton({ active, label, note, count, onClick }: { active: boolean; label: string; note: string; count: number; onClick: () => void }) {
  return <button className={active ? 'infrastructure-nav-button active' : 'infrastructure-nav-button'} onClick={onClick}><span><strong>{label}</strong><small>{note}</small></span><b>{count}</b></button>;
}

function InfrastructureStat({ label, value, action, onClick }: { label: string; value: number; action: string; onClick: () => void }) {
  return <button className="infrastructure-stat" onClick={onClick}><span>{label}</span><strong>{value}</strong><small>{action} →</small></button>;
}

function InfrastructureSectionHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return <div className="infrastructure-section-heading"><span className="mono-label">{eyebrow}</span><h2>{title}</h2><p>{description}</p></div>;
}

function ManagementNavButton({ active, label, note, count, onClick }: { active: boolean; label: string; note: string; count: number; onClick: () => void }) {
  return <button className={active ? 'management-nav-button active' : 'management-nav-button'} onClick={onClick}><span><strong>{label}</strong><small>{note}</small></span><b>{count}</b></button>;
}

function ManagementStat({ label, value, note, onClick }: { label: string; value: number; note: string; onClick: () => void }) {
  return <button className="management-stat" onClick={onClick}><span>{label}</span><strong>{value}</strong><small>{note} →</small></button>;
}

function ManagementSectionHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return <div className="management-section-heading"><span className="mono-label">{eyebrow}</span><h2>{title}</h2><p>{description}</p></div>;
}

function EntityList({ title, data, fields }: { title: string; data: Entity[]; fields: string[] }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>{title}</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => (
          <div className="data-row compact">
            <div className="data-main">
              <Typography.Text strong>{item.name ?? item.id}</Typography.Text>
              <Typography.Text type="secondary">{fields.map((field) => formatEntityValue(item[field])).join(' / ')}</Typography.Text>
            </div>
          </div>
        )}
      />
    </div>
  );
}


function DeploymentStateList({ data, state }: { data: Entity[]; state: AppState }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>当前版本</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => (
          <div className="data-row compact">
            <div className="data-main">
              <Typography.Text strong>{formatServerRef(item.server_id, state)}</Typography.Text>
              <Typography.Text type="secondary">{formatReleaseContext(item, state)}</Typography.Text>
              <Typography.Text type="secondary">{`部署 ${shortID(item.deploy_record_id)} / ${formatDateTime(item.updated_at)}`}</Typography.Text>
            </div>
          </div>
        )}
      />
    </div>
  );
}

function DeploymentTargetList({ data, state, onOpen, onToggleEnabled }: { data: Entity[]; state: AppState; onOpen?: (item: Entity) => void; onToggleEnabled?: (item: Entity, enabled: boolean) => void }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>部署目标</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          return (
            <div className="data-row compact">
              <div className="data-main" onClick={onOpen ? () => onOpen(item) : undefined} style={onOpen ? { cursor: 'pointer' } : undefined}>
                <Typography.Text strong>{formatTarget(item, targetRefFor(item, state))}</Typography.Text>
                <Typography.Text type="secondary">
                  {[
                    namedRef(findByID(state.services, item.service_id), item.service_id, 'name'),
                    namedRef(findByID(state.environments, item.environment_id), item.environment_id, 'name'),
                  ].join(' / ')}
                </Typography.Text>
                <Typography.Text type="secondary">{`超时 ${item.timeout_seconds ?? '-'} 秒`}</Typography.Text>
              </div>
              {onToggleEnabled ? <Button size="small" onClick={() => onToggleEnabled(item, !enabled)}>{enabled ? '禁用' : '启用'}</Button> : null}
            </div>
          );
        }}
      />
    </div>
  );
}

function formatEntityValue(value: Entity[string]) {
  if (Array.isArray(value)) {
    return value.length > 0 ? value.join(', ') : '-';
  }
  return value ?? '-';
}

function ServiceDetail({ service, versions, targets, environments, states }: { service?: Entity; versions: Entity[]; targets: Entity[]; environments: Entity[]; states: Entity[] }) {
  if (!service) return <div className="form-empty">选择服务查看版本、环境和部署目标。</div>;
  const serviceTargets = targets.filter((item) => String(item.service_id) === String(service.id));
  const environmentIDs = new Set(serviceTargets.map((item) => String(item.environment_id)));
  const availableEnvironments = environments.filter((item) => environmentIDs.has(String(item.id)));
  const serviceStates = states.filter((item) => String(item.service_id) === String(service.id));
  return <div className="service-detail-grid"><div><span className="mono-label">服务</span><h3>{service.name}</h3><p>{service.description || '暂无描述'}</p><KeyValueGrid values={[["可用环境", availableEnvironments.length], ["部署目标", serviceTargets.length], ["历史版本", versions.length], ["服务器当前版本", serviceStates.length]]} /></div><div><span className="mono-label">可用环境</span><div className="detail-chip-list">{availableEnvironments.length ? availableEnvironments.map((item) => <span className={item.is_production ? 'detail-chip production' : 'detail-chip'} key={String(item.id)}>{item.name}{item.is_production ? ' · 生产' : ''}</span>) : <span className="detail-muted">尚未配置部署目标</span>}</div><span className="mono-label">部署目标</span><div className="detail-list">{serviceTargets.map((item) => <div key={String(item.id)}><strong>{item.executor_type} / {item.target_type}</strong><small>{`${environments.find((environment) => String(environment.id) === String(item.environment_id))?.name ?? item.environment_id} · ${item.enabled === false ? '已停用' : '已启用'}`}</small></div>)}</div></div><div><span className="mono-label">最近版本</span><div className="detail-list">{versions.length ? versions.slice(0, 6).map((item) => { const runURL = versionRunURL(item.metadata); return <div key={String(item.id)}><strong>{item.version}</strong><Tag color={item.source === 'ci' ? 'blue' : 'default'}>{item.source === 'ci' ? 'CI' : '手动'}</Tag><small>{`${item.commit_sha ? String(item.commit_sha).slice(0, 8) : '无 commit'} · ${maskArtifactURL(item.artifact_url)}`}</small>{runURL ? <a className="detail-link" href={runURL} target="_blank" rel="noreferrer">外部运行</a> : null}</div>; }) : <span className="detail-muted">暂无版本</span>}</div></div></div>;
}

function maskArtifactURL(value: Entity[string]) {
  const raw = String(value ?? '');
  if (!raw) return '未提供制品地址';
  try {
    const url = new URL(raw);
    return `${url.origin}${url.pathname}${url.search ? '?…' : ''}`;
  } catch {
    return raw.length > 48 ? `${raw.slice(0, 45)}…` : raw;
  }
}

// 安全解析版本 metadata，取出外部运行链接；解析失败返回空串。
function versionRunURL(metadata: Entity[string]): string {
  const raw = String(metadata ?? '');
  if (!raw) return '';
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const runURL = parsed?.run_url;
    return typeof runURL === 'string' ? runURL : '';
  } catch {
    return '';
  }
}

// 通用可编辑列表：行可点击打开编辑抽屉，行内含启用/禁用开关；环境额外支持行内冻结/解冻。
function EditableInventoryList({ title, data, nameField, subFields, onOpen, onToggleEnabled, frozenField, onToggleFrozen }: { title: string; data: Entity[]; nameField: string; subFields: string[]; onOpen: (item: Entity) => void; onToggleEnabled?: (item: Entity, enabled: boolean) => void; frozenField?: string; onToggleFrozen?: (item: Entity, frozen: boolean) => void }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>{title}</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          const frozen = frozenField ? item[frozenField] === true : false;
          return (
            <div className="data-row">
              <div className="data-main" role="button" onClick={() => onOpen(item)} style={{ cursor: 'pointer' }}>
                <Space>
                  <Typography.Text strong>{selectLabel(item, nameField)}</Typography.Text>
                  {item.enabled !== undefined ? <StatusTag value={enabled ? 'enabled' : 'disabled'} /> : null}
                  {frozenField ? <StatusTag value="frozen" /> : null}
                </Space>
                <Typography.Text type="secondary">{subFields.map((field) => formatEntityValue(item[field])).join(' / ')}</Typography.Text>
              </div>
              <Space size="small">
                {onToggleFrozen && frozenField ? (
                  <Button size="small" onClick={() => onToggleFrozen(item, !frozen)}>
                    {frozen ? '解冻' : '冻结'}
                  </Button>
                ) : null}
                {onToggleEnabled && item.enabled !== undefined ? (
                  <Button size="small" onClick={() => onToggleEnabled(item, !enabled)}>
                    {enabled ? '禁用' : '启用'}
                  </Button>
                ) : null}
              </Space>
            </div>
          );
        }}
      />
    </div>
  );
}

// 抽屉表单基座：open 时回填字段，保存调 PATCH。
function EntityDrawer({ open, title, selected, onClose, onDone, fields }: { open: boolean; title: string; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void; fields: (form: ReturnType<typeof Form.useForm>[0]) => ReactNode }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (open && selected) {
      form.setFieldsValue(selected);
    }
  }, [open, selected, form]);
  async function submit() {
    if (!selected) return;
    let values: Entity;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setLoading(true);
    try {
      await apiPatch<Entity>(`${entityEndpoint(selected)}/${selected.id}`, values);
      onDone(String(selected.id ?? ''));
    } finally {
      setLoading(false);
    }
  }
  return (
    <Drawer title={title} open={open} onClose={onClose} width={480} footer={null} destroyOnClose>
      <Form form={form} layout="vertical">
        {fields(form)}
        <Space style={{ marginTop: 8 }}>
          <Button type="primary" loading={loading} onClick={() => void submit()}>保存</Button>
          <Button onClick={onClose}>取消</Button>
        </Space>
      </Form>
    </Drawer>
  );
}

// 由实体推断 PATCH 端点；selected 需带 _kind 标记（由列表传入）。
function entityEndpoint(selected: Entity): string {
  const kind = String(selected._kind ?? '');
  switch (kind) {
    case 'project': return '/api/v1/projects';
    case 'service': return '/api/v1/services';
    case 'environment': return '/api/v1/environments';
    case 'server': return '/api/v1/servers';
    case 'server-group': return '/api/v1/server-groups';
    case 'deployment-target': return '/api/v1/deployment-targets';
    default: return '/api/v1';
  }
}

function ProjectEditorDrawer({ open, selected, onClose, onDone }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void }) {
  return (
    <EntityDrawer open={open} title="编辑项目" selected={selected} onClose={onClose} onDone={onDone} fields={() => <>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}><Input /></Form.Item>
      <Typography.Text type="secondary">修改 Slug 可能影响已配置的 CI 引用，请谨慎。</Typography.Text>
      <Form.Item name="description" label="描述"><Input /></Form.Item>
      <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
    </>} />
  );
}

function ServiceEditorDrawer({ open, selected, onClose, onDone }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void }) {
  return (
    <EntityDrawer open={open} title="编辑服务" selected={selected} onClose={onClose} onDone={onDone} fields={() => <>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}><Input /></Form.Item>
      <Typography.Text type="secondary">修改 Slug 可能影响已配置的 CI 引用，请谨慎。服务归属项目创建时确定，不支持迁移。</Typography.Text>
      <Form.Item name="description" label="描述"><Input /></Form.Item>
      <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
    </>} />
  );
}

function EnvironmentEditorDrawer({ open, selected, onClose, onDone }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void }) {
  return (
    <EntityDrawer open={open} title="编辑环境" selected={selected} onClose={onClose} onDone={onDone} fields={() => <>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}><Input /></Form.Item>
      <Form.Item name="is_production" valuePropName="checked"><Checkbox>生产环境（固定要求管理员确认）</Checkbox></Form.Item>
      <Form.Item name="release_frozen" valuePropName="checked"><Checkbox>冻结此环境的发布</Checkbox></Form.Item>
      <Typography.Text type="secondary">冻结会阻断新建和确认；已入队任务暂停领取，运行中的任务继续。</Typography.Text>
    </>} />
  );
}

function ServerEditorDrawer({ open, selected, onClose, onDone, servers, credentials }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void; servers: Entity[]; credentials: Entity[] }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState('');
  const role = Form.useWatch('role', form);
  const selectedID = String(selected?.id ?? '');
  const gateways = servers.filter((server) => server.role === 'gateway' && server.enabled !== false && String(server.id) !== selectedID);
  useEffect(() => {
    if (open && selected) {
      form.setFieldsValue(selected);
      setTestResult('');
    }
  }, [open, selected, form]);
  async function submit() {
    if (!selected) return;
    let values: Entity;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setLoading(true);
    try {
      await apiPatch<Entity>(`/api/v1/servers/${selected.id}`, values);
      onDone(selectedID);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败');
    } finally {
      setLoading(false);
    }
  }
  async function test() {
    if (!selectedID) return;
    setTesting(true);
    setTestResult('');
    try {
      const body = await apiPost<{ result: Entity }>(`/api/v1/servers/${selectedID}/test`, {});
      const result = body.result;
      setTestResult(result.status === 'success' ? '连接成功' : `连接失败：${result.error_message ?? result.error_code ?? result.status}`);
      onDone(selectedID);
    } catch (err) {
      setTestResult(err instanceof Error ? err.message : '连接测试失败');
    } finally {
      setTesting(false);
    }
  }
  return (
    <Drawer title="编辑服务器" open={open} onClose={onClose} width={480} footer={null} destroyOnClose>
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="role" label="服务器角色" rules={[{ required: true }]}><Select options={[{ label: '应用服务器', value: 'application' }, { label: '网关服务器', value: 'gateway' }]} onChange={() => form.setFieldsValue({ gateway_id: undefined })} /></Form.Item>
        <Form.Item name="host" label="Host" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="port" label="Port"><Input type="number" min={1} /></Form.Item>
        <Form.Item name="username" label="Username" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="auth_type" label="认证方式"><Select options={[{ label: 'private_key', value: 'private_key' }, { label: 'password', value: 'password' }, { label: 'none', value: 'none' }]} /></Form.Item>
        <Form.Item name="credential_ref" label="凭据"><Select allowClear options={credentials.map(userOption)} /></Form.Item>
        {role === 'application' ? <Form.Item name="gateway_id" label="跳转网关"><Select allowClear placeholder="不选则直连" options={gateways.map(entityOption)} /></Form.Item> : <Typography.Text type="secondary">网关直接由发布服务连接，不能再配置上游网关。</Typography.Text>}
        <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
        <Space wrap>
          <Button type="primary" loading={loading} onClick={() => void submit()}>保存</Button>
          <Button loading={testing} onClick={() => void test()}>测试 SSH</Button>
          <Button onClick={onClose}>取消</Button>
        </Space>
        {testResult ? <div className="test-result">{testResult}</div> : <Typography.Text type="secondary">最近测试：{selected?.last_check_status ?? '未测试'} / {formatDateTime(selected?.last_check_at)}</Typography.Text>}
      </Form>
    </Drawer>
  );
}

function ServerGroupEditorDrawer({ open, selected, onClose, onDone, servers }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void; servers: Entity[] }) {
  return (
    <EntityDrawer open={open} title="编辑服务器组" selected={selected} onClose={onClose} onDone={onDone} fields={() => <>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
      <Form.Item name="description" label="描述"><Input /></Form.Item>
      <Form.Item name="server_ids" label="成员服务器" rules={[{ required: true }]}><Select mode="multiple" options={servers.map(entityOption)} /></Form.Item>
      <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
    </>} />
  );
}

function DeploymentTargetEditorDrawer({ open, selected, onClose, onDone, servers, serverGroups }: { open: boolean; selected: Entity | undefined; onClose: () => void; onDone: (id: string) => void; servers: Entity[]; serverGroups: Entity[] }) {
  const [form] = Form.useForm();
  const targetType = Form.useWatch('target_type', form) as string | undefined;
  useEffect(() => {
    if (open && selected) {
      form.setFieldsValue(selected);
    }
  }, [open, selected, form]);
  const targetOptions = targetType === 'server_group' ? serverGroups : servers;
  async function submit() {
    if (!selected) return;
    let values: Entity;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    try {
      await apiPatch<Entity>(`/api/v1/deployment-targets/${selected.id}`, values);
      onDone(String(selected.id ?? ''));
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败');
    }
  }
  return (
    <Drawer title="编辑部署目标" open={open} onClose={onClose} width={520} footer={null} destroyOnClose>
      <Form form={form} layout="vertical">
        <Form.Item name="target_type" label="目标类型"><Select options={[{ label: '服务器', value: 'server' }, { label: '服务器组', value: 'server_group' }]} /></Form.Item>
        <Form.Item name="target_ref_id" label="运行目标"><Select options={targetOptions.map(entityOption)} /></Form.Item>
        <Form.Item name="executor_type" label="执行器"><Select options={[{ label: 'mock', value: 'mock' }, { label: 'ssh', value: 'ssh' }]} /></Form.Item>
        <Form.Item name="artifact_type" label="制品类型" tooltip="oci_image 要求版本 artifact_url 为完整 digest 引用；version_only 由脚本按版本号自行解析"><Select options={[{ label: 'version_only（按版本号解析）', value: 'version_only' }, { label: 'oci_image（OCI 镜像 digest）', value: 'oci_image' }]} /></Form.Item>
        <Form.Item name="script_path" label="Script Path"><Input /></Form.Item>
        <Form.Item name="working_dir" label="Working Dir"><Input /></Form.Item>
        <Form.Item name="env_vars" label="环境变量 JSON"><Input.TextArea rows={3} /></Form.Item>
        <Form.Item name="timeout_seconds" label="超时秒数"><Input type="number" min={1} /></Form.Item>
        <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
        <Space>
          <Button type="primary" onClick={() => void submit()}>保存</Button>
          <Button onClick={onClose}>取消</Button>
        </Space>
      </Form>
    </Drawer>
  );
}

// 创建抽屉薄壳：按 creatingKind 渲染对应创建表单，底部渲染「下一步」CTA。
// onCreated 由 App 提供：刷新数据 + 按业务流设定 nextCreateAction（链式推进）。
const createDrawerTitles: Record<CreatingKind, string> = {
  project: '新建项目',
  service: '新建服务',
  version: '新建版本',
  environment: '新建环境',
  server: '新建服务器',
  'server-group': '新建服务器组',
  'deployment-target': '新建部署目标',
};

const mgmtCreateTitles: Record<ManagementCreatingKind, string> = {
  user: '新建用户',
  'api-key': '新建访问密钥',
  notification: '新建通知配置',
  credential: '新建凭据',
};

function ProjectForm({ onDone }: { onDone: (project: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const project = await apiPost<Entity>('/api/v1/projects', values);
      form.resetFields();
      onDone(project);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="description" label="描述">
        <Input />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建项目
      </Button>
    </Form>
  );
}

function ServiceForm({ projects, selectedProjectID, onDone }: { projects: Entity[]; selectedProjectID?: string; onDone: (service: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (!form.getFieldValue('project_id')) {
      form.setFieldValue('project_id', selectedProjectID || projects[0]?.id);
    }
  }, [form, projects, selectedProjectID]);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const service = await apiPost<Entity>('/api/v1/services', values);
      form.resetFields();
      form.setFieldValue('project_id', selectedProjectID || projects[0]?.id);
      onDone(service);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="project_id" label="项目" rules={[{ required: true }]}>
        <Select options={projects.map(entityOption)} />
      </Form.Item>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="description" label="描述">
        <Input />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建服务
      </Button>
    </Form>
  );
}

function VersionForm({ services, selectedServiceID, onDone }: { services: Entity[]; selectedServiceID?: string; onDone: (version: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (!form.getFieldValue('service_id')) {
      form.setFieldValue('service_id', selectedServiceID || services[0]?.id);
    }
  }, [form, selectedServiceID, services]);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const serviceID = String(values.service_id ?? '');
      const version = await apiPost<Entity>(`/api/v1/services/${serviceID}/versions`, {
        version: values.version,
        commit_sha: values.commit_sha,
        artifact_url: values.artifact_url,
        source: 'manual',
        metadata: values.metadata || '{}',
      });
      form.resetFields();
      form.setFieldValue('service_id', selectedServiceID || services[0]?.id);
      onDone(version);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ metadata: '{}' }} onFinish={(values) => void submit(values)}>
      <Form.Item name="service_id" label="服务" rules={[{ required: true }]}>
        <Select options={services.map(entityOption)} />
      </Form.Item>
      <Form.Item name="version" label="版本号" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="commit_sha" label="Commit SHA">
        <Input />
      </Form.Item>
      <Form.Item name="artifact_url" label="Artifact URL">
        <Input />
      </Form.Item>
      <Form.Item name="metadata" label="Metadata JSON">
        <Input.TextArea rows={3} />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建版本
      </Button>
    </Form>
  );
}

function EnvironmentForm({ onDone }: { onDone: (environment: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const environment = await apiPost<Entity>('/api/v1/environments', values);
      form.resetFields();
      onDone(environment);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ is_production: false }} onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="slug" label="Slug" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="is_production" valuePropName="checked">
        <Checkbox>生产环境</Checkbox>
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建环境
      </Button>
    </Form>
  );
}

function ServerForm({ servers, credentials, onCredentialsChanged, onDone }: { servers: Entity[]; credentials: Entity[]; onCredentialsChanged?: () => void; onDone: (server: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState('');
  // 凭据下拉受控：点「+新建凭据」时关闭下拉并打开二级小 Drawer。
  const [credSelectOpen, setCredSelectOpen] = useState(false);
  const [credDrawerOpen, setCredDrawerOpen] = useState(false);
  const [credForm] = Form.useForm();
  const [credLoading, setCredLoading] = useState(false);
  const authType = Form.useWatch('auth_type', form);
  const role = Form.useWatch('role', form);
  const gateways = servers.filter((server) => server.role === 'gateway' && server.enabled !== false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const server = await apiPost<Entity>('/api/v1/servers', {
        ...values,
        port: Number(values.port || 22),
      });
      form.resetFields();
      onDone(server);
    } finally {
      setLoading(false);
    }
  }
  // 一次性连接校验，不落库；结果仅作提示，不限制创建。
  async function testConnection() {
    const values = form.getFieldsValue();
    if (!values.host || !values.username) {
      setTestResult('请先填写 Host 与 Username');
      return;
    }
    setTesting(true);
    setTestResult('');
    try {
      const body = await apiPost<{ result: Entity }>('/api/v1/servers/test', {
        ...values,
        port: Number(values.port || 22),
      });
      const result = body.result;
      setTestResult(result.status === 'success' ? '连接成功' : `连接失败：${result.error_message ?? result.error_code ?? result.status}`);
    } catch (err) {
      setTestResult(err instanceof Error ? err.message : '连接测试失败');
    } finally {
      setTesting(false);
    }
  }
  // 内联新建凭据：提交后刷新凭据列表、回填 credential_ref、关闭二级 Drawer；失败保输入。
  async function submitCredential() {
    let values: Entity;
    try {
      values = await credForm.validateFields();
    } catch {
      return;
    }
    setCredLoading(true);
    try {
      const credential = await apiPost<Entity>('/api/v1/credentials', values);
      onCredentialsChanged?.();
      form.setFieldValue('credential_ref', credential.id);
      credForm.resetFields();
      setCredDrawerOpen(false);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存凭据失败');
    } finally {
      setCredLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ port: 22, auth_type: 'none', role: 'application' }} onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="role" label="服务器角色" rules={[{ required: true }]}>
        <Select options={[{ label: '应用服务器', value: 'application' }, { label: '网关服务器', value: 'gateway' }]} onChange={() => form.setFieldsValue({ gateway_id: undefined })} />
      </Form.Item>
      <Form.Item name="host" label="Host" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="port" label="Port">
        <Input type="number" min={1} />
      </Form.Item>
      <Form.Item name="username" label="Username" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="auth_type" label="认证方式" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'none', value: 'none' },
            { label: 'private_key', value: 'private_key' },
            { label: 'password', value: 'password' },
          ]}
        />
      </Form.Item>
      <Form.Item
        name="credential_ref"
        label="凭据"
        rules={authType === 'none' ? [] : [{ required: true, message: '请选择凭据' }]}
      >
        <Select
          allowClear
          open={credSelectOpen}
          onOpenChange={setCredSelectOpen}
          options={credentials.map(entityOption)}
          disabled={authType === 'none'}
          popupRender={(menu) => (
            <>
              {menu}
              <div className="select-foot-create">
                <Button size="small" type="link" onClick={() => { setCredSelectOpen(false); setCredDrawerOpen(true); }}>+ 新建凭据</Button>
              </div>
            </>
          )}
        />
      </Form.Item>
      {role === 'application' ? <Form.Item name="gateway_id" label="跳转网关"><Select allowClear placeholder="不选则直连" options={gateways.map(entityOption)} /></Form.Item> : <Typography.Text type="secondary">网关直接由发布服务连接；应用服务器经它建立隧道后仍使用自己的凭据登录。</Typography.Text>}
      <Space wrap>
        <Button type="primary" htmlType="submit" loading={loading}>
          创建服务器
        </Button>
        <Button loading={testing} onClick={() => void testConnection()}>
          测试连接
        </Button>
      </Space>
      {testResult ? <div className="test-result">{testResult}</div> : null}
      <Drawer title="新建凭据" open={credDrawerOpen} onClose={() => setCredDrawerOpen(false)} width={360} footer={null} destroyOnClose>
        <Form form={credForm} layout="vertical" initialValues={{ type: 'private_key' }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select options={[{ label: 'private_key', value: 'private_key' }, { label: 'password', value: 'password' }]} />
          </Form.Item>
          <Form.Item name="secret" label="Secret" rules={[{ required: true }]}><Input.TextArea rows={6} /></Form.Item>
          <Space>
            <Button type="primary" loading={credLoading} onClick={() => void submitCredential()}>保存凭据</Button>
            <Button onClick={() => setCredDrawerOpen(false)}>取消</Button>
          </Space>
        </Form>
      </Drawer>
    </Form>
  );
}

function ServerGroupForm({ servers, onDone }: { servers: Entity[]; onDone: (serverGroup: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [serverSelectOpen, setServerSelectOpen] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    setServerSelectOpen(false);
    try {
      const serverGroup = await apiPost<Entity>('/api/v1/server-groups', values);
      form.resetFields();
      onDone(serverGroup);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="description" label="描述">
        <Input />
      </Form.Item>
      <Form.Item name="server_ids" label="服务器" rules={[{ required: true }]}>
        <Select
          mode="multiple"
          open={serverSelectOpen}
          onOpenChange={setServerSelectOpen}
          onSelect={() => setServerSelectOpen(false)}
          options={servers.map(entityOption)}
        />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建服务器组
      </Button>
    </Form>
  );
}

function DeploymentTargetForm({
  services,
  environments,
  servers,
  serverGroups,
  selectedServiceID,
  selectedEnvironmentID,
  preferredTargetRef,
  onDone,
}: {
  services: Entity[];
  environments: Entity[];
  servers: Entity[];
  serverGroups: Entity[];
  selectedServiceID: string;
  selectedEnvironmentID: string;
  preferredTargetRef: ManualTargetRef;
  onDone: (target: Entity) => void;
}) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const targetType = Form.useWatch('target_type', form) ?? 'server';
  const targetOptions = targetType === 'server_group' ? serverGroups : servers;
  useEffect(() => {
    const current = form.getFieldsValue(['service_id', 'environment_id', 'target_ref_id']);
    const nextValues: Partial<Entity> = {};
    const preferredOptions = preferredTargetRef.targetType === 'server_group' ? serverGroups : servers;
    const hasPreferredTarget =
      preferredTargetRef.targetRefID &&
      preferredOptions.some((item) => String(item.id) === preferredTargetRef.targetRefID);
    if (hasPreferredTarget) {
      nextValues.target_type = preferredTargetRef.targetType;
      nextValues.target_ref_id = preferredTargetRef.targetRefID;
    }
    if (!current.service_id && selectedServiceID) {
      nextValues.service_id = selectedServiceID;
    }
    if (!current.environment_id && selectedEnvironmentID) {
      nextValues.environment_id = selectedEnvironmentID;
    }
    if (!hasPreferredTarget && !current.target_ref_id && targetOptions[0]?.id) {
      nextValues.target_ref_id = String(targetOptions[0].id);
    }
    if (Object.keys(nextValues).length > 0) {
      form.setFieldsValue(nextValues);
    }
  }, [form, preferredTargetRef, selectedEnvironmentID, selectedServiceID, serverGroups, servers, targetOptions]);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const target = await apiPost<Entity>('/api/v1/deployment-targets', {
        ...values,
        env_vars: values.env_vars || '{}',
        timeout_seconds: Number(values.timeout_seconds || 60),
      });
      form.resetFields();
      onDone(target);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={{ executor_type: 'mock', target_type: 'server', artifact_type: 'version_only', env_vars: '{}', timeout_seconds: 60 }}
      onFinish={(values) => void submit(values)}
    >
      <Form.Item name="service_id" label="服务" rules={[{ required: true }]}>
        <Select options={services.map(entityOption)} />
      </Form.Item>
      <Form.Item name="environment_id" label="环境" rules={[{ required: true }]}>
        <Select options={environments.map(entityOption)} />
      </Form.Item>
      <Form.Item name="target_type" label="目标类型" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'server', value: 'server' },
            { label: 'server_group', value: 'server_group' },
          ]}
          onChange={() => form.setFieldValue('target_ref_id', undefined)}
        />
      </Form.Item>
      <Form.Item name="target_ref_id" label={targetType === 'server_group' ? '服务器组' : '服务器'} rules={[{ required: true }]}>
        <Select options={targetOptions.map(entityOption)} />
      </Form.Item>
      <Form.Item name="executor_type" label="执行器" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'mock', value: 'mock' },
            { label: 'ssh', value: 'ssh' },
          ]}
        />
      </Form.Item>
      <Form.Item name="artifact_type" label="制品类型" tooltip="oci_image 要求版本 artifact_url 为完整 digest 引用；version_only 由脚本按版本号自行解析">
        <Select
          options={[
            { label: 'version_only（按版本号解析）', value: 'version_only' },
            { label: 'oci_image（OCI 镜像 digest）', value: 'oci_image' },
          ]}
        />
      </Form.Item>
      <Form.Item name="script_path" label="Script Path">
        <Input />
      </Form.Item>
      <Form.Item name="working_dir" label="Working Dir">
        <Input />
      </Form.Item>
      <Form.Item name="env_vars" label="Env Vars JSON">
        <Input.TextArea rows={3} />
      </Form.Item>
      <Form.Item name="timeout_seconds" label="Timeout Seconds">
        <Input type="number" min={1} />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建发布目标
      </Button>
    </Form>
  );
}

function UserForm({ onDone }: { onDone: (user: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const user = await apiPost<Entity>('/api/v1/users', values);
      form.resetFields();
      onDone(user);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ role: 'employee' }} onFinish={(values) => void submit(values)}>
      <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="display_name" label="显示名">
        <Input />
      </Form.Item>
      <Form.Item name="role" label="角色" rules={[{ required: true }]}>
        <Select
          options={[
            { label: '员工（employee）', value: 'employee' },
            { label: '管理员（admin）', value: 'admin' },
          ]}
        />
      </Form.Item>
      <Form.Item name="password" label="初始密码" rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}>
        <Input.Password />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建用户
      </Button>
    </Form>
  );
}

function UserList({ data, onDone }: { data: Entity[]; onDone: () => void }) {
  const [busyID, setBusyID] = useState('');
  async function setEnabled(item: Entity, enabled: boolean) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiPatch<Entity>(`/api/v1/users/${item.id}`, { enabled });
      onDone();
    } finally {
      setBusyID('');
    }
  }
  return (
    <div className="mini-list">
      <Typography.Title level={4}>确认用户</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          return (
            <div className="data-row">
              <div className="data-main">
                <Space>
                  <Typography.Text strong>{item.display_name ?? item.username ?? item.id}</Typography.Text>
                  <StatusTag value={enabled ? 'enabled' : 'disabled'} />
                </Space>
                <Typography.Text type="secondary">{`${item.username ?? '-'} / ${roleLabel(item.role)}`}</Typography.Text>
              </div>
              <Button loading={busyID === item.id} onClick={() => void setEnabled(item, !enabled)}>
                {enabled ? '禁用' : '启用'}
              </Button>
            </div>
          );
        }}
      />
    </div>
  );
}

function entityOption(item: Entity) {
  return { label: selectLabel(item, 'name'), value: String(item.id) };
}

function NotificationForm({ onDone }: { onDone: () => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      await apiPost<Entity>('/api/v1/notification-configs', values);
      form.resetFields();
      onDone();
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ channel: 'wecom_robot' }} onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="channel" label="渠道" rules={[{ required: true }]}>
        <Select options={[{ label: 'wecom_robot', value: 'wecom_robot' }]} />
      </Form.Item>
      <Form.Item name="webhook_url" label="Webhook URL" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        保存通知
      </Button>
    </Form>
  );
}

function NotificationList({ data, onTest }: { data: Entity[]; onTest: () => void }) {
  const [testingID, setTestingID] = useState('');
  const [busyID, setBusyID] = useState('');
  const [editingID, setEditingID] = useState('');
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const editing = findByID(data, editingID);
  async function testConfig(id: string) {
    setTestingID(id);
    try {
      await apiPost<Entity>(`/api/v1/notification-configs/${id}/test`, {});
    } catch {
      // Failed webhook tests still create delivery records; refresh so the failure is visible.
    } finally {
      onTest();
      setTestingID('');
    }
  }
  async function setEnabled(item: Entity, enabled: boolean) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiPatch<Entity>(`/api/v1/notification-configs/${item.id}`, { enabled });
      onTest();
    } finally {
      setBusyID('');
    }
  }
  async function deleteConfig(item: Entity) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiDelete<Entity>(`/api/v1/notification-configs/${item.id}`);
      onTest();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败');
    } finally {
      setBusyID('');
    }
  }
  function openEdit(item: Entity) {
    setEditingID(String(item.id ?? ''));
    form.setFieldsValue({ name: item.name, webhook_url: '', enabled: item.enabled !== false });
  }
  async function saveEdit() {
    if (!editingID) return;
    let values: Entity;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setSaving(true);
    try {
      // webhook_url 留空表示不改，只更新名称与启用状态
      const patch: Entity = { name: values.name, enabled: values.enabled };
      if (values.webhook_url) {
        patch.webhook_url = values.webhook_url;
      }
      await apiPatch<Entity>(`/api/v1/notification-configs/${editingID}`, patch);
      setEditingID('');
      onTest();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="mini-list">
      <Typography.Title level={4}>通知配置</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          return (
            <div className="data-row">
              <div className="data-main">
                <Space>
                  <Typography.Text strong>{item.name ?? item.id}</Typography.Text>
                  <StatusTag value={enabled ? 'enabled' : 'disabled'} />
                </Space>
                <Typography.Text type="secondary">{String(item.channel ?? '-')}</Typography.Text>
              </div>
              <Space>
                <Button onClick={() => openEdit(item)}>编辑</Button>
                <Button loading={testingID === item.id} disabled={!enabled} onClick={() => void testConfig(item.id as string)}>
                  测试
                </Button>
                <Button loading={busyID === item.id} onClick={() => void setEnabled(item, !enabled)}>
                  {enabled ? '禁用' : '启用'}
                </Button>
                <Popconfirm title="删除通知配置" description="历史投递记录会保留，配置名显示为已删除。" onConfirm={() => void deleteConfig(item)}>
                  <Button danger loading={busyID === item.id}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            </div>
          );
        }}
      />
      <Drawer title="编辑通知配置" open={!!editing} onClose={() => setEditingID('')} width={440} footer={null} destroyOnClose>
        {editing ? (
          <Form form={form} layout="vertical">
            <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="webhook_url" label="Webhook URL（留空不改）"><Input placeholder="https://qyapi.weixin.qq.com/..." /></Form.Item>
            <Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item>
            <Space>
              <Button type="primary" loading={saving} onClick={() => void saveEdit()}>保存</Button>
              <Button onClick={() => setEditingID('')}>取消</Button>
            </Space>
          </Form>
        ) : null}
      </Drawer>
    </div>
  );
}

function CredentialForm({ onDone }: { onDone: (credential: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const credential = await apiPost<Entity>('/api/v1/credentials', values);
      form.resetFields();
      onDone(credential);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="type" label="类型" initialValue="private_key" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'private_key', value: 'private_key' },
            { label: 'password', value: 'password' },
          ]}
        />
      </Form.Item>
      <Form.Item name="secret" label="Secret" rules={[{ required: true }]}>
        <Input.TextArea rows={6} />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        保存凭据
      </Button>
    </Form>
  );
}

function CredentialList({ data, servers, onDone }: { data: Entity[]; servers: Entity[]; onDone: () => void }) {
  const [editingID, setEditingID] = useState('');
  const [busyID, setBusyID] = useState('');
  const [form] = Form.useForm();
  const editing = findByID(data, editingID);
  function open(item: Entity) {
    setEditingID(String(item.id ?? ''));
    form.setFieldsValue({ name: item.name, description: item.description, enabled: item.enabled !== false });
  }
  async function submit() {
    if (!editingID) return;
    const values = await form.validateFields();
    setBusyID(editingID);
    try {
      await apiPatch<Entity>(`/api/v1/credentials/${editingID}`, values);
      setEditingID('');
      onDone();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败');
    } finally {
      setBusyID('');
    }
  }
  async function setEnabled(item: Entity, enabled: boolean) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiPatch<Entity>(`/api/v1/credentials/${item.id}`, { enabled });
      onDone();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败');
    } finally {
      setBusyID('');
    }
  }
  async function deleteCredential(item: Entity) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiDelete<Entity>(`/api/v1/credentials/${item.id}`);
      onDone();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败');
    } finally {
      setBusyID('');
    }
  }
  // 计算引用该凭据的服务器，供禁用/删除提示
  function referencedServers(credentialID: ScalarValue) {
    const id = scalarRef(credentialID);
    return servers.filter((server) => scalarRef(server.credential_ref) === id);
  }
  return (
    <div className="mini-list">
      <Typography.Title level={4}>凭据</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          const refs = referencedServers(item.id);
          return (
            <div className="data-row">
              <div className="data-main">
                <Space>
                  <Typography.Text strong>{item.name ?? item.id}</Typography.Text>
                  <StatusTag value={enabled ? 'enabled' : 'disabled'} />
                </Space>
                <Typography.Text type="secondary">{`${item.type ?? '-'} / ${item.description ?? '-'}`}</Typography.Text>
                {refs.length > 0 ? <Typography.Text type="warning">{`被 ${refs.length} 台服务器引用`}</Typography.Text> : null}
              </div>
              <Space>
                <Button onClick={() => open(item)}>编辑</Button>
                {enabled && refs.length > 0 ? (
                  <Popconfirm
                    title="禁用凭据"
                    description={`该凭据被以下服务器引用，禁用后它们的 SSH 连接将失效：${refs.map((server) => String(server.name ?? server.id)).join('、')}`}
                    onConfirm={() => void setEnabled(item, false)}
                  >
                    <Button loading={busyID === item.id}>禁用</Button>
                  </Popconfirm>
                ) : (
                  <Button loading={busyID === item.id} onClick={() => void setEnabled(item, !enabled)}>
                    {enabled ? '禁用' : '启用'}
                  </Button>
                )}
                <Popconfirm
                  title="删除凭据"
                  description={refs.length > 0 ? '凭据仍被服务器引用，请先解除引用。' : '删除后不可恢复。'}
                  disabled={refs.length > 0}
                  onConfirm={() => void deleteCredential(item)}
                >
                  <Button danger loading={busyID === item.id} disabled={refs.length > 0}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            </div>
          );
        }}
      />
      <Drawer title="编辑凭据" open={!!editing} onClose={() => setEditingID('')} width={420} footer={null}>
        {editing ? (
          <Form form={form} layout="vertical">
            <Form.Item name="name" label="名称" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item label="类型">
              <Input value={String(editing.type ?? '-')} disabled />
            </Form.Item>
            <Form.Item name="description" label="描述">
              <Input.TextArea rows={3} />
            </Form.Item>
            <Form.Item name="enabled" valuePropName="checked">
              <Checkbox>启用</Checkbox>
            </Form.Item>
            <Typography.Text type="secondary">Secret 创建后不可修改；如需更换请删除后重建，并更新引用该凭据的服务器。</Typography.Text>
            <div style={{ marginTop: 16 }}>
              <Space>
                <Button type="primary" loading={busyID === editingID} onClick={() => void submit()}>
                  保存
                </Button>
                <Button onClick={() => setEditingID('')}>取消</Button>
              </Space>
            </div>
          </Form>
        ) : null}
      </Drawer>
    </div>
  );
}

// 通知投递记录：config_id 找不到对应配置时显示"已删除配置"。
function NotificationDeliveryList({ data, configs }: { data: Entity[]; configs: Entity[] }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>最近投递</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const config = findByID(configs, scalarRef(item.config_id));
          return (
            <div className="data-row compact">
              <div className="data-main">
                <Space>
                  <Typography.Text strong>{config ? (config.name ?? config.id) : '已删除配置'}</Typography.Text>
                  <StatusTag value={String(item.status ?? '-')} />
                </Space>
                <Typography.Text type="secondary">{`${item.event_type ?? '-'}${item.last_error ? ' · ' + item.last_error : ''}`}</Typography.Text>
              </div>
            </div>
          );
        }}
      />
    </div>
  );
}

function APIKeyForm({ users, onCreated, ownKey = false }: { users: Entity[]; onCreated: () => void; ownKey?: boolean }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [plaintext, setPlaintext] = useState('');
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const scopes = Array.isArray(values.scopes) ? (values.scopes as string[]) : [];
      const body = await apiPost<APIKeyCreateResponse>('/api/v1/api-keys', {
        ...values,
        scopes: JSON.stringify(scopes.length > 0 ? scopes : ['release:create']),
      });
      setPlaintext(body.plaintext);
      form.resetFields();
      // 创建成功后只刷新列表，不关闭抽屉：明文必须留在页面上供用户保存。
      onCreated();
    } finally {
      setLoading(false);
    }
  }
  return (
    <Space orientation="vertical" className="wide" size="middle">
      {plaintext ? (
        <Alert
          type="success"
          showIcon
          message="访问密钥明文只显示一次"
          description={<Typography.Text copyable>{plaintext}</Typography.Text>}
        />
      ) : null}
      <Form
        form={form}
        layout="vertical"
        initialValues={{ scopes: ['release:create', 'release:confirm'] }}
        onFinish={(values) => void submit(values)}
        hidden={plaintext !== ''}
      >
        <Form.Item name="name" label="名称" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        {ownKey ? <Typography.Text type="secondary">该访问密钥将归属当前登录用户。</Typography.Text> : <Form.Item name="owner_user_id" label="归属用户" rules={[{ required: true }]}><Select options={users.map(userOption)} showSearch optionFilterProp="label" /></Form.Item>}
        <Form.Item name="scopes" label="权限范围" rules={[{ required: true, message: '请至少选择一个权限' }]}>
          <Checkbox.Group options={API_KEY_SCOPE_OPTIONS} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={loading}>
          创建访问密钥
        </Button>
      </Form>
    </Space>
  );
}

function APIKeyList({ data, users, onDone }: { data: Entity[]; users: Entity[]; onDone: () => void }) {
  const [busyID, setBusyID] = useState('');
  async function setEnabled(item: Entity, enabled: boolean) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiPatch<Entity>(`/api/v1/api-keys/${item.id}`, { enabled });
      onDone();
    } finally {
      setBusyID('');
    }
  }
  async function deleteKey(item: Entity) {
    setBusyID(String(item.id ?? ''));
    try {
      await apiDelete<Entity>(`/api/v1/api-keys/${item.id}`);
      onDone();
    } finally {
      setBusyID('');
    }
  }
  function scopeLabels(raw: Entity[string]) {
    let scopes: unknown[] = [];
    try {
      const parsed = JSON.parse(String(raw ?? '[]'));
      if (Array.isArray(parsed)) {
        scopes = parsed;
      }
    } catch {
      scopes = [];
    }
    return scopes.map((scope) => API_KEY_SCOPE_OPTIONS.find((option) => option.value === String(scope))?.label ?? String(scope)).join('、') || '无';
  }
  return (
    <div className="mini-list">
      <Typography.Title level={4}>访问密钥</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => {
          const enabled = item.enabled !== false;
          const ownerID = scalarRef(item.owner_user_id);
          const owner = findByID(users, ownerID);
          return (
            <div className="data-row">
              <div className="data-main">
                <Space>
                  <Typography.Text strong>{item.name ?? item.id}</Typography.Text>
                  <StatusTag value={enabled ? 'enabled' : 'disabled'} />
                </Space>
                <Typography.Text type="secondary">{`${item.prefix ?? '-'} / 归属 ${owner ? (owner.display_name ?? owner.username ?? shortID(ownerID)) : shortID(ownerID)}`}</Typography.Text>
                <Typography.Text type="secondary">{scopeLabels(item.scopes)}</Typography.Text>
              </div>
              <Space>
                <Button loading={busyID === item.id} onClick={() => void setEnabled(item, !enabled)}>
                  {enabled ? '禁用' : '启用'}
                </Button>
                <Popconfirm title="删除访问密钥" description="删除后不会再出现在列表中。" onConfirm={() => void deleteKey(item)}>
                  <Button danger loading={busyID === item.id}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            </div>
          );
        }}
      />
    </div>
  );
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function findByID(items: Entity[], id?: string | number | boolean | null) {
  if (!id) {
    return undefined;
  }
  return items.find((item) => String(item.id) === String(id));
}

function keepOrFirst(id: string, items: Entity[]) {
  if (findByID(items, id)) {
    return id;
  }
  return items[0]?.id ? String(items[0].id) : '';
}

function selectionFromRelease(release: Entity, fallback: Selection): Selection {
  return {
    ...fallback,
    serviceID: releaseSelectionValue(release.service_id, fallback.serviceID),
    environmentID: releaseSelectionValue(release.environment_id, fallback.environmentID),
    versionID: releaseSelectionValue(release.service_version_id, fallback.versionID),
    targetID: releaseSelectionValue(release.deployment_target_id, fallback.targetID),
  };
}

function releaseSelectionValue(value: Entity[string], fallback: string) {
  return value ? String(value) : fallback;
}

function filterTargets(items: Entity[], serviceID?: string | number | boolean | null, environmentID?: string | number | boolean | null) {
  return items.filter((item) => {
    const serviceMatched = !serviceID || String(item.service_id) === String(serviceID);
    const environmentMatched = !environmentID || String(item.environment_id) === String(environmentID);
    return serviceMatched && environmentMatched;
  });
}

function formatReleaseContext(item: Entity, state: AppState) {
  const service = findByID(state.services, item.service_id);
  const environment = findByID(state.environments, item.environment_id);
  const version = findByID(state.versions, item.service_version_id);
  return [
    namedRef(service, item.service_id, 'name'),
    namedRef(environment, item.environment_id, 'name'),
    namedRef(version, item.service_version_id, 'version'),
  ].join(' / ');
}

// 发布记录上下文：从 item 自带的 release_*_id 字段拼展示（后端 LEFT JOIN release_requests 提供）。
function formatDeployContext(item: Entity, state: AppState) {
  const svcID = String(item.release_service_id ?? '');
  const envID = String(item.release_environment_id ?? '');
  const verID = String(item.release_service_version_id ?? '');
  if (!svcID && !envID && !verID) {
    return `发布单 ${shortID(item.release_request_id)}`;
  }
  return `${shortID(item.release_request_id)} · ${namedRef(findByID(state.services, svcID), svcID, 'name')} / ${namedRef(findByID(state.environments, envID), envID, 'name')} / ${namedRef(findByID(state.versions, verID), verID, 'version')}`;
}

function formatEventContext(item: Entity, state: AppState) {
  const parts = [];
  const actor = formatActor(item.actor_type, item.actor_id, state);
  if (actor) {
    parts.push(actor);
  }
  if (item.api_key_id) {
    parts.push(`访问密钥 ${shortID(item.api_key_id)}`);
  }
  if (item.deploy_record_id) {
    parts.push(`部署 ${shortID(item.deploy_record_id)}`);
  }
  if (item.created_at) {
    parts.push(formatDateTime(item.created_at));
  }
  return parts.length > 0 ? parts.join(' / ') : '-';
}

function formatActor(actorType: Entity[string], actorID: Entity[string], state: AppState) {
  if (!actorType && !actorID) {
    return '';
  }
  const type = scalarRef(actorType);
  const id = scalarRef(actorID);
  if (type === 'user') {
    return `用户 ${namedRef(findByID(state.users, id), id, 'display_name')}`;
  }
  return `${type ?? 'actor'} ${shortID(id)}`;
}

function formatServerRef(serverID: Entity[string], state: AppState) {
  const id = scalarRef(serverID);
  return namedRef(findByID(state.servers, id), id, 'name');
}

function targetRefFor(item: Entity, state: AppState) {
  return item.target_type === 'server_group'
    ? findByID(state.serverGroups, item.target_ref_id)
    : findByID(state.servers, item.target_ref_id);
}

function namedRef(item: Entity | undefined, fallback: Entity[string], field: string) {
  if (!item) {
    return shortID(fallback);
  }
  const name = item[field] ?? item.name ?? item.id;
  return `${name ?? '-'} (${shortID(item.id)})`;
}

function scalarRef(value: Entity[string]): ScalarValue {
  return Array.isArray(value) ? value[0] : value;
}

function shortID(value: Entity[string]) {
  if (!value) {
    return '-';
  }
  const text = String(value);
  const separator = text.indexOf('_');
  if (separator > 0 && text.length - separator > 10) {
    return `${text.slice(0, separator)}_${text.slice(separator + 1, separator + 7)}`;
  }
  return text.length > 12 ? `${text.slice(0, 12)}...` : text;
}

function formatTarget(item?: Entity, targetRef?: Entity) {
  if (!item) {
    return '-';
  }
  return `${item.executor_type ?? '-'} / ${item.target_type ?? '-'} / ${targetRef?.name ?? item.target_ref_id ?? item.id ?? '-'}`;
}

function selectLabel(item: Entity, nameField: string) {
  if (nameField === 'executor_type') {
    return formatTarget(item);
  }
  return String(item[nameField] ?? item.name ?? item.id);
}

// 用户角色中文映射，便于理解
function roleLabel(role: Entity[string]) {
  switch (String(role ?? '')) {
    case 'admin':
      return '管理员';
    case 'employee':
      return '员工';
    default:
      return String(role ?? '-');
  }
}

// 用户下拉选项：优先显示名，回退到用户名
function userOption(item: Entity) {
  return { label: String(item.display_name ?? item.username ?? item.id), value: String(item.id) };
}

// 访问密钥 scope 选项：须与后端 allowedAPIKeyScopes（internal/httpapi/inventory.go）保持同步
const API_KEY_SCOPE_OPTIONS: Array<{ label: string; value: string }> = [
  { label: '读取资源清单（inventory:read）', value: 'inventory:read' },
  { label: '读取发布（release:read）', value: 'release:read' },
  { label: '创建发布（release:create）', value: 'release:create' },
  { label: '确认发布（release:confirm）', value: 'release:confirm' },
  { label: '回滚发布（release:rollback）', value: 'release:rollback' },
  { label: '读取部署（deploy:read）', value: 'deploy:read' },
  { label: '登记版本（version:write）', value: 'version:write' },
  { label: '管理写（admin:write）', value: 'admin:write' },
];

// 上海时区时间格式化（存储/传输为 UTC，展示本地化）
function formatDateTime(value: Entity[string]) {
  if (!value) {
    return '-';
  }
  const time = new Date(String(value)).getTime();
  if (Number.isNaN(time)) {
    return String(value);
  }
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(time));
}

async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(path);
  return readAPI<T>(response);
}

async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  return readAPI<T>(response);
}

async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  return readAPI<T>(response);
}

async function apiDelete<T>(path: string): Promise<T> {
  const response = await fetch(path, { method: 'DELETE' });
  return readAPI<T>(response);
}

async function readAPI<T>(response: Response): Promise<T> {
  const body = (await response.json()) as { data?: T; error?: { message: string } };
  if (!response.ok) {
    throw new Error(body.error?.message ?? `HTTP ${response.status}`);
  }
  return (body.data ?? []) as T;
}
