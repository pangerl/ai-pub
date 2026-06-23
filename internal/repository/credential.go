package repository

import (
	"context"
	"database/sql"
	"errors"

	"ai-pub/internal/domain"
)

// ErrCredentialInUse 表示凭据仍被服务器引用，不能删除。
var ErrCredentialInUse = errors.New("credential is still referenced by servers")

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

// UpdateCredential 更新凭据可变字段（name/description/enabled），不允许改 type 与 secret。
func (s Store) UpdateCredential(ctx context.Context, id string, item domain.Credential) (domain.Credential, error) {
	existing, err := s.GetCredential(ctx, id)
	if err != nil {
		return domain.Credential{}, err
	}
	existing.Name = choose(item.Name, existing.Name)
	existing.Description = item.Description
	existing.Enabled = item.Enabled
	existing.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE credentials SET name = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		existing.Name, existing.Description, boolInt(existing.Enabled), formatTime(existing.UpdatedAt), id)
	if err != nil {
		return domain.Credential{}, err
	}
	return existing, nil
}

// DeleteCredential 在单事务内检查服务器引用并删除凭据。
// MySQL 下用 SELECT ... FOR UPDATE 对引用该凭据的服务器行加锁，阻塞并发的服务器插入/引用写入，
// 真正消除"计数后并发插入同一 credential_ref"的 TOCTOU 窗口。
// SQLite（测试用）不支持 FOR UPDATE，退化为普通 SELECT；测试为单连接无并发，不影响覆盖。
func (s Store) DeleteCredential(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	countSQL := `SELECT COUNT(*) FROM servers WHERE credential_ref = ?`
	if isMySQL(s.db) {
		countSQL = `SELECT COUNT(*) FROM servers WHERE credential_ref = ? FOR UPDATE`
	}
	var count int
	if err := tx.QueryRowContext(ctx, countSQL, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrCredentialInUse
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountServersByCredential 返回引用指定凭据的服务器数，供前端禁用提示使用。
func (s Store) CountServersByCredential(ctx context.Context, credentialID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers WHERE credential_ref = ?`, credentialID).Scan(&count)
	return count, err
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
