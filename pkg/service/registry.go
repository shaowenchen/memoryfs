package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

type registrySetRequest struct {
	ChunkID   string   `json:"chunk_id"`
	Replicas  []string `json:"replicas"`
	Epoch     uint64   `json:"epoch"`
	ChainID   uint32   `json:"chain_id,omitempty"`
	ChainVer  uint64   `json:"chain_ver,omitempty"`
	UpdateVer uint64   `json:"update_ver,omitempty"`
	CommitVer uint64   `json:"commit_ver,omitempty"`
	State     string   `json:"state,omitempty"`
}

type registryDeleteRequest struct {
	ChunkID string `json:"chunk_id"`
}

var registryHTTPClient = &http.Client{Timeout: 15 * time.Second}

func normalizeURIPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

func leaderAPIURL(leader, uriPrefix, path string) string {
	return strings.TrimRight(leader, "/") + normalizeURIPrefix(uriPrefix) + path
}

// RecordChunkRegistry stores chunk replica locations in the Raft-backed registry.
func (s *Service) RecordChunkRegistry(ctx context.Context, chunkID string, replicas []string) error {
	meta := s.getMeta(chunkID)
	epoch := s.syncClusterEpoch()
	loc := chunk.Location{
		ChunkID:   chunkID,
		Replicas:  replicas,
		Epoch:     epoch,
		ChainID:   meta.ChainID,
		ChainVer:  meta.ChainVer,
		UpdateVer: meta.UpdateVer,
		CommitVer: meta.CommitVer,
		State:     string(meta.State),
	}
	if s.IsLeader() {
		return s.cfg.Registry.SetLocation(loc)
	}
	leader, err := s.LeaderHTTP()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(registrySetRequest{
		ChunkID: chunkID, Replicas: replicas, Epoch: epoch,
		ChainID: loc.ChainID, ChainVer: loc.ChainVer, UpdateVer: loc.UpdateVer, CommitVer: loc.CommitVer, State: loc.State,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, leaderAPIURL(leader, s.cfg.URIPrefix, "/v1/chunks/registry/set"), bytes.NewReader(body))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, leaderAPIURL(leader, s.cfg.URIPrefix, "/v1/chunks/registry/delete"), bytes.NewReader(body))
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
	meta := s.getMeta(chunkID)
	return s.cfg.Registry.SetLocation(chunk.Location{
		ChunkID:   chunkID,
		Replicas:  replicas,
		Epoch:     epoch,
		ChainID:   meta.ChainID,
		ChainVer:  meta.ChainVer,
		UpdateVer: meta.UpdateVer,
		CommitVer: meta.CommitVer,
		State:     string(meta.State),
	})
}

// ApplyRegistryDelete removes registry metadata on the leader.
func (s *Service) ApplyRegistryDelete(_ context.Context, chunkID string) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}
	return s.cfg.Registry.Delete(chunkID)
}
