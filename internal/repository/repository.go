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
INSERT INTO service_versions (id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, registration_idempotency_key, registration_request_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceID, item.Version, item.CommitSHA, item.ArtifactURL, item.Source, item.Metadata, item.CreatedByType, item.CreatedByID, nullString(item.RegistrationIdempotencyKey), nullString(item.RegistrationRequestHash), formatTime(item.CreatedAt))
	return item, err
}

func (s Store) ListServiceVersions(ctx context.Context, serviceID string) ([]domain.ServiceVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, registration_idempotency_key, registration_request_hash, created_at
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
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO environments (id, name, slug, is_production, release_frozen, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Slug, boolInt(item.IsProduction), boolInt(item.ReleaseFrozen), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListEnvironments(ctx context.Context) ([]domain.Environment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, slug, is_production, release_frozen, created_at, updated_at
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
SELECT id, name, slug, is_production, release_frozen, created_at, updated_at FROM environments WHERE id = ?`, id)
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
	existing.ReleaseFrozen = item.ReleaseFrozen
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE environments SET name = ?, slug = ?, is_production = ?, release_frozen = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Slug, boolInt(existing.IsProduction), boolInt(existing.ReleaseFrozen), formatTime(existing.UpdatedAt), id)
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
	if item.Role == "" {
		item.Role = "application"
	}
	if err := s.validateServerGateway(ctx, item); err != nil {
		return domain.Server{}, err
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO servers (id, name, host, port, username, auth_type, credential_ref, role, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Host, item.Port, item.Username, item.AuthType, item.CredentialRef, item.Role, item.GatewayID, boolInt(item.Enabled), item.LastCheckStatus, nullableTimePtr(item.LastCheckAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListServers(ctx context.Context) ([]domain.Server, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, host, port, username, auth_type, credential_ref, role, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at
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
SELECT id, name, host, port, username, auth_type, credential_ref, role, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at
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
	existing.Role = choose(item.Role, existing.Role)
	existing.GatewayID = item.GatewayID
	existing.Enabled = item.Enabled
	if err := s.validateServerGateway(ctx, existing); err != nil {
		return domain.Server{}, err
	}
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE servers SET name = ?, host = ?, port = ?, username = ?, auth_type = ?, credential_ref = ?, role = ?, gateway_id = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Host, existing.Port, existing.Username, existing.AuthType, existing.CredentialRef, existing.Role, existing.GatewayID, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	return existing, err
}

func (s Store) validateServerGateway(ctx context.Context, item domain.Server) error {
	if item.Role != "gateway" && item.Role != "application" {
		return fmt.Errorf("server role must be gateway or application")
	}
	if item.Role == "gateway" && item.GatewayID != "" {
		return fmt.Errorf("gateway server cannot use another gateway")
	}
	if item.GatewayID == "" {
		var dependentCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers WHERE gateway_id = ? AND id <> ?`, item.ID, item.ID).Scan(&dependentCount); err != nil {
			return err
		}
		if dependentCount > 0 {
			if item.Role != "gateway" {
				return fmt.Errorf("gateway server is used by application servers and must remain a gateway")
			}
			if !item.Enabled {
				return fmt.Errorf("gateway server is used by application servers and cannot be disabled")
			}
		}
		return nil
	}
	if item.GatewayID == item.ID {
		return fmt.Errorf("server cannot use itself as gateway")
	}
	gateway, err := s.GetServer(ctx, item.GatewayID)
	if err != nil {
		return fmt.Errorf("gateway server is not available: %w", err)
	}
	if gateway.Role != "gateway" {
		return fmt.Errorf("selected server is not a gateway")
	}
	if !gateway.Enabled {
		return fmt.Errorf("gateway server is disabled")
	}
	if item.Role != "application" {
		return fmt.Errorf("only application server can use a gateway")
	}
	return nil
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
	if item.ArtifactType == "" {
		item.ArtifactType = "version_only"
	}
	if item.SSH == nil && (item.TargetType != "" || item.TargetRefID != "" || item.ScriptPath != "" || item.WorkingDir != "" || item.EnvVars != "") {
		item.SSH = &domain.SSHDeploymentTarget{
			TargetType:  item.TargetType,
			TargetRefID: item.TargetRefID,
			ScriptPath:  item.ScriptPath,
			WorkingDir:  item.WorkingDir,
			EnvVars:     item.EnvVars,
		}
	}
	if item.ExecutorType == "ssh" {
		if item.SSH == nil {
			return domain.DeploymentTarget{}, fmt.Errorf("ssh deployment target config is required")
		}
		if item.SSH.EnvVars == "" {
			item.SSH.EnvVars = "{}"
		}
		item.SSH.DeploymentTargetID = item.ID
	}
	if item.ExecutorType == "k8s" {
		if item.K8s == nil {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target config is required")
		}
		if item.ArtifactType != "oci_image" {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target requires artifact_type oci_image")
		}
		if strings.TrimSpace(item.K8s.ClusterID) == "" || strings.TrimSpace(item.K8s.Namespace) == "" || strings.TrimSpace(item.K8s.DeploymentName) == "" || strings.TrimSpace(item.K8s.ContainerName) == "" {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target requires cluster_id, namespace, deployment_name and container_name")
		}
		item.K8s.DeploymentTargetID = item.ID
		if _, err := s.GetK8sCluster(ctx, item.K8s.ClusterID); err != nil {
			return domain.DeploymentTarget{}, err
		}
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeploymentTarget{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO deployment_targets (id, service_id, environment_id, executor_type, artifact_type, timeout_seconds, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceID, item.EnvironmentID, item.ExecutorType, item.ArtifactType, item.TimeoutSeconds, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt)); err != nil {
		return domain.DeploymentTarget{}, err
	}
	if item.SSH != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ssh_deployment_targets (deployment_target_id, target_type, target_ref_id, script_path, working_dir, env_vars)
VALUES (?, ?, ?, ?, ?, ?)`,
			item.ID, item.SSH.TargetType, item.SSH.TargetRefID, item.SSH.ScriptPath, item.SSH.WorkingDir, item.SSH.EnvVars); err != nil {
			return domain.DeploymentTarget{}, err
		}
	}
	if item.K8s != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO k8s_deployment_targets (deployment_target_id, cluster_id, namespace, deployment_name, container_name)
VALUES (?, ?, ?, ?, ?)`,
			item.ID, item.K8s.ClusterID, item.K8s.Namespace, item.K8s.DeploymentName, item.K8s.ContainerName); err != nil {
			return domain.DeploymentTarget{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.DeploymentTarget{}, err
	}
	return item, nil
}

func (s Store) ListDeploymentTargets(ctx context.Context, serviceID string, environmentID string) ([]domain.DeploymentTarget, error) {
	query := `
SELECT id, service_id, environment_id, executor_type, artifact_type, timeout_seconds, enabled, created_at, updated_at
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := s.attachDeploymentTargetConfig(ctx, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s Store) GetDeploymentTarget(ctx context.Context, id string) (domain.DeploymentTarget, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, service_id, environment_id, executor_type, artifact_type, timeout_seconds, enabled, created_at, updated_at
FROM deployment_targets WHERE id = ?`, id)
	item, err := scanDeploymentTarget(row)
	if err != nil {
		return item, normalizeNotFound(err)
	}
	if err := s.attachDeploymentTargetConfig(ctx, &item); err != nil {
		return domain.DeploymentTarget{}, err
	}
	return item, nil
}

