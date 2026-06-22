package main

import (
	"context"
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
	"github.com/shaowenchen/memoryfs/pkg/cluster"
	"github.com/shaowenchen/memoryfs/pkg/grpcserver"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/node"
	"github.com/shaowenchen/memoryfs/pkg/ports"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
	"github.com/shaowenchen/memoryfs/pkg/service"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

func main() {
	id := flag.String("id", "n1", "node id")
	httpAddr := flag.String("http", ports.HTTPListen(), "HTTP listen address")
	advertiseHTTP := flag.String("advertise-http", "", "HTTP address advertised to peers (default: derived from -http)")
	grpcAddr := flag.String("grpc", ports.GRPCListen(), "gRPC listen address")
	rdmaAddr := flag.String("rdma", ports.RDMAListen(), "RDMA listen address")
	raftAddr := flag.String("raft", ports.RaftListen(), "Raft listen address")
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
	if *standalone {
		if err := ensureRootAsLeader(context.Background(), rn, metaStore, 30*time.Second); err != nil {
			log.Fatalf("meta store: %v", err)
		}
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
	if pm, ok := chunkStore.(*chunk.PreallocMemory); ok {
		log.Printf("chunk storage: memory quota %d bytes (exact-size allocation, RSS scales with data)", pm.ReservedBytes())
	}

	httpTP := transport.NewPrefixedHTTPTransport(*uriPrefix)
	grpcTP := transport.NewGRPCTransportWithHTTP(httpTP)
	rdmaTP := transport.NewRDMATransport(grpcTP)
	multiTP := transport.NewMultiTransport(rdmaTP, grpcTP, httpTP)

	maintCtx, maintCancel := context.WithCancel(context.Background())
	defer maintCancel()

	svc := service.New(service.Config{
		NodeID:        *id,
		NodeHTTP:      httpURL,
		URIPrefix:     *uriPrefix,
		RaftNode:      rn,
		Meta:          metaStore,
		Chunks:        chunkStore,
		Registry:      chunk.NewRegistry(rn.KV()),
		Lifecycle:     lifecycle.NewManager(),
		Transport:     multiTP,
		ReplicaFactor: *replicaFactor,
		DefaultTTL:    *defaultTTL,
		DiskQuotaGB:   *diskQuotaGB,
	})
	svc.LoadClusterConfig()

	if !*standalone {
		self := cluster.Member{
			ID: *id, HTTP: httpURL, Raft: raftAdvertise, GRPC: *grpcAddr, RDMA: *rdmaAddr,
		}
		go cluster.RunLeaderLoop(context.Background(), svc.Membership(), self, func(ctx context.Context) error {
			if err := svc.PersistReplicaFactor(); err != nil {
				log.Printf("warning: persist replica factor: %v", err)
			}
			if err := ensureRootAsLeader(ctx, rn, metaStore, 2*time.Minute); err != nil {
				return err
			}
			log.Printf("leader ready: root inode initialized")
			return nil
		})
	} else if err := cluster.NewMembership(rn, *id).RegisterSelf(cluster.Member{
		ID: *id, HTTP: httpURL, Raft: raftAdvertise, GRPC: *grpcAddr, RDMA: *rdmaAddr,
	}); err != nil {
		log.Printf("warning: register self: %v", err)
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

	go serveHTTP(*httpAddr, httpSrv.Handler(), &httpServer)
	go serveGRPC(*grpcAddr, grpcSrv)

	if *join != "" && !*standalone {
		member := cluster.Member{
			ID: *id, Raft: raftAdvertise, HTTP: httpURL,
			GRPC: *grpcAddr, RDMA: *rdmaAddr,
		}
		startClusterJoin(*join, *uriPrefix, rn, member)
	}

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

func ensureRootAsLeader(ctx context.Context, rn *raftnode.Node, store *meta.LocalStore, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if rn.IsLeader() {
			if err := store.EnsureRoot(ctx); err != nil && !meta.IsNotLeaderErr(err) {
				return err
			}
			if _, err := store.GetAttr(ctx, meta.RootIno()); err == nil {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting to initialize root inode as leader")
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

func startClusterJoin(joinURL, uriPrefix string, rn *raftnode.Node, member cluster.Member) {
	opt := cluster.JoinOptions{
		LeaderURL: joinURL,
		URIPrefix: uriPrefix,
		Member:    member,
	}
	already, err := rn.HasServer(member.ID)
	if err != nil {
		log.Printf("cluster join: membership check: %v", err)
	}
	run := func() {
		if err := cluster.Join(context.Background(), opt); err != nil {
			if already {
				log.Printf("warning: cluster re-join: %v", err)
				return
			}
			log.Fatalf("join cluster: %v", err)
		}
		if already {
			log.Printf("cluster re-joined via %s as %s", joinURL, member.ID)
		} else {
			log.Printf("joined cluster via %s as %s", joinURL, member.ID)
		}
	}
	if already {
		log.Printf("cluster join: %s already in raft config; re-registering in background", member.ID)
		go run()
		return
	}
	run()
}
