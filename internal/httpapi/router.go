package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ai-pub/internal/app"
	"ai-pub/internal/config"
	"ai-pub/internal/crypto"
	"ai-pub/internal/executor"
	"ai-pub/internal/repository"
)

type Dependencies struct {
	DB     *sql.DB
	Config config.Config
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	store := repository.NewStore(deps.DB)
	box := crypto.NewBox(deps.Config.AppEncryptionKey)
	credentials := app.NewCredentialService(store, box)
	infrastructure := app.NewInfrastructureService(store, credentials)
	notifications := app.NewNotificationService(store, box, nil)
	releases := app.NewReleaseService(store, notifications)
	if !deps.Config.ExecutorK8sDisabled {
		releases = releases.WithK8sPreflightChecker(executor.K8s{Credentials: credentials, Clusters: store})
	}
	mux.HandleFunc("GET /healthz", healthz(deps))
	mux.HandleFunc("POST /api/v1/auth/login", login(store, deps.Config.JWTSecret))
	mux.HandleFunc("GET /api/v1/auth/me", currentUser())
	mux.HandleFunc("POST /api/v1/auth/logout", logout())
	mux.HandleFunc("GET /api/v1/projects", withOptionalAPIKeyScope(store, "inventory:read", listProjects(store)))
	mux.HandleFunc("POST /api/v1/projects", withAdmin(store, deps.Config.JWTSecret, createProject(store)))
	mux.HandleFunc("GET /api/v1/projects/{id}", withOptionalAPIKeyScope(store, "inventory:read", getProject(store)))
	mux.HandleFunc("PATCH /api/v1/projects/{id}", withAdmin(store, deps.Config.JWTSecret, patchProject(store)))
	mux.HandleFunc("GET /api/v1/services", withOptionalAPIKeyScope(store, "inventory:read", listServices(store)))
	mux.HandleFunc("POST /api/v1/services", withAdmin(store, deps.Config.JWTSecret, createService(store)))
	mux.HandleFunc("GET /api/v1/services/{id}", withOptionalAPIKeyScope(store, "inventory:read", getService(store)))
	mux.HandleFunc("PATCH /api/v1/services/{id}", withAdmin(store, deps.Config.JWTSecret, patchService(store)))
	mux.HandleFunc("GET /api/v1/services/{id}/versions", withOptionalAPIKeyScope(store, "inventory:read", listServiceVersions(store)))
	mux.HandleFunc("POST /api/v1/services/{id}/versions", withAdmin(store, deps.Config.JWTSecret, createServiceVersion(store)))
	mux.HandleFunc("GET /api/v1/environments", withOptionalAPIKeyScope(store, "inventory:read", listEnvironments(store)))
	mux.HandleFunc("POST /api/v1/environments", withAdmin(store, deps.Config.JWTSecret, createEnvironment(store)))
	mux.HandleFunc("PATCH /api/v1/environments/{id}", withAdmin(store, deps.Config.JWTSecret, patchEnvironment(store)))
	mux.HandleFunc("GET /api/v1/servers", withOptionalAPIKeyScope(store, "inventory:read", listServers(store)))
	mux.HandleFunc("POST /api/v1/servers", withAdmin(store, deps.Config.JWTSecret, createServer(store)))
	mux.HandleFunc("PATCH /api/v1/servers/{id}", withAdmin(store, deps.Config.JWTSecret, patchServer(store)))
	mux.HandleFunc("POST /api/v1/servers/test", withAdmin(store, deps.Config.JWTSecret, testServerConfig(infrastructure, !deps.Config.ExecutorSSHDisabled)))
	mux.HandleFunc("POST /api/v1/servers/{id}/test", withAdmin(store, deps.Config.JWTSecret, testServer(infrastructure, !deps.Config.ExecutorSSHDisabled)))
	mux.HandleFunc("GET /api/v1/server-groups", withOptionalAPIKeyScope(store, "inventory:read", listServerGroups(store)))
	mux.HandleFunc("POST /api/v1/server-groups", withAdmin(store, deps.Config.JWTSecret, createServerGroup(store)))
	mux.HandleFunc("PATCH /api/v1/server-groups/{id}", withAdmin(store, deps.Config.JWTSecret, patchServerGroup(store)))
	mux.HandleFunc("GET /api/v1/k8s-clusters", withOptionalAPIKeyScope(store, "inventory:read", listK8sClusters(infrastructure)))
	mux.HandleFunc("POST /api/v1/k8s-clusters", withAdmin(store, deps.Config.JWTSecret, createK8sCluster(infrastructure)))
	mux.HandleFunc("PATCH /api/v1/k8s-clusters/{id}", withAdmin(store, deps.Config.JWTSecret, patchK8sCluster(infrastructure)))
	mux.HandleFunc("DELETE /api/v1/k8s-clusters/{id}", withAdmin(store, deps.Config.JWTSecret, deleteK8sCluster(infrastructure)))
	mux.HandleFunc("GET /api/v1/deployment-targets", withOptionalAPIKeyScope(store, "inventory:read", listDeploymentTargets(store)))
	mux.HandleFunc("POST /api/v1/deployment-targets", withAdmin(store, deps.Config.JWTSecret, createDeploymentTarget(store)))
	mux.HandleFunc("PATCH /api/v1/deployment-targets/{id}", withAdmin(store, deps.Config.JWTSecret, patchDeploymentTarget(store)))
	mux.HandleFunc("GET /api/v1/users", withSessionUser(store, deps.Config.JWTSecret, listUsers(store)))
	mux.HandleFunc("POST /api/v1/users", withAdmin(store, deps.Config.JWTSecret, createSessionUser(store)))
	mux.HandleFunc("PATCH /api/v1/users/{id}", withAdmin(store, deps.Config.JWTSecret, patchUser(store, deps.Config)))
	mux.HandleFunc("POST /api/v1/users/{id}/password", withAdminSession(deps.Config.JWTSecret, resetUserPassword(store, deps.Config)))
	mux.HandleFunc("GET /api/v1/api-keys", listAPIKeys(store, deps.Config.JWTSecret))
	mux.HandleFunc("POST /api/v1/api-keys", createAPIKey(store, deps.Config.JWTSecret))
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", patchAPIKey(store, deps.Config.JWTSecret))
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", deleteAPIKey(store, deps.Config.JWTSecret))
	mux.HandleFunc("GET /api/v1/credentials", withAdmin(store, deps.Config.JWTSecret, listCredentials(credentials)))
	mux.HandleFunc("POST /api/v1/credentials", withAdmin(store, deps.Config.JWTSecret, createCredential(credentials)))
	mux.HandleFunc("PATCH /api/v1/credentials/{id}", withAdmin(store, deps.Config.JWTSecret, patchCredential(credentials)))
	mux.HandleFunc("DELETE /api/v1/credentials/{id}", withAdmin(store, deps.Config.JWTSecret, deleteCredential(credentials)))
	mux.HandleFunc("GET /api/v1/notification-configs", withAdmin(store, deps.Config.JWTSecret, listNotificationConfigs(notifications)))
	mux.HandleFunc("POST /api/v1/notification-configs", withAdmin(store, deps.Config.JWTSecret, createNotificationConfig(notifications)))
	mux.HandleFunc("PATCH /api/v1/notification-configs/{id}", withAdmin(store, deps.Config.JWTSecret, patchNotificationConfig(notifications)))
	mux.HandleFunc("DELETE /api/v1/notification-configs/{id}", withAdmin(store, deps.Config.JWTSecret, deleteNotificationConfig(notifications)))
	mux.HandleFunc("POST /api/v1/notification-configs/{id}/test", withAdmin(store, deps.Config.JWTSecret, testNotificationConfig(notifications)))
	mux.HandleFunc("GET /api/v1/notification-deliveries", withAdmin(store, deps.Config.JWTSecret, listNotificationDeliveries(notifications)))
	mux.HandleFunc("POST /api/v1/version-registrations", registerVersion(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/preflight", preflightRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests", createRelease(releases, store))
	mux.HandleFunc("GET /api/v1/release-requests", listReleases(releases, store))
	mux.HandleFunc("GET /api/v1/release-requests/{id}", getRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/preflight", preflightExistingRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/confirm", confirmRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/reject", rejectRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/cancel", cancelRelease(releases, store))
	mux.HandleFunc("GET /api/v1/release-requests/{id}/rollback-candidates", rollbackCandidates(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/rollback", createRollbackRelease(releases, store))
	mux.HandleFunc("POST /api/v1/release-requests/{id}/retry", retryRelease(releases, store))
	mux.HandleFunc("GET /api/v1/release-requests/{id}/events", listReleaseEvents(releases, store))
	mux.HandleFunc("GET /api/v1/deploy-records", listDeployRecords(store))
	mux.HandleFunc("GET /api/v1/deploy-records/{id}", getDeployRecord(store))
	mux.HandleFunc("GET /api/v1/deploy-records/{id}/target-logs", listDeployTargetLogs(store))
	mux.HandleFunc("GET /api/v1/deploy-records/{id}/server-logs", listDeployTargetLogs(store))
	mux.HandleFunc("GET /api/v1/deployment-states", listDeploymentStates(store))
	mux.HandleFunc("GET /api/v1/server-deployment-states", listDeploymentStates(store))
	mux.HandleFunc("GET /api/v1/ops/summary", withOptionalAPIKeyScope(store, "deploy:read", opsSummary(store)))
	mux.HandleFunc("GET /api/v1/agent/services", agentListServices(store))
	mux.HandleFunc("GET /api/v1/agent/environments", agentListEnvironments(store))
	mux.HandleFunc("GET /api/v1/agent/services/{id}/versions", agentListServiceVersions(store))
	mux.HandleFunc("GET /api/v1/agent/deployment-targets", agentListDeploymentTargets(store))
	mux.HandleFunc("POST /api/v1/agent/release-intents/preflight", agentPreflightRelease(releases, store))
	mux.HandleFunc("POST /api/v1/agent/release-requests", agentCreateRelease(releases, store))
	mux.HandleFunc("POST /api/v1/agent/release-requests/{id}/confirm", agentConfirmRelease(releases, store))
	mux.HandleFunc("GET /api/v1/agent/release-requests/{id}/summary", agentReleaseSummary(releases, store))
	if deps.Config.WebDir != "" {
		mux.Handle("GET /", spa(deps.Config.WebDir))
	}
	return requestID(requireSessionOrAPIKey(store, deps.Config.JWTSecret, mux))
}

func healthz(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		code := http.StatusOK
		if deps.DB == nil || deps.DB.PingContext(r.Context()) != nil {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}
		writeData(w, r, code, map[string]any{
			"status":     status,
			"db_dialect": deps.Config.DBDialect,
			"time":       time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

type response struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

type errorResponse struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeData(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{
		Data:      data,
		RequestID: requestIDFrom(r),
	})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		status = http.StatusNotFound
		code = "not_found"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error:     apiError{Code: code, Message: err.Error()},
		RequestID: requestIDFrom(r),
	})
}

// parseIntDefault 解析 query 参数为 int，空串或非法时返回默认值。用于分页/时间范围参数。
func parseIntDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
