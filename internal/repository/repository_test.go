package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/domain"
	"ai-pub/internal/migration"
)

func TestInventoryCRUDAndAPIKeyPlaintextOnce(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	project, err := store.CreateProject(ctx, domain.Project{Name: "供应链系统", Slug: "supply-chain"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, domain.Service{ProjectID: project.ID, Name: "订单服务", Slug: "order-api"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{ServiceID: service.ID, Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := store.CreateEnvironment(ctx, domain.Environment{Name: "测试环境", Slug: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := store.CreateServer(ctx, domain.Server{Name: "mock-1", Host: "127.0.0.1", Username: "deploy", AuthType: "none"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:      service.ID,
		EnvironmentID:  env.ID,
		ExecutorType:   "mock",
		TargetType:     "server",
		TargetRefID:    server.ID,
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, domain.User{Username: "admin", DisplayName: "管理员", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAPIKey(ctx, domain.APIKey{Name: "CI", OwnerUserID: user.ID, Scopes: `["release:create"]`})
	if err != nil {
		t.Fatal(err)
	}
	if key.Plaintext == "" {
		t.Fatal("expected plaintext api key at creation")
	}
	keys, err := store.ListAPIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Prefix == "" {
		t.Fatalf("expected listed key prefix only, got %#v", keys)
	}
	if version.ID == "" || target.ID == "" {
		t.Fatal("expected created version and target ids")
	}
}

func newTestStore(t *testing.T) Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	runner := migration.NewRunner(db, "sqlite", os.DirFS("../.."))
	if _, err := runner.Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}
