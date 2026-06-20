package repository

import (
	"context"
	"database/sql"

	"ai-pub/internal/domain"
)

type CredentialSecret struct {
	Credential domain.Credential
	Secret     string
}

func (s Store) CreateCredential(ctx context.Context, item domain.Credential, secretEnc string) (domain.Credential, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("cred")
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO credentials (id, name, type, secret_enc, description, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Type, secretEnc, item.Description, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListCredentials(ctx context.Context) ([]domain.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, type, description, enabled, created_at, updated_at
FROM credentials ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Credential{}
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetCredential(ctx context.Context, id string) (domain.Credential, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, type, description, enabled, created_at, updated_at
FROM credentials WHERE id = ?`, id)
	item, err := scanCredential(row)
	return item, normalizeNotFound(err)
}

func (s Store) GetCredentialSecret(ctx context.Context, id string) (CredentialSecret, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, type, secret_enc, description, enabled, created_at, updated_at
FROM credentials WHERE id = ? AND enabled = 1`, id)
	var item domain.Credential
	var secretEnc string
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Type, &secretEnc, &item.Description, &enabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return CredentialSecret{}, ErrNotFound
	}
	if err != nil {
		return CredentialSecret{}, err
	}
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return CredentialSecret{Credential: item, Secret: secretEnc}, nil
}

func scanCredential(row rowScanner) (domain.Credential, error) {
	var item domain.Credential
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Name, &item.Type, &item.Description, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}
