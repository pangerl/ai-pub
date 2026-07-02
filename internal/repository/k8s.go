package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ai-pub/internal/domain"
)

var ErrK8sClusterInUse = errors.New("k8s cluster is still referenced by deployment targets")

func (s Store) CreateK8sCluster(ctx context.Context, item domain.K8sCluster) (domain.K8sCluster, error) {
	if err := s.validateK8sClusterCredential(ctx, item.CredentialRef); err != nil {
		return domain.K8sCluster{}, err
	}
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("k8s")
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO k8s_clusters (id, name, credential_ref, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.CredentialRef, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListK8sClusters(ctx context.Context) ([]domain.K8sCluster, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, credential_ref, enabled, created_at, updated_at
FROM k8s_clusters ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.K8sCluster{}
	for rows.Next() {
		item, err := scanK8sCluster(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetK8sCluster(ctx context.Context, id string) (domain.K8sCluster, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, credential_ref, enabled, created_at, updated_at
FROM k8s_clusters WHERE id = ?`, id)
	item, err := scanK8sCluster(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateK8sCluster(ctx context.Context, id string, item domain.K8sCluster) (domain.K8sCluster, error) {
	existing, err := s.GetK8sCluster(ctx, id)
	if err != nil {
		return domain.K8sCluster{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	if item.CredentialRef != "" {
		if err := s.validateK8sClusterCredential(ctx, item.CredentialRef); err != nil {
			return domain.K8sCluster{}, err
		}
		existing.CredentialRef = item.CredentialRef
	}
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE k8s_clusters SET name = ?, credential_ref = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.CredentialRef, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	if err != nil {
		return domain.K8sCluster{}, err
	}
	return existing, nil
}

func (s Store) DeleteK8sCluster(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	countSQL := `SELECT COUNT(*) FROM k8s_deployment_targets WHERE cluster_id = ?`
	if isMySQL(s.db) {
		countSQL = `SELECT COUNT(*) FROM k8s_deployment_targets WHERE cluster_id = ? FOR UPDATE`
	}
	var count int
	if err := tx.QueryRowContext(ctx, countSQL, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrK8sClusterInUse
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM k8s_clusters WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s Store) validateK8sClusterCredential(ctx context.Context, credentialRef string) error {
	if credentialRef == "" {
		return fmt.Errorf("kubeconfig credential is required")
	}
	credential, err := s.GetCredential(ctx, credentialRef)
	if err != nil {
		return fmt.Errorf("kubeconfig credential is not available: %w", err)
	}
	if credential.Type != "kubeconfig" {
		return fmt.Errorf("credential type must be kubeconfig")
	}
	if !credential.Enabled {
		return fmt.Errorf("kubeconfig credential is disabled")
	}
	return nil
}

func scanK8sCluster(row rowScanner) (domain.K8sCluster, error) {
	var item domain.K8sCluster
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.CredentialRef, &enabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return domain.K8sCluster{}, ErrNotFound
	}
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}
