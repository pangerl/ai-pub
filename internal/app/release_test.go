package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/domain"
	"ai-pub/internal/migration"
	"ai-pub/internal/repository"
)

func TestReleaseServiceM2Flow(t *testing.T) {
	db, store := newReleaseTestStore(t)
	service := NewReleaseService(store)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)

	preflight, err := service.Preflight(ctx, PreflightInput{
		ServiceID:          fixture.service.ID,
		EnvironmentID:      fixture.testEnv.ID,
		ServiceVersionID:   fixture.version.ID,
		DeploymentTargetID: fixture.testTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Result != "pass" || preflight.NextAction != "self_confirm" {
		t.Fatalf("unexpected non-prod preflight: %#v", preflight)
	}
	if !hasPreflightItem(preflight, "artifact_url_missing", "warning") {
		t.Fatalf("expected missing artifact warning, got %#v", preflight)
	}
	conflictTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     fixture.service.ID,
		EnvironmentID: fixture.testEnv.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   fixture.server.ID,
		EnvVars:       `{"AI_PUB_VERSION":"bad"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := service.Preflight(ctx, PreflightInput{
		ServiceID:          fixture.service.ID,
		EnvironmentID:      fixture.testEnv.ID,
		ServiceVersionID:   fixture.version.ID,
		DeploymentTargetID: conflictTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if conflict.Result != "block" || !hasPreflightItem(conflict, "reserved_env_var", "block") {
		t.Fatalf("expected reserved env var block, got %#v", conflict)
	}

	created, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		IdempotencyKey: "idem-1",
		CreatedByType:  "user",
		CreatedByID:    fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	same, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		IdempotencyKey: "idem-1",
		CreatedByType:  "user",
		CreatedByID:    fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if same.ID != created.ID {
		t.Fatal("expected idempotent create to return the original release")
	}
	otherVersion, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{ServiceID: fixture.service.ID, Version: "v-idem-conflict"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   otherVersion.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		IdempotencyKey: "idem-1",
		CreatedByType:  "user",
		CreatedByID:    fixture.employee.ID,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict for changed version, got %v", err)
	}

	queued, err := service.Confirm(ctx, created.ID, ConfirmInput{UserID: fixture.employee.ID})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" {
		t.Fatalf("expected queued, got %s", queued.Status)
	}
	confirmedAgain, err := service.Confirm(ctx, created.ID, ConfirmInput{UserID: fixture.employee.ID})
	if err != nil {
		t.Fatal(err)
	}
	if confirmedAgain.Status != "queued" {
		t.Fatalf("expected duplicate confirm to return queued release, got %s", confirmedAgain.Status)
	}
	events, err := service.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countReleaseEventType(events, "preflight_checked") != 1 {
		t.Fatalf("expected create to record preflight_checked once, got %#v", events)
	}
	if countReleaseEventType(events, "release_confirmed") != 1 {
		t.Fatalf("expected duplicate confirm not to create another release_confirmed event, got %#v", events)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "release_confirmed" {
		t.Fatalf("expected release_confirmed event, got %#v", events)
	}

	if _, err := service.PreflightExisting(ctx, created.ID, Actor{Type: "user", ID: fixture.employee.ID}); err != nil {
		t.Fatal(err)
	}
	events, err = service.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].EventType != "preflight_checked" {
		t.Fatalf("expected preflight_checked event, got %#v", events)
	}
	if countReleaseEventType(events, "preflight_checked") != 2 {
		t.Fatalf("expected create and explicit preflight events, got %#v", events)
	}

	if _, err := db.Exec(`UPDATE release_requests SET status = 'running' WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Preflight(ctx, PreflightInput{
		ServiceID:          fixture.service.ID,
		EnvironmentID:      fixture.testEnv.ID,
		ServiceVersionID:   fixture.version.ID,
		DeploymentTargetID: fixture.testTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Result != "block" {
		t.Fatalf("expected running conflict block, got %#v", blocked)
	}

	if _, err := service.SetFreeze(ctx, "environment", fixture.testEnv.ID, true); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.Preflight(ctx, PreflightInput{
		ServiceID:          fixture.service.ID,
		EnvironmentID:      fixture.testEnv.ID,
		ServiceVersionID:   fixture.version.ID,
		DeploymentTargetID: fixture.testTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Result != "block" {
		t.Fatalf("expected freeze block, got %#v", frozen)
	}

	prod, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.prodEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.prodTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, prod.ID, ConfirmInput{UserID: fixture.employee.ID}); err == nil {
		t.Fatal("expected employee confirm on production release to fail")
	}
	if _, err := service.Confirm(ctx, prod.ID, ConfirmInput{UserID: fixture.admin.ID}); err != nil {
		t.Fatal(err)
	}

	_, _, err = service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if !errors.Is(err, ErrPreflightBlocked) {
		t.Fatalf("expected frozen/running preflight block, got %v", err)
	}
}

func TestReleaseServiceEffectivePolicy(t *testing.T) {
	_, store := newReleaseTestStore(t)
	service := NewReleaseService(store)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)

	if _, err := service.SavePolicy(ctx, PolicyInput{
		ScopeType:           "environment",
		ScopeID:             fixture.testEnv.ID,
		ConfirmMode:         "admin_confirm",
		ManualFreezeEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SavePolicy(ctx, PolicyInput{
		ScopeType:           "service",
		ScopeID:             fixture.service.ID,
		ConfirmMode:         "self_confirm",
		ManualFreezeEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	policy, err := service.EffectivePolicy(ctx, fixture.service.ID, fixture.testEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if policy.ConfirmMode != "self_confirm" || policy.ManualFreezeEnabled {
		t.Fatalf("expected service policy to override environment policy, got %#v", policy)
	}

	prodPolicy, err := service.EffectivePolicy(ctx, fixture.service.ID, fixture.prodEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prodPolicy.ConfirmMode != "admin_confirm" {
		t.Fatalf("expected production policy to force admin_confirm, got %#v", prodPolicy)
	}
}

func TestReleaseServiceRejectsDisabledConfirmUser(t *testing.T) {
	_, store := newReleaseTestStore(t)
	service := NewReleaseService(store)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)

	disabledUser := fixture.employee
	disabledUser.Enabled = false
	if _, err := store.UpdateUser(ctx, disabledUser.ID, disabledUser); err != nil {
		t.Fatal(err)
	}
	release, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, release.ID, ConfirmInput{UserID: fixture.employee.ID}); err == nil {
		t.Fatal("expected disabled user confirmation to fail")
	}
}

func TestReleaseServiceCancelIsRepeatSafe(t *testing.T) {
	_, store := newReleaseTestStore(t)
	service := NewReleaseService(store)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)

	created, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(ctx, created.ID, CancelInput{UserID: fixture.employee.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled release, got %s", cancelled.Status)
	}
	cancelledAgain, err := service.Cancel(ctx, created.ID, CancelInput{UserID: fixture.employee.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelledAgain.Status != "cancelled" {
		t.Fatalf("expected duplicate cancel to return cancelled release, got %s", cancelledAgain.Status)
	}
	events, err := service.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countReleaseEventType(events, "release_cancelled") != 1 {
		t.Fatalf("expected duplicate cancel not to create another release_cancelled event, got %#v", events)
	}
}

func TestReleaseServiceNotifiesAdminConfirmAndRollback(t *testing.T) {
	_, store := newReleaseTestStore(t)
	notifier := &captureNotifier{}
	service := NewReleaseService(store, notifier)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)

	prod, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.prodEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.prodTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected one notification event, got %#v", notifier.events)
	}
	pending := notifier.events[0]
	if pending.EventType != "prod_pending_confirm" || pending.ReleaseRequestID != prod.ID {
		t.Fatalf("unexpected pending notification: %#v", pending)
	}
	for _, want := range []string{"【发布待确认】", fixture.service.Name, fixture.prodEnv.Name, fixture.version.Version, fixture.employee.DisplayName, prod.ID} {
		if !strings.Contains(pending.Content, want) {
			t.Fatalf("expected pending content to contain %q, got %q", want, pending.Content)
		}
	}

	original, _, err := service.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.testEnv.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.testTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rollback, _, err := service.CreateRollback(ctx, original.ID, RollbackInput{
		ServiceVersionID: fixture.version.ID,
		CreatedByType:    "user",
		CreatedByID:      fixture.employee.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.events) != 2 {
		t.Fatalf("expected two notification events, got %#v", notifier.events)
	}
	rollbackEvent := notifier.events[1]
	if rollbackEvent.EventType != "rollback_requested" || rollbackEvent.ReleaseRequestID != rollback.ID {
		t.Fatalf("unexpected rollback notification: %#v", rollbackEvent)
	}
	for _, want := range []string{"【回滚申请】", fixture.service.Name, fixture.testEnv.Name, fixture.version.Version, original.ID, rollback.ID} {
		if !strings.Contains(rollbackEvent.Content, want) {
			t.Fatalf("expected rollback content to contain %q, got %q", want, rollbackEvent.Content)
		}
	}
}

type captureNotifier struct {
	events []NotificationEvent
}

func (n *captureNotifier) NotifyAll(_ context.Context, event NotificationEvent) {
	n.events = append(n.events, event)
}

func countReleaseEventType(events []domain.ReleaseEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

func hasPreflightItem(result PreflightResult, code string, level string) bool {
	for _, item := range result.Items {
		if item.Code == code && item.Level == level {
			return true
		}
	}
	return false
}

type releaseFixture struct {
	project    domain.Project
	service    domain.Service
	version    domain.ServiceVersion
	testEnv    domain.Environment
	prodEnv    domain.Environment
	server     domain.Server
	testTarget domain.DeploymentTarget
	prodTarget domain.DeploymentTarget
	employee   domain.User
	admin      domain.User
}

func createReleaseFixture(t *testing.T, store repository.Store) releaseFixture {
	t.Helper()
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
	testEnv, err := store.CreateEnvironment(ctx, domain.Environment{Name: "测试环境", Slug: "test"})
	if err != nil {
		t.Fatal(err)
	}
	prodEnv, err := store.CreateEnvironment(ctx, domain.Environment{Name: "生产环境", Slug: "prod", IsProduction: true})
	if err != nil {
		t.Fatal(err)
	}
	server, err := store.CreateServer(ctx, domain.Server{Name: "mock-1", Host: "127.0.0.1", Username: "deploy", AuthType: "none"})
	if err != nil {
		t.Fatal(err)
	}
	testTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     service.ID,
		EnvironmentID: testEnv.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   server.ID,
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	prodTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     service.ID,
		EnvironmentID: prodEnv.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   server.ID,
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	employee, err := store.CreateUser(ctx, domain.User{Username: "alice", DisplayName: "Alice", Role: "employee"})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := store.CreateUser(ctx, domain.User{Username: "admin", DisplayName: "Admin", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	return releaseFixture{
		project:    project,
		service:    service,
		version:    version,
		testEnv:    testEnv,
		prodEnv:    prodEnv,
		server:     server,
		testTarget: testTarget,
		prodTarget: prodTarget,
		employee:   employee,
		admin:      admin,
	}
}

func newReleaseTestStore(t *testing.T) (*sql.DB, repository.Store) {
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
	return db, repository.NewStore(db)
}
