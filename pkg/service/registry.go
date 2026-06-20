package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type registrySetRequest struct {
	ChunkID   string   `json:"chunk_id"`
	Replicas  []string `json:"replicas"`
	Epoch     uint64   `json:"epoch"`
}

type registryDeleteRequest struct {
	ChunkID string `json:"chunk_id"`
}

var registryHTTPClient = &http.Client{Timeout: 15 * time.Second}

// RecordChunkRegistry stores chunk replica locations in the Raft-backed registry.
func (s *Service) RecordChunkRegistry(ctx context.Context, chunkID string, replicas []string) error {
	epoch := s.syncClusterEpoch()
	if s.IsLeader() {
		return s.cfg.Registry.Set(chunkID, replicas, epoch)
	}
	leader, err := s.LeaderHTTP()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(registrySetRequest{ChunkID: chunkID, Replicas: replicas, Epoch: epoch})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(leader, "/")+"/v1/chunks/registry/set", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("registry set status %d", resp.StatusCode)
	}
	return nil
}

// DeleteChunkRegistry removes chunk location metadata from the registry.
func (s *Service) DeleteChunkRegistry(ctx context.Context, chunkID string) error {
	if s.IsLeader() {
		return s.cfg.Registry.Delete(chunkID)
	}
	leader, err := s.LeaderHTTP()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(registryDeleteRequest{ChunkID: chunkID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(leader, "/")+"/v1/chunks/registry/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("registry delete status %d", resp.StatusCode)
	}
	return nil
}

// ApplyRegistrySet applies a registry update on the leader.
func (s *Service) ApplyRegistrySet(_ context.Context, chunkID string, replicas []string, epoch uint64) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}
	return s.cfg.Registry.Set(chunkID, replicas, epoch)
}

// ApplyRegistryDelete removes registry metadata on the leader.
func (s *Service) ApplyRegistryDelete(_ context.Context, chunkID string) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}
	return s.cfg.Registry.Delete(chunkID)
}
