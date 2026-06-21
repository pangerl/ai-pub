package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-pub/internal/domain"
)

type Store struct {
	db *sql.DB
}

type APIKeyWithPlaintext struct {
	Key       domain.APIKey `json:"key"`
	Plaintext string        `json:"plaintext"`
}

var ErrNotFound = errors.New("not found")

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	now := nowUTC()
	if p.ID == "" {
		p.ID = domain.NewID("proj")
	}
	p.Enabled = true
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (id, name, slug, description, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Slug, p.Description, boolInt(p.Enabled), formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	return p, err
}

func (s Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, slug, description, enabled, created_at, updated_at
FROM projects ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Project{}
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetProject(ctx context.Context, id string) (domain.Project, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, slug, description, enabled, created_at, updated_at FROM projects WHERE id = ?`, id)
	item, err := scanProject(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateProject(ctx context.Context, id string, p domain.Project) (domain.Project, error) {
	existing, err := s.GetProject(ctx, id)
	if err != nil {
		return domain.Project{}, err
	}
	existing.Name = choose(p.Name, existing.Name)
	existing.Slug = choose(p.Slug, existing.Slug)
	existing.Description = p.Description
	existing.Enabled = p.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE projects SET name = ?, slug = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Slug, existing.Description, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) CreateService(ctx context.Context, item domain.Service) (domain.Service, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("svc")
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO services (id, project_id, name, slug, description, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ProjectID, item.Name, item.Slug, item.Description, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListServices(ctx context.Context, projectID string) ([]domain.Service, error) {
	query := `
SELECT id, project_id, name, slug, description, enabled, created_at, updated_at FROM services`
	var args []any
	if projectID != "" {
		query += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Service{}
	for rows.Next() {
		item, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetService(ctx context.Context, id string) (domain.Service, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, project_id, name, slug, description, enabled, created_at, updated_at FROM services WHERE id = ?`, id)
	item, err := scanService(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateService(ctx context.Context, id string, item domain.Service) (domain.Service, error) {
	existing, err := s.GetService(ctx, id)
	if err != nil {
		return domain.Service{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Slug = choose(item.Slug, existing.Slug)
	existing.Description = item.Description
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE services SET name = ?, slug = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Slug, existing.Description, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) CreateServiceVersion(ctx context.Context, item domain.ServiceVersion) (domain.ServiceVersion, error) {
	if item.ID == "" {
		item.ID = domain.NewID("ver")
	}
	if item.Source == "" {
		item.Source = "manual"
	}
	if item.Metadata == "" {
		item.Metadata = "{}"
	}
	item.CreatedAt = nowUTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO service_versions (id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceID, item.Version, item.CommitSHA, item.ArtifactURL, item.Source, item.Metadata, item.CreatedByType, item.CreatedByID, formatTime(item.CreatedAt))
	return item, err
}

func (s Store) ListServiceVersions(ctx context.Context, serviceID string) ([]domain.ServiceVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, created_at
FROM service_versions WHERE service_id = ? ORDER BY created_at DESC, id DESC`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ServiceVersion{}
	for rows.Next() {
		item, err := scanServiceVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) CreateEnvironment(ctx context.Context, item domain.Environment) (domain.Environment, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("env")
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO environments (id, name, slug, is_production, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Slug, boolInt(item.IsProduction), boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListEnvironments(ctx context.Context) ([]domain.Environment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, slug, is_production, enabled, created_at, updated_at
FROM environments ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Environment{}
	for rows.Next() {
		item, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetEnvironment(ctx context.Context, id string) (domain.Environment, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, slug, is_production, enabled, created_at, updated_at FROM environments WHERE id = ?`, id)
	item, err := scanEnvironment(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateEnvironment(ctx context.Context, id string, item domain.Environment) (domain.Environment, error) {
	existing, err := s.GetEnvironment(ctx, id)
	if err != nil {
		return domain.Environment{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Slug = choose(item.Slug, existing.Slug)
	existing.IsProduction = item.IsProduction
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE environments SET name = ?, slug = ?, is_production = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Slug, boolInt(existing.IsProduction), boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) CreateServer(ctx context.Context, item domain.Server) (domain.Server, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("srv")
	}
	if item.Port == 0 {
		item.Port = 22
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO servers (id, name, host, port, username, auth_type, credential_ref, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Host, item.Port, item.Username, item.AuthType, item.CredentialRef, item.GatewayID, boolInt(item.Enabled), item.LastCheckStatus, nullableTimePtr(item.LastCheckAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListServers(ctx context.Context) ([]domain.Server, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, host, port, username, auth_type, credential_ref, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at
FROM servers ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Server{}
	for rows.Next() {
		item, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetServer(ctx context.Context, id string) (domain.Server, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, host, port, username, auth_type, credential_ref, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at
FROM servers WHERE id = ?`, id)
	item, err := scanServer(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateServer(ctx context.Context, id string, item domain.Server) (domain.Server, error) {
	existing, err := s.GetServer(ctx, id)
	if err != nil {
		return domain.Server{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Host = choose(item.Host, existing.Host)
	if item.Port != 0 {
		existing.Port = item.Port
	}
	existing.Username = choose(item.Username, existing.Username)
	existing.AuthType = choose(item.AuthType, existing.AuthType)
	existing.CredentialRef = item.CredentialRef
	existing.GatewayID = item.GatewayID
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE servers SET name = ?, host = ?, port = ?, username = ?, auth_type = ?, credential_ref = ?, gateway_id = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Host, existing.Port, existing.Username, existing.AuthType, existing.CredentialRef, existing.GatewayID, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) UpdateServerCheck(ctx context.Context, id, status string) (domain.Server, error) {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `UPDATE servers SET last_check_status = ?, last_check_at = ?, updated_at = ? WHERE id = ?`, status, formatTime(now), formatTime(now), id)
	if err != nil {
		return domain.Server{}, err
	}
	return s.GetServer(ctx, id)
}

func (s Store) CreateServerGroup(ctx context.Context, item domain.ServerGroup) (domain.ServerGroup, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("sg")
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ServerGroup{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO server_groups (id, name, description, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Description, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt)); err != nil {
		return domain.ServerGroup{}, err
	}
	for _, serverID := range item.ServerIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO server_group_members (server_group_id, server_id) VALUES (?, ?)`, item.ID, serverID); err != nil {
			return domain.ServerGroup{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.ServerGroup{}, err
	}
	return item, nil
}

func (s Store) ListServerGroups(ctx context.Context) ([]domain.ServerGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, description, enabled, created_at, updated_at
FROM server_groups ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	items := []domain.ServerGroup{}
	for rows.Next() {
		item, err := scanServerGroup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		memberIDs, err := s.listServerGroupMembers(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].ServerIDs = memberIDs
	}
	return items, nil
}

func (s Store) GetServerGroup(ctx context.Context, id string) (domain.ServerGroup, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, enabled, created_at, updated_at FROM server_groups WHERE id = ?`, id)
	item, err := scanServerGroup(row)
	if err != nil {
		return domain.ServerGroup{}, normalizeNotFound(err)
	}
	item.ServerIDs, err = s.listServerGroupMembers(ctx, id)
	return item, err
}

func (s Store) UpdateServerGroup(ctx context.Context, id string, item domain.ServerGroup) (domain.ServerGroup, error) {
	existing, err := s.GetServerGroup(ctx, id)
	if err != nil {
		return domain.ServerGroup{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Description = item.Description
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ServerGroup{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE server_groups SET name = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`, existing.Name, existing.Description, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id); err != nil {
		return domain.ServerGroup{}, err
	}
	if item.ServerIDs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM server_group_members WHERE server_group_id = ?`, id); err != nil {
			return domain.ServerGroup{}, err
		}
		for _, serverID := range item.ServerIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO server_group_members (server_group_id, server_id) VALUES (?, ?)`, id, serverID); err != nil {
				return domain.ServerGroup{}, err
			}
		}
		existing.ServerIDs = item.ServerIDs
	}
	if err := tx.Commit(); err != nil {
		return domain.ServerGroup{}, err
	}
	return existing, nil
}

func (s Store) listServerGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT server_id FROM server_group_members WHERE server_group_id = ? ORDER BY server_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s Store) CreateDeploymentTarget(ctx context.Context, item domain.DeploymentTarget) (domain.DeploymentTarget, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("target")
	}
	if item.TimeoutSeconds == 0 {
		item.TimeoutSeconds = 300
	}
	if item.EnvVars == "" {
		item.EnvVars = "{}"
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO deployment_targets (id, service_id, environment_id, executor_type, target_type, target_ref_id, script_path, working_dir, env_vars, timeout_seconds, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceID, item.EnvironmentID, item.ExecutorType, item.TargetType, item.TargetRefID, item.ScriptPath, item.WorkingDir, item.EnvVars, item.TimeoutSeconds, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListDeploymentTargets(ctx context.Context, serviceID string, environmentID string) ([]domain.DeploymentTarget, error) {
	query := `
SELECT id, service_id, environment_id, executor_type, target_type, target_ref_id, script_path, working_dir, env_vars, timeout_seconds, enabled, created_at, updated_at
FROM deployment_targets`
	var clauses []string
	var args []any
	if serviceID != "" {
		clauses = append(clauses, "service_id = ?")
		args = append(args, serviceID)
	}
	if environmentID != "" {
		clauses = append(clauses, "environment_id = ?")
		args = append(args, environmentID)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DeploymentTarget{}
	for rows.Next() {
		item, err := scanDeploymentTarget(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetDeploymentTarget(ctx context.Context, id string) (domain.DeploymentTarget, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, service_id, environment_id, executor_type, target_type, target_ref_id, script_path, working_dir, env_vars, timeout_seconds, enabled, created_at, updated_at
FROM deployment_targets WHERE id = ?`, id)
	item, err := scanDeploymentTarget(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateDeploymentTarget(ctx context.Context, id string, item domain.DeploymentTarget) (domain.DeploymentTarget, error) {
	existing, err := s.GetDeploymentTarget(ctx, id)
	if err != nil {
		return domain.DeploymentTarget{}, err
	}
	existing.ExecutorType = choose(item.ExecutorType, existing.ExecutorType)
	existing.TargetType = choose(item.TargetType, existing.TargetType)
	existing.TargetRefID = choose(item.TargetRefID, existing.TargetRefID)
	existing.ScriptPath = item.ScriptPath
	existing.WorkingDir = item.WorkingDir
	existing.EnvVars = choose(item.EnvVars, existing.EnvVars)
	if item.TimeoutSeconds != 0 {
		existing.TimeoutSeconds = item.TimeoutSeconds
	}
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE deployment_targets
SET executor_type = ?, target_type = ?, target_ref_id = ?, script_path = ?, working_dir = ?, env_vars = ?, timeout_seconds = ?, enabled = ?, updated_at = ?
WHERE id = ?`,
		existing.ExecutorType, existing.TargetType, existing.TargetRefID, existing.ScriptPath, existing.WorkingDir, existing.EnvVars, existing.TimeoutSeconds, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) CreateUser(ctx context.Context, item domain.User) (domain.User, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("user")
	}
	if item.Role == "" {
		item.Role = "employee"
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (id, username, display_name, role, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Username, item.DisplayName, item.Role, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) CreateUserWithPassword(ctx context.Context, item domain.User) (domain.User, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("user")
	}
	if item.Role == "" {
		item.Role = "employee"
	}
	item.Enabled = true
	item.SessionVersion = 1
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (id, username, display_name, role, enabled, password_hash, session_version, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Username, item.DisplayName, item.Role, boolInt(item.Enabled), item.PasswordHash, item.SessionVersion, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, display_name, role, enabled, created_at, updated_at
FROM users ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.User{}
	for rows.Next() {
		item, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) UpdateUser(ctx context.Context, id string, item domain.User) (domain.User, error) {
	existing, err := s.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	existing.DisplayName = choose(item.DisplayName, existing.DisplayName)
	existing.Role = choose(item.Role, existing.Role)
	existing.Enabled = item.Enabled
	existing.SessionVersion++
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE users SET display_name = ?, role = ?, enabled = ?, session_version = session_version + 1, updated_at = ? WHERE id = ?`,
		existing.DisplayName, existing.Role, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, display_name, role, enabled, password_hash, session_version, created_at, updated_at
FROM users WHERE username = ?`, username)
	item, err := scanUserWithCredentials(row)
	return item, normalizeNotFound(err)
}

func (s Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s Store) SetUserPassword(ctx context.Context, id, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET password_hash = ?, session_version = session_version + 1, updated_at = ? WHERE id = ?`, passwordHash, formatTime(nowUTC()), id)
	return err
}

func (s Store) CreateAPIKey(ctx context.Context, item domain.APIKey) (APIKeyWithPlaintext, error) {
	plaintext, err := newAPIKeySecret()
	if err != nil {
		return APIKeyWithPlaintext{}, err
	}
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("key")
	}
	if item.Scopes == "" {
		item.Scopes = "[]"
	}
	item.Prefix = plaintext[:12]
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	hash := hashSecret(plaintext)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO api_keys (id, name, prefix, key_hash, owner_type, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Prefix, hash, item.OwnerType, item.OwnerID, item.Scopes, nullableTimePtr(item.ExpiresAt), boolInt(item.Enabled), nullableTimePtr(item.LastUsedAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	if err != nil {
		return APIKeyWithPlaintext{}, err
	}
	return APIKeyWithPlaintext{Key: item, Plaintext: plaintext}, nil
}

func (s Store) ListAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, prefix, owner_type, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.APIKey{}
	for rows.Next() {
		item, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) ListAPIKeysByOwner(ctx context.Context, ownerType, ownerID string) ([]domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, prefix, owner_type, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys WHERE owner_type = ? AND owner_id = ? ORDER BY created_at DESC, id DESC`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.APIKey{}
	for rows.Next() {
		item, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetAPIKey(ctx context.Context, id string) (domain.APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, prefix, owner_type, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys WHERE id = ?`, id)
	item, err := scanAPIKey(row)
	return item, normalizeNotFound(err)
}

func (s Store) GetAPIKeyBySecret(ctx context.Context, plaintext string) (domain.APIKey, error) {
	if len(plaintext) < 12 {
		return domain.APIKey{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, prefix, owner_type, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys WHERE prefix = ? AND key_hash = ?`, plaintext[:12], hashSecret(plaintext))
	item, err := scanAPIKey(row)
	return item, normalizeNotFound(err)
}

func (s Store) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ?, updated_at = ? WHERE id = ?`, formatTime(now), formatTime(now), id)
	return err
}

func (s Store) UpdateAPIKey(ctx context.Context, id string, item domain.APIKey) (domain.APIKey, error) {
	existing, err := s.GetAPIKey(ctx, id)
	if err != nil {
		return domain.APIKey{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Scopes = choose(item.Scopes, existing.Scopes)
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE api_keys SET name = ?, scopes = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Scopes, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) DeleteAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(row rowScanner) (domain.Project, error) {
	var item domain.Project
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Slug, &item.Description, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanService(row rowScanner) (domain.Service, error) {
	var item domain.Service
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Slug, &item.Description, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanServiceVersion(row rowScanner) (domain.ServiceVersion, error) {
	var item domain.ServiceVersion
	var createdAt string
	err := row.Scan(&item.ID, &item.ServiceID, &item.Version, &item.CommitSHA, &item.ArtifactURL, &item.Source, &item.Metadata, &item.CreatedByType, &item.CreatedByID, &createdAt)
	item.CreatedAt = parseTime(createdAt)
	return item, err
}

func scanEnvironment(row rowScanner) (domain.Environment, error) {
	var item domain.Environment
	var isProduction, enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Slug, &isProduction, &enabled, &createdAt, &updatedAt)
	item.IsProduction = isProduction == 1
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanServer(row rowScanner) (domain.Server, error) {
	var item domain.Server
	var enabled int
	var lastCheckAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.AuthType, &item.CredentialRef, &item.GatewayID, &enabled, &item.LastCheckStatus, &lastCheckAt, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	if lastCheckAt.Valid {
		t := parseTime(lastCheckAt.String)
		item.LastCheckAt = &t
	}
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanServerGroup(row rowScanner) (domain.ServerGroup, error) {
	var item domain.ServerGroup
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Description, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanDeploymentTarget(row rowScanner) (domain.DeploymentTarget, error) {
	var item domain.DeploymentTarget
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.ServiceID, &item.EnvironmentID, &item.ExecutorType, &item.TargetType, &item.TargetRefID, &item.ScriptPath, &item.WorkingDir, &item.EnvVars, &item.TimeoutSeconds, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanUser(row rowScanner) (domain.User, error) {
	var item domain.User
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Username, &item.DisplayName, &item.Role, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanUserWithCredentials(row rowScanner) (domain.User, error) {
	var item domain.User
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Username, &item.DisplayName, &item.Role, &enabled, &item.PasswordHash, &item.SessionVersion, &createdAt, &updatedAt)
	if err != nil {
		return domain.User{}, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func scanAPIKey(row rowScanner) (domain.APIKey, error) {
	var item domain.APIKey
	var enabled int
	var expiresAt, lastUsedAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Prefix, &item.OwnerType, &item.OwnerID, &item.Scopes, &expiresAt, &enabled, &lastUsedAt, &createdAt, &updatedAt)
	if expiresAt.Valid {
		t := parseTime(expiresAt.String)
		item.ExpiresAt = &t
	}
	if lastUsedAt.Valid {
		t := parseTime(lastUsedAt.String)
		item.LastUsedAt = &t
	}
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func normalizeNotFound(err error) error {
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return err
}

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func nullableTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func choose(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func newAPIKeySecret() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "ak_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
