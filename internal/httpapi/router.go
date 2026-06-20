package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ai-pub/internal/app"
	"ai-pub/internal/config"
	"ai-pub/internal/crypto"
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
	notifications := app.NewNotificationService(store, box, nil)
	releases := app.NewReleaseService(store, notifications)
	mux.HandleFunc("GET /healthz", healthz(deps))
	mux.HandleFunc("GET /api/v1/projects", listProjects(store))
	mux.HandleFunc("POST /api/v1/projects", withOptionalAPIKeyScope(store, "admin:write", createProject(store)))
	mux.HandleFunc("GET /api/v1/projects/{id}", getProject(store))
	mux.HandleFunc("PATCH /api/v1/projects/{id}", withOptionalAPIKeyScope(store, "admin:write", patchProject(store)))
	mux.HandleFunc("GET /api/v1/services", listServices(store))
	mux.HandleFunc("POST /api/v1/services", withOptionalAPIKeyScope(store, "admin:write", createService(store)))
	mux.HandleFunc("GET /api/v1/services/{id}", getService(store))
	mux.HandleFunc("PATCH /api/v1/services/{id}", withOptionalAPIKeyScope(store, "admin:write", patchService(store)))
	mux.HandleFunc("GET /api/v1/services/{id}/versions", listServiceVersions(store))
	mux.HandleFunc("POST /api/v1/services/{id}/versions", withOptionalAPIKeyScope(store, "admin:write", createServiceVersion(store)))
	mux.HandleFunc("GET /api/v1/environments", listEnvironments(store))
	mux.HandleFunc("POST /api/v1/environments", withOptionalAPIKeyScope(store, "admin:write", createEnvironment(store)))
	mux.HandleFunc("GET /api/v1/servers", listServers(store))
	mux.HandleFunc("POST /api/v1/servers", withOptionalAPIKeyScope(store, "admin:write", createServer(store)))
	mux.HandleFunc("GET /api/v1/server-groups", listServerGroups(store))
	mux.HandleFunc("POST /api/v1/server-groups", withOptionalAPIKeyScope(store, "admin:write", createServerGroup(store)))
	mux.HandleFunc("GET /api/v1/deployment-targets", listDeploymentTargets(store))
	mux.HandleFunc("POST /api/v1/deployment-targets", withOptionalAPIKeyScope(store, "admin:write", createDeploymentTarget(store)))
	mux.HandleFunc("PATCH /api/v1/deployment-targets/{id}", withOptionalAPIKeyScope(store, "admin:write", patchDeploymentTarget(store)))
	mux.HandleFunc("GET /api/v1/users", listUsers(store))
	mux.HandleFunc("POST /api/v1/users", withOptionalAPIKeyScope(store, "admin:write", createUser(store)))
	mux.HandleFunc("PATCH /api/v1/users/{id}", withOptionalAPIKeyScope(store, "admin:write", patchUser(store)))
	mux.HandleFunc("GET /api/v1/api-keys", listAPIKeys(store))
	mux.HandleFunc("POST /api/v1/api-keys", withOptionalAPIKeyScope(store, "admin:write", createAPIKey(store)))
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", withOptionalAPIKeyScope(store, "admin:write", patchAPIKey(store)))
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", withOptionalAPIKeyScope(store, "admin:write", deleteAPIKey(store)))
	mux.HandleFunc("GET /api/v1/credentials", listCredentials(credentials))
	mux.HandleFunc("POST /api/v1/credentials", withOptionalAPIKeyScope(store, "admin:write", createCredential(credentials)))
	mux.HandleFunc("GET /api/v1/notification-configs", listNotificationConfigs(notifications))
	mux.HandleFunc("POST /api/v1/notification-configs", withOptionalAPIKeyScope(store, "admin:write", createNotificationConfig(notifications)))
	mux.HandleFunc("PATCH /api/v1/notification-configs/{id}", withOptionalAPIKeyScope(store, "admin:write", patchNotificationConfig(notifications)))
	mux.HandleFunc("POST /api/v1/notification-configs/{id}/test", withOptionalAPIKeyScope(store, "admin:write", testNotificationConfig(notifications)))
	mux.HandleFunc("GET /api/v1/notification-deliveries", listNotificationDeliveries(notifications))
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
	mux.HandleFunc("GET /api/v1/release-requests/{id}/events", listReleaseEvents(releases, store))
	mux.HandleFunc("GET /api/v1/release-policies", listReleasePolicies(releases))
	mux.HandleFunc("POST /api/v1/release-policies", withOptionalAPIKeyScope(store, "admin:write", saveReleasePolicy(releases)))
	mux.HandleFunc("GET /api/v1/release-policies/effective", getEffectiveReleasePolicy(releases))
	mux.HandleFunc("POST /api/v1/release-policies/freeze", withOptionalAPIKeyScope(store, "admin:write", freezeReleasePolicy(releases, true)))
	mux.HandleFunc("POST /api/v1/release-policies/unfreeze", withOptionalAPIKeyScope(store, "admin:write", freezeReleasePolicy(releases, false)))
	mux.HandleFunc("GET /api/v1/deploy-records", listDeployRecords(store))
	mux.HandleFunc("GET /api/v1/deploy-records/{id}", getDeployRecord(store))
	mux.HandleFunc("GET /api/v1/deploy-records/{id}/server-logs", listServerDeployLogs(store))
	mux.HandleFunc("GET /api/v1/server-deployment-states", listServerDeploymentStates(store))
	mux.HandleFunc("GET /api/v1/ops/summary", opsSummary(store))
	return requestID(mux)
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
			"db_dialect": "mysql",
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
