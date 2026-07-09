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

	"ai-pub/internal/auth"
	"ai-pub/internal/config"
	"ai-pub/internal/domain"
	"ai-pub/internal/migration"
	"ai-pub/internal/repository"
)

func TestSessionLoginProtectsBusinessAPIsAndAdminWrites(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := migration.NewRunner(db, "sqlite", os.DirFS("../..")).Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("employee-password")
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(db)
	employee, err := store.CreateUserWithPassword(context.Background(), domain.User{Username: "employee", DisplayName: "Employee", Role: "employee", PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Dependencies{DB: db, Config: config.Config{JWTSecret: "test-session-secret"}})

	anonymous := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	if rec := httptest.NewRecorder(); func() *httptest.ResponseRecorder { router.ServeHTTP(rec, anonymous); return rec }().Code != http.StatusUnauthorized {
		t.Fatal("anonymous business request must be rejected")
	}

	body, _ := json.Marshal(map[string]string{"username": "employee", "password": "employee-password"})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK || len(loginRecorder.Result().Cookies()) != 1 {
		t.Fatalf("unexpected login response: %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookie := loginRecorder.Result().Cookies()[0]

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(cookie)
	meRecorder := httptest.NewRecorder()
	router.ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected me response: %d %s", meRecorder.Code, meRecorder.Body.String())
	}
	assertNoStoreJSON(t, meRecorder)

	projectRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader([]byte(`{"name":"forbidden","slug":"forbidden"}`)))
	projectRequest.Header.Set("Content-Type", "application/json")
	projectRequest.AddCookie(cookie)
	projectRecorder := httptest.NewRecorder()
	router.ServeHTTP(projectRecorder, projectRequest)
	if projectRecorder.Code != http.StatusForbidden {
		t.Fatalf("employee admin write got %d: %s", projectRecorder.Code, projectRecorder.Body.String())
	}

	keyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(`{"name":"employee-ci","owner_user_id":"forged-owner","scopes":"[\"release:create\"]"}`))
	keyRequest.Header.Set("Content-Type", "application/json")
	keyRequest.AddCookie(cookie)
	keyRecorder := httptest.NewRecorder()
	router.ServeHTTP(keyRecorder, keyRequest)
	if keyRecorder.Code != http.StatusCreated {
		t.Fatalf("employee key create got %d: %s", keyRecorder.Code, keyRecorder.Body.String())
	}
	var keyResponse struct {
		Data struct {
			Key       domain.APIKey `json:"key"`
			Plaintext string        `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(keyRecorder.Body.Bytes(), &keyResponse); err != nil {
		t.Fatal(err)
	}
	if keyResponse.Data.Key.OwnerUserID != employee.ID {
		t.Fatalf("employee must not choose API key owner: %#v", keyResponse.Data.Key)
	}
	for _, scopes := range []string{`["admin:write"]`, `["*"]`, `["unknown:scope"]`} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(`{"name":"invalid","scopes":`+scopes+`}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("employee key scopes %s got %d: %s", scopes, recorder.Code, recorder.Body.String())
		}
	}
	if apiKeyHasScope(domain.APIKey{Scopes: `["*"]`}, "admin:write") {
		t.Fatal("legacy wildcard scope must not authorize requests")
	}
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+keyResponse.Data.Key.ID, bytes.NewBufferString(`{"scopes":"[\"release:create\",\"release:read\"]"}`))
	patchRequest.Header.Set("Content-Type", "application/json")
	patchRequest.AddCookie(cookie)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusForbidden {
		t.Fatalf("employee scope expansion got %d: %s", patchRecorder.Code, patchRecorder.Body.String())
	}

	invalidBearer := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	invalidBearer.Header.Set("Authorization", "Bearer not-a-real-key")
	invalidBearerRecorder := httptest.NewRecorder()
	router.ServeHTTP(invalidBearerRecorder, invalidBearer)
	if invalidBearerRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer got %d: %s", invalidBearerRecorder.Code, invalidBearerRecorder.Body.String())
	}
	assertNoStoreJSON(t, invalidBearerRecorder)

	inventoryKeyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(`{"name":"inventory","scopes":"[\"inventory:read\"]"}`))
	inventoryKeyRequest.Header.Set("Content-Type", "application/json")
	inventoryKeyRequest.AddCookie(cookie)
	inventoryKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(inventoryKeyRecorder, inventoryKeyRequest)
	if inventoryKeyRecorder.Code != http.StatusCreated {
		t.Fatalf("inventory key create got %d: %s", inventoryKeyRecorder.Code, inventoryKeyRecorder.Body.String())
	}
	var inventoryKeyResponse struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(inventoryKeyRecorder.Body.Bytes(), &inventoryKeyResponse); err != nil {
		t.Fatal(err)
	}
	inventoryRead := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	inventoryRead.Header.Set("Authorization", "Bearer "+inventoryKeyResponse.Data.Plaintext)
	inventoryReadRecorder := httptest.NewRecorder()
	router.ServeHTTP(inventoryReadRecorder, inventoryRead)
	if inventoryReadRecorder.Code != http.StatusOK {
		t.Fatalf("inventory key read got %d: %s", inventoryReadRecorder.Code, inventoryReadRecorder.Body.String())
	}
	releaseKeyRead := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	releaseKeyRead.Header.Set("Authorization", "Bearer "+keyResponse.Data.Plaintext)
	releaseKeyReadRecorder := httptest.NewRecorder()
	router.ServeHTTP(releaseKeyReadRecorder, releaseKeyRead)
	if releaseKeyReadRecorder.Code != http.StatusForbidden {
		t.Fatalf("release key inventory read got %d: %s", releaseKeyReadRecorder.Code, releaseKeyReadRecorder.Body.String())
	}

	listKeysRequest := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	listKeysRequest.AddCookie(cookie)
	listKeysRecorder := httptest.NewRecorder()
	router.ServeHTTP(listKeysRecorder, listKeysRequest)
	if listKeysRecorder.Code != http.StatusOK {
		t.Fatalf("employee key list got %d: %s", listKeysRecorder.Code, listKeysRecorder.Body.String())
	}
	var listKeysResponse struct {
		Data []domain.APIKey `json:"data"`
	}
	if err := json.Unmarshal(listKeysRecorder.Body.Bytes(), &listKeysResponse); err != nil {
		t.Fatal(err)
	}
	foundInitial := false
	for _, key := range listKeysResponse.Data {
		if key.OwnerUserID != employee.ID {
			t.Fatalf("employee should only see own keys, got %#v", listKeysResponse.Data)
		}
		foundInitial = foundInitial || key.ID == keyResponse.Data.Key.ID
	}
	if !foundInitial {
		t.Fatalf("employee should only see own key, got %#v", listKeysResponse.Data)
	}

	otherKey, err := store.CreateAPIKey(context.Background(), domain.APIKey{Name: "other-ci", OwnerUserID: "other-user", Scopes: `["release:create"]`})
	if err != nil {
		t.Fatal(err)
	}
	patchKeyRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+otherKey.Key.ID, bytes.NewBufferString(`{"enabled":false}`))
	patchKeyRequest.Header.Set("Content-Type", "application/json")
	patchKeyRequest.AddCookie(cookie)
	patchKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchKeyRecorder, patchKeyRequest)
	if patchKeyRecorder.Code != http.StatusForbidden {
		t.Fatalf("employee key patch got %d: %s", patchKeyRecorder.Code, patchKeyRecorder.Body.String())
	}

	deleteKeyRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+keyResponse.Data.Key.ID, nil)
	deleteKeyRequest.AddCookie(cookie)
	deleteKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteKeyRecorder, deleteKeyRequest)
	if deleteKeyRecorder.Code != http.StatusOK {
		t.Fatalf("employee key delete got %d: %s", deleteKeyRecorder.Code, deleteKeyRecorder.Body.String())
	}
}

