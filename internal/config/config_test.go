package config

import "testing"

func TestMySQLDSNIsRequired(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("MYSQL_DSN", "")

	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing MYSQL_DSN to fail validation")
	}

	t.Setenv("MYSQL_DSN", "ai_pub:ai_pub@tcp(mysql:3306)/ai_pub?parseTime=true")
	if err := Load().Validate(); err != nil {
		t.Fatalf("expected MySQL config to be valid: %v", err)
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
