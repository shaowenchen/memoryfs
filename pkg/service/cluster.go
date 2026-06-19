package service

import (
	"fmt"
	"strconv"
)

const clusterEpochKey = "memoryfs:cluster:epoch"

// LoadClusterConfig reads persisted cluster settings from KV.
func (s *Service) LoadClusterConfig() {
	if rf, err := s.loadReplicaFactor(); err == nil && rf > 0 {
		s.cfg.ReplicaFactor = rf
	}
	if epoch, err := s.clusterEpoch(); err == nil && epoch > 0 {
		s.cfg.Lifecycle.SetEpoch(epoch)
	}
}

func (s *Service) loadReplicaFactor() (int, error) {
	data, err := s.cfg.RaftNode.KV().Get(configRFKey)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(string(data))
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid replica factor")
	}
	return v, nil
}

func (s *Service) clusterEpoch() (uint64, error) {
	data, err := s.cfg.RaftNode.KV().Get(clusterEpochKey)
	if err != nil {
		return s.cfg.Lifecycle.Epoch(), err
	}
	v, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return s.cfg.Lifecycle.Epoch(), err
	}
	return v, nil
}

func (s *Service) syncClusterEpoch() uint64 {
	epoch, err := s.clusterEpoch()
	if err != nil {
		return s.cfg.Lifecycle.Epoch()
	}
	s.cfg.Lifecycle.SetEpoch(epoch)
	return epoch
}
