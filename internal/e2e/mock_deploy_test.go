package e2e

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/app"
	"ai-pub/internal/crypto"
	"ai-pub/internal/domain"
	"ai-pub/internal/migration"
	"ai-pub/internal/repository"
	"ai-pub/internal/worker"
)

func TestSQLiteMockDeploy(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)

	runMockRelease(t, ctx, db, store, fixture, fixture.successTarget, "success")
	runMockRelease(t, ctx, db, store, fixture, fixture.failureTarget, "failed")
}

func TestSQLiteMockDeployWithServerGroupTarget(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	group, err := store.CreateServerGroup(ctx, domain.ServerGroup{
		Name:      "mock-group",
		ServerIDs: []string{fixture.server.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     fixture.service.ID,
		EnvironmentID: fixture.env.ID,
		ExecutorType:  "mock",
		TargetType:    "server_group",
		TargetRefID:   group.ID,
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	runMockRelease(t, ctx, db, store, fixture, target, "success")
}

func TestSQLiteMockDeployPartialSkipsRemainingServers(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	second, err := store.CreateServer(ctx, domain.Server{Name: "mock-2", Host: "127.0.0.2", Username: "deploy", AuthType: "none"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateServer(ctx, domain.Server{Name: "mock-3", Host: "127.0.0.3", Username: "deploy", AuthType: "none"})
	if err != nil {
		t.Fatal(err)
	}
	serverIDs := []string{fixture.server.ID, second.ID, third.ID}
	sort.Strings(serverIDs)
	failID := serverIDs[1]
	group, err := store.CreateServerGroup(ctx, domain.ServerGroup{
		Name:      "partial-group",
		ServerIDs: serverIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     fixture.service.ID,
		EnvironmentID: fixture.env.ID,
		ExecutorType:  "mock",
		TargetType:    "server_group",
		TargetRefID:   group.ID,
		EnvVars:       `{"MOCK_FAIL_SERVER_ID":"` + failID + `"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := runMockReleaseReturning(t, ctx, db, store, fixture, target, "partial")
	if done.Status != "failed" || done.SummaryStatus != "partial" {
		t.Fatalf("expected release failed with partial summary, got %#v", done)
	}

	var deployID string
	if err := db.QueryRow(`SELECT id FROM deploy_records WHERE release_request_id = ?`, done.ID).Scan(&deployID); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetDeployRecord(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	if record.SuccessServers != 1 || record.FailedServers != 1 || record.SkippedServers != 1 {
		t.Fatalf("expected success=1 failed=1 skipped=1, got %#v", record)
	}
	logs, err := store.ListServerDeployLogs(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]int{}
	for _, log := range logs {
		statuses[log.Status]++
	}
	if statuses["success"] != 1 || statuses["failed"] != 1 || statuses["skipped"] != 1 {
		t.Fatalf("expected success/failed/skipped logs, got %#v", logs)
	}
	states, err := store.ListServerDeploymentStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("expected only successful server state to update, got %#v", states)
	}
}

func TestSQLiteDeployUsesQueuedTargetSnapshot(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	releases := app.NewReleaseService(store)
	created, _, err := releases.Create(ctx, app.CreateReleaseInput{
		PreflightInput: app.PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.env.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: fixture.successTarget.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.user.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releases.Confirm(ctx, created.ID, app.ConfirmInput{UserID: fixture.user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDeploymentTarget(ctx, fixture.successTarget.ID, domain.DeploymentTarget{
		ExecutorType:   "mock",
		TargetType:     "server",
		TargetRefID:    fixture.server.ID,
		EnvVars:        `{"MOCK_FAIL":"true"}`,
		TimeoutSeconds: 60,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}

	credentialService := app.NewCredentialService(store, crypto.NewBox(""))
	workerService := worker.NewService(store, credentialService, nil, "snapshot_worker")
	if err := workerService.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := releases.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.SummaryStatus != "success" {
		t.Fatalf("expected queued snapshot to keep original success target, got %#v", done)
	}
	var deployID string
	if err := db.QueryRow(`SELECT id FROM deploy_records WHERE release_request_id = ?`, created.ID).Scan(&deployID); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetDeployRecord(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(record.TargetSnapshot, `"deployment_target"`) ||
		!strings.Contains(record.TargetSnapshot, `"servers"`) ||
		strings.Contains(record.TargetSnapshot, "MOCK_FAIL") {
		t.Fatalf("expected immutable target and server snapshot, got %s", record.TargetSnapshot)
	}
}

func TestSQLiteClaimSkipsRunningServerConflict(t *testing.T) {
	_, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	releases := app.NewReleaseService(store)
	first := createQueuedRelease(t, ctx, releases, fixture, fixture.successTarget)
	second := createQueuedRelease(t, ctx, releases, fixture, fixture.successTarget)

	claimed, err := store.ClaimNextDeploy(ctx, "worker_one")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Release.ID != first.ID && claimed.Release.ID != second.ID {
		t.Fatalf("expected one of the queued releases to be claimed, got %#v", claimed.Release)
	}
	_, err = store.ClaimNextDeploy(ctx, "worker_two")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected same-server queued release to remain blocked, got %v", err)
	}
}

func TestSQLiteFreezePausesQueuedDeployClaim(t *testing.T) {
	_, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	releases := app.NewReleaseService(store)
	createQueuedRelease(t, ctx, releases, fixture, fixture.successTarget)
	if _, err := releases.SetFreeze(ctx, "environment", fixture.env.ID, true); err != nil {
		t.Fatal(err)
	}

	_, err := store.ClaimNextDeploy(ctx, "frozen_worker")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected frozen queued release to pause claiming, got %v", err)
	}
	if _, err := releases.SetFreeze(ctx, "environment", fixture.env.ID, false); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextDeploy(ctx, "unfrozen_worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Release.ID == "" {
		t.Fatalf("expected queued release after unfreeze, got %#v", claimed)
	}
}

func TestSQLiteCancelQueuedReleaseCancelsDeployRecord(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	releases := app.NewReleaseService(store)
	queued := createQueuedRelease(t, ctx, releases, fixture, fixture.successTarget)

	cancelled, err := releases.Cancel(ctx, queued.ID, app.CancelInput{UserID: fixture.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled release, got %#v", cancelled)
	}
	var deployID string
	if err := db.QueryRow(`SELECT id FROM deploy_records WHERE release_request_id = ?`, queued.ID).Scan(&deployID); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetDeployRecord(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "cancelled" {
		t.Fatalf("expected cancelled deploy record, got %#v", record)
	}
	summary, err := store.OpsSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.QueuedDeploys != 0 {
		t.Fatalf("expected cancelled deploy not to count as queued, got %#v", summary)
	}
	if _, err := store.ClaimNextDeploy(ctx, "cancelled_worker"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected cancelled deploy not to be claimable, got %v", err)
	}
}

func TestSQLiteMockDeployFailureSendsNotification(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	calls := 0
	client := &http.Client{Transport: e2eRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("{}")),
			Header:     make(http.Header),
		}, nil
	})}

	box := crypto.NewBox("")
	notificationService := app.NewNotificationService(store, box, client)
	if _, err := notificationService.CreateConfig(ctx, app.CreateNotificationConfigInput{
		Name:       "wecom",
		Channel:    "wecom_robot",
		WebhookURL: "http://wecom.test/webhook",
	}); err != nil {
		t.Fatal(err)
	}
	runMockReleaseWithNotifications(t, ctx, db, store, fixture, fixture.failureTarget, "failed", &notificationService)
	deliveries, err := notificationService.ListDeliveries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(deliveries) != 1 || deliveries[0].Status != "sent" {
		t.Fatalf("expected one sent notification, calls=%d deliveries=%#v", calls, deliveries)
	}
}

func TestSQLiteRollbackDeploy(t *testing.T) {
	db, store := newE2EStore(t)
	ctx := context.Background()
	fixture := seedE2E(t, store)
	releases := app.NewReleaseService(store)

	first := runMockReleaseReturning(t, ctx, db, store, fixture, fixture.successTarget, "success")
	version2, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{ServiceID: fixture.service.ID, Version: "v2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.version = version2
	second := runMockReleaseReturning(t, ctx, db, store, fixture, fixture.successTarget, "success")
	candidates, err := releases.RollbackCandidates(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 || candidates[0].ID != first.ServiceVersionID {
		t.Fatalf("expected first version as rollback candidate, got %#v", candidates)
	}
	rollback, _, err := releases.CreateRollback(ctx, second.ID, app.RollbackInput{
		ServiceVersionID: first.ServiceVersionID,
		CreatedByType:    "user",
		CreatedByID:      fixture.user.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releases.Confirm(ctx, rollback.ID, app.ConfirmInput{UserID: fixture.user.ID}); err != nil {
		t.Fatal(err)
	}
	credentialService := app.NewCredentialService(store, crypto.NewBox(""))
	workerService := worker.NewService(store, credentialService, nil, "rollback_worker")
	if err := workerService.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	states, err := store.ListServerDeploymentStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ServiceVersionID != first.ServiceVersionID {
		t.Fatalf("expected state rolled back to first version, got %#v", states)
	}
}

type e2eRoundTripFunc func(*http.Request) (*http.Response, error)

func (f e2eRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func runMockRelease(t *testing.T, ctx context.Context, db *sql.DB, store repository.Store, fixture e2eFixture, target domain.DeploymentTarget, want string) {
	t.Helper()
	_ = runMockReleaseWithNotifications(t, ctx, db, store, fixture, target, want, nil)
}

func runMockReleaseReturning(t *testing.T, ctx context.Context, db *sql.DB, store repository.Store, fixture e2eFixture, target domain.DeploymentTarget, want string) domain.ReleaseRequest {
	t.Helper()
	return runMockReleaseWithNotifications(t, ctx, db, store, fixture, target, want, nil)
}

func runMockReleaseWithNotifications(t *testing.T, ctx context.Context, db *sql.DB, store repository.Store, fixture e2eFixture, target domain.DeploymentTarget, want string, notifications *app.NotificationService) domain.ReleaseRequest {
	t.Helper()
	releases := app.NewReleaseService(store)
	created := createQueuedRelease(t, ctx, releases, fixture, target)
	if created.Status != "queued" {
		t.Fatalf("expected queued release, got %s", created.Status)
	}

	credentialService := app.NewCredentialService(store, crypto.NewBox(""))
	workerService := worker.NewService(store, credentialService, notifications, "test_worker")
	if err := workerService.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := releases.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.SummaryStatus != want {
		t.Fatalf("expected summary %s, got release=%#v", want, done)
	}

	var deployID string
	if err := db.QueryRow(`SELECT id FROM deploy_records WHERE release_request_id = ?`, created.ID).Scan(&deployID); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetDeployRecord(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != want {
		t.Fatalf("expected deploy %s, got %#v", want, record)
	}
	logs, err := store.ListServerDeployLogs(ctx, deployID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected server logs, got %#v", logs)
	}
	if want != "partial" && (len(logs) != 1 || logs[0].Status != want) {
		t.Fatalf("expected one %s server log, got %#v", want, logs)
	}
	if want == "success" && !strings.Contains(logs[0].LogOutput, fixture.version.Version) {
		t.Fatalf("expected mock log to include service version %q, got %#v", fixture.version.Version, logs[0])
	}
	events, err := releases.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 4 || events[len(events)-1].EventType != "deploy_finished" {
		t.Fatalf("expected deploy_finished event, got %#v", events)
	}
	return done
}

func createQueuedRelease(t *testing.T, ctx context.Context, releases app.ReleaseService, fixture e2eFixture, target domain.DeploymentTarget) domain.ReleaseRequest {
	t.Helper()
	created, _, err := releases.Create(ctx, app.CreateReleaseInput{
		PreflightInput: app.PreflightInput{
			ServiceID:          fixture.service.ID,
			EnvironmentID:      fixture.env.ID,
			ServiceVersionID:   fixture.version.ID,
			DeploymentTargetID: target.ID,
		},
		CreatedByType: "user",
		CreatedByID:   fixture.user.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := releases.Confirm(ctx, created.ID, app.ConfirmInput{UserID: fixture.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" {
		t.Fatalf("expected queued release, got %s", queued.Status)
	}
	return queued
}

type e2eFixture struct {
	project       domain.Project
	service       domain.Service
	version       domain.ServiceVersion
	env           domain.Environment
	server        domain.Server
	successTarget domain.DeploymentTarget
	failureTarget domain.DeploymentTarget
	user          domain.User
}

func seedE2E(t *testing.T, store repository.Store) e2eFixture {
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
	env, err := store.CreateEnvironment(ctx, domain.Environment{Name: "测试环境", Slug: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := store.CreateServer(ctx, domain.Server{Name: "mock-1", Host: "127.0.0.1", Username: "deploy", AuthType: "none"})
	if err != nil {
		t.Fatal(err)
	}
	successTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     service.ID,
		EnvironmentID: env.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   server.ID,
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	failureTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     service.ID,
		EnvironmentID: env.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   server.ID,
		EnvVars:       `{"MOCK_FAIL":"true"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, domain.User{Username: "alice", DisplayName: "Alice", Role: "employee"})
	if err != nil {
		t.Fatal(err)
	}
	return e2eFixture{
		project:       project,
		service:       service,
		version:       version,
		env:           env,
		server:        server,
		successTarget: successTarget,
		failureTarget: failureTarget,
		user:          user,
	}
}

func newE2EStore(t *testing.T) (*sql.DB, repository.Store) {
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
