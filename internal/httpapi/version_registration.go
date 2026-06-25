package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ai-pub/internal/app"
	"ai-pub/internal/repository"
)

// versionRegistrationRequest 是外部 CI 版本登记的请求体。
type versionRegistrationRequest struct {
	ProjectKey  string          `json:"project_key"`
	ServiceKey  string          `json:"service_key"`
	Version     string          `json:"version"`
	CommitSHA   string          `json:"commit_sha"`
	ArtifactURL string          `json:"artifact_url"`
	Metadata    json.RawMessage `json:"metadata"`
}

// registerVersion 处理外部 CI 版本登记，强制写入 source=ci 与 API Key 身份。
func registerVersion(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 登记接口仅允许 version:write API Key；不接受会话用户绕过登记审计。
		key, ok := authorizeOptionalAPIKey(w, r, store, "version:write")
		if !ok {
			return
		}
		if key.ID == "" {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", errUnauthorized)
			return
		}

		var body versionRegistrationRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.ProjectKey = strings.TrimSpace(body.ProjectKey)
		body.ServiceKey = strings.TrimSpace(body.ServiceKey)
		body.Version = strings.TrimSpace(body.Version)
		if body.ProjectKey == "" || body.ServiceKey == "" || body.Version == "" {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", errors.New("project_key, service_key, version are required"))
			return
		}

		metadata := "{}"
		if len(body.Metadata) > 0 {
			metadata = string(body.Metadata)
		}

		input := app.VersionRegistrationInput{
			ProjectKey:     body.ProjectKey,
			ServiceKey:     body.ServiceKey,
			Version:        body.Version,
			CommitSHA:      body.CommitSHA,
			ArtifactURL:    body.ArtifactURL,
			Metadata:       metadata,
			IdempotencyKey: r.Header.Get("Idempotency-Key"),
			APIKeyID:       key.ID,
		}

		result, err := service.RegisterVersion(r.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, app.ErrServiceNotFound):
				writeError(w, r, http.StatusNotFound, "service_not_found", err)
			case errors.Is(err, app.ErrServiceDisabled):
				writeError(w, r, http.StatusConflict, "service_disabled", err)
			case errors.Is(err, app.ErrIdempotencyConflict):
				writeError(w, r, http.StatusConflict, "idempotency_conflict", err)
			case errors.Is(err, app.ErrVersionConflict):
				writeError(w, r, http.StatusConflict, "version_conflict", err)
			default:
				writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			}
			return
		}

		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		writeData(w, r, status, result.Version)
	}
}
