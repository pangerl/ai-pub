import {
  Alert,
  Button,
  Checkbox,
  ConfigProvider,
  Form,
  Input,
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
  releases: Entity[];
  deploys: Entity[];
  events: Entity[];
  serverLogs: Entity[];
  states: Entity[];
  policies: Entity[];
  notificationConfigs: Entity[];
  notificationDeliveries: Entity[];
  ops: Entity | null;
};

type Selection = {
  serviceID: string;
  environmentID: string;
  versionID: string;
  targetID: string;
  userID: string;
};

type ListFilters = {
  scoped: boolean;
  releaseStatus: string;
  deployStatus: string;
};

type ManualTargetRef = {
  targetType: string;
  targetRefID: string;
};

type Page = 'workbench' | 'create' | 'releases' | 'release-detail' | 'deploys' | 'configuration' | 'policy' | 'management' | 'api-keys';
type ReleaseView = 'pending' | 'mine' | 'all';
type InfrastructureView = 'overview' | 'application' | 'runtime' | 'targeting' | 'state';
type ManagementView = 'overview' | 'users' | 'access' | 'notifications' | 'credentials';
type PolicyView = 'overview' | 'rules';

type AppRoute = {
  page: Page;
  releaseID?: string;
};

function routeFromLocation(): AppRoute {
  const path = window.location.pathname.replace(/\/+$/, '') || '/';
  const releaseMatch = path.match(/^\/releases\/([^/]+)$/);
  if (releaseMatch && releaseMatch[1] !== 'new') return { page: 'release-detail', releaseID: decodeURIComponent(releaseMatch[1]) };
  switch (path) {
    case '/releases/new': return { page: 'create' };
    case '/releases': return { page: 'releases' };
    case '/deploys': return { page: 'deploys' };
    case '/configuration': return { page: 'configuration' };
    case '/policy': return { page: 'policy' };
    case '/management': return { page: 'management' };
    case '/access-keys': return { page: 'api-keys' };
    default: return { page: 'workbench' };
  }
}

