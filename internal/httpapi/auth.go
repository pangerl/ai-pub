package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

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