func (s Store) UpdateDeploymentTarget(ctx context.Context, id string, item domain.DeploymentTarget) (domain.DeploymentTarget, error) {
	existing, err := s.GetDeploymentTarget(ctx, id)
	if err != nil {
		return domain.DeploymentTarget{}, err
	}
	existing.ExecutorType = choose(item.ExecutorType, existing.ExecutorType)
	existing.ArtifactType = choose(item.ArtifactType, existing.ArtifactType)
	if item.SSH == nil && (item.TargetType != "" || item.TargetRefID != "" || item.ScriptPath != "" || item.WorkingDir != "" || item.EnvVars != "") {
		item.SSH = &domain.SSHDeploymentTarget{
			TargetType:  item.TargetType,
			TargetRefID: item.TargetRefID,
			ScriptPath:  item.ScriptPath,
			WorkingDir:  item.WorkingDir,
			EnvVars:     item.EnvVars,
		}
	}
	if item.SSH != nil {
		if item.SSH.EnvVars == "" {
			item.SSH.EnvVars = "{}"
		}
		item.SSH.DeploymentTargetID = id
		existing.SSH = item.SSH
	}
	if item.K8s != nil {
		if strings.TrimSpace(item.K8s.ClusterID) == "" || strings.TrimSpace(item.K8s.Namespace) == "" || strings.TrimSpace(item.K8s.DeploymentName) == "" || strings.TrimSpace(item.K8s.ContainerName) == "" {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target requires cluster_id, namespace, deployment_name and container_name")
		}
		item.K8s.DeploymentTargetID = id
		if _, err := s.GetK8sCluster(ctx, item.K8s.ClusterID); err != nil {
			return domain.DeploymentTarget{}, err
		}
		existing.K8s = item.K8s
	}
	if existing.ExecutorType == "k8s" {
		if existing.ArtifactType != "oci_image" {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target requires artifact_type oci_image")
		}
		if existing.K8s == nil {
			return domain.DeploymentTarget{}, fmt.Errorf("k8s deployment target config is required")
		}
	}
	if existing.ExecutorType == "ssh" && existing.SSH == nil {
		return domain.DeploymentTarget{}, fmt.Errorf("ssh deployment target config is required")
	}
	if item.TimeoutSeconds != 0 {
		existing.TimeoutSeconds = item.TimeoutSeconds
	}
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeploymentTarget{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE deployment_targets
SET executor_type = ?, artifact_type = ?, timeout_seconds = ?, enabled = ?, updated_at = ?
WHERE id = ?`,
		existing.ExecutorType, existing.ArtifactType, existing.TimeoutSeconds, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id); err != nil {
		return domain.DeploymentTarget{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_deployment_targets WHERE deployment_target_id = ?`, id); err != nil {
		return domain.DeploymentTarget{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM k8s_deployment_targets WHERE deployment_target_id = ?`, id); err != nil {
		return domain.DeploymentTarget{}, err
	}
	if existing.ExecutorType == "ssh" && existing.SSH != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ssh_deployment_targets (deployment_target_id, target_type, target_ref_id, script_path, working_dir, env_vars)
VALUES (?, ?, ?, ?, ?, ?)`,
			id, existing.SSH.TargetType, existing.SSH.TargetRefID, existing.SSH.ScriptPath, existing.SSH.WorkingDir, existing.SSH.EnvVars); err != nil {
			return domain.DeploymentTarget{}, err
		}
	}
	if existing.ExecutorType == "k8s" && existing.K8s != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO k8s_deployment_targets (deployment_target_id, cluster_id, namespace, deployment_name, container_name)
VALUES (?, ?, ?, ?, ?)`,
			id, existing.K8s.ClusterID, existing.K8s.Namespace, existing.K8s.DeploymentName, existing.K8s.ContainerName); err != nil {
			return domain.DeploymentTarget{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.DeploymentTarget{}, err
	}
	return existing, nil
}

func (s Store) attachDeploymentTargetConfig(ctx context.Context, item *domain.DeploymentTarget) error {
	row := s.db.QueryRowContext(ctx, `
SELECT deployment_target_id, target_type, target_ref_id, script_path, working_dir, env_vars
FROM ssh_deployment_targets WHERE deployment_target_id = ?`, item.ID)
	var ssh domain.SSHDeploymentTarget
	if err := row.Scan(&ssh.DeploymentTargetID, &ssh.TargetType, &ssh.TargetRefID, &ssh.ScriptPath, &ssh.WorkingDir, &ssh.EnvVars); err != nil {
		if item.ExecutorType == "ssh" {
			return normalizeNotFound(err)
		}
	} else {
		item.SSH = &ssh
	}
	row = s.db.QueryRowContext(ctx, `
SELECT deployment_target_id, cluster_id, namespace, deployment_name, container_name
FROM k8s_deployment_targets WHERE deployment_target_id = ?`, item.ID)
	var k8s domain.K8sDeploymentTarget
	if err := row.Scan(&k8s.DeploymentTargetID, &k8s.ClusterID, &k8s.Namespace, &k8s.DeploymentName, &k8s.ContainerName); err != nil {
		if item.ExecutorType == "k8s" {
			return normalizeNotFound(err)
		}
		return nil
	}
	item.K8s = &k8s
	return nil
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

func (s Store) CountEnabledAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1`).Scan(&count)
	return count, err
}

