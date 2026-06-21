package cluster

// Member is a cluster node's identity and reachability addresses.
type Member struct {
	ID   string
	HTTP string
	Raft string
	GRPC string
	RDMA string
}