func TestProtectedUsersAndPasswordReset(t *testing.T) {
	db := newHTTPTestDB(t)
	store := repository.NewStore(db)
	admin := createHTTPTestUser(t, store, "admin", "Root Admin", "admin", "admin-password")
	manager := createHTTPTestUser(t, store, "manager", "Manager", "admin", "manager-password")
	demo := createHTTPTestUser(t, store, "demo", "Demo", "admin", "demo-password")
	employee := createHTTPTestUser(t, store, "employee", "Employee", "employee", "employee-password")
	router := NewRouter(Dependencies{DB: db, Config: config.Config{JWTSecret: "test-session-secret", DemoMode: true, DemoProtectedUsernames: "demo"}})

	managerCookie := loginCookie(t, router, "manager", "manager-password")
	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+admin.ID, map[string]any{"display_name": "Changed"}, managerCookie, http.StatusForbidden)
	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+admin.ID, map[string]any{"enabled": false}, managerCookie, http.StatusForbidden)
	postWithCookieExpectStatus(t, router, "/api/v1/users/"+admin.ID+"/password", map[string]any{"password": "changed-password"}, managerCookie, http.StatusForbidden)

	adminKey, err := store.CreateAPIKey(context.Background(), domain.APIKey{Name: "admin-key", OwnerUserID: manager.ID, Scopes: `["admin:write"]`})
	if err != nil {
		t.Fatal(err)
	}
	patchWithHeadersExpectStatus(t, router, "/api/v1/users/"+admin.ID, map[string]any{"enabled": false}, map[string]string{"Authorization": "Bearer " + adminKey.Plaintext}, http.StatusForbidden)
	postWithHeadersExpectStatus(t, router, "/api/v1/users/"+employee.ID+"/password", map[string]any{"password": "key-password"}, map[string]string{"Authorization": "Bearer " + adminKey.Plaintext}, http.StatusUnauthorized)

	demoCookie := loginCookie(t, router, "demo", "demo-password")
	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+demo.ID, map[string]any{"enabled": false}, demoCookie, http.StatusForbidden)
	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+demo.ID, map[string]any{"role": "employee"}, demoCookie, http.StatusForbidden)
	postWithCookieExpectStatus(t, router, "/api/v1/users/"+demo.ID+"/password", map[string]any{"password": "changed-demo-password"}, demoCookie, http.StatusForbidden)

	postWithCookieExpectStatus(t, router, "/api/v1/users/"+employee.ID+"/password", map[string]any{"password": "employee-new-password"}, managerCookie, http.StatusOK)
	loginExpectStatus(t, router, "employee", "employee-password", http.StatusUnauthorized)
	loginExpectStatus(t, router, "employee", "employee-new-password", http.StatusOK)

	adminCookie := loginCookie(t, router, "admin", "admin-password")
	postWithCookieExpectStatus(t, router, "/api/v1/users/"+admin.ID+"/password", map[string]any{"password": "admin-new-password"}, adminCookie, http.StatusOK)
	loginExpectStatus(t, router, "admin", "admin-password", http.StatusUnauthorized)
	loginExpectStatus(t, router, "admin", "admin-new-password", http.StatusOK)
}

