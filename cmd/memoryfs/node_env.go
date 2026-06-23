package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// runNodeEnv builds the equivalent of `memoryfs node ...` flags from
// MEMORYFS_* environment variables and delegates to runNode. It replaces
// the legacy deploy/scripts/node-start.sh wrapper used as the Kubernetes
// entry point.
//
// Recognised env vars (all optional unless noted):
//
//	POD_NAME                       node id (default: $MEMORYFS_ID, then "n1")
//	POD_NAMESPACE                  used to derive headless-service URLs
//	MEMORYFS_ID                    explicit node id
//	MEMORYFS_HTTP_LISTEN           ":19800"
//	MEMORYFS_GRPC_LISTEN           ":19801"
//	MEMORYFS_RAFT_LISTEN           ":19802"
//	MEMORYFS_RDMA_LISTEN           ":19803"
//	MEMORYFS_HTTP_URL              override -advertise-http
//	MEMORYFS_RAFT_URL              override -advertise-raft
//	MEMORYFS_HOST_IP               (k8s status.hostIP) advertise on host network
//	MEMORYFS_HEADLESS_SERVICE      headless svc name (used when POD_NAME is set)
//	MEMORYFS_BOOTSTRAP=true        bootstrap a new raft cluster
//	MEMORYFS_STANDALONE=true       single node (no raft)
//	MEMORYFS_JOIN=<leader-http>    join existing cluster
//	MEMORYFS_INSTANCE_ID           instance subdir under MEMORYFS_STORAGE_ROOT
//	MEMORYFS_STORAGE_ROOT          storage root (paired with INSTANCE_ID)
//	MEMORYFS_DATA                  raft/meta data root (default /data)
//	MEMORYFS_CHUNK_DIR             chunk dir override
//	MEMORYFS_CHUNK_BACKEND         chunk backend (default tiered)
//	MEMORYFS_REPLICA_FACTOR        default 2
//	MEMORYFS_MEM_CACHE_MB          default 512
//	MEMORYFS_DISK_QUOTA_GB         default 0
//	MEMORYFS_GC_INTERVAL           default 5m
//	MEMORYFS_FLUSH_INTERVAL        default 30s
//	MEMORYFS_DEFAULT_TTL           default 0
//	MEMORYFS_MAX_FILE_AGE          default 0
//	MEMORYFS_API_TOKEN             bearer token
//	MEMORYFS_URI_PREFIX            HTTP URI prefix
//	MEMORYFS_RAFT_RESET=true       wipe raft.db / snapshots before start
//
// Any extra positional args are appended after the env-derived flags so an
// operator can override any single flag from the command line.
func runNodeEnv(extra []string) {
	httpListen := envOr("MEMORYFS_HTTP_LISTEN", ":19800")
	grpcListen := envOr("MEMORYFS_GRPC_LISTEN", ":19801")
	raftListen := envOr("MEMORYFS_RAFT_LISTEN", ":19802")
	rdmaListen := envOr("MEMORYFS_RDMA_LISTEN", ":19803")

	id := os.Getenv("POD_NAME")
	if id == "" {
		id = envOr("MEMORYFS_ID", "n1")
	}

	bootstrap := envBool("MEMORYFS_BOOTSTRAP")
	standalone := envBool("MEMORYFS_STANDALONE")
	joinURL := os.Getenv("MEMORYFS_JOIN")
	headless := os.Getenv("MEMORYFS_HEADLESS_SERVICE")
	podName := os.Getenv("POD_NAME")
	namespace := envOr("POD_NAMESPACE", "default")
	httpPort := portFromListen(httpListen, "19800")
	raftPort := portFromListen(raftListen, "19802")
	grpcPort := portFromListen(grpcListen, "19801")
	rdmaPort := portFromListen(rdmaListen, "19803")

	if podName != "" && !bootstrap && !standalone && joinURL == "" {
		if ord := lastDashOrdinal(podName); ord == "0" {
			bootstrap = true
		} else if headless != "" {
			base := stripLastDashSegment(podName)
			joinURL = fmt.Sprintf("http://%s-0.%s.%s.svc:%s", base, headless, namespace, httpPort)
		}
	}

	var dataRoot, nodeData string
	instanceID := os.Getenv("MEMORYFS_INSTANCE_ID")
	storageRoot := os.Getenv("MEMORYFS_STORAGE_ROOT")
	if instanceID != "" && storageRoot != "" {
		dataRoot = filepath.Join(storageRoot, instanceID)
	} else {
		dataRoot = envOr("MEMORYFS_DATA", "/data")
	}
	nodeData = filepath.Join(dataRoot, id)
	chunkDir := envOr("MEMORYFS_CHUNK_DIR", filepath.Join(nodeData, "chunks"))

	if err := os.MkdirAll(nodeData, 0o755); err != nil {
		log.Fatalf("node-env: mkdir %s: %v", nodeData, err)
	}
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		log.Fatalf("node-env: mkdir %s: %v", chunkDir, err)
	}

	if envBool("MEMORYFS_RAFT_RESET") {
		log.Printf("memoryfs: MEMORYFS_RAFT_RESET=true, clearing raft state in %s", nodeData)
		_ = os.RemoveAll(filepath.Join(nodeData, "raft.db"))
		_ = os.RemoveAll(filepath.Join(nodeData, "snapshots"))
	}

	chunkBackend := envOr("MEMORYFS_CHUNK_BACKEND", "tiered")
	replicaFactor := normalizeUint(envOr("MEMORYFS_REPLICA_FACTOR", "2"))
	memCacheMB := normalizeUint(envOr("MEMORYFS_MEM_CACHE_MB", "512"))
	diskQuotaGB := normalizeUint(envOr("MEMORYFS_DISK_QUOTA_GB", "0"))
	gcInterval := envOr("MEMORYFS_GC_INTERVAL", "5m")
	flushInterval := envOr("MEMORYFS_FLUSH_INTERVAL", "30s")
	defaultTTL := envOr("MEMORYFS_DEFAULT_TTL", "0")
	maxFileAge := envOr("MEMORYFS_MAX_FILE_AGE", "0")
	apiToken := os.Getenv("MEMORYFS_API_TOKEN")
	uriPrefix := os.Getenv("MEMORYFS_URI_PREFIX")

	advertiseHTTP, advertiseRaft := deriveAdvertise(podName, headless, namespace,
		httpListen, raftListen, httpPort, raftPort)
	if v := os.Getenv("MEMORYFS_HOST_IP"); v != "" {
		raftListen = v + ":" + raftPort
		grpcListen = v + ":" + grpcPort
		rdmaListen = v + ":" + rdmaPort
	}
	if v := os.Getenv("MEMORYFS_RAFT_URL"); v != "" {
		advertiseRaft = v
	}

	log.Printf("memoryfs node-env: id=%s bootstrap=%v join=%s data=%s advertise_http=%s advertise_raft=%s raft_listen=%s",
		id, bootstrap, joinOrNone(joinURL), nodeData, advertiseHTTP, advertiseRaft, raftListen)

	args := []string{
		"-id", id,
		"-http", httpListen,
		"-advertise-http", advertiseHTTP,
		"-grpc", grpcListen,
		"-rdma", rdmaListen,
		"-raft", raftListen,
		"-advertise-raft", advertiseRaft,
		"-data", dataRoot,
		"-chunk-dir", chunkDir,
		"-chunk-backend", chunkBackend,
		"-replica-factor", replicaFactor,
		"-mem-cache-mb", memCacheMB,
		"-disk-quota-gb", diskQuotaGB,
		"-gc-interval", gcInterval,
		"-flush-interval", flushInterval,
		"-default-ttl", defaultTTL,
		"-max-file-age", maxFileAge,
	}
	if bootstrap {
		args = append(args, "-bootstrap")
	}
	if standalone {
		args = append(args, "-standalone")
	}
	if joinURL != "" {
		args = append(args, "-join", joinURL)
	}
	if apiToken != "" {
		args = append(args, "-api-token", apiToken)
	}
	if uriPrefix != "" {
		args = append(args, "-uri-prefix", uriPrefix)
	}
	args = append(args, extra...)

	runNode(args)
}

