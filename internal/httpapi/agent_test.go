package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ai-pub/internal/config"
)

func TestAgentReleaseFlow(t *testing.T) {
	db := newHTTPTestDB(t)
	router := NewRouter(Dependencies{DB: db, Config: config.Config{}})

	project := postForData(t, router, "/api/v1/projects", map[string]any{
		"name": "供应链系统",
		"slug": "supply-chain",
	})
	service := postForData(t, router, "/api/v1/services", map[string]any{
		"project_id": project["id"],
		"name":       "订单服务",
		"slug":       "order-api",
	})
	postForData(t, router, "/api/v1/services", map[string]any{
		"project_id": project["id"],
		"name":       "订单后台",
		"slug":       "order-admin",
	})
	version := postForData(t, router, "/api/v1/services/"+service["id"].(string)+"/versions", map[string]any{
		"version": "v1.2.3",
	})
	testEnv := postForData(t, router, "/api/v1/environments", map[string]any{
		"name":          "测试环境",
		"slug":          "test",
		"is_production": false,
	})
	prodEnv := postForData(t, router, "/api/v1/environments", map[string]any{
		"name":          "生产环境",
		"slug":          "prod",
		"is_production": true,
	})
	server := postForData(t, router, "/api/v1/servers", map[string]any{
		"name":      "mock-1",
		"host":      "127.0.0.1",
		"username":  "deploy",
		"auth_type": "none",
	})
	testTarget := postForData(t, router, "/api/v1/deployment-targets", map[string]any{
		"service_id":      service["id"],
		"environment_id":  testEnv["id"],
		"executor_type":   "mock",
		"target_type":     "server",
		"target_ref_id":   server["id"],
		"timeout_seconds": 60,
		"env_vars":        "{}",
	})
	prodTarget := postForData(t, router, "/api/v1/deployment-targets", map[string]any{
		"service_id":      service["id"],
		"environment_id":  prodEnv["id"],
		"executor_type":   "mock",
		"target_type":     "server",
		"target_ref_id":   server["id"],
		"timeout_seconds": 60,
		"env_vars":        "{}",
	})
	owner := postForData(t, router, "/api/v1/users", map[string]any{
		"username":     "agent-owner",
		"display_name": "Agent Owner",
		"role":         "admin",
		"password":     "agent-owner-password",
	})
	key := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":          "agent",
		"owner_user_id": owner["id"],
		"scopes":        `["inventory:read","release:create","release:confirm","release:read"]`,
	})
	headers := map[string]string{"Authorization": "Bearer " + key["plaintext"].(string)}

	services := getForDataWithHeaders(t, router, "/api/v1/agent/services?q=订单", headers).(map[string]any)
	if int(services["count"].(float64)) != 2 {
		t.Fatalf("expected two service candidates so agent cannot guess, got %#v", services)
	}
	versions := getForDataWithHeaders(t, router, "/api/v1/agent/services/"+service["id"].(string)+"/versions?q=v1.2", headers).(map[string]any)
	if int(versions["count"].(float64)) != 1 {
		t.Fatalf("expected one version candidate, got %#v", versions)
	}
	targets := getForDataWithHeaders(t, router, "/api/v1/agent/deployment-targets?service_id="+service["id"].(string)+"&environment_id="+testEnv["id"].(string), headers).(map[string]any)
	if int(targets["count"].(float64)) != 1 {
		t.Fatalf("expected one deployment target candidate, got %#v", targets)
	}

	preflight := postForStatusWithHeaders(t, router, "/api/v1/agent/release-intents/preflight", map[string]any{
		"service_id":           service["id"],
		"environment_id":       testEnv["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": testTarget["id"],
	}, http.StatusOK, headers)
	if preflight["result"] != "pass" || preflight["next_action"] != "self_confirm" {
		t.Fatalf("expected non-production pass preflight, got %#v", preflight)
	}

	created := postForStatusWithHeaders(t, router, "/api/v1/agent/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       testEnv["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": testTarget["id"],
		"idempotency_key":      "agent-release-1",
		"source":               "api",
		"created_by_type":      "user",
		"agent_name":           "codex",
		"skill_version":        "0.1.0",
		"intent_summary":       "发布订单服务 v1.2.3 到测试环境",
		"client_request_id":    "req-agent-1",
		"conversation_ref":     "thread-local",
	}, http.StatusCreated, headers)
	release := created["release"].(map[string]any)
	if release["source"] != "ai_agent" {
		t.Fatalf("expected source=ai_agent enforced by server, got %#v", release)
	}
	keyID := key["key"].(map[string]any)["id"].(string)
	if release["created_by_type"] != "api_key" || release["created_by_id"] != keyID {
		t.Fatalf("expected api key actor to be enforced, got %#v", release)
	}
	metadata := release["metadata"].(string)
	for _, want := range []string{"agent_release", "codex", "0.1.0", "req-agent-1"} {
		if !strings.Contains(metadata, want) {
			t.Fatalf("expected metadata %q to contain %q", metadata, want)
		}
	}

	confirmed := postForStatusWithHeaders(t, router, "/api/v1/agent/release-requests/"+release["id"].(string)+"/confirm", map[string]any{}, http.StatusOK, headers)
	if confirmed["status"] != "queued" {
		t.Fatalf("expected non-production agent release to be queued after confirm, got %#v", confirmed)
	}
	summary := getForDataWithHeaders(t, router, "/api/v1/agent/release-requests/"+release["id"].(string)+"/summary", headers).(map[string]any)
	if summary["next_action"] != "wait" {
		t.Fatalf("expected queued summary next_action wait, got %#v", summary)
	}
	deployRecords := summary["deploy_records"].(map[string]any)
	if int(deployRecords["total"].(float64)) != 1 {
		t.Fatalf("expected summary to include one deploy record, got %#v", summary)
	}

	prodCreated := postForStatusWithHeaders(t, router, "/api/v1/agent/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       prodEnv["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": prodTarget["id"],
		"idempotency_key":      "agent-prod-release-1",
		"agent_name":           "codex",
		"skill_version":        "0.1.0",
		"intent_summary":       "发布订单服务 v1.2.3 到生产环境",
	}, http.StatusCreated, headers)
	prodRelease := prodCreated["release"].(map[string]any)
	if prodCreated["next_action"] != "admin_confirm" || prodRelease["source"] != "ai_agent" {
		t.Fatalf("expected production agent release to wait for admin, got %#v", prodCreated)
	}
	postExpectStatusWithHeaders(t, router, "/api/v1/agent/release-requests/"+prodRelease["id"].(string)+"/confirm", map[string]any{}, http.StatusBadRequest, headers)

	events := getForDataWithHeaders(t, router, "/api/v1/release-requests/"+release["id"].(string)+"/events", headers)
	eventBytes, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(eventBytes, []byte(keyID)) || !bytes.Contains(eventBytes, []byte("api_key_id")) {
		t.Fatalf("expected API key actor in agent release events, got %s", eventBytes)
	}
}
