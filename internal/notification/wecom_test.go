package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestWeComRobotSendRejectsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"invalid webhook"}`))
	}))
	defer server.Close()

	err := (WeComRobot{}).Send(context.Background(), server.URL, "", "test")
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

	if err := (WeComRobot{}).Send(context.Background(), server.URL, "", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestSignedWebhookURLPreservesWebhookKey(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_123)
	got, err := signedWebhookURL("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key", "signing-secret", now)
	if err != nil {
		t.Fatal(err)
	}

	mac := hmac.New(sha256.New, []byte("signing-secret"))
	_, _ = mac.Write([]byte(strconv.FormatInt(now.UnixMilli(), 10) + "\n" + "signing-secret"))
	wantSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	want := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key&sign=" + url.QueryEscape(wantSign) + "&timestamp=1700000000123"
	if got != want {
		t.Fatalf("signed URL = %q, want %q", got, want)
	}
}
