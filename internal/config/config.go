package config

import (
	"errors"
	"os"
)

type Config struct {
	AppEnv             string
	HTTPAddr           string
	MySQLDSN           string
	AppEncryptionKey   string
	JWTSecret          string
	MigrationAuto      bool
	MigrationCheckOnly bool
	WorkerEnabled      bool
}

func Load() Config {
	return Config{
		AppEnv:             env("APP_ENV", "dev"),
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		MySQLDSN:           os.Getenv("MYSQL_DSN"),
		AppEncryptionKey:   os.Getenv("APP_ENCRYPTION_KEY"),
		JWTSecret:          env("JWT_SECRET", "dev-secret-change-me"),
		MigrationAuto:      envBool("MIGRATION_AUTO", true),
		MigrationCheckOnly: envBool("MIGRATION_CHECK_ONLY", false),
		WorkerEnabled:      envBool("WORKER_ENABLED", true),
	}
}

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("HTTP_ADDR is required")
	}
	if c.MySQLDSN == "" {
		return errors.New("MYSQL_DSN is required")
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