func deriveAdvertise(podName, headless, namespace,
	httpListen, raftListen, httpPort, raftPort string) (string, string) {
	if v := os.Getenv("MEMORYFS_HTTP_URL"); v != "" {
		raftAdv := raftListen
		if hostIP := os.Getenv("MEMORYFS_HOST_IP"); hostIP != "" {
			raftAdv = hostIP + ":" + raftPort
		}
		return v, raftAdv
	}
	if hostIP := os.Getenv("MEMORYFS_HOST_IP"); hostIP != "" {
		return fmt.Sprintf("http://%s:%s", hostIP, httpPort), hostIP + ":" + raftPort
	}
	if podName != "" && headless != "" {
		fqdn := fmt.Sprintf("%s.%s.%s.svc.cluster.local", podName, headless, namespace)
		return fmt.Sprintf("http://%s:%s", fqdn, httpPort), fqdn + ":" + raftPort
	}
	if strings.HasPrefix(httpListen, ":") {
		return "http://127.0.0.1" + httpListen, raftListen
	}
	return "http://" + httpListen, raftListen
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "true" || v == "1" || v == "yes"
}

func normalizeUint(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	if len(out) == 0 {
		return "0"
	}
	return string(out)
}

func portFromListen(addr, fallback string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
		return addr[i+1:]
	}
	return fallback
}

func lastDashOrdinal(name string) string {
	if i := strings.LastIndex(name, "-"); i >= 0 && i < len(name)-1 {
		return name[i+1:]
	}
	return ""
}

func stripLastDashSegment(name string) string {
	if i := strings.LastIndex(name, "-"); i >= 0 {
		return name[:i]
	}
	return name
}

func joinOrNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}
