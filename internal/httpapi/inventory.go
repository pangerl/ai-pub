package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

func listProjects(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListProjects(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createProject(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.Project
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateProject(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func getProject(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := store.GetProject(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func patchProject(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetProject(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			Name        *string `json:"name"`
			Slug        *string `json:"slug"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.Name != nil {
			existing.Name = *patch.Name
		}
		if patch.Slug != nil {
			existing.Slug = *patch.Slug
		}
		if patch.Description != nil {
			existing.Description = *patch.Description
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateProject(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func listServices(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListServices(r.Context(), r.URL.Query().Get("project_id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createService(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.Service
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateService(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func getService(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := store.GetService(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func patchService(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetService(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			Name        *string `json:"name"`
			Slug        *string `json:"slug"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.Name != nil {
			existing.Name = *patch.Name
		}
		if patch.Slug != nil {
			existing.Slug = *patch.Slug
		}
		if patch.Description != nil {
			existing.Description = *patch.Description
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateService(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func listServiceVersions(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListServiceVersions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createServiceVersion(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.ServiceVersion
		if !decodeJSON(w, r, &input) {
			return
		}
		input.ServiceID = r.PathValue("id")
		item, err := store.CreateServiceVersion(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func listEnvironments(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListEnvironments(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createEnvironment(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.Environment
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateEnvironment(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func listServers(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListServers(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createServer(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.Server
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateServer(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func listServerGroups(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListServerGroups(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createServerGroup(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.ServerGroup
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateServerGroup(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func listDeploymentTargets(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListDeploymentTargets(r.Context(), r.URL.Query().Get("service_id"), r.URL.Query().Get("environment_id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createDeploymentTarget(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.DeploymentTarget
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateDeploymentTarget(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func patchDeploymentTarget(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetDeploymentTarget(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			ExecutorType   *string `json:"executor_type"`
			TargetType     *string `json:"target_type"`
			TargetRefID    *string `json:"target_ref_id"`
			ScriptPath     *string `json:"script_path"`
			WorkingDir     *string `json:"working_dir"`
			EnvVars        *string `json:"env_vars"`
			TimeoutSeconds *int    `json:"timeout_seconds"`
			Enabled        *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.ExecutorType != nil {
			existing.ExecutorType = *patch.ExecutorType
		}
		if patch.TargetType != nil {
			existing.TargetType = *patch.TargetType
		}
		if patch.TargetRefID != nil {
			existing.TargetRefID = *patch.TargetRefID
		}
		if patch.ScriptPath != nil {
			existing.ScriptPath = *patch.ScriptPath
		}
		if patch.WorkingDir != nil {
			existing.WorkingDir = *patch.WorkingDir
		}
		if patch.EnvVars != nil {
			existing.EnvVars = *patch.EnvVars
		}
		if patch.TimeoutSeconds != nil {
			existing.TimeoutSeconds = *patch.TimeoutSeconds
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateDeploymentTarget(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func listUsers(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListUsers(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createUser(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.User
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateUser(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func patchUser(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetUser(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			DisplayName *string `json:"display_name"`
			Role        *string `json:"role"`
			Enabled     *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.DisplayName != nil {
			existing.DisplayName = *patch.DisplayName
		}
		if patch.Role != nil {
			existing.Role = *patch.Role
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateUser(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func listAPIKeys(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListAPIKeys(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createAPIKey(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.APIKey
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := store.CreateAPIKey(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func patchAPIKey(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetAPIKey(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			Name    *string `json:"name"`
			Scopes  *string `json:"scopes"`
			Enabled *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.Name != nil {
			existing.Name = *patch.Name
		}
		if patch.Scopes != nil {
			existing.Scopes = *patch.Scopes
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateAPIKey(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func deleteAPIKey(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := requireID(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		if err := store.DeleteAPIKey(r.Context(), id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]string{"id": id})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
		return false
	}
	return true
}

func requireID(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	return nil
}
