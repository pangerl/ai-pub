package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/config"
	"ai-pub/internal/migration"
	"ai-pub/internal/repository"
)

func TestEnsureBootstrapAdminReportsPasswordEnvName(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := migration.NewRunner(db, "sqlite", os.DirFS("../..")).Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	err = ensureBootstrapAdmin(context.Background(), repository.NewStore(db), config.Config{
		BootstrapAdminUsername: "admin",
		BootstrapAdminPassword: "short",
	})
	if err == nil {
		t.Fatal("expected short bootstrap password to fail")
	}
	if !strings.Contains(err.Error(), "BOOTSTRAP_ADMIN_PASSWORD") {
		t.Fatalf("expected error to mention BOOTSTRAP_ADMIN_PASSWORD, got %q", err.Error())
	}
}
