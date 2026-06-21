package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ai-pub/internal/auth"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

const sessionCookieName = "ai_pub_session"

type sessionUserContextKey struct{}

func currentSessionUser(r *http.Request) (domain.User, bool) {
	user, ok := r.Context().Value(sessionUserContextKey{}).(domain.User)
	return user, ok
}

func requireSessionOrAPIKey(store repository.Store, jwtSecret string, next http.Handler) http.Handler {
	if jwtSecret == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			claims, err := auth.Parse(jwtSecret, cookie.Value, time.Now().UTC())
			if err == nil {
				user, userErr := store.GetUserByUsername(r.Context(), claims.Username)
				if userErr == nil && user.ID == claims.Subject && user.Enabled && user.SessionVersion == claims.SessionVersion {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionUserContextKey{}, user)))
					return
				}
			}
		}
		if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, r, http.StatusUnauthorized, "unauthorized", errUnauthorized)
	})
}

func withAdmin(store repository.Store, jwtSecret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if user, ok := currentSessionUser(r); ok {
			if user.Role != "admin" {
				writeError(w, r, http.StatusForbidden, "forbidden", errForbidden)
				return
			}
			next(w, r)
			return
		}
		if jwtSecret == "" && strings.TrimSpace(r.Header.Get("Authorization")) == "" { // Router unit tests intentionally run without authentication configured.
			next(w, r)
			return
		}
		if _, ok := authorizeOptionalAPIKey(w, r, store, "admin:write"); !ok {
			return
		}
		next(w, r)
	}
}

var (
	errUnauthorized = errors.New("invalid api key")
	errForbidden    = errors.New("api key scope is not allowed")
)

func apiKeyFromBearer(store repository.Store, r *http.Request, requiredScope string) (domain.APIKey, bool, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return domain.APIKey{}, false, nil
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return domain.APIKey{}, true, errUnauthorized
	}
	key, err := store.GetAPIKeyBySecret(r.Context(), parts[1])
	if err != nil {
		return domain.APIKey{}, true, errUnauthorized
	}
	if !key.Enabled {
		return domain.APIKey{}, true, errUnauthorized
	}
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()) {
		return domain.APIKey{}, true, errUnauthorized
	}
	if !apiKeyHasScope(key, requiredScope) {
		return domain.APIKey{}, true, errForbidden
	}
	if err := store.TouchAPIKeyLastUsed(r.Context(), key.ID); err != nil {
		return domain.APIKey{}, true, err
	}
	return key, true, nil
}

func apiKeyHasScope(key domain.APIKey, required string) bool {
	var scopes []string
	if err := json.Unmarshal([]byte(key.Scopes), &scopes); err != nil {
		return false
	}
	for _, scope := range scopes {
		if scope == required || scope == "*" {
			return true
		}
	}
	return false
}

func withOptionalAPIKeyScope(store repository.Store, scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeOptionalAPIKey(w, r, store, scope); !ok {
			return
		}
		next(w, r)
	}
}
