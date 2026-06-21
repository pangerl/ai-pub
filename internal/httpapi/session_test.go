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

	keyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(`{"name":"employee-ci","owner_type":"user","owner_id":"forged-owner","scopes":"[\"release:create\"]"}`))
	keyRequest.Header.Set("Content-Type", "application/json")
	keyRequest.AddCookie(cookie)
	keyRecorder := httptest.NewRecorder()
	router.ServeHTTP(keyRecorder, keyRequest)
	if keyRecorder.Code != http.StatusCreated {
		t.Fatalf("employee key create got %d: %s", keyRecorder.Code, keyRecorder.Body.String())
	}
	var keyResponse struct {
		Data struct {
			Key domain.APIKey `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(keyRecorder.Body.Bytes(), &keyResponse); err != nil {
		t.Fatal(err)
	}
	if keyResponse.Data.Key.OwnerID != employee.ID || keyResponse.Data.Key.OwnerType != "user" {
		t.Fatalf("employee must not choose API key owner: %#v", keyResponse.Data.Key)
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
	if len(listKeysResponse.Data) != 1 || listKeysResponse.Data[0].ID != keyResponse.Data.Key.ID {
		t.Fatalf("employee should only see own key, got %#v", listKeysResponse.Data)
	}

	otherKey, err := store.CreateAPIKey(context.Background(), domain.APIKey{Name: "other-ci", OwnerType: "user", OwnerID: "other-user", Scopes: `["release:create"]`})
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
