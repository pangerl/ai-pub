package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ai-pub/internal/app"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

const defaultAgentActorID = "agent"

type agentCandidateResponse[T any] struct {
	Items []T    `json:"items"`
	Count int    `json:"count"`
	Query string `json:"query,omitempty"`
}

type agentCreateReleaseInput struct {
	app.PreflightInput
	IdempotencyKey  string `json:"idempotency_key"`
	AgentName       string `json:"agent_name"`
	SkillVersion    string `json:"skill_version"`
	IntentSummary   string `json:"intent_summary"`
	ClientRequestID string `json:"client_request_id"`
	ConversationRef string `json:"conversation_ref"`
}

type agentSummary struct {
	Release       domain.ReleaseRequest   `json:"release"`
	Events        []domain.ReleaseEvent   `json:"events"`
	DeployRecords repository.PagedDeploys `json:"deploy_records"`
	NextAction    string                  `json:"next_action"`
}

func agentListServices(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "inventory:read"); !ok {
			return
		}
		q := searchQuery(r)
		items, err := store.ListServices(r.Context(), r.URL.Query().Get("project_id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		filtered := filterServices(items, q)
		writeData(w, r, http.StatusOK, agentCandidateResponse[domain.Service]{
			Items: filtered,
			Count: len(filtered),
			Query: q,
		})
	}
}

func agentListEnvironments(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "inventory:read"); !ok {
			return
		}
		q := searchQuery(r)
		items, err := store.ListEnvironments(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		filtered := filterEnvironments(items, q)
		writeData(w, r, http.StatusOK, agentCandidateResponse[domain.Environment]{
			Items: filtered,
			Count: len(filtered),
			Query: q,
		})
	}
}

func agentListServiceVersions(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "inventory:read"); !ok {
			return
		}
		q := searchQuery(r)
		items, err := store.ListServiceVersions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		filtered := filterServiceVersions(items, q)
		writeData(w, r, http.StatusOK, agentCandidateResponse[domain.ServiceVersion]{
			Items: filtered,
			Count: len(filtered),
			Query: q,
		})
	}
}

func agentListDeploymentTargets(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "inventory:read"); !ok {
			return
		}
		q := r.URL.Query()
		items, err := store.ListDeploymentTargets(r.Context(), q.Get("service_id"), q.Get("environment_id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, agentCandidateResponse[domain.DeploymentTarget]{
			Items: items,
			Count: len(items),
		})
	}
}

func agentPreflightRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.PreflightInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:create"); !ok {
			return
		}
		result, err := service.Preflight(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, result)
	}
}

func agentCreateRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input agentCreateReleaseInput
		if !decodeJSON(w, r, &input) {
			return
		}
		createInput := app.CreateReleaseInput{
			PreflightInput: input.PreflightInput,
			Source:         "ai_agent",
			IdempotencyKey: input.IdempotencyKey,
			Metadata:       agentMetadata(input),
		}
		if createInput.IdempotencyKey == "" {
			createInput.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		if user, ok := currentSessionUser(r); ok {
			createInput.CreatedByType = "user"
			createInput.CreatedByID = user.ID
			createInput.AuthorizedByUserID = user.ID
		} else {
			key, ok, err := apiKeyFromBearer(store, r, "release:create")
			if err != nil {
				if errors.Is(err, errForbidden) {
					writeError(w, r, http.StatusForbidden, "forbidden", err)
					return
				}
				writeError(w, r, http.StatusUnauthorized, "unauthorized", err)
				return
			}
			if ok && key.ID != "" {
				createInput.CreatedByType = "api_key"
				createInput.CreatedByID = key.ID
				createInput.APIKeyID = key.ID
			} else {
				createInput.CreatedByType = "system"
				createInput.CreatedByID = defaultAgentActorID
			}
		}
		release, preflight, err := service.Create(r.Context(), createInput)
		if err != nil {
			if errors.Is(err, app.ErrPreflightBlocked) {
				writeError(w, r, http.StatusConflict, "preflight_blocked", err)
				return
			}
			if errors.Is(err, app.ErrIdempotencyConflict) {
				writeError(w, r, http.StatusConflict, "idempotency_conflict", err)
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, map[string]any{
			"release":     release,
			"next_action": preflight.NextAction,
			"preflight":   preflight,
		})
	}
}

func agentConfirmRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input := app.ConfirmInput{}
		if user, ok := currentSessionUser(r); ok {
			input.Actor = app.Actor{Type: "user", ID: user.ID}
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:confirm"); !ok {
			return
		} else if key.ID != "" {
			input.Actor = app.Actor{Type: "api_key", ID: key.ID, APIKeyID: key.ID}
		}
		item, err := service.Confirm(r.Context(), r.PathValue("id"), input)
		if err != nil {
			if errors.Is(err, app.ErrPreflightBlocked) {
				writeError(w, r, http.StatusConflict, "preflight_blocked", err)
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_state", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func agentReleaseSummary(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:read"); !ok {
			return
		}
		release, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		events, err := service.ListEvents(r.Context(), release.ID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		deploys, err := store.ListDeployRecords(r.Context(), repository.DeployListFilter{
			ReleaseRequestID: release.ID,
			Page:             1,
			PageSize:         5,
		})
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, agentSummary{
			Release:       release,
			Events:        events,
			DeployRecords: deploys,
			NextAction:    nextAgentAction(release),
		})
	}
}

func agentMetadata(input agentCreateReleaseInput) string {
	values := map[string]string{
		"type":              "agent_release",
		"agent_name":        strings.TrimSpace(input.AgentName),
		"skill_version":     strings.TrimSpace(input.SkillVersion),
		"intent_summary":    strings.TrimSpace(input.IntentSummary),
		"client_request_id": strings.TrimSpace(input.ClientRequestID),
		"conversation_ref":  strings.TrimSpace(input.ConversationRef),
	}
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return `{"type":"agent_release"}`
	}
	return string(raw)
}

func nextAgentAction(release domain.ReleaseRequest) string {
	switch release.Status {
	case "pending_confirm":
		return "confirm_required"
	case "queued", "running":
		return "wait"
	case "success", "failed", "cancelled", "rejected":
		return "done"
	default:
		return "inspect"
	}
}

func searchQuery(r *http.Request) string {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	return q
}

func filterServices(items []domain.Service, q string) []domain.Service {
	if q == "" {
		return items
	}
	out := []domain.Service{}
	for _, item := range items {
		if matchesAgentQuery(q, item.ID, item.Name, item.Slug) {
			out = append(out, item)
		}
	}
	return out
}

func filterEnvironments(items []domain.Environment, q string) []domain.Environment {
	if q == "" {
		return items
	}
	out := []domain.Environment{}
	for _, item := range items {
		if matchesAgentQuery(q, item.ID, item.Name, item.Slug) {
			out = append(out, item)
		}
	}
	return out
}

func filterServiceVersions(items []domain.ServiceVersion, q string) []domain.ServiceVersion {
	if q == "" {
		return items
	}
	out := []domain.ServiceVersion{}
	for _, item := range items {
		if matchesAgentQuery(q, item.ID, item.Version, item.CommitSHA) {
			out = append(out, item)
		}
	}
	return out
}

func matchesAgentQuery(q string, values ...string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == q || strings.Contains(value, q) {
			return true
		}
	}
	return false
}
