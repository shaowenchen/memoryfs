package service

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricChunks = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "chunks_total",
		Help: "Local chunk count on this node",
	})
	metricDiskBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "disk_bytes",
		Help: "Local disk bytes used for chunks",
	})
	metricMemBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "mem_bytes",
		Help: "In-memory chunk payload bytes (primary storage in memoryfs)",
	})
	metricRepairPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "repair_queue_pending",
		Help: "Pending replica repair jobs",
	})
	metricClusterEpoch = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "cluster_epoch",
		Help: "Cluster membership epoch",
	})
	metricIsLeader = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "is_leader",
		Help: "1 if this node is raft leader",
	})
	metricNodeState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "memoryfs", Name: "node_state",
		Help: "Node lifecycle state (1=active,2=draining,3=drained)",
	}, []string{"state"})
)

func init() {
	prometheus.MustRegister(metricChunks, metricDiskBytes, metricMemBytes,
		metricRepairPending, metricClusterEpoch, metricIsLeader, metricNodeState)
}

// UpdateMetrics refreshes Prometheus gauges from current node state.
func (s *Service) UpdateMetrics() {
	st := s.Stats()
	metricChunks.Set(float64(st.ChunkCount))
	metricDiskBytes.Set(float64(st.DiskBytes))
	metricMemBytes.Set(float64(st.MemBytes))
	metricRepairPending.Set(float64(s.RepairInfo(0).Pending))
	metricClusterEpoch.Set(float64(st.ClusterEpoch))

	if s.IsLeader() {
		metricIsLeader.Set(1)
	} else {
		metricIsLeader.Set(0)
	}
	for _, name := range []string{"active", "draining", "drained"} {
		metricNodeState.WithLabelValues(name).Set(0)
	}
	stateVal := map[string]float64{"active": 1, "draining": 2, "drained": 3}
	if v, ok := stateVal[st.NodeState]; ok {
		metricNodeState.WithLabelValues(st.NodeState).Set(v)
	}
}
