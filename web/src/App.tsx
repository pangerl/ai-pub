import {
  Alert,
  Badge,
  Button,
  Checkbox,
  ConfigProvider,
  Descriptions,
  Divider,
  Form,
  Input,
  Layout,
  Popconfirm,
  Select,
  Space,
  Tabs,
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
  const activeReleaseID = activeRelease?.id as string | undefined;

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
      user: findByID(state.users, selection.userID) ?? state.users[0],
      release: activeRelease,
    };
  }, [activeRelease, selection, state]);

  const releaseByID = useMemo(() => {
    const byID = new Map<string, Entity>();
    for (const release of state.releases) {
      if (release.id) {
        byID.set(String(release.id), release);
      }
    }
    return byID;
  }, [state.releases]);

  const filteredReleases = useMemo(
    () =>
      state.releases.filter((item) => {
        const statusMatched = filters.releaseStatus === 'all' || item.status === filters.releaseStatus;
        const scopeMatched = !filters.scoped || releaseMatchesSelection(item, selected.service?.id, selected.environment?.id);
        return statusMatched && scopeMatched;
      }),
    [filters.releaseStatus, filters.scoped, selected.environment?.id, selected.service?.id, state.releases],
  );

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
        apiGet<Entity[]>('/api/v1/users'),
        apiGet<Entity[]>('/api/v1/api-keys'),
        apiGet<Entity[]>('/api/v1/credentials'),
        apiGet<Entity[]>('/api/v1/release-requests'),
        apiGet<Entity[]>('/api/v1/deploy-records'),
        apiGet<Entity[]>('/api/v1/server-deployment-states'),
        apiGet<Entity[]>('/api/v1/release-policies'),
        apiGet<Entity[]>('/api/v1/notification-configs'),
        apiGet<Entity[]>('/api/v1/notification-deliveries'),
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
  }, [activeDeployID, activeReleaseID, selection.serviceID]);

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
    void refreshAll();
  }, [refreshAll]);

  useEffect(() => {
    setSelection((current) => {
      const serviceID = keepOrFirst(current.serviceID, state.services);
      const environmentID = keepOrFirst(current.environmentID, state.environments);
      const versionID = keepOrFirst(current.versionID, state.versions);
      const targets = filterTargets(state.targets, serviceID, environmentID);
      const targetID = keepOrFirst(current.targetID, targets.length > 0 ? targets : state.targets);
      const userID = keepOrFirst(current.userID, state.users);
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
  }, [state.environments, state.services, state.targets, state.users, state.versions]);

  useEffect(() => {
    setPreflight(null);
  }, [selection.environmentID, selection.serviceID, selection.targetID, selection.versionID]);

  async function createDemo() {
    setLoading(true);
    try {
      const suffix = Date.now().toString(36);
      const project = await apiPost<Entity>('/api/v1/projects', {
        name: '供应链系统',
        slug: `supply-chain-${suffix}`,
      });
      const service = await apiPost<Entity>('/api/v1/services', {
        project_id: project.id,
        name: '订单服务',
        slug: `order-api-${suffix}`,
      });
      const version = await apiPost<Entity>(`/api/v1/services/${service.id}/versions`, {
        version: `v${new Date().toISOString().replaceAll(/[-:.TZ]/g, '').slice(0, 12)}`,
        source: 'manual',
      });
      const environment = await apiPost<Entity>('/api/v1/environments', {
        name: '测试环境',
        slug: `test-${suffix}`,
        is_production: false,
      });
      const server = await apiPost<Entity>('/api/v1/servers', {
        name: `mock-${suffix}`,
        host: '127.0.0.1',
        username: 'deploy',
        auth_type: 'none',
      });
      const target = await apiPost<Entity>('/api/v1/deployment-targets', {
        service_id: service.id,
        environment_id: environment.id,
        executor_type: 'mock',
        target_type: 'server',
        target_ref_id: server.id,
        timeout_seconds: 60,
        env_vars: '{}',
      });
      const user = await apiPost<Entity>('/api/v1/users', {
        username: `alice-${suffix}`,
        display_name: 'Alice',
        role: 'employee',
      });
      const nextSelection = {
        serviceID: String(service.id),
        environmentID: String(environment.id),
        versionID: String(version.id),
        targetID: String(target.id),
        userID: String(user.id),
      };
      setActiveRelease(null);
      setActiveDeployID('');
      setSelection(nextSelection);
      await refreshAll(null, nextSelection);
      api.success(`Demo 配置已创建：${version.version}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Demo 创建失败');
    } finally {
      setLoading(false);
    }
  }

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

  async function createFailingRelease() {
    if (!selected.service || !selected.environment || !selected.version || !selected.server || !selected.user) {
      setError('请先创建 Demo 配置');
      return;
    }
    setLoading(true);
    try {
      const target = await apiPost<Entity>('/api/v1/deployment-targets', {
        service_id: selected.service.id,
        environment_id: selected.environment.id,
        executor_type: 'mock',
        target_type: 'server',
        target_ref_id: selected.server.id,
        timeout_seconds: 60,
        env_vars: '{"MOCK_FAIL":"1"}',
      });
      const body = await apiPost<ReleaseResponse>('/api/v1/release-requests', {
        service_id: selected.service.id,
        environment_id: selected.environment.id,
        service_version_id: selected.version.id,
        deployment_target_id: target.id,
        created_by_type: 'user',
        created_by_id: selected.user.id,
        idempotency_key: `web-fail-${Date.now()}`,
      });
      const nextSelection = selectionFromRelease(body.release, selection);
      setActiveRelease(body.release);
      setSelection(nextSelection);
      await refreshAll(body.release.id as string, nextSelection);
      api.success('失败模拟发布单已创建');
    } catch (err) {
      setError(err instanceof Error ? err.message : '失败模拟创建失败');
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
  const canRollbackRelease = activeReleaseStatus === 'success' || activeReleaseStatus === 'partial';

  return (
    <ConfigProvider theme={{ token: { borderRadius: 6, colorPrimary: '#246bfe' } }}>
      {contextHolder}
      <Layout className="shell">
        <Layout.Header className="topbar">
          <Typography.Title level={4}>ai-pub</Typography.Title>
          <Space>
            <Badge status={status === 'ok' ? 'success' : 'warning'} text={status} />
            <Button onClick={() => void refreshAll()}>刷新</Button>
          </Space>
        </Layout.Header>
        <Layout.Content className="content">
          {error ? <Alert className="notice" type="warning" message={error} showIcon closable onClose={() => setError('')} /> : null}
          <Tabs
            items={[
              {
                key: 'workbench',
                label: '工作台',
                children: (
                  <section className="panel">
                    <Space orientation="vertical" size={18}>
                      <Typography.Title level={2}>发布工作台</Typography.Title>
                      <Space wrap>
                        <Button type="primary" loading={loading} onClick={() => void createDemo()}>
                          初始化 Mock 配置
                        </Button>
                        <Button loading={loading} onClick={() => void runPreflight()}>
                          执行预检
                        </Button>
                        <Button loading={loading} onClick={() => void createRelease()}>
                          创建发布单
                        </Button>
                        <Button loading={loading} disabled={!canConfirmRelease} onClick={() => void confirmRelease()}>
                          确认入队
                        </Button>
                        <Button danger loading={loading} disabled={!canRejectRelease} onClick={() => void rejectRelease()}>
                          驳回
                        </Button>
                        <Button loading={loading} disabled={!canCancelRelease} onClick={() => void cancelRelease()}>
                          取消
                        </Button>
                        <Button loading={loading} disabled={!canRollbackRelease} onClick={() => void rollbackRelease()}>
                          创建回滚单
                        </Button>
                        <Button danger loading={loading} onClick={() => void createFailingRelease()}>
                          模拟失败发布
                        </Button>
                      </Space>
                      <div className="selector-grid">
                        <LabeledSelect
                          label="服务"
                          value={selected.service?.id}
                          options={state.services}
                          nameField="name"
                          onChange={(value) => changeSelection({ serviceID: value, versionID: '', targetID: '' })}
                        />
                        <LabeledSelect
                          label="环境"
                          value={selected.environment?.id}
                          options={state.environments}
                          nameField="name"
                          onChange={(value) => changeSelection({ environmentID: value, targetID: '' })}
                        />
                        <LabeledSelect
                          label="版本"
                          value={selected.version?.id}
                          options={state.versions}
                          nameField="version"
                          onChange={(value) => changeSelection({ versionID: value })}
                        />
                        <LabeledSelect
                          label="部署目标"
                          value={selected.target?.id}
                          options={selected.targetOptions}
                          nameField="executor_type"
                          onChange={(value) => changeSelection({ targetID: value })}
                        />
                        <LabeledSelect
                          label="确认用户"
                          value={selected.user?.id}
                          options={state.users}
                          nameField="display_name"
                          onChange={(value) => setSelection((current) => ({ ...current, userID: value }))}
                        />
                      </div>
                      <PreflightPanel result={preflight} />
                      <Descriptions bordered size="small" column={3}>
                        <Descriptions.Item label="排队">{state.ops?.queued_deploys ?? 0}</Descriptions.Item>
                        <Descriptions.Item label="运行中">{state.ops?.running_deploys ?? 0}</Descriptions.Item>
                        <Descriptions.Item label="通知失败">{state.ops?.failed_notifications ?? 0}</Descriptions.Item>
                      </Descriptions>
                      <Descriptions bordered size="small" column={2}>
                        <Descriptions.Item label="项目">{selected.project?.name ?? '-'}</Descriptions.Item>
                        <Descriptions.Item label="服务">{selected.service?.name ?? '-'}</Descriptions.Item>
                        <Descriptions.Item label="版本">{selected.version?.version ?? '-'}</Descriptions.Item>
                        <Descriptions.Item label="环境">{selected.environment?.name ?? '-'}</Descriptions.Item>
                        <Descriptions.Item label="部署目标">{formatTarget(selected.target, selected.targetRef)}</Descriptions.Item>
                        <Descriptions.Item label="发布单">
                          {selected.release?.id ? (
                            <Space>
                              <span>{selected.release.id}</span>
                              <StatusTag value={selected.release.status as string} />
                            </Space>
                          ) : (
                            '-'
                          )}
                        </Descriptions.Item>
                      </Descriptions>
                    </Space>
                  </section>
                ),
              },
              {
                key: 'inventory',
                label: '基础配置',
                children: (
                  <section className="panel grid-panel">
                    <EntityList title="项目" data={state.projects} fields={['name', 'slug']} />
                    <EntityList title="服务" data={state.services} fields={['name', 'slug']} />
                    <EntityList title="版本" data={state.versions} fields={['version', 'source']} />
                    <EntityList title="环境" data={state.environments} fields={['name', 'slug']} />
                    <EntityList title="服务器" data={state.servers} fields={['name', 'host']} />
                    <ServerGroupList data={state.serverGroups} state={state} />
                    <DeploymentTargetList data={state.targets} state={state} />
                    <DeploymentStateList data={state.states} state={state} />
                  </section>
                ),
              },
              {
                key: 'releases',
                label: '发布中心',
                children: (
                  <section className="panel">
                    <ListToolbar
                      scoped={filters.scoped}
                      status={filters.releaseStatus}
                      statusOptions={releaseStatusOptions}
                      onScopedChange={(scoped) => changeReleaseFilters({ scoped })}
                      onStatusChange={(releaseStatus) => changeReleaseFilters({ releaseStatus })}
                    />
                    <DataList
                      data={filteredReleases}
                      renderItem={(item) => (
                        <div className={rowClass(item, selected.release?.id)}>
                          <div className="data-main">
                            <Space>
                              <Typography.Text strong>{item.id}</Typography.Text>
                              <StatusTag value={item.status as string} />
                            </Space>
                            <Typography.Text type="secondary">
                              {formatReleaseContext(item, state)}
                            </Typography.Text>
                            {item.summary_message ? (
                              <Typography.Text type={isBadStatus(item.status) ? 'danger' : 'secondary'}>
                                {item.summary_message}
                              </Typography.Text>
                            ) : null}
                          </div>
                          <Button onClick={() => void selectRelease(item)}>查看</Button>
                        </div>
                      )}
                    />
                    <Divider />
                    <Typography.Title level={4}>事件流 {selected.release?.id ? <Typography.Text type="secondary">{selected.release.id}</Typography.Text> : null}</Typography.Title>
                    <DataList
                      data={state.events}
                      renderItem={(item) => (
                        <div className="data-row">
                          <div className="data-main">
                            <Typography.Text strong>{item.event_type}</Typography.Text>
                            {item.message ? <Typography.Text>{item.message}</Typography.Text> : null}
                            <Typography.Text type="secondary">{formatEventContext(item, state)}</Typography.Text>
                          </div>
                        </div>
                      )}
                    />
                  </section>
                ),
              },
              {
                key: 'deploys',
                label: '发布记录',
                children: (
                  <section className="panel">
                    <ListToolbar
                      scoped={filters.scoped}
                      status={filters.deployStatus}
                      statusOptions={deployStatusOptions}
                      onScopedChange={(scoped) => changeDeployFilters({ scoped })}
                      onStatusChange={(deployStatus) => changeDeployFilters({ deployStatus })}
                    />
                    <DataList
                      data={filteredDeploys}
                      renderItem={(item) => (
                        <div className={rowClass(item, activeDeployID)}>
                          <div className="data-main">
                            <Space>
                              <Typography.Text strong>{item.id}</Typography.Text>
                              <StatusTag value={item.status as string} />
                            </Space>
                            <Typography.Text type="secondary">
                              {formatDeployContext(item, releaseByID, state)}
                            </Typography.Text>
                            <Typography.Text type="secondary">
                              {`total=${item.total_servers ?? 0} success=${item.success_servers ?? 0} failed=${item.failed_servers ?? 0} skipped=${item.skipped_servers ?? 0}`}
                            </Typography.Text>
                            {isBadStatus(item.status) ? (
                              <Typography.Text type="danger">
                                {`失败 ${item.failed_servers ?? 0}，跳过 ${item.skipped_servers ?? 0}`}
                              </Typography.Text>
                            ) : null}
                          </div>
                          <Button onClick={() => void selectDeploy(item.id as string)}>查看日志</Button>
                        </div>
                      )}
                    />
                    <Divider />
                    <Typography.Title level={4}>服务器日志 {activeDeployID ? <Typography.Text type="secondary">{activeDeployID}</Typography.Text> : null}</Typography.Title>
                    <DataList
                      data={state.serverLogs}
                      renderItem={(item) => (
                        <div className={rowClass(item)}>
                          <div className="data-main">
                            <Space>
                              <Typography.Text strong>{formatServerRef(item.server_id, state)}</Typography.Text>
                              <StatusTag value={item.status as string} />
                            </Space>
                            {item.error_code ? <Typography.Text type="danger">{item.error_code}</Typography.Text> : null}
                            <Typography.Text type={isBadStatus(item.status) ? 'danger' : 'secondary'}>
                              {`${item.error_message ?? item.log_output ?? ''}`}
                            </Typography.Text>
                          </div>
                        </div>
                      )}
                    />
                  </section>
                ),
              },
              {
                key: 'manual',
                label: '手动创建',
                children: (
                  <section className="panel">
                    <Typography.Title level={4}>基础配置</Typography.Title>
                    <div className="form-grid">
                      <div>
                        <Typography.Title level={5}>项目</Typography.Title>
                        <ProjectForm onDone={() => void refreshAll()} />
                      </div>
                      <div>
                        <Typography.Title level={5}>服务</Typography.Title>
                        <ServiceForm
                          projects={state.projects}
                          onDone={(service) =>
                            refreshWithSelection({ serviceID: String(service.id ?? ''), versionID: '', targetID: '' })
                          }
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>版本</Typography.Title>
                        <VersionForm
                          services={state.services}
                          onDone={(version) =>
                            refreshWithSelection({
                              serviceID: String(version.service_id ?? ''),
                              versionID: String(version.id ?? ''),
                            })
                          }
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>环境</Typography.Title>
                        <EnvironmentForm
                          onDone={(environment) =>
                            refreshWithSelection({ environmentID: String(environment.id ?? ''), targetID: '' })
                          }
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>服务器</Typography.Title>
                        <ServerForm
                          credentials={state.credentials}
                          onDone={(server) => {
                            setManualTargetRef({ targetType: 'server', targetRefID: String(server.id ?? '') });
                            void refreshAll();
                          }}
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>服务器组</Typography.Title>
                        <ServerGroupForm
                          servers={state.servers}
                          onDone={(serverGroup) => {
                            setManualTargetRef({ targetType: 'server_group', targetRefID: String(serverGroup.id ?? '') });
                            void refreshAll();
                          }}
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>发布目标</Typography.Title>
                        <DeploymentTargetForm
                          services={state.services}
                          environments={state.environments}
                          servers={state.servers}
                          serverGroups={state.serverGroups}
                          selectedServiceID={selection.serviceID}
                          selectedEnvironmentID={selection.environmentID}
                          preferredTargetRef={manualTargetRef}
                          onDone={(target) =>
                            refreshWithSelection({
                              serviceID: String(target.service_id ?? ''),
                              environmentID: String(target.environment_id ?? ''),
                              targetID: String(target.id ?? ''),
                            })
                          }
                        />
                      </div>
                      <div>
                        <Typography.Title level={5}>确认用户</Typography.Title>
                        <UserForm onDone={(user) => refreshWithSelection({ userID: String(user.id ?? '') })} />
                      </div>
                    </div>
                    <Divider />
                    <Typography.Title level={4}>策略与集成</Typography.Title>
                    <div className="form-grid">
                      <div>
                        <Typography.Title level={4}>发布策略</Typography.Title>
                        <PolicyForm
                          services={state.services}
                          environments={state.environments}
                          onDone={() => void refreshAll()}
                        />
                      </div>
                      <div>
                        <Typography.Title level={4}>通知配置</Typography.Title>
                        <NotificationForm onDone={() => void refreshAll()} />
                      </div>
                      <div>
                        <Typography.Title level={4}>创建凭据</Typography.Title>
                        <CredentialForm onDone={() => void refreshAll()} />
                      </div>
                      <div>
                        <Typography.Title level={4}>API Key</Typography.Title>
                        <APIKeyForm users={state.users} onDone={() => void refreshAll()} />
                      </div>
                    </div>
                    <Divider />
                    <div className="grid-panel">
                      <UserList data={state.users} onDone={() => void refreshAll()} />
                      <EntityList title="发布策略" data={state.policies} fields={['scope_type', 'scope_id', 'confirm_mode', 'manual_freeze_enabled']} />
                      <NotificationList data={state.notificationConfigs} onTest={() => void refreshAll()} />
                      <EntityList title="通知投递" data={state.notificationDeliveries} fields={['event_type', 'status', 'last_error']} />
                      <EntityList title="凭据" data={state.credentials} fields={['type', 'enabled', 'description']} />
                      <APIKeyList data={state.apiKeys} onDone={() => void refreshAll()} />
                    </div>
                  </section>
                ),
              },
            ]}
          />
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}

function DataList({ data, renderItem }: { data: Entity[]; renderItem: (item: Entity) => ReactNode }) {
  if (data.length === 0) {
    return <div className="empty-state">No data</div>;
  }
  return <div className="data-list">{data.map((item, index) => <div key={String(item.id ?? index)}>{renderItem(item)}</div>)}</div>;
}

function ListToolbar({
  scoped,
  status,
  statusOptions,
  onScopedChange,
  onStatusChange,
}: {
  scoped: boolean;
  status: string;
  statusOptions: { label: string; value: string }[];
  onScopedChange: (value: boolean) => void;
  onStatusChange: (value: string) => void;
}) {
  return (
    <div className="list-toolbar">
      <Checkbox checked={scoped} onChange={(event) => onScopedChange(event.target.checked)}>
        当前服务/环境
      </Checkbox>
      <Select value={status} options={statusOptions} onChange={onStatusChange} className="status-filter" />
    </div>
  );
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
    value === 'success' || value === 'pass'
      ? 'green'
      : value === 'failed' || value === 'partial' || value === 'block' || value === 'rejected'
        ? 'red'
        : value === 'queued' || value === 'running'
          ? 'blue'
          : value === 'warning'
            ? 'orange'
            : value === 'cancelled'
              ? 'default'
            : 'default';
  return <Tag color={color}>{value ?? '-'}</Tag>;
}

function rowClass(item: Entity, activeID?: string | number | boolean | null) {
  const classes = ['data-row'];
  if (isBadStatus(item.status)) {
    classes.push('danger');
  }
  if (activeID && String(item.id) === String(activeID)) {
    classes.push('active');
  }
  return classes.join(' ');
}

function isBadStatus(value: unknown) {
  return value === 'failed' || value === 'partial' || value === 'skipped';
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

function APIKeyForm({ users, onDone }: { users: Entity[]; onDone: () => void }) {
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
          message="API Key 明文只显示一次"
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
        <Form.Item name="owner_id" label="归属用户" rules={[{ required: true }]}>
          <Select options={users.map(entityOption)} />
        </Form.Item>
        <Form.Item name="scopes" label="Scopes JSON" rules={[{ required: true }]}>
          <Input.TextArea rows={3} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={loading}>
          创建 API Key
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
      <Typography.Title level={4}>API Key</Typography.Title>
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
                <Popconfirm title="删除 API Key" description="删除后不会再出现在列表中。" onConfirm={() => void deleteKey(item)}>
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
    parts.push(`API Key ${shortID(item.api_key_id)}`);
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
