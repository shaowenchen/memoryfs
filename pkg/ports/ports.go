package ports

// Default MemoryFS service ports (avoid common 8080/9090 conflicts).
const (
	HTTP = "19800"
	GRPC = "19801"
	Raft = "19802"
	RDMA = "19803"
)

// HTTPListen returns the default HTTP bind address.
func HTTPListen() string { return ":" + HTTP }

// GRPCListen returns the default gRPC bind address.
func GRPCListen() string { return ":" + GRPC }

// RaftListen returns the default Raft bind address.
func RaftListen() string { return ":" + Raft }

// RDMAListen returns the default RDMA bind address.
func RDMAListen() string { return ":" + RDMA }

// DefaultHTTPURL is the local dev seed node URL.
func DefaultHTTPURL() string { return "http://127.0.0.1:" + HTTP }
