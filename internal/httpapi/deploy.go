package httpapi

import (
	"net/http"

	"ai-pub/internal/repository"
)

func listDeployRecords(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "deploy:read"); !ok {
			return
		}
		q := r.URL.Query()
		filter := repository.DeployListFilter{
			ReleaseRequestID: q.Get("release_request_id"),
			ServiceID:        q.Get("service_id"),
			EnvironmentID:    q.Get("environment_id"),
			Status:           q.Get("status"),
			Page:             parseIntDefault(q.Get("page"), 1),
			PageSize:         parseIntDefault(q.Get("page_size"), 50),
		}
		result, err := store.ListDeployRecords(r.Context(), filter)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, result)
	}
}

func getDeployRecord(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "deploy:read"); !ok {
			return
		}
		item, err := store.GetDeployRecord(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func listDeployTargetLogs(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "deploy:read"); !ok {
			return
		}
		items, err := store.ListDeployTargetLogs(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func listDeploymentStates(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, "deploy:read"); !ok {
			return
		}
		items, err := store.ListDeploymentStates(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func opsSummary(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := store.OpsSummary(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}
