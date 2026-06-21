package node

import (
	"net/http"
	"strings"
)

// NormalizeURIPrefix ensures a path-style URI prefix (e.g. "/memoryfs").
func NormalizeURIPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

// PrefixMiddleware strips uriPrefix from incoming paths. Health and metrics stay
// available without the prefix for in-cluster probes and Prometheus scraping.
func PrefixMiddleware(prefix string, next http.Handler) http.Handler {
	prefix = NormalizeURIPrefix(prefix)
	if prefix == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, prefix) {
			clone := r.Clone(r.Context())
			clone.URL.Path = strings.TrimPrefix(path, prefix)
			if clone.URL.Path == "" {
				clone.URL.Path = "/"
			}
			next.ServeHTTP(w, clone)
			return
		}
		if probePath(path) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func probePath(path string) bool {
	switch path {
	case "/health", "/metrics", "/readyz", "/livez", "/v1/stats":
		return true
	default:
		return false
	}
}