func TestLastEnabledAdminCannotBeRemoved(t *testing.T) {
	db := newHTTPTestDB(t)
	store := repository.NewStore(db)
	solo := createHTTPTestUser(t, store, "solo-admin", "Solo", "admin", "solo-password")
	router := NewRouter(Dependencies{DB: db, Config: config.Config{JWTSecret: "test-session-secret"}})
	cookie := loginCookie(t, router, "solo-admin", "solo-password")

	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+solo.ID, map[string]any{"enabled": false}, cookie, http.StatusBadRequest)
	patchWithCookieExpectStatus(t, router, "/api/v1/users/"+solo.ID, map[string]any{"role": "employee"}, cookie, http.StatusBadRequest)
}

func TestUserDeleteSessionProtections(t *testing.T) {
	db := newHTTPTestDB(t)
	store := repository.NewStore(db)
	admin := createHTTPTestUser(t, store, "admin", "Root Admin", "admin", "admin-password")
	manager := createHTTPTestUser(t, store, "manager", "Manager", "admin", "manager-password")
	demo := createHTTPTestUser(t, store, "demo", "Demo", "admin", "demo-password")
	employee := createHTTPTestUser(t, store, "employee", "Employee", "employee", "employee-password")
	router := NewRouter(Dependencies{DB: db, Config: config.Config{JWTSecret: "test-session-secret", DemoMode: true, DemoProtectedUsernames: "demo"}})

	managerCookie := loginCookie(t, router, "manager", "manager-password")
	deleteWithCookieExpectStatus(t, router, "/api/v1/users/"+admin.ID, managerCookie, http.StatusForbidden)
	deleteWithCookieExpectStatus(t, router, "/api/v1/users/"+demo.ID, managerCookie, http.StatusForbidden)
	deleteWithCookieExpectStatus(t, router, "/api/v1/users/"+manager.ID, managerCookie, http.StatusForbidden)

	adminKey, err := store.CreateAPIKey(context.Background(), domain.APIKey{Name: "admin-key", OwnerUserID: manager.ID, Scopes: `["admin:write"]`})
	if err != nil {
		t.Fatal(err)
	}
	deleteWithHeadersExpectStatus(t, router, "/api/v1/users/"+employee.ID, map[string]string{"Authorization": "Bearer " + adminKey.Plaintext}, http.StatusUnauthorized)

	deleteWithCookieExpectStatus(t, router, "/api/v1/users/"+employee.ID, managerCookie, http.StatusOK)
	loginExpectStatus(t, router, "employee", "employee-password", http.StatusUnauthorized)
}

