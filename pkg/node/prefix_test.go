package node

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeURIPrefix(t *testing.T) {
	tests := map[string]string{
		"":           "",
		"/":          "",
		"memoryfs":   "/memoryfs",
		"/memoryfs/": "/memoryfs",
		" /memoryfs ": "/memoryfs",
	}
	for in, want := range tests {
		if got := NormalizeURIPrefix(in); got != want {
			t.Fatalf("NormalizeURIPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrefixMiddleware(t *testing.T) {
	var seen string
	h := PrefixMiddleware("/memoryfs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/memoryfs/v1/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen != "/v1/stats" {
		t.Fatalf("prefixed route: code=%d path=%q", rec.Code, seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen != "/health" {
		t.Fatalf("probe route: code=%d path=%q", rec.Code, seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unprefixed api without prefix: code=%d", rec.Code)
	}
}
