package httpapi

import (
	"errors"
	"net/http"

	"ai-pub/internal/app"
	"ai-pub/internal/repository"
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

func patchCredential(service app.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := requireID(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		var input app.UpdateCredentialInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.Update(r.Context(), id, input)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, "not_found", err)
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func deleteCredential(service app.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := requireID(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		err := service.Delete(r.Context(), id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, "not_found", err)
				return
			}
			if errors.Is(err, repository.ErrCredentialInUse) {
				writeError(w, r, http.StatusConflict, "credential_in_use", err)
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]string{"id": id})
	}
}
