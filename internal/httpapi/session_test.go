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
