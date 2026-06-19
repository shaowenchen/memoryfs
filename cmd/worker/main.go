package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	redisAddr := flag.String("redis", "127.0.0.1:6379", "redis address for worker registration")
	flag.Parse()

	store := newChunkStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/chunks/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/chunks/")
		if id == "" {
			http.Error(w, "missing chunk id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			data, ok := store.get(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			store.put(id, data)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			store.delete(id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	server := &http.Server{Addr: *addr, Handler: mux}
	url := workerURL(*addr)

	if *redisAddr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Printf("warning: redis unavailable: %v", err)
		} else if err := rdb.SAdd(ctx, "memoryfs:workers", url).Err(); err != nil {
			log.Printf("warning: register worker: %v", err)
		} else {
			log.Printf("registered worker at %s", url)
		}
		cancel()
	}

	go func() {
		log.Printf("memoryfs worker listening on %s", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func workerURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "http") {
		return addr
	}
	return "http://" + addr
}

type chunkStore struct {
	mu     sync.RWMutex
	chunks map[string][]byte
}

func newChunkStore() *chunkStore {
	return &chunkStore{chunks: make(map[string][]byte)}
}

func (s *chunkStore) get(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.chunks[id]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, true
}

func (s *chunkStore) put(id string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.chunks[id] = cp
}

func (s *chunkStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chunks, id)
}
