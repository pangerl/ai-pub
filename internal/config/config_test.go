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

func TestExecutorDisabledFlagsDefaultToEnabled(t *testing.T) {
	t.Setenv("EXECUTOR_SSH_DISABLED", "")
	t.Setenv("EXECUTOR_K8S_DISABLED", "")

	cfg := Load()
	if cfg.ExecutorSSHDisabled || cfg.ExecutorK8sDisabled {
		t.Fatalf("expected executor disabled flags to default false, got ssh=%v k8s=%v", cfg.ExecutorSSHDisabled, cfg.ExecutorK8sDisabled)
	}

	t.Setenv("EXECUTOR_SSH_DISABLED", "true")
	t.Setenv("EXECUTOR_K8S_DISABLED", "true")
	cfg = Load()
	if !cfg.ExecutorSSHDisabled || !cfg.ExecutorK8sDisabled {
		t.Fatalf("expected executor disabled flags from env, got ssh=%v k8s=%v", cfg.ExecutorSSHDisabled, cfg.ExecutorK8sDisabled)
	}
}

func TestDemoProtectionConfigDefaults(t *testing.T) {
	t.Setenv("DEMO_MODE", "")
	t.Setenv("DEMO_PROTECTED_USERNAMES", "")

	cfg := Load()
	if cfg.DemoMode || cfg.DemoProtectedUsernames != "demo" {
		t.Fatalf("expected demo protection defaults, got mode=%v users=%q", cfg.DemoMode, cfg.DemoProtectedUsernames)
	}

	t.Setenv("DEMO_MODE", "true")
	t.Setenv("DEMO_PROTECTED_USERNAMES", "demo,guest")
	cfg = Load()
	if !cfg.DemoMode || cfg.DemoProtectedUsernames != "demo,guest" {
		t.Fatalf("expected demo protection env, got mode=%v users=%q", cfg.DemoMode, cfg.DemoProtectedUsernames)
	}
}
