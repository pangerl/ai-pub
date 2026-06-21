package httpapi

import (
	"errors"
	"net/http"

	"ai-pub/internal/app"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

func preflightRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
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

func preflightExistingRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := authorizeOptionalAPIKey(w, r, store, "release:read")
		if !ok {
			return
		}
		actor := app.Actor{Type: "system", ID: "web"}
		if key.ID != "" {
			actor = app.Actor{Type: "api_key", ID: key.ID, APIKeyID: key.ID}
		}
		result, err := service.PreflightExisting(r.Context(), r.PathValue("id"), actor)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, result)
	}
}

func createRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.CreateReleaseInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Source = "web"
			input.CreatedByType = "user"
			input.CreatedByID = user.ID
			input.AuthorizedByUserID = ""
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
			if ok {
				input.Source = "api"
				input.CreatedByType = "api_key"
				input.CreatedByID = key.ID
				input.APIKeyID = key.ID
				input.AuthorizedByUserID = ""
			}
		}
		if input.IdempotencyKey == "" {
			input.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		release, preflight, err := service.Create(r.Context(), input)
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

func listReleases(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:read"); !ok {
			return
		}
		items, err := service.List(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func getRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:read"); !ok {
			return
		}
		item, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func confirmRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.ConfirmInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Actor = app.Actor{Type: "user", ID: user.ID}
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:confirm"); !ok {
			return
		} else if key.ID != "" {
			input.Actor = app.Actor{Type: "api_key", ID: key.ID, APIKeyID: key.ID}
		}
		item, err := service.Confirm(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_state", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func authorizeOptionalAPIKey(w http.ResponseWriter, r *http.Request, store repository.Store, scope string) (domain.APIKey, bool) {
	if _, ok := currentSessionUser(r); ok {
		return domain.APIKey{}, true
	}
	key, _, err := apiKeyFromBearer(store, r, scope)
	if err == nil {
		return key, true
	}
	if errors.Is(err, errForbidden) {
		writeError(w, r, http.StatusForbidden, "forbidden", err)
		return domain.APIKey{}, false
	}
	writeError(w, r, http.StatusUnauthorized, "unauthorized", err)
	return domain.APIKey{}, false
}

func rejectRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.RejectInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Actor = app.Actor{Type: "user", ID: user.ID}
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:confirm"); !ok {
			return
		} else if key.ID != "" {
			input.Actor = app.Actor{Type: "api_key", ID: key.ID, APIKeyID: key.ID}
		}
		item, err := service.Reject(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_state", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func cancelRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.CancelInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Actor = app.Actor{Type: "user", ID: user.ID}
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:confirm"); !ok {
			return
		} else if key.ID != "" {
			input.Actor = app.Actor{Type: "api_key", ID: key.ID, APIKeyID: key.ID}
		}
		item, err := service.Cancel(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_state", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func rollbackCandidates(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:read"); !ok {
			return
		}
		items, err := service.RollbackCandidates(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createRollbackRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.RollbackInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Source = "web"
			input.CreatedByType = "user"
			input.CreatedByID = user.ID
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:rollback"); !ok {
			return
		} else if key.ID != "" {
			input.Source = "api"
			input.CreatedByType = "api_key"
			input.CreatedByID = key.ID
			input.APIKeyID = key.ID
		}
		if input.IdempotencyKey == "" {
			input.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		release, preflight, err := service.CreateRollback(r.Context(), r.PathValue("id"), input)
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

func retryRelease(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.RetryInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if user, ok := currentSessionUser(r); ok {
			input.Source = "web"
			input.CreatedByType = "user"
			input.CreatedByID = user.ID
		} else if key, ok := authorizeOptionalAPIKey(w, r, store, "release:create"); !ok {
			return
		} else if key.ID != "" {
			input.Source = "api"
			input.CreatedByType = "api_key"
			input.CreatedByID = key.ID
			input.APIKeyID = key.ID
		}
		if input.IdempotencyKey == "" {
			input.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		release, preflight, err := service.Retry(r.Context(), r.PathValue("id"), input)
		if err != nil {
			if errors.Is(err, app.ErrPreflightBlocked) {
				writeError(w, r, http.StatusConflict, "preflight_blocked", err)
				return
			}
			if errors.Is(err, app.ErrIdempotencyConflict) {
				writeError(w, r, http.StatusConflict, "idempotency_conflict", err)
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_state", err)
			return
		}
		writeData(w, r, http.StatusCreated, map[string]any{"release": release, "next_action": preflight.NextAction, "preflight": preflight})
	}
}

func listReleaseEvents(service app.ReleaseService, store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "release:read"); !ok {
			return
		}
		items, err := service.ListEvents(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func listReleasePolicies(service app.ReleaseService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListPolicies(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func getEffectiveReleasePolicy(service app.ReleaseService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := service.EffectivePolicy(r.Context(), r.URL.Query().Get("service_id"), r.URL.Query().Get("environment_id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func saveReleasePolicy(service app.ReleaseService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.PolicyInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.SavePolicy(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func freezeReleasePolicy(service app.ReleaseService, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ScopeType string `json:"scope_type"`
			ScopeID   string `json:"scope_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.SetFreeze(r.Context(), input.ScopeType, input.ScopeID, enabled)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}
