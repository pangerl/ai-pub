package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"ai-pub/internal/crypto"
	"ai-pub/internal/domain"
)

func TestNotificationServiceSendsAndRecordsDelivery(t *testing.T) {
	_, store := newReleaseTestStore(t)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Get("timestamp") == "" || req.URL.Query().Get("sign") == "" {
			t.Fatal("expected signed webhook request")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	service := NewNotificationService(store, crypto.NewBox("test-key"), client)
	config, err := service.CreateConfig(context.Background(), CreateNotificationConfigInput{
		Name:       "wecom",
		Channel:    "wecom_robot",
		WebhookURL: "http://wecom.test/webhook",
		Secret:     "signing-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	configs, err := service.ListConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || configs[0].ID != config.ID {
		t.Fatalf("expected safe config list, got %#v", configs)
	}
	delivery, err := service.Test(context.Background(), config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "sent" || calls != 1 {
		t.Fatalf("expected sent delivery and one call, got %#v calls=%d", delivery, calls)
	}
}

func TestNotificationServiceRecordsReleaseEvents(t *testing.T) {
	_, store := newReleaseTestStore(t)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)
	releaseService := NewReleaseService(store)
	release, _, err := releaseService.Create(ctx, CreateReleaseInput{
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
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	service := NewNotificationService(store, crypto.NewBox("test-key"), client)
	if _, err := service.CreateConfig(ctx, CreateNotificationConfigInput{
		Name:       "wecom",
		Channel:    "wecom_robot",
		WebhookURL: "http://wecom.test/webhook",
	}); err != nil {
		t.Fatal(err)
	}

	service.NotifyAll(ctx, NotificationEvent{
		EventType:        "prod_pending_confirm",
		ReleaseRequestID: release.ID,
		Content:          "content",
	})
	events, err := releaseService.ListEvents(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventType(t, events, "notification_sent")

	_, err = service.Test(ctx, serviceConfigID(t, service))
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterTest, err := releaseService.ListEvents(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(eventsAfterTest, "notification_sent") != 1 {
		t.Fatalf("expected test notification not to add release event, got %#v", eventsAfterTest)
	}
}

func TestNotificationServiceRecordsFailedReleaseEvent(t *testing.T) {
	_, store := newReleaseTestStore(t)
	ctx := context.Background()
	fixture := createReleaseFixture(t, store)
	releaseService := NewReleaseService(store)
	release, _, err := releaseService.Create(ctx, CreateReleaseInput{
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
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("webhook down")
	})}
	service := NewNotificationService(store, crypto.NewBox("test-key"), client)
	if _, err := service.CreateConfig(ctx, CreateNotificationConfigInput{
		Name:       "wecom",
		Channel:    "wecom_robot",
		WebhookURL: "http://wecom.test/webhook",
	}); err != nil {
		t.Fatal(err)
	}

	service.NotifyAll(ctx, NotificationEvent{
		EventType:        "deploy_failed",
		ReleaseRequestID: release.ID,
		Content:          "content",
	})
	events, err := releaseService.ListEvents(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventType(t, events, "notification_failed")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func serviceConfigID(t *testing.T, service NotificationService) string {
	t.Helper()
	configs, err := service.ListConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one notification config, got %#v", configs)
	}
	return configs[0].ID
}

func assertEventType(t *testing.T, events []domain.ReleaseEvent, eventType string) {
	t.Helper()
	if countEventType(events, eventType) == 0 {
		t.Fatalf("expected event %s, got %#v", eventType, events)
	}
}

func countEventType(events []domain.ReleaseEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}
