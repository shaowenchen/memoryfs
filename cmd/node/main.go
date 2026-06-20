package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "github.com/shaowenchen/memoryfs/api/memoryfs/v1"
	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/grpcserver"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/node"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
	"github.com/shaowenchen/memoryfs/pkg/service"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

func main() {
	id := flag.String("id", "n1", "node id")
	httpAddr := flag.String("http", ":8080", "HTTP listen address")
	advertiseHTTP := flag.String("advertise-http", "", "HTTP address advertised to peers (default: derived from -http)")
	grpcAddr := flag.String("grpc", ":9090", "gRPC listen address")
	rdmaAddr := flag.String("rdma", ":9092", "RDMA listen address")
	raftAddr := flag.String("raft", ":8081", "Raft listen address")
	advertiseRaft := flag.String("advertise-raft", "", "Raft address advertised to peers (default: same as -raft)")
	dataDir := flag.String("data", "./data", "data directory for raft/meta")
	chunkDir := flag.String("chunk-dir", "", "local disk directory for chunks (default: {data}/{id}/chunks)")
	chunkBackend := flag.String("chunk-backend", "disk", "chunk backend: disk, tiered, or memory")
	replicaFactor := flag.Int("replica-factor", 2, "chunk replication factor across nodes")
	memCacheMB := flag.Int64("mem-cache-mb", 0, "in-memory read cache size in MB (0=disabled, tiered backend enables 512MB default)")
	diskQuotaGB := flag.Int64("disk-quota-gb", 0, "local disk quota in GB (0=unlimited)")
	gcInterval := flag.Duration("gc-interval", 5*time.Minute, "orphan chunk GC interval (0=disabled)")
	flushInterval := flag.Duration("flush-interval", 30*time.Second, "local chunk flush/fsync interval (0=disabled except on shutdown)")
	maxFileAge := flag.Duration("max-file-age", 0, "expire files older than this duration (0=disabled)")
	defaultTTL := flag.Duration("default-ttl", 0, "TTL for newly created files (0=disabled)")
	bootstrap := flag.Bool("bootstrap", false, "bootstrap a new raft cluster")
	standalone := flag.Bool("standalone", false, "run without raft (single node)")
	join := flag.String("join", "", "join an existing cluster via leader HTTP URL")
	apiToken := flag.String("api-token", "", "optional bearer token for mutating API calls")
	uriPrefix := flag.String("uri-prefix", "", "HTTP URI prefix for dashboard and API (e.g. /memoryfs)")
	flag.Parse()

	httpURL := normalizeHTTP(*httpAddr)
	if *advertiseHTTP != "" {
		httpURL = normalizeHTTP(*advertiseHTTP)
	}
	nodeDataDir := filepath.Join(strings.TrimSuffix(*dataDir, "/"), *id)
	chunkPath := *chunkDir
	if chunkPath == "" {
		chunkPath = filepath.Join(nodeDataDir, "chunks")
	}

	raftAdvertise := *advertiseRaft
	if raftAdvertise == "" {
		raftAdvertise = *raftAddr
	}

	cfg := raftnode.Config{
		ID:            *id,
		RaftAddr:      *raftAddr,
		AdvertiseRaft: raftAdvertise,
		DataDir:       nodeDataDir,
		Bootstrap:     *bootstrap,
		Standalone:    *standalone,
		HTTPAddr:      httpURL,
	}

	rn, err := raftnode.Start(cfg)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	defer func() { _ = rn.Close() }()

	metaStore, err := meta.NewLocalStore(rn.KV())
	if err != nil {
		log.Fatalf("meta store: %v", err)
	}

	chunkStore, err := chunk.OpenStoreWithOptions(chunk.OpenStoreOptions{
		Backend:     *chunkBackend,
		Dir:         chunkPath,
		MemCacheMB:  *memCacheMB,
		DiskQuotaGB: *diskQuotaGB,
	})
	if err != nil {
		log.Fatalf("chunk store: %v", err)
	}
	log.Printf("chunk storage: backend=%s dir=%s local_chunks=%d", *chunkBackend, chunkPath, chunkStore.Count())

	httpTP := transport.NewHTTPTransport()
	grpcTP := transport.NewGRPCTransport()
	rdmaTP := transport.NewRDMATransport(grpcTP)
	multiTP := transport.NewMultiTransport(rdmaTP, grpcTP, httpTP)

	maintCtx, maintCancel := context.WithCancel(context.Background())
	defer maintCancel()

	svc := service.New(service.Config{
		NodeID:        *id,
		NodeHTTP:      httpURL,
		RaftNode:      rn,
		Meta:          metaStore,
		Chunks:        chunkStore,
		Registry:      chunk.NewRegistry(rn.KV()),
		Lifecycle:     lifecycle.NewManager(),
		Transport:     multiTP,
		ReplicaFactor: *replicaFactor,
		DefaultTTL:    *defaultTTL,
	})
	svc.LoadClusterConfig()

	if rn.IsLeader() || *standalone {
		if err := rn.RegisterSelf(); err != nil {
			log.Printf("warning: register self: %v", err)
		}
		if err := svc.PersistReplicaFactor(); err != nil {
			log.Printf("warning: persist replica factor: %v", err)
		}
	}

	svc.Ready(context.Background())

	svc.StartMaintenance(maintCtx, service.MaintenanceConfig{
		GCInterval:    *gcInterval,
		FlushInterval: *flushInterval,
		TTL:           *maxFileAge,
		DefaultTTL:    *defaultTTL,
	})

	httpSrv := node.NewServer(svc, *apiToken, *uriPrefix)
	grpcSrv := grpc.NewServer(grpc.MaxRecvMsgSize(16<<20), grpc.MaxSendMsgSize(16<<20))
	pb.RegisterMemoryFSServer(grpcSrv, grpcserver.New(svc))

	if *join != "" {
		go joinClusterWithRetry(*join, *id, raftAdvertise, httpURL, *grpcAddr, *rdmaAddr)
	}

	go serveHTTP(*httpAddr, httpSrv.Handler(), &httpServer)
	go serveGRPC(*grpcAddr, grpcSrv)

	log.Printf("memoryfs node %s: http=%s grpc=%s chunk_dir=%s rf=%d dashboard=%s",
		*id, *httpAddr, *grpcAddr, chunkPath, svc.ReplicaFactor(), dashboardAddr(*httpAddr, *uriPrefix))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("draining node %s before shutdown...", *id)
	remaining, drained, err := svc.Drain(context.Background(), false)
	if err != nil {
		log.Printf("drain incomplete: %v (remaining=%d)", err, remaining)
	} else if drained {
		log.Printf("node drained successfully")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(shutdownCtx)
	}
	grpcSrv.GracefulStop()
}

