package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

type Runner struct {
	db      *sql.DB
	dialect string
	files   fs.FS
}

type Report struct {
	Applied []string
	Pending []string
}

type migrationFile struct {
	Version  string
	Name     string
	Path     string
	SQL      string
	Checksum string
}

func NewRunner(db *sql.DB, dialect string, files fs.FS) Runner {
	return Runner{db: db, dialect: dialect, files: files}
}

func (r Runner) Run(ctx context.Context, checkOnly bool) (Report, error) {
	if r.dialect != "sqlite" && r.dialect != "mysql" {
		return Report{}, fmt.Errorf("migration dialect %q is not implemented", r.dialect)
	}
	if err := r.ensureTable(ctx); err != nil {
		return Report{}, err
	}
	items, err := r.readMigrations()
	if err != nil {
		return Report{}, err
	}

	report := Report{}
	for _, item := range items {
		applied, err := r.isApplied(ctx, item)
		if err != nil {
			return report, err
		}
		if applied {
			continue
		}
		report.Pending = append(report.Pending, item.Name)
		if checkOnly {
			continue
		}
		if err := r.apply(ctx, item); err != nil {
			return report, err
		}
		report.Applied = append(report.Applied, item.Name)
	}
	return report, nil
}

func (r Runner) ensureTable(ctx context.Context) error {
	query := `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	checksum TEXT NOT NULL,
	applied_at TEXT NOT NULL
);`
	if r.dialect == "mysql" {
		query = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version VARCHAR(64) PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	checksum VARCHAR(64) NOT NULL,
	applied_at VARCHAR(64) NOT NULL
);`
	}
	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func (r Runner) isApplied(ctx context.Context, item migrationFile) (bool, error) {
	var checksum string
	err := r.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, item.Version).Scan(&checksum)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration %s: %w", item.Name, err)
	}
	if checksum != item.Checksum {
		return false, fmt.Errorf("migration %s checksum changed", item.Name)
	}
	return true, nil
}

func (r Runner) apply(ctx context.Context, item migrationFile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, statement := range splitStatements(item.SQL) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %s: %w", item.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		item.Version,
		item.Name,
		item.Checksum,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration %s: %w", item.Name, err)
	}
	return tx.Commit()
}

func splitStatements(sqlText string) []string {
	raw := strings.Split(trimSQLComments(sqlText), ";")
	statements := make([]string, 0, len(raw))
	trigger := ""
	for _, item := range raw {
		statement := strings.TrimSpace(item)
		if statement != "" {
			if trigger != "" {
				trigger += ";\n" + statement
				if strings.EqualFold(statement, "END") {
					statements = append(statements, trigger)
					trigger = ""
				}
				continue
			}
			if strings.HasPrefix(strings.ToUpper(statement), "CREATE TRIGGER ") {
				trigger = statement
				continue
			}
			statements = append(statements, statement)
		}
	}
	if trigger != "" {
		statements = append(statements, trigger)
	}
	return statements
}

func trimSQLComments(sqlText string) string {
	lines := strings.Split(sqlText, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func (r Runner) readMigrations() ([]migrationFile, error) {
	root := path.Join("migrations", r.dialect)
	entries, err := fs.ReadDir(r.files, root)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var items []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		filePath := path.Join(root, entry.Name())
		body, err := fs.ReadFile(r.files, filePath)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		items = append(items, migrationFile{
			Version:  parts[0],
			Name:     entry.Name(),
			Path:     filePath,
			SQL:      string(body),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Version < items[j].Version
	})
	return items, nil
}
