package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-pub/internal/config"
)

// postVersionRegistration 发起版本登记请求，返回状态码与解析后的响应体。
func postVersionRegistration(t *testing.T, handler http.Handler, headers map[string]string, body map[string]any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/version-registrations", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&out)
	return rec.Code, out
}

func TestVersionRegistrationFlow(t *testing.T) {
	db := newHTTPTestDB(t)
	router := NewRouter(Dependencies{DB: db, Config: config.Config{}})

	// 预置项目、服务（slug 即 CI 登记用的 key）。
	project := postForData(t, router, "/api/v1/projects", map[string]any{
		"name": "供应链系统",
		"slug": "supply-chain",
	})
	service := postForData(t, router, "/api/v1/services", map[string]any{
		"project_id": project["id"],
		"name":       "订单服务",
		"slug":       "order-api",
	})

	// 建一个 admin 用户与一个 version:write API Key。
	user := postForData(t, router, "/api/v1/users", map[string]any{
		"username":     "admin",
		"display_name": "管理员",
		"role":         "admin",
		"password":     "local-test-password",
	})
	writeKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":          "CI Register",
		"owner_user_id": user["id"],
		"scopes":        `["version:write"]`,
	})
	bearer := "Bearer " + writeKey["plaintext"].(string)

	regBody := func(version, commit, artifact string) map[string]any {
		return map[string]any{
			"project_key":  "supply-chain",
			"service_key":  "order-api",
			"version":      version,
			"commit_sha":   commit,
			"artifact_url": artifact,
			"metadata": map[string]any{
				"provider": "gitlab",
				"run_id":   "9281",
			},
		}
	}

	// 1. 首次登记 → 201。
	status, body := postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:9281",
	}, regBody("2026.06.23-1842", "7f3c6b5b", "harbor.example/team/order-api@sha256:abc"))
	if status != http.StatusCreated {
		t.Fatalf("expected 201 on first registration, got %d body %v", status, body)
	}
	versionID := body["data"].(map[string]any)["id"].(string)

	// 2. 同幂等键、同指纹重试 → 200，返回同一版本。
	status, body = postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:9281",
	}, regBody("2026.06.23-1842", "7f3c6b5b", "harbor.example/team/order-api@sha256:abc"))
	if status != http.StatusOK {
		t.Fatalf("expected 200 on idempotent retry, got %d", status)
	}
	if body["data"].(map[string]any)["id"].(string) != versionID {
		t.Fatalf("expected same version id on idempotent retry")
	}

	// 3. 同幂等键、不同指纹 → 409 idempotency_conflict。
	status, _ = postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:9281",
	}, regBody("2026.06.23-1842", "different", "harbor.example/team/order-api@sha256:abc"))
	if status != http.StatusConflict {
		t.Fatalf("expected 409 idempotency_conflict, got %d", status)
	}

	// 4. 不同幂等键、同版本同 commit 制品 → 200。
	status, _ = postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:9999",
	}, regBody("2026.06.23-1842", "7f3c6b5b", "harbor.example/team/order-api@sha256:abc"))
	if status != http.StatusOK {
		t.Fatalf("expected 200 on same version same fingerprint, got %d", status)
	}

	// 5. 不同幂等键、同版本但制品不同 → 409 version_conflict。
	status, _ = postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:7777",
	}, regBody("2026.06.23-1842", "7f3c6b5b", "harbor.example/team/order-api@sha256:DIFFERENT"))
	if status != http.StatusConflict {
		t.Fatalf("expected 409 version_conflict, got %d", status)
	}

	// 6. 服务不存在 → 404。
	status, _ = postVersionRegistration(t, router, map[string]string{
		"Authorization":    bearer,
		"Idempotency-Key": "gitlab:miss",
	}, map[string]any{
		"project_key":  "supply-chain",
		"service_key":  "missing-service",
		"version":      "2026.06.23-9999",
		"commit_sha":   "x",
		"artifact_url": "y",
	})
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for missing service, got %d", status)
	}
	_ = service

	// 7. 缺少 version:write scope 的 Key → 403。
	readOnlyKey := postForData(t, router, "/api/v1/api-keys", map[string]any{
		"name":          "Read Only",
		"owner_user_id": user["id"],
		"scopes":        `["release:read"]`,
	})
	status, _ = postVersionRegistration(t, router, map[string]string{
		"Authorization":    "Bearer " + readOnlyKey["plaintext"].(string),
		"Idempotency-Key": "gitlab:forbidden",
	}, regBody("2026.06.23-forbidden", "x", "y"))
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for missing version:write scope, got %d", status)
	}

	// 8. 来源由服务端强制写入为 ci，不可伪造为 manual。
	if body["data"].(map[string]any)["source"] != "ci" {
		t.Fatalf("expected source=ci forced by server, got %v", body["data"])
	}

	// 9. 手动登记同版本号 → 409 version_conflict（手动与 CI 共享版本唯一性）。
	postExpectStatusWithHeaders(t, router, "/api/v1/services/"+service["id"].(string)+"/versions", map[string]any{
		"version": "2026.06.23-1842",
		"source":  "manual",
	}, http.StatusConflict, nil)

	// 10. 手动登记新版本号 → 201，且 source 被强制为 manual。
	manualVersion := postForData(t, router, "/api/v1/services/"+service["id"].(string)+"/versions", map[string]any{
		"version": "manual-1",
		"source":  "ci", // 试图伪造，应被服务端覆盖为 manual
	})
	if manualVersion["source"] != "manual" {
		t.Fatalf("expected source=manual forced by server, got %v", manualVersion["source"])
	}
}
