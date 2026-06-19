package node

import (
	_ "embed"
	"net/http"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" && r.URL.Path != "/dashboard/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dashboardHTML)
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
