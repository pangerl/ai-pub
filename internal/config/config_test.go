package config

import "testing"

func TestDatabaseConfigByDialect(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DB_DIALECT", "")
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("SQLITE_PATH", "")

	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing MYSQL_DSN to fail validation")
	}

	t.Setenv("MYSQL_DSN", "ai_pub:ai_pub@tcp(mysql:3306)/ai_pub?parseTime=true")
	if err := Load().Validate(); err != nil {
		t.Fatalf("expected MySQL config to be valid: %v", err)
	}

	t.Setenv("DB_DIALECT", "sqlite")
	t.Setenv("SQLITE_PATH", "data/demo.db")
	t.Setenv("MYSQL_DSN", "")
	if err := Load().Validate(); err != nil {
		t.Fatalf("expected SQLite demo config to be valid: %v", err)
	}

	t.Setenv("APP_ENV", "prod")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected SQLite to be rejected in prod")
	}
}

func TestProdRequiresSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ENCRYPTION_KEY", "")
	t.Setenv("MYSQL_DSN", "ai_pub:ai_pub@tcp(mysql:3306)/ai_pub?parseTime=true")

	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected prod config without secrets to fail")
	}
}

func TestMigrationAutoCanBeDisabled(t *testing.T) {
	t.Setenv("MIGRATION_AUTO", "false")

	if Load().MigrationAuto {
		t.Fatal("expected MIGRATION_AUTO=false to disable automatic migrations")
	}
}