func createHTTPTestUser(t *testing.T, store repository.Store, username, displayName, role, password string) domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUserWithPassword(context.Background(), domain.User{Username: username, DisplayName: displayName, Role: role, PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func loginCookie(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	req := jsonRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login %s got status %d body %s", username, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}

func assertNoStoreJSON(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected no-store cache header, got %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("expected no-cache pragma, got %q", rec.Header().Get("Pragma"))
	}
	if rec.Header().Get("Expires") != "0" {
		t.Fatalf("expected expires 0, got %q", rec.Header().Get("Expires"))
	}
	vary := rec.Header().Values("Vary")
	if !containsHeaderValue(vary, "Authorization") || !containsHeaderValue(vary, "Cookie") {
		t.Fatalf("expected Vary Authorization and Cookie, got %#v", vary)
	}
}

func containsHeaderValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func loginExpectStatus(t *testing.T, handler http.Handler, username, password string, status int) {
	t.Helper()
	req := jsonRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("login %s got status %d body %s", username, rec.Code, rec.Body.String())
	}
}

func patchWithCookieExpectStatus(t *testing.T, handler http.Handler, path string, body map[string]any, cookie *http.Cookie, status int) {
	t.Helper()
	req := jsonRequest(t, http.MethodPatch, path, body)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("PATCH %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func patchWithHeadersExpectStatus(t *testing.T, handler http.Handler, path string, body map[string]any, headers map[string]string, status int) {
	t.Helper()
	req := jsonRequest(t, http.MethodPatch, path, body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("PATCH %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func postWithCookieExpectStatus(t *testing.T, handler http.Handler, path string, body map[string]any, cookie *http.Cookie, status int) {
	t.Helper()
	req := jsonRequest(t, http.MethodPost, path, body)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("POST %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func postWithHeadersExpectStatus(t *testing.T, handler http.Handler, path string, body map[string]any, headers map[string]string, status int) {
	t.Helper()
	req := jsonRequest(t, http.MethodPost, path, body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("POST %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func deleteWithCookieExpectStatus(t *testing.T, handler http.Handler, path string, cookie *http.Cookie, status int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("DELETE %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func deleteWithHeadersExpectStatus(t *testing.T, handler http.Handler, path string, headers map[string]string, status int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("DELETE %s got status %d body %s", path, rec.Code, rec.Body.String())
	}
}

func jsonRequest(t *testing.T, method, path string, body map[string]any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}
