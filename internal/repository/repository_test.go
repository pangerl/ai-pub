package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func TestServerResultJSONUsesAPIFieldNames(t *testing.T) {
	exitCode := 0
	raw, err := json.Marshal(ServerResult{
		Status:       "success",
		ExitCode:     &exitCode,
		DurationMS:   12,
		LogOutput:    "ok",
		ErrorCode:    "connect_failed",
		ErrorMessage: "failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"status", "exit_code", "duration_ms", "log_output", "error_code", "error_message"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected API field %q in %s", key, raw)
		}
	}
	if _, ok := body["Status"]; ok {
		t.Fatalf("server result must not expose Go field names: %s", raw)
	}
}

func TestApplicationServerCanUseOnlyAnEnabledGateway(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gateway, err := store.CreateServer(ctx, domain.Server{
		Name: "bastion", Host: "gateway.example", Username: "jump", AuthType: "none", Role: "gateway",
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := store.CreateServer(ctx, domain.Server{
		Name: "app", Host: "app.internal", Username: "deploy", AuthType: "none", GatewayID: gateway.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if application.Role != "application" || application.GatewayID != gateway.ID {
		t.Fatalf("expected application server to retain gateway, got %#v", application)
	}
	plain, err := store.CreateServer(ctx, domain.Server{
		Name: "plain", Host: "plain.internal", Username: "deploy", AuthType: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateServer(ctx, domain.Server{
		Name: "invalid", Host: "invalid.internal", Username: "deploy", AuthType: "none", GatewayID: plain.ID,
	}); err == nil {
		t.Fatal("expected non-gateway reference to be rejected")
	}
	gateway.Enabled = false
	if _, err := store.UpdateServer(ctx, gateway.ID, gateway); err == nil {
		t.Fatal("expected used gateway to remain enabled")
	}
	gateway.Enabled = true
	gateway.Role = "application"
	if _, err := store.UpdateServer(ctx, gateway.ID, gateway); err == nil {
		t.Fatal("expected used gateway to retain gateway role")
	}
}

func TestK8sClusterAndDeploymentTargetPersistence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	credential, err := store.CreateCredential(ctx, domain.Credential{Name: "test kubeconfig", Type: "kubeconfig"}, "encrypted")
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := store.CreateK8sCluster(ctx, domain.K8sCluster{Name: "test-cluster", CredentialRef: credential.ID})
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, domain.Project{Name: "供应链系统", Slug: "supply-chain-k8s"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, domain.Service{ProjectID: project.ID, Name: "订单服务", Slug: "order-api-k8s"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := store.CreateEnvironment(ctx, domain.Environment{Name: "测试环境", Slug: "test-k8s"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:      service.ID,
		EnvironmentID:  env.ID,
		ExecutorType:   "k8s",
		ArtifactType:   "oci_image",
		TimeoutSeconds: 180,
		K8s: &domain.K8sDeploymentTarget{
			ClusterID:      cluster.ID,
			Namespace:      "default",
			DeploymentName: "order-api",
			ContainerName:  "app",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetDeploymentTarget(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.K8s == nil || got.K8s.ClusterID != cluster.ID || got.K8s.DeploymentName != "order-api" {
		t.Fatalf("expected k8s config to round-trip, got %#v", got)
	}
	if got.SSH != nil {
		t.Fatalf("k8s target must not include ssh config: %#v", got.SSH)
	}
	if err := store.DeleteK8sCluster(ctx, cluster.ID); !errors.Is(err, ErrK8sClusterInUse) {
		t.Fatalf("expected cluster in-use error, got %v", err)
	}
	if err := store.DeleteCredential(ctx, credential.ID); !errors.Is(err, ErrCredentialInUse) {
		t.Fatalf("expected credential in-use error, got %v", err)
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
