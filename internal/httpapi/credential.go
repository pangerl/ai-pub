package httpapi

import (
	"net/http"

	"ai-pub/internal/app"
)

func listCredentials(service app.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.List(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createCredential(service app.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.CreateCredentialInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.Create(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}
