package node

import (
	"bytes"
	_ "embed"
	"net/http"
	"strings"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

const dashboardPrefixPlaceholder = "__MEMORYFS_URI_PREFIX__"

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path != "/" && path != "/dashboard" && path != "/dashboard/" {
		http.NotFound(w, r)
		return
	}
	body := bytes.ReplaceAll(dashboardHTML, []byte(dashboardPrefixPlaceholder), []byte(s.uriPrefix))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// DashboardURL returns the external dashboard path including uri prefix.
func DashboardURL(uriPrefix string) string {
	prefix := NormalizeURIPrefix(uriPrefix)
	if prefix == "" {
		return "/dashboard"
	}
	return strings.TrimSuffix(prefix, "/") + "/dashboard"
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ov := s.svc.ClusterOverview(r.Context())
	writeJSON(w, http.StatusOK, fsResponse{Overview: &ov})
}

func (s *Server) handleRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fixed, failed := s.svc.RunRepair(r.Context())
	writeJSON(w, http.StatusOK, fsResponse{
		RepairFixed: fixed, RepairFailed: failed,
		RepairPending: s.svc.RepairInfo(0).Pending,
	})
}

func (s *Server) handleRepairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info := s.svc.RepairInfo(50)
	writeJSON(w, http.StatusOK, fsResponse{Repair: &info})
}
