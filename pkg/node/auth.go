package node

import (
	"net/http"
	"strings"
)

// AuthMiddleware optionally enforces Bearer token on mutating API calls.
func AuthMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) || strings.TrimPrefix(auth, prefix) != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiresAuth(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		return false
	}
	path := r.URL.Path
	switch {
	case path == "/health", path == "/metrics", path == "/v1/stats",
		path == "/v1/cluster/overview", path == "/v1/repair",
		strings.HasPrefix(path, "/dashboard"), path == "/":
		return false
	case strings.HasPrefix(path, "/v1/fs/getattr"), strings.HasPrefix(path, "/v1/fs/lookup"),
		strings.HasPrefix(path, "/v1/fs/readdir"):
		return false
	case strings.HasPrefix(path, "/v1/cluster/leader"), strings.HasPrefix(path, "/v1/cluster/nodes"):
		return false
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/chunks/"):
		return false
	}
	return strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/chunks/")
}
