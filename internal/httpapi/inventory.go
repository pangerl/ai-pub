package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ai-pub/internal/auth"
	"ai-pub/internal/config"
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
		// 手动登记来源与创建者身份由服务端强制写入，不允许请求体伪造。
		input.Source = "manual"
		input.RegistrationIdempotencyKey = ""
		input.RegistrationRequestHash = ""
		if user, ok := currentSessionUser(r); ok {
			input.CreatedByType = "user"
			input.CreatedByID = user.ID
		}
		// 手动登记不使用幂等键：同版本号一律 409，不覆盖已有版本。
		if _, err := store.FindServiceVersionByServiceAndVersion(r.Context(), input.ServiceID, input.Version); err == nil {
			writeError(w, r, http.StatusConflict, "version_conflict", errors.New("version already exists"))
			return
		} else if !errors.Is(err, repository.ErrNotFound) {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		item, err := store.CreateServiceVersion(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		_, _ = store.CreateServiceVersionEvent(r.Context(), domain.ServiceVersionEvent{
			ServiceVersionID: item.ID,
			EventType:        "version_registered",
			ActorType:        "user",
			ActorID:          item.CreatedByID,
			Message:          "管理员手动登记版本",
			Metadata:         item.Metadata,
		})
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

func patchEnvironment(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetEnvironment(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusNotFound, "not_found", err)
			return
		}
		var patch struct {
			Name          *string `json:"name"`
			Slug          *string `json:"slug"`
			IsProduction  *bool   `json:"is_production"`
			ReleaseFrozen *bool   `json:"release_frozen"`
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
		if patch.IsProduction != nil {
			existing.IsProduction = *patch.IsProduction
		}
		if patch.ReleaseFrozen != nil {
			existing.ReleaseFrozen = *patch.ReleaseFrozen
		}
		item, err := store.UpdateEnvironment(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
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
		if err := validateServerCredential(r.Context(), store, input.AuthType, input.CredentialRef); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
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

func patchServer(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetServer(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusNotFound, "not_found", err)
			return
		}
		var patch struct {
			Name          *string `json:"name"`
			Host          *string `json:"host"`
			Port          *int    `json:"port"`
			Username      *string `json:"username"`
			AuthType      *string `json:"auth_type"`
			CredentialRef *string `json:"credential_ref"`
			Role          *string `json:"role"`
			GatewayID     *string `json:"gateway_id"`
			Enabled       *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.Name != nil {
			existing.Name = *patch.Name
		}
		if patch.Host != nil {
			existing.Host = *patch.Host
		}
		if patch.Port != nil {
			existing.Port = *patch.Port
		}
		if patch.Username != nil {
			existing.Username = *patch.Username
		}
		if patch.AuthType != nil {
			existing.AuthType = *patch.AuthType
		}
		if patch.CredentialRef != nil {
			existing.CredentialRef = *patch.CredentialRef
		}
		if patch.Role != nil {
			existing.Role = *patch.Role
		}
		if patch.GatewayID != nil {
			existing.GatewayID = *patch.GatewayID
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		if err := validateServerCredential(r.Context(), store, existing.AuthType, existing.CredentialRef); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		item, err := store.UpdateServer(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

// validateServerCredential 校验服务器认证配置：
// auth_type 必须是 none/password/private_key 之一（白名单，拒绝空串与未知值，避免绕过校验）；
// 非 none 时必须引用存在且启用的凭据，与凭据禁用/删除形成闭环，避免运行时 SSH 失败。
func validateServerCredential(ctx context.Context, store repository.Store, authType string, credentialRef string) error {
	switch authType {
	case "none":
		return nil
	case "password", "private_key":
		// 继续做凭据引用校验
	default:
		return errors.New("auth_type must be one of none, password, private_key")
	}
	if credentialRef == "" {
		return errors.New("credential_ref is required when auth_type is not none")
	}
	cred, err := store.GetCredential(ctx, credentialRef)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("credential_ref references a non-existent credential")
		}
		return err
	}
	if !cred.Enabled {
		return errors.New("credential_ref references a disabled credential")
	}
	return nil
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

func patchServerGroup(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetServerGroup(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusNotFound, "not_found", err)
			return
		}
		var patch struct {
			Name        *string   `json:"name"`
			Description *string   `json:"description"`
			ServerIDs   *[]string `json:"server_ids"`
			Enabled     *bool     `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.Name != nil {
			existing.Name = *patch.Name
		}
		if patch.Description != nil {
			existing.Description = *patch.Description
		}
		if patch.ServerIDs != nil {
			existing.ServerIDs = *patch.ServerIDs
		}
		if patch.Enabled != nil {
			existing.Enabled = *patch.Enabled
		}
		item, err := store.UpdateServerGroup(r.Context(), existing.ID, existing)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
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
		var input deploymentTargetPayload
		if !decodeJSON(w, r, &input) {
			return
		}
		target := input.toDomain()
		item, err := store.CreateDeploymentTarget(r.Context(), target)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

type deploymentTargetPayload struct {
	domain.DeploymentTarget
	TargetType  string `json:"target_type"`
	TargetRefID string `json:"target_ref_id"`
	ScriptPath  string `json:"script_path"`
	WorkingDir  string `json:"working_dir"`
	EnvVars     string `json:"env_vars"`
}

func (p deploymentTargetPayload) toDomain() domain.DeploymentTarget {
	item := p.DeploymentTarget
	if item.SSH == nil && (p.TargetType != "" || p.TargetRefID != "" || p.ScriptPath != "" || p.WorkingDir != "" || p.EnvVars != "") {
		item.SSH = &domain.SSHDeploymentTarget{
			TargetType:  p.TargetType,
			TargetRefID: p.TargetRefID,
			ScriptPath:  p.ScriptPath,
			WorkingDir:  p.WorkingDir,
			EnvVars:     p.EnvVars,
		}
	}
	return item
}

func patchDeploymentTarget(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.GetDeploymentTarget(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		var patch struct {
			ExecutorType   *string                     `json:"executor_type"`
			ArtifactType   *string                     `json:"artifact_type"`
			SSH            *domain.SSHDeploymentTarget `json:"ssh"`
			K8s            *domain.K8sDeploymentTarget `json:"k8s"`
			TargetType     *string                     `json:"target_type"`
			TargetRefID    *string                     `json:"target_ref_id"`
			ScriptPath     *string                     `json:"script_path"`
			WorkingDir     *string                     `json:"working_dir"`
			EnvVars        *string                     `json:"env_vars"`
			TimeoutSeconds *int                        `json:"timeout_seconds"`
			Enabled        *bool                       `json:"enabled"`
		}
		if !decodeJSON(w, r, &patch) {
			return
		}
		if patch.ExecutorType != nil {
			existing.ExecutorType = *patch.ExecutorType
		}
		if patch.ArtifactType != nil {
			existing.ArtifactType = *patch.ArtifactType
		}
		if patch.SSH != nil {
			existing.SSH = patch.SSH
		}
		if patch.TargetType != nil || patch.TargetRefID != nil || patch.ScriptPath != nil || patch.WorkingDir != nil || patch.EnvVars != nil {
			ssh := domain.SSHDeploymentTarget{}
			if existing.SSH != nil {
				ssh = *existing.SSH
			}
			if patch.TargetType != nil {
				ssh.TargetType = *patch.TargetType
			}
			if patch.TargetRefID != nil {
				ssh.TargetRefID = *patch.TargetRefID
			}
			if patch.ScriptPath != nil {
				ssh.ScriptPath = *patch.ScriptPath
			}
			if patch.WorkingDir != nil {
				ssh.WorkingDir = *patch.WorkingDir
			}
			if patch.EnvVars != nil {
				ssh.EnvVars = *patch.EnvVars
			}
			existing.SSH = &ssh
		}
		if patch.K8s != nil {
			existing.K8s = patch.K8s
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
		// 管理员返回完整字段（管理界面需要 role/enabled）；普通用户仅返回用户目录，屏蔽 role/enabled 等敏感字段。
		if user, ok := currentSessionUser(r); ok && user.Role == "admin" {
			writeData(w, r, http.StatusOK, items)
			return
		}
		directory := make([]map[string]any, 0, len(items))
		for _, item := range items {
			directory = append(directory, map[string]any{
				"id":           item.ID,
				"username":     item.Username,
				"display_name": item.DisplayName,
			})
		}
		writeData(w, r, http.StatusOK, directory)
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

func patchUser(store repository.Store, cfg config.Config) http.HandlerFunc {
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
		next := existing
		if patch.DisplayName != nil {
			next.DisplayName = strings.TrimSpace(*patch.DisplayName)
		}
		if patch.Role != nil {
			next.Role = *patch.Role
		}
		if patch.Enabled != nil {
			next.Enabled = *patch.Enabled
		}
		actor, hasActor := currentSessionUser(r)
		if err := validateUserPatch(r.Context(), store, cfg, existing, next, actor, hasActor); err != nil {
			writeUserProtectionError(w, r, err)
			return
		}
		item, err := store.UpdateUser(r.Context(), existing.ID, next)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func resetUserPassword(store repository.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, err := store.GetUser(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		actor, ok := currentSessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", errUnauthorized)
			return
		}
		if err := validatePasswordReset(cfg, target, actor); err != nil {
			writeUserProtectionError(w, r, err)
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		if err := store.SetUserPassword(r.Context(), target.ID, hash); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]string{"id": target.ID})
	}
}

func deleteUser(store repository.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, err := store.GetUser(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, "not_found", err)
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		actor, hasActor := currentSessionUser(r)
		if err := validateUserDelete(r.Context(), store, cfg, target, actor, hasActor); err != nil {
			writeUserProtectionError(w, r, err)
			return
		}
		if err := store.DeleteUser(r.Context(), target.ID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, "not_found", err)
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]string{"id": target.ID})
	}
}

var (
	errProtectedUser     = errors.New("protected user cannot be modified")
	errLastAdminRequired = errors.New("at least one enabled admin is required")
	errDeleteSelf        = errors.New("current user cannot be deleted")
)

func validateUserPatch(ctx context.Context, store repository.Store, cfg config.Config, existing, next, actor domain.User, hasActor bool) error {
	if isProtectedUser(existing, cfg) {
		if !hasActor || !canModifyProtectedUser(existing, actor, cfg) {
			return errProtectedUser
		}
		if next.Role != "admin" || !next.Enabled {
			return errProtectedUser
		}
	}
	if existing.Role == "admin" && existing.Enabled && (next.Role != "admin" || !next.Enabled) {
		count, err := store.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return errLastAdminRequired
		}
	}
	return nil
}

func validatePasswordReset(cfg config.Config, target, actor domain.User) error {
	if isDemoProtected(target, cfg) {
		return errProtectedUser
	}
	if strings.EqualFold(target.Username, "admin") && !strings.EqualFold(actor.Username, "admin") {
		return errProtectedUser
	}
	return nil
}

func validateUserDelete(ctx context.Context, store repository.Store, cfg config.Config, target, actor domain.User, hasActor bool) error {
	if isProtectedUser(target, cfg) {
		return errProtectedUser
	}
	if hasActor && target.ID == actor.ID {
		return errDeleteSelf
	}
	if target.Role == "admin" && target.Enabled {
		count, err := store.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return errLastAdminRequired
		}
	}
	return nil
}

func canModifyProtectedUser(target, actor domain.User, cfg config.Config) bool {
	return strings.EqualFold(target.Username, "admin") && strings.EqualFold(actor.Username, "admin") && !isDemoProtected(target, cfg)
}

func isProtectedUser(user domain.User, cfg config.Config) bool {
	return strings.EqualFold(user.Username, "admin") || isDemoProtected(user, cfg)
}

func isDemoProtected(user domain.User, cfg config.Config) bool {
	if !cfg.DemoMode {
		return false
	}
	return protectedUsernames(cfg)[strings.ToLower(strings.TrimSpace(user.Username))]
}

func protectedUsernames(cfg config.Config) map[string]bool {
	items := map[string]bool{}
	for _, username := range strings.Split(cfg.DemoProtectedUsernames, ",") {
		username = strings.ToLower(strings.TrimSpace(username))
		if username != "" {
			items[username] = true
		}
	}
	return items
}

func writeUserProtectionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errProtectedUser):
		writeError(w, r, http.StatusForbidden, "protected_user", err)
	case errors.Is(err, errDeleteSelf):
		writeError(w, r, http.StatusForbidden, "forbidden", err)
	case errors.Is(err, errLastAdminRequired):
		writeError(w, r, http.StatusBadRequest, "last_admin_required", err)
	default:
		writeError(w, r, http.StatusInternalServerError, "internal_error", err)
	}
}

func apiKeyManager(w http.ResponseWriter, r *http.Request, jwtSecret string) (domain.User, bool) {
	if user, ok := currentSessionUser(r); ok {
		return user, true
	}
	if jwtSecret == "" && r.Header.Get("Authorization") == "" {
		return domain.User{Role: "admin"}, true
	}
	writeError(w, r, http.StatusUnauthorized, "unauthorized", errUnauthorized)
	return domain.User{}, false
}

func canManageAPIKey(user domain.User, key domain.APIKey) bool {
	return user.Role == "admin" || key.OwnerUserID == user.ID
}

func listAPIKeys(store repository.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := apiKeyManager(w, r, jwtSecret)
		if !ok {
			return
		}
		var (
			items []domain.APIKey
			err   error
		)
		if user.Role == "admin" {
			items, err = store.ListAPIKeys(r.Context())
		} else {
			items, err = store.ListAPIKeysByUser(r.Context(), user.ID)
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createAPIKey(store repository.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := apiKeyManager(w, r, jwtSecret)
		if !ok {
			return
		}
		var input domain.APIKey
		if !decodeJSON(w, r, &input) {
			return
		}
		if user.Role != "admin" {
			input.OwnerUserID = user.ID
		}
		if err := validateAPIKeyScopes(input.Scopes, user.Role == "admin"); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
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

func patchAPIKey(store repository.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := apiKeyManager(w, r, jwtSecret)
		if !ok {
			return
		}
		existing, err := store.GetAPIKey(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		if !canManageAPIKey(user, existing) {
			writeError(w, r, http.StatusForbidden, "forbidden", errForbidden)
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
			if err := validateAPIKeyScopes(*patch.Scopes, user.Role == "admin"); err != nil {
				writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
				return
			}
			if user.Role != "admin" && !scopesAreSubset(*patch.Scopes, existing.Scopes) {
				writeError(w, r, http.StatusForbidden, "forbidden", errors.New("non-admin api key scopes may only be reduced"))
				return
			}
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

var allowedAPIKeyScopes = map[string]bool{
	"inventory:read":   true,
	"release:read":     true,
	"release:create":   true,
	"release:confirm":  true,
	"release:rollback": true,
	"deploy:read":      true,
	"version:write":    true,
	"admin:write":      true,
}

func validateAPIKeyScopes(raw string, isAdmin bool) error {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return errors.New("scopes must be a JSON array")
	}
	if scopes == nil {
		return errors.New("scopes must be a JSON array")
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if !allowedAPIKeyScopes[scope] {
			return errors.New("unsupported api key scope")
		}
		if seen[scope] {
			return errors.New("duplicate api key scope")
		}
		if scope == "admin:write" && !isAdmin {
			return errors.New("non-admin cannot grant admin:write")
		}
		seen[scope] = true
	}
	return nil
}

func scopesAreSubset(nextRaw, currentRaw string) bool {
	var next, current []string
	if json.Unmarshal([]byte(nextRaw), &next) != nil || json.Unmarshal([]byte(currentRaw), &current) != nil {
		return false
	}
	allowed := make(map[string]bool, len(current))
	for _, scope := range current {
		allowed[scope] = true
	}
	for _, scope := range next {
		if !allowed[scope] {
			return false
		}
	}
	return true
}

func deleteAPIKey(store repository.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := apiKeyManager(w, r, jwtSecret)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if err := requireID(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		existing, err := store.GetAPIKey(r.Context(), id)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		if !canManageAPIKey(user, existing) {
			writeError(w, r, http.StatusForbidden, "forbidden", errForbidden)
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
