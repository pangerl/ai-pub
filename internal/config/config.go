package config

import (
	"errors"
	"os"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	WebDir                 string
	DBDialect              string
	MySQLDSN               string
	SQLitePath             string
	AppEncryptionKey       string
	JWTSecret              string
	BootstrapAdminUsername string
	BootstrapAdminPassword string
	MigrationAuto          bool
	MigrationCheckOnly     bool
	WorkerEnabled          bool
	ExecutorSSHDisabled    bool
	ExecutorK8sDisabled    bool
	DemoMode               bool
	DemoProtectedUsernames string
}

func Load() Config {
	return Config{
		AppEnv:                 env("APP_ENV", "dev"),
		HTTPAddr:               env("HTTP_ADDR", ":8080"),
		WebDir:                 env("WEB_DIR", "web/dist"),
		DBDialect:              env("DB_DIALECT", "mysql"),
		MySQLDSN:               os.Getenv("MYSQL_DSN"),
		SQLitePath:             env("SQLITE_PATH", "data/ai-pub.db"),
		AppEncryptionKey:       os.Getenv("APP_ENCRYPTION_KEY"),
		JWTSecret:              env("JWT_SECRET", "dev-secret-change-me"),
		BootstrapAdminUsername: env("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		MigrationAuto:          envBool("MIGRATION_AUTO", true),
		MigrationCheckOnly:     envBool("MIGRATION_CHECK_ONLY", false),
		WorkerEnabled:          envBool("WORKER_ENABLED", true),
		ExecutorSSHDisabled:    envBool("EXECUTOR_SSH_DISABLED", false),
		ExecutorK8sDisabled:    envBool("EXECUTOR_K8S_DISABLED", false),
		DemoMode:               envBool("DEMO_MODE", false),
		DemoProtectedUsernames: env("DEMO_PROTECTED_USERNAMES", "demo"),
	}
}

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("HTTP_ADDR is required")
	}
	switch c.DBDialect {
	case "mysql":
		if c.MySQLDSN == "" {
			return errors.New("MYSQL_DSN is required")
		}
	case "sqlite":
		if c.SQLitePath == "" {
			return errors.New("SQLITE_PATH is required")
		}
		if c.AppEnv == "prod" {
			return errors.New("DB_DIALECT=sqlite is only supported for demo/local mode")
		}
	default:
		return errors.New("DB_DIALECT must be mysql or sqlite")
	}
	if c.AppEnv == "prod" {
		if c.AppEncryptionKey == "" {
			return errors.New("APP_ENCRYPTION_KEY is required in prod")
		}
		if c.JWTSecret == "" || c.JWTSecret == "dev-secret-change-me" {
			return errors.New("JWT_SECRET must be set in prod")
		}
	}
	return nil
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}
