package cluster

const (
	// EpochKey is incremented when membership changes.
	EpochKey = "memoryfs:cluster:epoch"
)

func nodeGRPCKey(id string) string { return "memoryfs:node:grpc:" + id }
func nodeRDMAKey(id string) string { return "memoryfs:node:rdma:" + id }
