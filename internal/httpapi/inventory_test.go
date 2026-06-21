package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/config"
	"ai-pub/internal/migration"
)

func TestInventoryAPIFlow(t *testing.T) {
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
	version := postForData(t, router, "/api/v1/services/"+service["id"].(string)+"/versions", map[string]any{
		"version": "v1.0.0",
		"source":  "manual",
	})
	env := postForData(t, router, "/api/v1/environments", map[string]any{
		"name":          "测试环境",
		"slug":          "test",
		"is_production": false,
	})
	patchedEnvironment := patchForData(t, router, "/api/v1/environments/"+env["id"].(string), map[string]any{
		"name": "测试环境（已更新）",
	})
	if patchedEnvironment["name"] != "测试环境（已更新）" {
		t.Fatalf("expected patched environment, got %#v", patchedEnvironment)
	}
	server := postForData(t, router, "/api/v1/servers", map[string]any{
		"name":      "mock-1",
		"host":      "127.0.0.1",
		"username":  "deploy",
		"auth_type": "none",
	})
	patchedServer := patchForData(t, router, "/api/v1/servers/"+server["id"].(string), map[string]any{
		"host":    "127.0.0.2",
		"enabled": false,
	})
	if patchedServer["host"] != "127.0.0.2" || patchedServer["enabled"] != false {
		t.Fatalf("expected patched server, got %#v", patchedServer)
	}
	server = patchForData(t, router, "/api/v1/servers/"+server["id"].(string), map[string]any{"enabled": true})
	serverGroup := postForData(t, router, "/api/v1/server-groups", map[string]any{
		"name":       "mock-group",
		"server_ids": []string{server["id"].(string)},
	})
	patchedGroup := patchForData(t, router, "/api/v1/server-groups/"+serverGroup["id"].(string), map[string]any{
		"description": "patched group",
		"server_ids":  []string{server["id"].(string)},
	})
	if patchedGroup["description"] != "patched group" {
		t.Fatalf("expected patched server group, got %#v", patchedGroup)
	}
	serverGroups := getForData(t, router, "/api/v1/server-groups")
	serverGroupBytes, err := json.Marshal(serverGroups)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(serverGroupBytes, []byte(serverGroup["id"].(string))) || !bytes.Contains(serverGroupBytes, []byte(server["id"].(string))) {
		t.Fatalf("expected server group with member server, got %s", serverGroupBytes)
	}
	target := postForData(t, router, "/api/v1/deployment-targets", map[string]any{
		"service_id":       service["id"],
		"environment_id":   env["id"],
		"executor_type":    "mock",
		"target_type":      "server",
		"target_ref_id":    server["id"],
		"timeout_seconds":  60,
		"service_version":  version["id"],
		"ignored_by_m1":    true,
		"still_json_safe":  true,
		"another_ignored":  "ok",
		"env_vars":         "{}",
		"script_path":      "",
		"working_dir":      "",
		"deployment_label": "mock",
	})
	if target["id"] == "" {
		t.Fatal("expected deployment target id")
	}

	user := postForData(t, router, "/api/v1/users", map[string]any{
		"username":     "admin",
		"display_name": "管理员",
		"role":         "admin",
		"password":     "local-test-password",
	})
	patchedUser := patchForData(t, router, "/api/v1/users/"+user["id"].(string), map[string]any{
		"display_name": "发布管理员",
		"role":         "employee",
		"enabled":      false,
	})
	if patchedUser["display_name"] != "发布管理员" || patchedUser["role"] != "employee" || patchedUser["enabled"] != false {
		t.Fatalf("expected patched user fields, got %#v", patchedUser)
	}
	user = patchForData(t, router, "/api/v1/users/"+user["id"].(string), map[string]any{
		"role":    "admin",
		"enabled": true,
	})
	keyBody := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "CI",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["release:create"]`,
	})
	if keyBody["plaintext"] == "" {
		t.Fatal("expected plaintext api key on create response")
	}

	keys := getForData(t, router, "/api/v1/api-keys")
	raw, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("plaintext")) {
		t.Fatalf("api key list must not include plaintext: %s", raw)
	}
	key := keyBody["key"].(map[string]any)
	postExpectStatusWithHeaders(t, router, "/api/v1/projects", map[string]any{
		"name": "Forbidden Admin Project",
		"slug": "forbidden-admin-project",
	}, http.StatusForbidden, map[string]string{"Authorization": "Bearer " + keyBody["plaintext"].(string)})
	adminKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "Admin",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["admin:write"]`,
	})
	adminProject := postForStatusWithHeaders(t, router, "/api/v1/projects", map[string]any{
		"name": "Admin Project",
		"slug": "admin-project",
	}, http.StatusCreated, map[string]string{"Authorization": "Bearer " + adminKey["plaintext"].(string)})
	if adminProject["id"] == "" {
		t.Fatalf("expected admin:write key to create project, got %#v", adminProject)
	}
	apiPreflight := postForStatusWithHeaders(t, router, "/api/v1/release-requests/preflight", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
	}, http.StatusOK, map[string]string{"Authorization": "Bearer " + keyBody["plaintext"].(string)})
	if apiPreflight["result"] != "pass" {
		t.Fatalf("expected api key preflight pass, got %#v", apiPreflight)
	}
	apiCreated := postForStatusWithHeaders(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"created_by_type":      "user",
		"created_by_id":        user["id"],
		"idempotency_key":      "api-key-release-1",
	}, http.StatusCreated, map[string]string{"Authorization": "Bearer " + keyBody["plaintext"].(string)})
	apiRelease := apiCreated["release"].(map[string]any)
	if apiRelease["created_by_type"] != "api_key" || apiRelease["created_by_id"] != key["id"] || apiRelease["source"] != "api" {
		t.Fatalf("expected api key actor to be enforced, got %#v", apiRelease)
	}
	conflictVersion := postForData(t, router, "/api/v1/services/"+service["id"].(string)+"/versions", map[string]any{
		"version": "v1.0.1",
		"source":  "manual",
	})
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   conflictVersion["id"],
		"deployment_target_id": target["id"],
		"idempotency_key":      "api-key-release-1",
	}, http.StatusConflict, map[string]string{"Authorization": "Bearer " + keyBody["plaintext"].(string)})
	apiEvents := getForData(t, router, "/api/v1/release-requests/"+apiRelease["id"].(string)+"/events")
	apiEventBytes, err := json.Marshal(apiEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(apiEventBytes, []byte(key["id"].(string))) ||
		!bytes.Contains(apiEventBytes, []byte("api_key_id")) ||
		!bytes.Contains(apiEventBytes, []byte("preflight_checked")) {
		t.Fatalf("expected api key id and preflight_checked in create events, got %s", apiEventBytes)
	}
	readOnlyKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "Read Only",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["release:read"]`,
	})
	readReleases := getForDataWithHeaders(t, router, "/api/v1/release-requests", map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	if readReleases == nil {
		t.Fatal("expected release:read key to list releases")
	}
	readEvents := getForDataWithHeaders(t, router, "/api/v1/release-requests/"+apiRelease["id"].(string)+"/events", map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	if readEvents == nil {
		t.Fatal("expected release:read key to list release events")
	}
	readRollbackCandidates := getForDataWithHeaders(t, router, "/api/v1/release-requests/"+apiRelease["id"].(string)+"/rollback-candidates", map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	if readRollbackCandidates == nil {
		t.Fatal("expected release:read key to list rollback candidates")
	}
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests/preflight", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
	}, http.StatusForbidden, map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"idempotency_key":      "api-key-release-forbidden",
	}, http.StatusForbidden, map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests/"+apiRelease["id"].(string)+"/rollback", map[string]any{
		"idempotency_key": "api-key-rollback-forbidden",
	}, http.StatusForbidden, map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	rollbackKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "Rollback",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["release:rollback"]`,
	})
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests/"+apiRelease["id"].(string)+"/rollback", map[string]any{
		"idempotency_key": "api-key-rollback-no-candidates",
	}, http.StatusBadRequest, map[string]string{"Authorization": "Bearer " + rollbackKey["plaintext"].(string)})
	confirmKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "Confirm",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["release:confirm"]`,
	})
	confirmTarget := postForData(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"created_by_type":      "user",
		"created_by_id":        user["id"],
		"idempotency_key":      "api-key-confirm-target",
	})
	confirmRelease := confirmTarget["release"].(map[string]any)
	confirmedByKey := postForStatusWithHeaders(t, router, "/api/v1/release-requests/"+confirmRelease["id"].(string)+"/confirm", map[string]any{
		"user_id": user["id"],
	}, http.StatusOK, map[string]string{"Authorization": "Bearer " + confirmKey["plaintext"].(string)})
	if confirmedByKey["status"] != "queued" {
		t.Fatalf("expected api key confirmed release to be queued, got %#v", confirmedByKey)
	}
	confirmEvents := getForData(t, router, "/api/v1/release-requests/"+confirmRelease["id"].(string)+"/events")
	confirmEventBytes, err := json.Marshal(confirmEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(confirmEventBytes, []byte(confirmKey["key"].(map[string]any)["id"].(string))) || !bytes.Contains(confirmEventBytes, []byte("api_key_id")) {
		t.Fatalf("expected api key id in release_confirmed event, got %s", confirmEventBytes)
	}
	rejectTarget := postForData(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"created_by_type":      "user",
		"created_by_id":        user["id"],
		"idempotency_key":      "api-key-reject-target",
	})
	rejectRelease := rejectTarget["release"].(map[string]any)
	rejectedByKey := postForStatusWithHeaders(t, router, "/api/v1/release-requests/"+rejectRelease["id"].(string)+"/reject", map[string]any{
		"user_id": user["id"],
		"reason":  "api key reject",
	}, http.StatusOK, map[string]string{"Authorization": "Bearer " + confirmKey["plaintext"].(string)})
	if rejectedByKey["status"] != "rejected" {
		t.Fatalf("expected api key rejected release, got %#v", rejectedByKey)
	}
	rejectEvents := getForData(t, router, "/api/v1/release-requests/"+rejectRelease["id"].(string)+"/events")
	rejectEventBytes, err := json.Marshal(rejectEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rejectEventBytes, []byte(confirmKey["key"].(map[string]any)["id"].(string))) || !bytes.Contains(rejectEventBytes, []byte("api_key_id")) {
		t.Fatalf("expected api key id in release_rejected event, got %s", rejectEventBytes)
	}
	cancelTarget := postForData(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"created_by_type":      "user",
		"created_by_id":        user["id"],
		"idempotency_key":      "api-key-cancel-target",
	})
	cancelRelease := cancelTarget["release"].(map[string]any)
	cancelledByKey := postForStatusWithHeaders(t, router, "/api/v1/release-requests/"+cancelRelease["id"].(string)+"/cancel", map[string]any{
		"user_id": user["id"],
	}, http.StatusOK, map[string]string{"Authorization": "Bearer " + confirmKey["plaintext"].(string)})
	if cancelledByKey["status"] != "cancelled" {
		t.Fatalf("expected api key cancelled release, got %#v", cancelledByKey)
	}
	cancelEvents := getForData(t, router, "/api/v1/release-requests/"+cancelRelease["id"].(string)+"/events")
	cancelEventBytes, err := json.Marshal(cancelEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cancelEventBytes, []byte(confirmKey["key"].(map[string]any)["id"].(string))) || !bytes.Contains(cancelEventBytes, []byte("api_key_id")) {
		t.Fatalf("expected api key id in release_cancelled event, got %s", cancelEventBytes)
	}
	deployReadKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":       "Deploy Read",
		"owner_type": "user",
		"owner_id":   user["id"],
		"scopes":     `["deploy:read"]`,
	})
	deployRecordsByKey := getForDataWithHeaders(t, router, "/api/v1/deploy-records", map[string]string{"Authorization": "Bearer " + deployReadKey["plaintext"].(string)})
	if deployRecordsByKey == nil {
		t.Fatal("expected deploy:read key to list deploy records")
	}
	getExpectStatusWithHeaders(t, router, "/api/v1/deploy-records", http.StatusForbidden, map[string]string{"Authorization": "Bearer " + readOnlyKey["plaintext"].(string)})
	patchedKey := patchForData(t, router, "/api/v1/api-keys/"+key["id"].(string), map[string]any{
		"enabled": false,
		"scopes":  `["release:create","release:read"]`,
	})
	if patchedKey["enabled"] != false {
		t.Fatalf("expected disabled api key, got %#v", patchedKey)
	}
	postExpectStatusWithHeaders(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"idempotency_key":      "api-key-release-disabled",
	}, http.StatusUnauthorized, map[string]string{"Authorization": "Bearer " + keyBody["plaintext"].(string)})
	deleteForData(t, router, "/api/v1/api-keys/"+key["id"].(string))
	keysAfterDelete := getForData(t, router, "/api/v1/api-keys")
	deletedBytes, err := json.Marshal(keysAfterDelete)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(deletedBytes, []byte(key["id"].(string))) {
		t.Fatalf("expected deleted api key to be removed, got %s", deletedBytes)
	}
	credential := postForData(t, router, "/api/v1/credentials", map[string]any{
		"name":   "deploy key",
		"type":   "private_key",
		"secret": "not-a-real-key",
	})
	if credential["id"] == "" {
		t.Fatal("expected credential id")
	}
	credentials := getForData(t, router, "/api/v1/credentials")
	credBytes, err := json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(credBytes, []byte("not-a-real-key")) || bytes.Contains(credBytes, []byte("secret")) {
		t.Fatalf("credential list must not expose secret: %s", credBytes)
	}
	notification := postForData(t, router, "/api/v1/notification-configs", map[string]any{
		"name":        "wecom",
		"channel":     "wecom_robot",
		"webhook_url": "http://wecom.test/webhook",
	})
	if notification["enabled"] != true {
		t.Fatalf("expected notification config enabled by default, got %#v", notification)
	}
	patchedNotification := patchForData(t, router, "/api/v1/notification-configs/"+notification["id"].(string), map[string]any{
		"name":    "wecom disabled",
		"enabled": false,
	})
	if patchedNotification["enabled"] != false || patchedNotification["name"] != "wecom disabled" {
		t.Fatalf("expected patched notification config, got %#v", patchedNotification)
	}
	postExpectStatusWithHeaders(t, router, "/api/v1/notification-configs/"+notification["id"].(string)+"/test", map[string]any{}, http.StatusNotFound, nil)

	preflight := postOKForData(t, router, "/api/v1/release-requests/preflight", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
	})
	if preflight["result"] != "pass" {
		t.Fatalf("expected preflight pass, got %#v", preflight)
	}
	created := postForData(t, router, "/api/v1/release-requests", map[string]any{
		"service_id":           service["id"],
		"environment_id":       env["id"],
		"service_version_id":   version["id"],
		"deployment_target_id": target["id"],
		"created_by_type":      "user",
		"created_by_id":        user["id"],
		"idempotency_key":      "http-idem-1",
	})
	release := created["release"].(map[string]any)
	existingPreflight := postOKForData(t, router, "/api/v1/release-requests/"+release["id"].(string)+"/preflight", map[string]any{})
	if existingPreflight["result"] != "pass" {
		t.Fatalf("expected existing release preflight pass, got %#v", existingPreflight)
	}
	confirmed := postOKForData(t, router, "/api/v1/release-requests/"+release["id"].(string)+"/confirm", map[string]any{
		"user_id": user["id"],
	})
	if confirmed["status"] != "queued" {
		t.Fatalf("expected queued release, got %#v", confirmed)
	}
	events := getForData(t, router, "/api/v1/release-requests/"+release["id"].(string)+"/events")
	eventBytes, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(eventBytes, []byte("preflight_checked")) || !bytes.Contains(eventBytes, []byte("release_confirmed")) {
		t.Fatalf("expected preflight_checked and release_confirmed events, got %s", eventBytes)
	}
	deploys := getForData(t, router, "/api/v1/deploy-records")
	deployBytes, err := json.Marshal(deploys)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(deployBytes, []byte(release["id"].(string))) {
		t.Fatalf("expected deploy record list to include release id, got %s", deployBytes)
	}
	states := getForData(t, router, "/api/v1/server-deployment-states")
	if states == nil {
		t.Fatal("expected server deployment states response")
	}
	ops := getForData(t, router, "/api/v1/ops/summary")
	opsBytes, err := json.Marshal(ops)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(opsBytes, []byte("total_release_requests")) {
		t.Fatalf("expected ops summary fields, got %s", opsBytes)
	}
}

func postForData(t *testing.T, handler http.Handler, path string, body map[string]any) map[string]any {
	t.Helper()
	return postForStatus(t, handler, path, body, http.StatusCreated)
}

func postOKForData(t *testing.T, handler http.Handler, path string, body map[string]any) map[string]any {
	t.Helper()
	return postForStatus(t, handler, path, body, http.StatusOK)
}

func postForStatus(t *testing.T, handler http.Handler, path string, body map[string]any, status int) map[string]any {
	t.Helper()
	return postForStatusWithHeaders(t, handler, path, body, status, nil)
}

func postForStatusWithHeaders(t *testing.T, handler http.Handler, path string, body map[string]any, status int, headers map[string]string) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("POST %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func postExpectStatusWithHeaders(t *testing.T, handler http.Handler, path string, body map[string]any, status int, headers map[string]string) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("POST %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func patchForData(t *testing.T, handler http.Handler, path string, body map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func deleteForData(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func getForData(t *testing.T, handler http.Handler, path string) any {
	t.Helper()
	return getForDataWithHeaders(t, handler, path, nil)
}

func getForDataWithHeaders(t *testing.T, handler http.Handler, path string, headers map[string]string) any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
	var out struct {
		Data any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func getExpectStatusWithHeaders(t *testing.T, handler http.Handler, path string, status int, headers map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("GET %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func newHTTPTestDB(t *testing.T) *sql.DB {
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
	return db
}