function pathForPage(page: Page, releaseID?: string) {
  if (page === 'release-detail' && releaseID) return `/releases/${encodeURIComponent(releaseID)}`;
  return ({ workbench: '/', create: '/releases/new', releases: '/releases', deploys: '/deploys', configuration: '/configuration', policy: '/policy', management: '/management', 'api-keys': '/access-keys', 'release-detail': '/releases' } as Record<Page, string>)[page];
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
  releases: [],
  deploys: [],
  events: [],
  serverLogs: [],
  states: [],
  policies: [],
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

export function App() {
  const [api, contextHolder] = message.useMessage();
  const [state, setState] = useState<AppState>(emptyState);
  const [health, setHealth] = useState<Entity | null>(null);
  const [activeRelease, setActiveRelease] = useState<Entity | null>(null);
  const [activeDeployID, setActiveDeployID] = useState('');
  const [selection, setSelection] = useState<Selection>({ serviceID: '', environmentID: '', versionID: '', targetID: '', userID: '' });
  const [manualTargetRef, setManualTargetRef] = useState<ManualTargetRef>({ targetType: '', targetRefID: '' });
  const [filters, setFilters] = useState<ListFilters>({ scoped: true, releaseStatus: 'all', deployStatus: 'all' });
  const [preflight, setPreflight] = useState<PreflightResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [page, setPageState] = useState<Page>(() => routeFromLocation().page);
  const [routeReleaseID, setRouteReleaseID] = useState<string | undefined>(() => routeFromLocation().releaseID);
  const [releaseView, setReleaseView] = useState<ReleaseView>('all');
  const [releaseQuery, setReleaseQuery] = useState('');
  const [releaseSource, setReleaseSource] = useState('all');
  const [releaseTimeRange, setReleaseTimeRange] = useState('all');
  const [releaseFilterNow, setReleaseFilterNow] = useState(0);
  const [infrastructureView, setInfrastructureView] = useState<InfrastructureView>('overview');
  const [managementView, setManagementView] = useState<ManagementView>('overview');
  const [policyView, setPolicyView] = useState<PolicyView>('overview');
  const [effectivePolicy, setEffectivePolicy] = useState<Entity | null>(null);
  const [currentUser, setCurrentUser] = useState<Entity | null>(null);
  const [authReady, setAuthReady] = useState(false);
  const activeReleaseID = activeRelease?.id as string | undefined;

  const setPage = useCallback((next: Page, releaseID?: string) => {
    const nextReleaseID = next === 'release-detail' ? releaseID : undefined;
    setPageState(next);
    setRouteReleaseID(nextReleaseID);
    window.history.pushState(null, '', pathForPage(next, nextReleaseID));
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
      user: currentUser ?? findByID(state.users, selection.userID) ?? state.users[0],
      release: activeRelease,
    };
  }, [activeRelease, currentUser, selection, state]);

  const releaseByID = useMemo(() => {
    const byID = new Map<string, Entity>();
    for (const release of state.releases) {
      if (release.id) {
        byID.set(String(release.id), release);
      }
    }
    return byID;
  }, [state.releases]);

  const filteredDeploys = useMemo(
    () =>
      state.deploys.filter((item) => {
        const statusMatched = filters.deployStatus === 'all' || item.status === filters.deployStatus;
        const release = releaseByID.get(String(item.release_request_id ?? ''));
        const scopeMatched = !filters.scoped || releaseMatchesSelection(release, selected.service?.id, selected.environment?.id);
        return statusMatched && scopeMatched;
      }),
    [filters.deployStatus, filters.scoped, releaseByID, selected.environment?.id, selected.service?.id, state.deploys],
  );

  const visibleReleases = useMemo(() => {
    return state.releases.filter((item) => {
      const statusMatched = filters.releaseStatus === 'all' || item.status === filters.releaseStatus;
      const serviceMatched = !selection.serviceID || String(item.service_id) === selection.serviceID;
      const environmentMatched = !selection.environmentID || String(item.environment_id) === selection.environmentID;
      const currentUserID = String(selected.user?.id ?? '');
      const viewMatched =
        releaseView === 'all' ||
        (releaseView === 'mine' && currentUserID !== '' && String(item.created_by_id) === currentUserID) ||
        (releaseView === 'pending' && item.status === 'pending_confirm');
      const sourceMatched = releaseSource === 'all' || item.source === releaseSource;
      const query = releaseQuery.trim().toLowerCase();
      const queryMatched = !query || [item.id, item.source, item.created_by_id, formatActor(item.created_by_type, item.created_by_id, state)].some((value) => String(value ?? '').toLowerCase().includes(query));
      const ageHours = releaseTimeRange === 'all' ? Infinity : Number(releaseTimeRange);
      const createdAt = new Date(String(item.created_at ?? '')).getTime();
      const timeMatched = !Number.isFinite(createdAt) || releaseFilterNow === 0 || releaseFilterNow - createdAt <= ageHours * 60 * 60 * 1000;
      return statusMatched && serviceMatched && environmentMatched && viewMatched && sourceMatched && queryMatched && timeMatched;
    });
  }, [filters.releaseStatus, releaseFilterNow, releaseQuery, releaseSource, releaseTimeRange, releaseView, selected.user?.id, selection.environmentID, selection.serviceID, state]);

  const workbenchReleases = useMemo(() => {
    const currentUserID = String(selected.user?.id ?? '');
    return {
      pending: state.releases.filter(
        (item) => item.status === 'pending_confirm' && (selected.user?.role === 'admin' || (currentUserID !== '' && String(item.created_by_id) === currentUserID)),
      ),
      running: state.releases.filter(
        (item) => item.status === 'running' && currentUserID !== '' && String(item.created_by_id) === currentUserID,
      ),
      failed: state.releases.filter((item) => item.status === 'failed' || item.status === 'partial').slice(0, 5),
    };
  }, [selected.user?.id, selected.user?.role, state.releases]);

  const setupSteps = useMemo(() => [
    { key: 'application', label: '定义应用', detail: '创建项目、服务和至少一个版本。', complete: state.projects.length > 0 && state.services.length > 0 && state.versions.length > 0 },
    { key: 'runtime', label: '准备运行环境', detail: '登记环境和至少一台服务器。', complete: state.environments.length > 0 && state.servers.length > 0 },
    { key: 'targeting', label: '建立部署连接', detail: '创建可启用的部署目标。', complete: state.targets.some((item) => item.enabled !== false) },
  ] as const, [state.environments.length, state.projects.length, state.servers.length, state.services.length, state.targets, state.versions.length]);
  const needsSetup = currentUser?.role === 'admin' && setupSteps.some((step) => !step.complete);

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
        releases,
        deploys,
        states,
        policies,
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
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/users') : Promise.resolve([]),
        apiGet<Entity[]>('/api/v1/api-keys'),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/credentials') : Promise.resolve([]),
        apiGet<Entity[]>('/api/v1/release-requests'),
        apiGet<Entity[]>('/api/v1/deploy-records'),
        apiGet<Entity[]>('/api/v1/server-deployment-states'),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/release-policies') : Promise.resolve([]),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/notification-configs') : Promise.resolve([]),
        currentUser?.role === 'admin' ? apiGet<Entity[]>('/api/v1/notification-deliveries') : Promise.resolve([]),
        apiGet<Entity>('/api/v1/ops/summary'),
      ]);
      const serviceID = preferredSelection?.serviceID || selection.serviceID || (services[0]?.id as string | undefined);
      const versions = serviceID ? await apiGet<Entity[]>(`/api/v1/services/${serviceID}/versions`) : [];
      const releaseID = preferredReleaseID === null ? undefined : ((preferredReleaseID ?? activeReleaseID) as string | undefined);
      const refreshedActiveRelease = releaseID ? findByID(releases, releaseID) : null;
      const events = releaseID ? await apiGet<Entity[]>(`/api/v1/release-requests/${releaseID}/events`) : [];
      const currentDeployID = activeDeployID || undefined;
      const serverLogs = currentDeployID ? await apiGet<Entity[]>(`/api/v1/deploy-records/${currentDeployID}/server-logs`) : [];
      setHealth(healthBody);
      if (preferredReleaseID !== undefined || activeReleaseID) {
        setActiveRelease(refreshedActiveRelease ?? null);
      }
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
        releases,
        deploys,
        events,
        serverLogs,
        states,
        policies,
        notificationConfigs,
        notificationDeliveries,
        ops,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败');
    }
  }, [activeDeployID, activeReleaseID, currentUser, selection.serviceID]);

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

  const changeReleaseFilters = useCallback((patch: Partial<ListFilters>) => {
    setActiveRelease(null);
    setState((current) => ({ ...current, events: [] }));
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  const changeDeployFilters = useCallback((patch: Partial<ListFilters>) => {
    setActiveDeployID('');
    setState((current) => ({ ...current, serverLogs: [] }));
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  useEffect(() => {
    void apiGet<Entity>('/api/v1/auth/me')
      .then((user) => {
        setCurrentUser(user);
        setSelection((current) => ({ ...current, userID: String(user.id ?? '') }));
      })
      .catch(() => setCurrentUser(null))
      .finally(() => setAuthReady(true));
  }, []);

  useEffect(() => {
    const onPopState = () => {
      const route = routeFromLocation();
      setPageState(route.page);
      setRouteReleaseID(route.releaseID);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => {
    if (currentUser) {
      void refreshAll(null, { userID: String(currentUser.id ?? '') });
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
    const release = findByID(state.releases, routeReleaseID);
    if (!release) {
      return;
    }
    const nextSelection = selectionFromRelease(release, selection);
    setActiveRelease(release);
    setSelection(nextSelection);
    void refreshAll(routeReleaseID, nextSelection);
    void apiPost<PreflightResult>(`/api/v1/release-requests/${routeReleaseID}/preflight`, {}).then(setPreflight).catch(() => setPreflight(null));
  }, [activeReleaseID, currentUser, page, refreshAll, routeReleaseID, selection, state.releases]);

  useEffect(() => {
    setSelection((current) => {
      const serviceID = keepOrFirst(current.serviceID, state.services);
      const environmentID = keepOrFirst(current.environmentID, state.environments);
      const versionID = keepOrFirst(current.versionID, state.versions);
      const targets = filterTargets(state.targets, serviceID, environmentID);
      const targetID = keepOrFirst(current.targetID, targets.length > 0 ? targets : state.targets);
      const userID = currentUser?.id ? String(currentUser.id) : keepOrFirst(current.userID, state.users);
      if (
        serviceID === current.serviceID &&
        environmentID === current.environmentID &&
        versionID === current.versionID &&
        targetID === current.targetID &&
        userID === current.userID
      ) {
        return current;
      }
      return { serviceID, environmentID, versionID, targetID, userID };
    });
  }, [currentUser?.id, state.environments, state.services, state.targets, state.users, state.versions]);

  useEffect(() => {
    if (currentUser?.role !== 'admin' || !selection.serviceID || !selection.environmentID) {
      setEffectivePolicy(null);
      return;
    }
    void apiGet<Entity>(`/api/v1/release-policies/effective?service_id=${encodeURIComponent(selection.serviceID)}&environment_id=${encodeURIComponent(selection.environmentID)}`)
      .then(setEffectivePolicy)
      .catch(() => setEffectivePolicy(null));
  }, [currentUser?.role, selection.environmentID, selection.serviceID]);

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
      setActiveRelease(body.release);
      await refreshAll(body.release.id as string);
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
      const nextSelection = selectionFromRelease(body.release, selection);
      setActiveRelease(body.release);
      setSelection(nextSelection);
      await refreshAll(body.release.id as string, nextSelection);
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
      const nextSelection = selectionFromRelease(body.release, selection);
      setActiveRelease(body.release);
      setSelection(nextSelection);
      await refreshAll(body.release.id as string, nextSelection);
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

  async function selectDeploy(deployID: string) {
    setActiveDeployID(deployID);
    await refreshServerLogs(deployID);
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

  const activeReleaseDeploys = state.deploys.filter((item) => String(item.release_request_id) === String(activeReleaseID));
  const activeDeploy = findByID(state.deploys, activeDeployID);

  async function signIn(username: string, password: string) {
    setLoading(true);
    setError('');
    try {
      const user = await apiPost<Entity>('/api/v1/auth/login', { username, password });
      setCurrentUser(user);
      setSelection((current) => ({ ...current, userID: String(user.id ?? '') }));
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
            {currentUser.role === 'admin' ? <NavButton active={page === 'policy'} onClick={() => setPage('policy')}>策略</NavButton> : null}
            {currentUser.role === 'admin' ? <NavButton active={page === 'management'} onClick={() => setPage('management')}>系统</NavButton> : null}
          </nav>
          <div className="header-actions">
            <span className={`health-dot ${status === 'ok' ? 'ok' : ''}`} title={`服务状态：${status}`} />
            <span className="current-user">{currentUser.display_name ?? currentUser.username} <small>{currentUser.role}</small></span>
            <Button className="quiet-button" onClick={() => setPage('api-keys')}>访问密钥</Button>
            <Button className="quiet-button" onClick={() => void refreshAll()}>刷新</Button>
            <Button className="quiet-button" onClick={() => void signOut()}>退出</Button>
          </div>
        </header>
        <main className="app-main">
          {error ? <Alert className="notice" type="error" message="操作未完成" description={error} showIcon closable onClose={() => setError('')} /> : null}

          {page === 'workbench' ? (
            <>
              <PageHeading eyebrow="RELEASE OPERATIONS" title={needsSetup ? '准备首个发布' : '发布工作台'} description={needsSetup ? '完成以下最小配置后，即可创建并执行发布。' : '从需要你处理的发布开始。'} action={<Button type="primary" onClick={() => setPage(needsSetup ? 'configuration' : 'create')}>{needsSetup ? '进入配置中心' : '创建发布单'}</Button>} />
              {needsSetup ? <SetupChecklist steps={setupSteps} onOpen={(key) => { setInfrastructureView(key); setPage('configuration'); }} /> : null}
              <section className="workbench-tasks">
                <SectionTitle title="需要处理的发布" meta="MY RELEASE WORK" />
                <div className="content-grid three-up">
                <TaskList title="待我确认" data={workbenchReleases.pending} state={state} empty="暂无待确认发布" onOpen={(item) => void selectRelease(item)} />
                <TaskList title="我发起的运行中" data={workbenchReleases.running} state={state} empty="暂无运行中发布" onOpen={(item) => void selectRelease(item)} />
                <TaskList title="最近失败" data={workbenchReleases.failed} state={state} empty="暂无失败发布" onOpen={(item) => void selectRelease(item)} />
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
                    <LabeledSelect label="版本" value={selected.version?.id} options={state.versions} nameField="version" onChange={(value) => changeSelection({ versionID: value })} />
                    <LabeledSelect label="部署目标" value={selected.target?.id} options={selected.targetOptions} nameField="executor_type" onChange={(value) => changeSelection({ targetID: value })} />
                  </div>
                  <div className="release-context">
                    <span>项目 <strong>{selected.project?.name ?? '-'}</strong></span>
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
              <PageHeading eyebrow="RELEASE CENTER" title="发布中心" description="查看、确认和追踪每一次发布。" action={<Space><Button onClick={() => setPage('deploys')}>发布记录</Button><Button type="primary" onClick={() => setPage('create')}>创建发布单</Button></Space>} />
              <section className="surface list-surface">
                <div className="list-toolbar-v2">
                  <div className="segmented-control">
                    <button className={releaseView === 'pending' ? 'active' : ''} onClick={() => setReleaseView('pending')}>待我确认</button>
                    <button className={releaseView === 'mine' ? 'active' : ''} onClick={() => setReleaseView('mine')}>我发起的</button>
                    <button className={releaseView === 'all' ? 'active' : ''} onClick={() => setReleaseView('all')}>全部发布</button>
                  </div>
                  <div className="filter-row">
                    <Select value={selection.serviceID || undefined} placeholder="全部服务" allowClear options={state.services.map(entityOption)} onChange={(value) => changeSelection({ serviceID: value ?? '', versionID: '', targetID: '' })} />
                    <Select value={selection.environmentID || undefined} placeholder="全部环境" allowClear options={state.environments.map(entityOption)} onChange={(value) => changeSelection({ environmentID: value ?? '', targetID: '' })} />
                    <Select value={filters.releaseStatus} options={releaseStatusOptions} onChange={(value) => changeReleaseFilters({ releaseStatus: value })} />
                    <Select value={releaseSource} options={[{ label: '全部来源', value: 'all' }, ...Array.from(new Set(state.releases.map((item) => String(item.source ?? 'web')))).map((value) => ({ label: value, value }))]} onChange={setReleaseSource} />
                    <Select value={releaseTimeRange} options={[{ label: '全部时间', value: 'all' }, { label: '近 24 小时', value: '24' }, { label: '近 7 天', value: '168' }]} onChange={(value) => { setReleaseTimeRange(value); setReleaseFilterNow(Date.now()); }} />
                    <Input className="release-search" value={releaseQuery} allowClear placeholder="搜索发布单、申请人或来源" onChange={(event) => setReleaseQuery(event.target.value)} />
                  </div>
                </div>
                <ReleaseRows data={visibleReleases} state={state} onOpen={(item) => void selectRelease(item)} />
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
                      ['服务', selected.service?.name], ['环境', selected.environment?.name], ['版本', selected.version?.version], ['部署目标', formatTarget(selected.target, selected.targetRef)], ['来源', activeRelease.source], ['申请人', formatActor(activeRelease.created_by_type, activeRelease.created_by_id, state)], ['创建时间', activeRelease.created_at], ['更新时间', activeRelease.updated_at],
                    ]} /></section>
                    <section className="surface"><SectionTitle title="预检与门禁" meta="PREFLIGHT" /><PreflightPanel result={preflight} /></section>
                    <section className="surface"><SectionTitle title="关联发布记录" meta="DEPLOY RECORDS" /><DeployRows data={activeReleaseDeploys} state={state} releases={releaseByID} onOpen={(item) => { void selectDeploy(String(item.id)); setPage('deploys'); }} /></section>
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
                <div className="list-toolbar-v2"><div className="filter-row"><Select value={filters.deployStatus} options={deployStatusOptions} onChange={(value) => changeDeployFilters({ deployStatus: value })} /><Checkbox checked={filters.scoped} onChange={(event) => changeDeployFilters({ scoped: event.target.checked })}>仅当前服务与环境</Checkbox></div></div>
                <DeployRows data={filteredDeploys} state={state} releases={releaseByID} onOpen={(item) => void selectDeploy(String(item.id))} />
              </section>
              {activeDeploy ? <section className="surface deploy-detail"><SectionTitle title="执行快照" meta={`DEPLOY ${shortID(activeDeploy.id)}`} /><KeyValueGrid values={[
                ['状态', activeDeploy.status], ['执行器', activeDeploy.executor_type], ['创建时间', activeDeploy.created_at], ['更新时间', activeDeploy.updated_at], ['目标服务器数', activeDeploy.total_servers], ['成功 / 失败 / 跳过', `${activeDeploy.success_servers ?? 0} / ${activeDeploy.failed_servers ?? 0} / ${activeDeploy.skipped_servers ?? 0}`],
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
                    <section className="surface infrastructure-guide"><span className="mono-label">推荐顺序</span><h2>把一次发布需要的对象连起来。</h2><ol><li><strong>定义应用</strong><span>创建项目、服务和可发布版本。</span></li><li><strong>准备运行环境</strong><span>登记环境、服务器以及需要批量执行的服务器组。</span></li><li><strong>建立部署连接</strong><span>将服务、环境和运行目标组成部署目标，发布时即可选择。</span></li></ol></section>
                  </> : null}
                  {infrastructureView === 'application' ? <>
                    <InfrastructureSectionHeading eyebrow="APPLICATION" title="应用与版本" description="这是发布内容的来源。先创建项目和服务，再为服务登记可部署版本。" />
                    <div className="infrastructure-columns"><section className="surface infrastructure-inventory"><SectionTitle title="已注册对象" meta="INVENTORY" /><div className="infrastructure-list-stack"><EntityList title="项目" data={state.projects} fields={['slug', 'description']} /><EntityList title="服务" data={state.services} fields={['slug', 'project_id']} /><EntityList title="当前服务版本" data={state.versions} fields={['version', 'source']} /></div></section><section className="surface infrastructure-actions"><SectionTitle title="逐步创建" meta="CREATE" /><div className="infrastructure-form-stack"><div><h3>1. 项目</h3><ProjectForm onDone={() => void refreshAll()} /></div><div><h3>2. 服务</h3><ServiceForm projects={state.projects} onDone={(service) => refreshWithSelection({ serviceID: String(service.id ?? ''), versionID: '', targetID: '' })} /></div><div><h3>3. 版本</h3><VersionForm services={state.services} onDone={(version) => refreshWithSelection({ serviceID: String(version.service_id ?? ''), versionID: String(version.id ?? '') })} /></div></div></section></div>
                    <section className="surface service-detail"><SectionTitle title="服务部署视图" meta="SERVICE" /><LabeledSelect label="服务" value={selected.service?.id} options={state.services} nameField="name" onChange={(value) => refreshWithSelection({ serviceID: value, versionID: '', targetID: '' })} /><ServiceDetail service={selected.service} versions={state.versions} targets={state.targets} environments={state.environments} states={state.states} /></section>
                  </> : null}
                  {infrastructureView === 'runtime' ? <>
                    <InfrastructureSectionHeading eyebrow="RUNTIME" title="运行环境" description="管理发布到哪里，以及哪些服务器作为一个批次共同执行。" />
                    <div className="infrastructure-columns"><section className="surface infrastructure-inventory"><SectionTitle title="运行资源" meta="INVENTORY" /><div className="infrastructure-list-stack"><EntityList title="环境" data={state.environments} fields={['slug', 'is_production']} /><EntityList title="服务器" data={state.servers} fields={['host', 'username', 'last_check_status']} /><ServerGroupList data={state.serverGroups} state={state} /></div></section><section className="surface infrastructure-actions"><SectionTitle title="新增运行资源" meta="CREATE" /><div className="infrastructure-form-stack"><div><h3>1. 环境</h3><EnvironmentForm onDone={(environment) => refreshWithSelection({ environmentID: String(environment.id ?? ''), targetID: '' })} /></div><div><h3>2. 服务器</h3><ServerForm credentials={state.credentials} onDone={(server) => { setManualTargetRef({ targetType: 'server', targetRefID: String(server.id ?? '') }); void refreshAll(); }} /></div><div><h3>3. 服务器组</h3><ServerGroupForm servers={state.servers} onDone={(group) => { setManualTargetRef({ targetType: 'server_group', targetRefID: String(group.id ?? '') }); void refreshAll(); }} /></div></div></section></div>
                    <section className="surface infrastructure-maintenance"><SectionTitle title="维护与连通性" meta="MANAGE" /><div className="infrastructure-maintenance-grid"><div><h3>环境</h3><EnvironmentEditor environments={state.environments} onDone={() => void refreshAll()} /></div><div><h3>服务器与 SSH</h3><ServerEditor servers={state.servers} credentials={state.credentials} onDone={() => void refreshAll()} /></div><div><h3>服务器组</h3><ServerGroupEditor groups={state.serverGroups} servers={state.servers} onDone={() => void refreshAll()} /></div></div></section>
                  </> : null}
                  {infrastructureView === 'targeting' ? <>
                    <InfrastructureSectionHeading eyebrow="DEPLOYMENT TARGET" title="部署连接" description="把服务、环境与服务器或服务器组组合为发布时可选择的部署目标。" />
                    <div className="infrastructure-columns"><section className="surface infrastructure-inventory"><SectionTitle title="现有部署目标" meta="INVENTORY" /><DeploymentTargetList data={state.targets} state={state} /><div className="targeting-note"><strong>当前选择</strong><span>{selected.service?.name ?? '未选择服务'} / {selected.environment?.name ?? '未选择环境'}</span></div></section><section className="surface infrastructure-actions"><SectionTitle title="新建部署目标" meta="CREATE" /><DeploymentTargetForm services={state.services} environments={state.environments} servers={state.servers} serverGroups={state.serverGroups} selectedServiceID={selection.serviceID} selectedEnvironmentID={selection.environmentID} preferredTargetRef={manualTargetRef} onDone={(target) => refreshWithSelection({ serviceID: String(target.service_id ?? ''), environmentID: String(target.environment_id ?? ''), targetID: String(target.id ?? '') })} /></section></div>
                    <section className="surface infrastructure-maintenance"><SectionTitle title="维护部署目标" meta="MANAGE" /><DeploymentTargetEditor targets={state.targets} servers={state.servers} serverGroups={state.serverGroups} onDone={() => void refreshAll()} /></section>
                  </> : null}
                  {infrastructureView === 'state' ? <>
                    <InfrastructureSectionHeading eyebrow="RUNTIME STATE" title="当前部署状态" description="查看每台服务器最后一次成功部署后的版本状态；它是运行视图，不是配置入口。" />
                    <section className="surface infrastructure-state"><DeploymentStateList data={state.states} state={state} /></section>
                  </> : null}
                </div>
              </section>
            </>
          ) : null}

          {page === 'policy' ? (
            <>
              <PageHeading eyebrow="RELEASE POLICY" title="发布策略中心" description="策略在创建和确认时都会重新校验；这里负责定义范围、确认方式与冻结门禁。" />
              <section className="surface policy-map"><div><span className="mono-label">策略优先级</span><h2>系统规则 → 环境覆盖 → 服务覆盖</h2><p>范围越具体，优先级越高；无论如何，生产发布仍必须由管理员确认。</p></div><div className="policy-flow"><span>系统</span><i>→</i><span>环境</span><i>→</i><span>服务</span><b>发布时取最终生效值</b></div></section>
              <section className="policy-workspace-layout"><nav className="policy-nav" aria-label="策略模块"><PolicyNavButton active={policyView === 'overview'} label="策略概览" note="查看门禁状态" count={state.policies.length} onClick={() => setPolicyView('overview')} /><PolicyNavButton active={policyView === 'rules'} label="策略规则" note="新增或修改覆盖规则" count={state.policies.filter((item) => item.manual_freeze_enabled === true).length} onClick={() => setPolicyView('rules')} /></nav><div className="policy-workspace">
                {policyView === 'overview' ? <><section className="surface policy-summary"><SectionTitle title="策略状态" meta="GUARDRAILS" /><div className="policy-stat-grid"><PolicyStat label="已配置策略" value={state.policies.length} note="系统、环境和服务范围" onClick={() => setPolicyView('rules')} /><PolicyStat label="冻结规则" value={state.policies.filter((item) => item.manual_freeze_enabled === true).length} note="阻断新建与确认" onClick={() => setPolicyView('rules')} /><PolicyStat label="管理员确认" value={state.policies.filter((item) => item.confirm_mode === 'admin_confirm').length} note="生产发布强制执行" onClick={() => setPolicyView('rules')} /></div></section><section className="surface policy-effective"><span className="mono-label">当前选择的最终策略</span><h2>{selected.service?.name ?? '选择服务'} <small>/</small> {selected.environment?.name ?? '选择环境'}</h2>{effectivePolicy ? <KeyValueGrid values={[["生效范围", formatPolicyScope(effectivePolicy, state)], ["确认方式", effectivePolicy.confirm_mode], ["发布冻结", effectivePolicy.manual_freeze_enabled ? '已冻结' : '未冻结']]} /> : <p>请在创建发布或基础设施页面选择服务和环境后查看最终生效策略。</p>}<div className="policy-effective-note">冻结会阻断新发布和待确认发布；已 queued 的任务暂停领取，running 任务继续执行。</div></section></> : null}
                {policyView === 'rules' ? <><PolicySectionHeading eyebrow="RULES" title="策略规则" description="优先使用范围最小的规则。保存后会立即影响后续创建和确认的预检。" /><div className="policy-columns"><section className="surface policy-inventory"><SectionTitle title="已配置规则" meta="INVENTORY" /><PolicyList data={state.policies} state={state} /></section><section className="surface policy-actions"><SectionTitle title="新增或更新规则" meta="EDIT" /><PolicyForm services={state.services} environments={state.environments} onDone={() => void refreshAll()} /></section></div></> : null}
              </div></section>
            </>
          ) : null}

          {page === 'management' ? <>
            <PageHeading eyebrow="ADMINISTRATION" title="管理控制台" description="管理人员、访问凭据和通知集成。高风险配置按职责单独处理。" />
            <section className="surface management-brief"><div><span className="mono-label">管理边界</span><h2>谁可以发布，系统如何连接与通知。</h2><p>用户与访问密钥控制访问；凭据仅供服务器连接引用；通知投递失败不会阻塞发布。</p></div><div className="management-risk-notes"><span><b>01</b> 用户角色决定生产发布确认权限</span><span><b>02</b> 访问密钥明文只在创建时出现</span><span><b>03</b> 凭据与 Webhook 不会在列表中回显</span></div></section>
            <section className="management-layout"><nav className="management-nav" aria-label="管理模块"><ManagementNavButton active={managementView === 'overview'} label="管理概览" note="查看关键状态" count={state.users.length} onClick={() => setManagementView('overview')} /><ManagementNavButton active={managementView === 'users'} label="用户与权限" note="确认发布身份" count={state.users.length} onClick={() => setManagementView('users')} /><ManagementNavButton active={managementView === 'access'} label="集成访问密钥" note="访问密钥与 scopes" count={state.apiKeys.length} onClick={() => setManagementView('access')} /><ManagementNavButton active={managementView === 'notifications'} label="通知与投递" note="机器人与发送记录" count={state.notificationConfigs.length} onClick={() => setManagementView('notifications')} /><ManagementNavButton active={managementView === 'credentials'} label="连接凭据" note="SSH 认证材料" count={state.credentials.length} onClick={() => setManagementView('credentials')} /></nav>
              <div className="management-workspace">
                {managementView === 'overview' ? <><section className="surface management-summary"><SectionTitle title="管理状态" meta="CONTROL PLANE" /><div className="management-stat-grid"><ManagementStat label="可用用户" value={state.users.filter((item) => item.enabled !== false).length} note="可登录并参与发布" onClick={() => setManagementView('users')} /><ManagementStat label="启用访问密钥" value={state.apiKeys.filter((item) => item.enabled !== false).length} note="供 CI/CD 和脚本调用" onClick={() => setManagementView('access')} /><ManagementStat label="启用通知" value={state.notificationConfigs.filter((item) => item.enabled !== false).length} note="企业微信机器人" onClick={() => setManagementView('notifications')} /><ManagementStat label="投递异常" value={state.notificationDeliveries.filter((item) => item.status !== 'sent').length} note="查看最近失败原因" onClick={() => setManagementView('notifications')} /></div></section><section className="surface management-guide"><span className="mono-label">日常管理</span><h2>只在需要时打开对应的管理面板。</h2><div><button onClick={() => setManagementView('users')}>新增发布用户 <span>用户与权限 →</span></button><button onClick={() => setManagementView('access')}>创建 CI/CD 访问密钥 <span>集成访问密钥 →</span></button><button onClick={() => setManagementView('notifications')}>测试通知机器人 <span>通知与投递 →</span></button></div></section></> : null}
                {managementView === 'users' ? <><ManagementSectionHeading eyebrow="IDENTITY" title="用户与权限" description="用户承担发布创建与确认身份。生产环境仍由管理员确认，不在此处绕过发布策略。" /><div className="management-columns"><section className="surface management-inventory"><SectionTitle title="现有用户" meta="INVENTORY" /><UserList data={state.users} onDone={() => void refreshAll()} /></section><section className="surface management-actions"><SectionTitle title="创建用户" meta="CREATE" /><UserForm onDone={(user) => refreshWithSelection({ userID: String(user.id ?? '') })} /></section></div></> : null}
                {managementView === 'access' ? <><ManagementSectionHeading eyebrow="ACCESS" title="集成访问密钥" description="管理员可管理全部访问密钥；普通用户在个人访问密钥页面仅管理自己的密钥。" /><div className="management-columns"><section className="surface management-inventory"><SectionTitle title="现有访问密钥" meta="INVENTORY" /><APIKeyList data={state.apiKeys} onDone={() => void refreshAll()} /></section><section className="surface management-actions"><SectionTitle title="创建集成访问密钥" meta="CREATE" /><APIKeyForm users={state.users} onDone={() => void refreshAll()} /></section></div></> : null}
                {managementView === 'notifications' ? <><ManagementSectionHeading eyebrow="NOTIFICATION" title="通知与投递" description="配置企业微信机器人、发送测试消息，并从投递记录定位失败原因。" /><div className="management-columns"><section className="surface management-inventory"><SectionTitle title="通知配置" meta="INVENTORY" /><NotificationList data={state.notificationConfigs} onTest={() => void refreshAll()} /></section><section className="surface management-actions"><SectionTitle title="新增通知配置" meta="CREATE" /><NotificationForm onDone={() => void refreshAll()} /></section></div><section className="surface management-deliveries"><SectionTitle title="通知投递记录" meta="DELIVERIES" /><EntityList title="最近投递" data={state.notificationDeliveries} fields={['event_type', 'status', 'last_error']} /></section></> : null}
                {managementView === 'credentials' ? <><ManagementSectionHeading eyebrow="CREDENTIAL" title="连接凭据" description="凭据只供服务器 SSH 连接引用；Secret 不会在创建后再次展示。" /><div className="management-columns"><section className="surface management-inventory"><SectionTitle title="已保存凭据" meta="INVENTORY" /><EntityList title="凭据" data={state.credentials} fields={['type', 'enabled', 'description']} /></section><section className="surface management-actions"><SectionTitle title="保存连接凭据" meta="CREATE" /><CredentialForm onDone={() => void refreshAll()} /></section></div></> : null}
              </div>
            </section>
          </> : null}
          {page === 'api-keys' ? <><PageHeading eyebrow="PERSONAL ACCESS" title="个人访问密钥" description="为 CI/CD 或本地脚本创建受 scope 限制的访问凭证。" /><section className="surface access-key-brief"><div><span className="mono-label">使用边界</span><h2>密钥只在创建时显示一次。</h2><p>请立即保存到受保护的 CI/CD 变量中。禁用或删除后，使用它的调用会立刻失效。</p></div><div className="access-key-facts"><span><b>{state.apiKeys.length}</b> 已创建</span><span><b>{state.apiKeys.filter((item) => item.enabled !== false).length}</b> 已启用</span><span>密钥归属当前登录用户</span></div></section><section className="access-key-layout"><div className="access-key-workspace"><section className="surface access-key-inventory"><SectionTitle title="我的访问密钥" meta="INVENTORY" /><APIKeyList data={state.apiKeys} onDone={() => void refreshAll()} /></section><section className="surface access-key-guide"><span className="mono-label">最小权限</span><h2>只授予调用真正需要的 scopes。</h2><p>发布创建、确认、回滚和读取日志分别对应不同 scope；生产发布依然受管理员确认约束。</p></section></div><section className="surface access-key-create"><SectionTitle title="创建访问密钥" meta="CREATE" /><APIKeyForm users={[]} ownKey onDone={() => void refreshAll()} /></section></section></> : null}
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

function ReleaseRows({ data, state, onOpen }: { data: Entity[]; state: AppState; onOpen: (item: Entity) => void }) {
  if (data.length === 0) return <div className="empty-state">没有匹配的发布单。</div>;
  return <div className="release-table">{data.map((item) => <button className="release-row" key={String(item.id)} onClick={() => onOpen(item)}><span className="release-id">{shortID(item.id)}</span><span><strong>{formatReleaseContext(item, state)}</strong><small>{`申请人：${formatActor(item.created_by_type, item.created_by_id, state)} · 来源：${item.source ?? '-'}`}</small></span><StatusTag value={releaseStatusValue(item)} /><span className="next-action">{releaseActionLabel(item)}</span><span aria-hidden="true">→</span></button>)}</div>;
}

function DeployRows({ data, state, releases, onOpen }: { data: Entity[]; state: AppState; releases: Map<string, Entity>; onOpen: (item: Entity) => void }) {
  if (data.length === 0) return <div className="inline-empty">暂无发布记录。</div>;
  return <div className="deploy-list">{data.map((item) => <button className="deploy-row" key={String(item.id)} onClick={() => onOpen(item)}><span><strong>{shortID(item.id)}</strong><small>{formatDeployContext(item, releases, state)}</small></span><span className="server-counts">成功 {item.success_servers ?? 0} / 失败 {item.failed_servers ?? 0} / 跳过 {item.skipped_servers ?? 0}</span><StatusTag value={String(item.status)} /><span aria-hidden="true">→</span></button>)}</div>;
}

function EventRows({ data, state }: { data: Entity[]; state: AppState }) {
  if (data.length === 0) return <div className="inline-empty">暂无事件。</div>;
  return <div className="event-list">{data.map((item) => <div className="event-row" key={String(item.id)}><span className="event-dot" /><div><strong>{item.event_type}</strong><p>{item.message ?? '—'}</p><small>{formatEventContext(item, state)}</small></div></div>)}</div>;
}

function ServerLogRows({ data, state }: { data: Entity[]; state: AppState }) {
  if (data.length === 0) return <div className="inline-empty">选择一条发布记录查看服务器日志。</div>;
  return <div className="server-log-list">{data.map((item) => <article className="server-log-row" key={String(item.id)}><div><strong>{formatServerRef(item.server_id, state)}</strong><StatusTag value={String(item.status)} /></div><small>{`开始：${item.started_at ?? '—'} · 结束：${item.finished_at ?? '—'} · 耗时：${item.duration_ms ?? 0}ms`}</small>{item.error_code ? <code>{item.error_code}</code> : null}<pre>{String(item.error_message ?? item.log_output ?? '暂无输出')}</pre></article>)}</div>;
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

function PolicyNavButton({ active, label, note, count, onClick }: { active: boolean; label: string; note: string; count: number; onClick: () => void }) {
  return <button className={active ? 'policy-nav-button active' : 'policy-nav-button'} onClick={onClick}><span><strong>{label}</strong><small>{note}</small></span><b>{count}</b></button>;
}

function PolicyStat({ label, value, note, onClick }: { label: string; value: number; note: string; onClick: () => void }) {
  return <button className="policy-stat" onClick={onClick}><span>{label}</span><strong>{value}</strong><small>{note} →</small></button>;
}

function PolicySectionHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return <div className="policy-section-heading"><span className="mono-label">{eyebrow}</span><h2>{title}</h2><p>{description}</p></div>;
}

function formatPolicyScope(item: Entity, state: AppState) {
  const scopeID = String(item.scope_id ?? '');
  if (item.scope_type === 'environment') return `环境 · ${namedRef(findByID(state.environments, scopeID), scopeID, 'name')}`;
  if (item.scope_type === 'service') return `服务 · ${namedRef(findByID(state.services, scopeID), scopeID, 'name')}`;
  return '系统默认';
}

function PolicyList({ data, state }: { data: Entity[]; state: AppState }) {
  return <div className="policy-list"><DataList data={data} renderItem={(item) => <div className="policy-row"><div><strong>{formatPolicyScope(item, state)}</strong><small>{`确认方式 · ${item.confirm_mode ?? 'self_confirm'}`}</small></div><div><StatusTag value={item.manual_freeze_enabled === true ? 'frozen' : 'active'} /></div></div>} /></div>;
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

function ServerGroupList({ data, state }: { data: Entity[]; state: AppState }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>服务器组</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => (
          <div className="data-row compact">
            <div className="data-main">
              <Typography.Text strong>{namedRef(item, item.id, 'name')}</Typography.Text>
              <Typography.Text type="secondary">{formatServerMembers(item.server_ids, state)}</Typography.Text>
              {item.description ? <Typography.Text type="secondary">{item.description}</Typography.Text> : null}
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
              <Typography.Text type="secondary">{`部署 ${shortID(item.deploy_record_id)} / ${item.updated_at ?? '-'}`}</Typography.Text>
            </div>
          </div>
        )}
      />
    </div>
  );
}

function DeploymentTargetList({ data, state }: { data: Entity[]; state: AppState }) {
  return (
    <div className="mini-list">
      <Typography.Title level={4}>部署目标</Typography.Title>
      <DataList
        data={data}
        renderItem={(item) => (
          <div className="data-row compact">
            <div className="data-main">
              <Typography.Text strong>{formatTarget(item, targetRefFor(item, state))}</Typography.Text>
              <Typography.Text type="secondary">
                {[
                  namedRef(findByID(state.services, item.service_id), item.service_id, 'name'),
                  namedRef(findByID(state.environments, item.environment_id), item.environment_id, 'name'),
                ].join(' / ')}
              </Typography.Text>
              <Typography.Text type="secondary">{`超时 ${item.timeout_seconds ?? '-'} 秒 / ${item.enabled === false ? 'disabled' : 'enabled'}`}</Typography.Text>
            </div>
          </div>
        )}
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
  return <div className="service-detail-grid"><div><span className="mono-label">服务</span><h3>{service.name}</h3><p>{service.description || '暂无描述'}</p><KeyValueGrid values={[["可用环境", availableEnvironments.length], ["部署目标", serviceTargets.length], ["历史版本", versions.length], ["服务器当前版本", serviceStates.length]]} /></div><div><span className="mono-label">可用环境</span><div className="detail-chip-list">{availableEnvironments.length ? availableEnvironments.map((item) => <span className={item.is_production ? 'detail-chip production' : 'detail-chip'} key={String(item.id)}>{item.name}{item.is_production ? ' · 生产' : ''}</span>) : <span className="detail-muted">尚未配置部署目标</span>}</div><span className="mono-label">部署目标</span><div className="detail-list">{serviceTargets.map((item) => <div key={String(item.id)}><strong>{item.executor_type} / {item.target_type}</strong><small>{`${environments.find((environment) => String(environment.id) === String(item.environment_id))?.name ?? item.environment_id} · ${item.enabled === false ? '已停用' : '已启用'}`}</small></div>)}</div></div><div><span className="mono-label">最近版本</span><div className="detail-list">{versions.length ? versions.slice(0, 6).map((item) => <div key={String(item.id)}><strong>{item.version}</strong><small>{`${item.commit_sha || '无 commit'} · ${maskArtifactURL(item.artifact_url)}`}</small></div>) : <span className="detail-muted">暂无版本</span>}</div></div></div>;
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

function EnvironmentEditor({ environments, onDone }: { environments: Entity[]; onDone: () => void }) {
  const [form] = Form.useForm();
  const [selectedID, setSelectedID] = useState('');
  const [loading, setLoading] = useState(false);
  const selected = findByID(environments, selectedID);
  function choose(id: string) { const item = findByID(environments, id); setSelectedID(id); form.setFieldsValue(item); }
  async function submit(values: Entity) { if (!selectedID) return; setLoading(true); try { await apiPatch<Entity>(`/api/v1/environments/${selectedID}`, values); onDone(); } finally { setLoading(false); } }
  return <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}><Form.Item label="选择环境"><Select value={selectedID || undefined} placeholder="选择环境" options={environments.map(entityOption)} onChange={choose} /></Form.Item>{selected ? <><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="slug" label="Slug" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="is_production" valuePropName="checked"><Checkbox>生产环境</Checkbox></Form.Item><Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item><Button type="primary" htmlType="submit" loading={loading}>保存环境</Button></> : <div className="form-empty">选择一项开始编辑。</div>}</Form>;
}

function ServerEditor({ servers, credentials, onDone }: { servers: Entity[]; credentials: Entity[]; onDone: () => void }) {
  const [form] = Form.useForm();
  const [selectedID, setSelectedID] = useState('');
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState('');
  const selected = findByID(servers, selectedID);
  function choose(id: string) { const item = findByID(servers, id); setSelectedID(id); form.setFieldsValue(item); setTestResult(''); }
  async function submit(values: Entity) { if (!selectedID) return; setLoading(true); try { await apiPatch<Entity>(`/api/v1/servers/${selectedID}`, values); onDone(); } finally { setLoading(false); } }
  async function test() { if (!selectedID) return; setTesting(true); setTestResult(''); try { const body = await apiPost<{ server: Entity; result: Entity }>(`/api/v1/servers/${selectedID}/test`, {}); setTestResult(`${body.result.status}: ${body.result.error_message ?? body.result.log_output ?? '连接成功'}`); onDone(); } catch (err) { setTestResult(err instanceof Error ? err.message : '连接测试失败'); } finally { setTesting(false); } }
  return <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}><Form.Item label="选择服务器"><Select value={selectedID || undefined} placeholder="选择服务器" options={servers.map(entityOption)} onChange={choose} /></Form.Item>{selected ? <><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="host" label="Host" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="port" label="Port"><Input type="number" min={1} /></Form.Item><Form.Item name="username" label="Username" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="auth_type" label="认证方式"><Select options={[{ label: 'private_key', value: 'private_key' }, { label: 'password', value: 'password' }, { label: 'none', value: 'none' }]} /></Form.Item><Form.Item name="credential_ref" label="凭据"><Select allowClear options={credentials.map(entityOption)} /></Form.Item><Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item><Space wrap><Button type="primary" htmlType="submit" loading={loading}>保存服务器</Button><Button loading={testing} onClick={() => void test()}>测试 SSH</Button></Space>{testResult ? <div className="test-result">{testResult}</div> : <Typography.Text type="secondary">最近测试：{selected.last_check_status ?? '未测试'} / {selected.last_check_at ?? '-'}</Typography.Text>}</> : <div className="form-empty">选择一台服务器开始编辑或测试。</div>}</Form>;
}

function ServerGroupEditor({ groups, servers, onDone }: { groups: Entity[]; servers: Entity[]; onDone: () => void }) {
  const [form] = Form.useForm();
  const [selectedID, setSelectedID] = useState('');
  const [loading, setLoading] = useState(false);
  const selected = findByID(groups, selectedID);
  function choose(id: string) { const item = findByID(groups, id); setSelectedID(id); form.setFieldsValue(item); }
  async function submit(values: Entity) { if (!selectedID) return; setLoading(true); try { await apiPatch<Entity>(`/api/v1/server-groups/${selectedID}`, values); onDone(); } finally { setLoading(false); } }
  return <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}><Form.Item label="选择服务器组"><Select value={selectedID || undefined} placeholder="选择服务器组" options={groups.map(entityOption)} onChange={choose} /></Form.Item>{selected ? <><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item><Form.Item name="description" label="描述"><Input /></Form.Item><Form.Item name="server_ids" label="成员服务器" rules={[{ required: true }]}><Select mode="multiple" options={servers.map(entityOption)} /></Form.Item><Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item><Button type="primary" htmlType="submit" loading={loading}>保存服务器组</Button></> : <div className="form-empty">选择一项开始编辑。</div>}</Form>;
}

function DeploymentTargetEditor({ targets, servers, serverGroups, onDone }: { targets: Entity[]; servers: Entity[]; serverGroups: Entity[]; onDone: () => void }) {
  const [form] = Form.useForm();
  const [selectedID, setSelectedID] = useState('');
  const [loading, setLoading] = useState(false);
  const selected = findByID(targets, selectedID);
  const targetType = Form.useWatch('target_type', form) as string | undefined;
  function choose(id: string) { const item = findByID(targets, id); setSelectedID(id); form.setFieldsValue(item); }
  async function submit(values: Entity) { if (!selectedID) return; setLoading(true); try { await apiPatch<Entity>(`/api/v1/deployment-targets/${selectedID}`, values); onDone(); } finally { setLoading(false); } }
  const targetOptions = targetType === 'server_group' ? serverGroups : servers;
  return <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}><Form.Item label="选择部署目标"><Select value={selectedID || undefined} placeholder="选择部署目标" options={targets.map((item) => ({ label: String(item.id), value: String(item.id) }))} onChange={choose} /></Form.Item>{selected ? <><Form.Item name="target_type" label="目标类型"><Select options={[{ label: '服务器', value: 'server' }, { label: '服务器组', value: 'server_group' }]} /></Form.Item><Form.Item name="target_ref_id" label="运行目标"><Select options={targetOptions.map(entityOption)} /></Form.Item><Form.Item name="executor_type" label="执行器"><Select options={[{ label: 'mock', value: 'mock' }, { label: 'ssh', value: 'ssh' }]} /></Form.Item><Form.Item name="script_path" label="Script Path"><Input /></Form.Item><Form.Item name="working_dir" label="Working Dir"><Input /></Form.Item><Form.Item name="env_vars" label="环境变量 JSON"><Input.TextArea rows={3} /></Form.Item><Form.Item name="timeout_seconds" label="超时秒数"><Input type="number" min={1} /></Form.Item><Form.Item name="enabled" valuePropName="checked"><Checkbox>启用</Checkbox></Form.Item><Button type="primary" htmlType="submit" loading={loading}>保存部署目标</Button></> : <div className="form-empty">选择一项开始编辑。</div>}</Form>;
}

function ProjectForm({ onDone }: { onDone: () => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      await apiPost<Entity>('/api/v1/projects', values);
      form.resetFields();
      onDone();
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

function ServiceForm({ projects, onDone }: { projects: Entity[]; onDone: (service: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const service = await apiPost<Entity>('/api/v1/services', values);
      form.resetFields();
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

function VersionForm({ services, onDone }: { services: Entity[]; onDone: (version: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
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

function ServerForm({ credentials, onDone }: { credentials: Entity[]; onDone: (server: Entity) => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const authType = Form.useWatch('auth_type', form);
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
  return (
    <Form form={form} layout="vertical" initialValues={{ port: 22, auth_type: 'none' }} onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label="名称" rules={[{ required: true }]}>
        <Input />
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
        <Select allowClear options={credentials.map(entityOption)} disabled={authType === 'none'} />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        创建服务器
      </Button>
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
      initialValues={{ executor_type: 'mock', target_type: 'server', env_vars: '{}', timeout_seconds: 60 }}
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
            { label: 'employee', value: 'employee' },
            { label: 'admin', value: 'admin' },
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
                <Typography.Text type="secondary">{`${item.username ?? '-'} / ${item.role ?? '-'}`}</Typography.Text>
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

function PolicyForm({ services, environments, onDone }: { services: Entity[]; environments: Entity[]; onDone: () => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const scopeType = Form.useWatch('scope_type', form) as string | undefined;
  async function submit(values: Entity) {
    setLoading(true);
    try {
      await apiPost<Entity>('/api/v1/release-policies', {
        ...values,
        scope_id: values.scope_type === 'system' ? '' : values.scope_id,
      });
      onDone();
    } finally {
      setLoading(false);
    }
  }
  return (
    <Form form={form} layout="vertical" initialValues={{ scope_type: 'system', confirm_mode: 'self_confirm', manual_freeze_enabled: false }} onFinish={(values) => void submit(values)}>
      <Form.Item name="scope_type" label="范围" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'system', value: 'system' },
            { label: 'environment', value: 'environment' },
            { label: 'service', value: 'service' },
          ]}
        />
      </Form.Item>
      {scopeType === 'environment' ? (
        <Form.Item name="scope_id" label="环境" rules={[{ required: true }]}>
          <Select options={environments.map((item) => ({ label: String(item.name ?? item.id), value: item.id }))} />
        </Form.Item>
      ) : null}
      {scopeType === 'service' ? (
        <Form.Item name="scope_id" label="服务" rules={[{ required: true }]}>
          <Select options={services.map((item) => ({ label: String(item.name ?? item.id), value: item.id }))} />
        </Form.Item>
      ) : null}
      <Form.Item name="confirm_mode" label="确认方式" rules={[{ required: true }]}>
        <Select
          options={[
            { label: 'self_confirm', value: 'self_confirm' },
            { label: 'admin_confirm', value: 'admin_confirm' },
          ]}
        />
      </Form.Item>
      <Form.Item name="manual_freeze_enabled" valuePropName="checked">
        <Checkbox>冻结发布</Checkbox>
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        保存策略
      </Button>
    </Form>
  );
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
      <Form.Item name="secret" label="Secret">
        <Input.Password />
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
                <Button loading={testingID === item.id} disabled={!enabled} onClick={() => void testConfig(item.id as string)}>
                  测试
                </Button>
                <Button loading={busyID === item.id} onClick={() => void setEnabled(item, !enabled)}>
                  {enabled ? '禁用' : '启用'}
                </Button>
              </Space>
            </div>
          );
        }}
      />
    </div>
  );
}

function CredentialForm({ onDone }: { onDone: () => void }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  async function submit(values: Entity) {
    setLoading(true);
    try {
      await apiPost<Entity>('/api/v1/credentials', values);
      form.resetFields();
      onDone();
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

function APIKeyForm({ users, onDone, ownKey = false }: { users: Entity[]; onDone: () => void; ownKey?: boolean }) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [plaintext, setPlaintext] = useState('');
  async function submit(values: Entity) {
    setLoading(true);
    try {
      const body = await apiPost<APIKeyCreateResponse>('/api/v1/api-keys', {
        ...values,
        owner_type: 'user',
        scopes: values.scopes || '["release:create"]',
      });
      setPlaintext(body.plaintext);
      form.resetFields();
      onDone();
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
        initialValues={{ owner_type: 'user', scopes: '["release:create","release:confirm"]' }}
        onFinish={(values) => void submit(values)}
      >
        <Form.Item name="name" label="名称" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        {ownKey ? <Typography.Text type="secondary">该访问密钥将归属当前登录用户。</Typography.Text> : <Form.Item name="owner_id" label="归属用户" rules={[{ required: true }]}><Select options={users.map(entityOption)} /></Form.Item>}
        <Form.Item name="scopes" label="Scopes JSON" rules={[{ required: true }]}>
          <Input.TextArea rows={3} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={loading}>
          创建访问密钥
        </Button>
      </Form>
    </Space>
  );
}

function APIKeyList({ data, onDone }: { data: Entity[]; onDone: () => void }) {
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
  return (
    <div className="mini-list">
      <Typography.Title level={4}>访问密钥</Typography.Title>
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
                <Typography.Text type="secondary">{`${item.prefix ?? '-'} / ${item.owner_type ?? '-'} / ${item.owner_id ?? '-'}`}</Typography.Text>
                <Typography.Text type="secondary">{`${item.scopes ?? '[]'}`}</Typography.Text>
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

function releaseMatchesSelection(item: Entity | undefined, serviceID?: string | number | boolean | null, environmentID?: string | number | boolean | null) {
  if (!item) {
    return false;
  }
  const serviceMatched = !serviceID || String(item.service_id) === String(serviceID);
  const environmentMatched = !environmentID || String(item.environment_id) === String(environmentID);
  return serviceMatched && environmentMatched;
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

function formatDeployContext(item: Entity, releases: Map<string, Entity>, state: AppState) {
  const release = releases.get(String(item.release_request_id ?? ''));
  if (!release) {
    return `发布单 ${shortID(item.release_request_id)}`;
  }
  return `${shortID(release.id)} · ${formatReleaseContext(release, state)}`;
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
    parts.push(String(item.created_at));
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

function formatServerMembers(value: Entity[string], state: AppState) {
  const ids = Array.isArray(value) ? value : value ? [value] : [];
  if (ids.length === 0) {
    return '无服务器';
  }
  return ids.map((id) => formatServerRef(id, state)).join(', ');
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