var httpServer *http.Server

func serveHTTP(addr string, handler http.Handler, ref **http.Server) {
	server := &http.Server{Addr: addr, Handler: handler}
	*ref = server
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http listen: %v", err)
	}
}

func serveGRPC(addr string, srv *grpc.Server) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("grpc serve: %v", err)
	}
}

func normalizeHTTP(addr string) string {
	if strings.HasPrefix(addr, "http") {
		return addr
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + addr
}

func joinClusterWithRetry(leaderURL, id, raftAddr, httpAddr, grpcAddr, rdmaAddr string) {
	const maxAttempts = 60
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		time.Sleep(time.Duration(min(attempt, 5)) * time.Second)
		if err := joinCluster(leaderURL, id, raftAddr, httpAddr, grpcAddr, rdmaAddr); err != nil {
			log.Printf("join cluster attempt %d/%d: %v", attempt, maxAttempts, err)
			continue
		}
		log.Printf("joined cluster via %s as %s", leaderURL, id)
		return
	}
	log.Printf("join cluster failed after %d attempts", maxAttempts)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func joinCluster(leaderURL, id, raftAddr, httpAddr, grpcAddr, rdmaAddr string) error {
	body, _ := json.Marshal(map[string]string{
		"id": id, "raft_addr": raftAddr, "http_addr": httpAddr,
		"grpc_addr": grpcAddr, "rdma_addr": rdmaAddr,
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(leaderURL, "/")+"/v1/cluster/join", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("join status %d", resp.StatusCode)
	}
	return nil
}

func dashboardAddr(httpListen, uriPrefix string) string {
	path := node.DashboardURL(uriPrefix)
	if strings.HasPrefix(httpListen, ":") {
		return "http://127.0.0.1" + httpListen + path
	}
	if strings.HasPrefix(httpListen, "http") {
		return strings.TrimRight(httpListen, "/") + path
	}
	return "http://" + httpListen + path
}
