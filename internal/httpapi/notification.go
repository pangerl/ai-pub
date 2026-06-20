package httpapi

import (
	"net/http"

	"ai-pub/internal/app"
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
