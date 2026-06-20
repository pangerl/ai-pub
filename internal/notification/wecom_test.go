package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWeComRobotSendRejectsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"invalid webhook"}`))
	}))
	defer server.Close()

	err := (WeComRobot{}).Send(context.Background(), server.URL, "test")
	if err == nil || err.Error() != "wecom webhook error 93000: invalid webhook" {
		t.Fatalf("expected webhook business error, got %v", err)
	}
}

func TestWeComRobotSendAcceptsErrCodeZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	if err := (WeComRobot{}).Send(context.Background(), server.URL, "test"); err != nil {
		t.Fatal(err)
	}
}
