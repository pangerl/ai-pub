package httpapi

import (
	"errors"
	"net/http"

	"ai-pub/internal/app"
	"ai-pub/internal/repository"
)

func listNotificationConfigs(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListConfigs(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createNotificationConfig(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.CreateNotificationConfigInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.CreateConfig(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func patchNotificationConfig(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.PatchNotificationConfigInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.PatchConfig(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func testNotificationConfig(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := service.Test(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "executor_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func deleteNotificationConfig(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := requireID(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		if err := service.DeleteConfig(r.Context(), id); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, "not_found", err)
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]string{"id": id})
	}
}

func listNotificationDeliveries(service app.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListDeliveries(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}
