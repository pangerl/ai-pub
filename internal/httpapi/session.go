package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ai-pub/internal/auth"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

func login(store repository.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		user, err := store.GetUserByUsername(r.Context(), strings.TrimSpace(input.Username))
		if err != nil || !user.Enabled || !auth.VerifyPassword(user.PasswordHash, input.Password) {
			writeError(w, r, http.StatusUnauthorized, "invalid_credentials", errors.New("用户名或密码错误"))
			return
		}
		expiresAt := time.Now().UTC().Add(12 * time.Hour)
		token, err := auth.Sign(jwtSecret, auth.Claims{Subject: user.ID, Username: user.Username, Role: user.Role, SessionVersion: user.SessionVersion, ExpiresAt: expiresAt.Unix()})
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiresAt, Secure: r.TLS != nil})
		writeData(w, r, http.StatusOK, user)
	}
}

func currentUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentSessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", errUnauthorized)
			return
		}
		writeData(w, r, http.StatusOK, user)
	}
}

func logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		writeData(w, r, http.StatusOK, map[string]bool{"logged_out": true})
	}
}

func createSessionUser(store repository.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
			Password    string `json:"password"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		item, err := store.CreateUserWithPassword(r.Context(), domain.User{Username: strings.TrimSpace(input.Username), DisplayName: strings.TrimSpace(input.DisplayName), Role: input.Role, PasswordHash: hash})
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}