func (s Store) SetUserPassword(ctx context.Context, id, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET password_hash = ?, session_version = session_version + 1, updated_at = ? WHERE id = ?`, passwordHash, formatTime(nowUTC()), id)
	return err
}

func (s Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_keys WHERE owner_type = 'user' AND owner_id = ?`, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
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
	return tx.Commit()
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
		item.ID, item.Name, item.Prefix, hash, "user", item.OwnerUserID, item.Scopes, nullableTimePtr(item.ExpiresAt), boolInt(item.Enabled), nullableTimePtr(item.LastUsedAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	if err != nil {
		return APIKeyWithPlaintext{}, err
	}
	return APIKeyWithPlaintext{Key: item, Plaintext: plaintext}, nil
}

func (s Store) ListAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, prefix, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
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

func (s Store) ListAPIKeysByUser(ctx context.Context, userID string) ([]domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, prefix, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys WHERE owner_type = 'user' AND owner_id = ? ORDER BY created_at DESC, id DESC`, userID)
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
SELECT id, name, prefix, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
FROM api_keys WHERE id = ?`, id)
	item, err := scanAPIKey(row)
	return item, normalizeNotFound(err)
}

func (s Store) GetAPIKeyBySecret(ctx context.Context, plaintext string) (domain.APIKey, error) {
	if len(plaintext) < 12 {
		return domain.APIKey{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, prefix, owner_id, scopes, expires_at, enabled, last_used_at, created_at, updated_at
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
	var idempotencyKey, requestHash sql.NullString
	err := row.Scan(&item.ID, &item.ServiceID, &item.Version, &item.CommitSHA, &item.ArtifactURL, &item.Source, &item.Metadata, &item.CreatedByType, &item.CreatedByID, &idempotencyKey, &requestHash, &createdAt)
	item.RegistrationIdempotencyKey = nullStringValue(idempotencyKey)
	item.RegistrationRequestHash = nullStringValue(requestHash)
	item.CreatedAt = parseTime(createdAt)
	return item, err
}

func scanEnvironment(row rowScanner) (domain.Environment, error) {
	var item domain.Environment
	var isProduction, releaseFrozen int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Slug, &isProduction, &releaseFrozen, &createdAt, &updatedAt)
	item.IsProduction = isProduction == 1
	item.ReleaseFrozen = releaseFrozen == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanServer(row rowScanner) (domain.Server, error) {
	var item domain.Server
	var enabled int
	var lastCheckAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.AuthType, &item.CredentialRef, &item.Role, &item.GatewayID, &enabled, &item.LastCheckStatus, &lastCheckAt, &createdAt, &updatedAt)
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
	err := row.Scan(&item.ID, &item.ServiceID, &item.EnvironmentID, &item.ExecutorType, &item.ArtifactType, &item.TimeoutSeconds, &enabled, &createdAt, &updatedAt)
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
	err := row.Scan(&item.ID, &item.Name, &item.Prefix, &item.OwnerUserID, &item.Scopes, &expiresAt, &enabled, &lastUsedAt, &createdAt, &updatedAt)
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

// isMySQL 判断底层驱动是否为 MySQL，用于选择 MySQL 专有语法（如 FOR UPDATE 行锁）。
func isMySQL(db *sql.DB) bool {
	if db == nil {
		return false
	}
	return strings.Contains(fmt.Sprintf("%T", db.Driver()), "mysql")
}
