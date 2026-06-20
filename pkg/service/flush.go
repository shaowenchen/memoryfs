package service

import (
	"log"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

// FlushChunks persists local chunk data to durable storage.
func (s *Service) FlushChunks() (int, error) {
	return chunk.FlushStore(s.cfg.Chunks)
}

// FlushChunksLogged flushes chunks and writes a log line when data was synced.
func (s *Service) FlushChunksLogged() (int, error) {
	n, err := s.FlushChunks()
	if err != nil {
		return n, err
	}
	if n > 0 {
		log.Printf("flush: synced %d chunks to disk", n)
	}
	return n, nil
}
