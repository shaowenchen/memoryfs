package cluster

import (
	"context"
	"fmt"

	"github.com/shaowenchen/memoryfs/pkg/raftnode"
)

// Membership manages ordered cluster join: bootstrap leader, follower register, node list sync.
//
//   - Pod-0 bootstrap becomes Raft leader and registers itself.
//   - Pod-1/2 join the leader HTTP endpoint; leader admits them to Raft + KV.
//   - After each admit, the full member list is replicated via Raft KV to all nodes.
type Membership struct {
	raft *raftnode.Node
	self string
}

// NewMembership creates a membership manager for a raft-backed node.
func NewMembership(rn *raftnode.Node, nodeID string) *Membership {
	return &Membership{raft: rn, self: nodeID}
}

// IsLeader reports whether this node is the current Raft leader.
func (m *Membership) IsLeader() bool { return m.raft.IsLeader() }

// RegisterSelf records the local node in the cluster registry (leader / bootstrap).
func (m *Membership) RegisterSelf(member Member) error {
	if member.ID == "" {
		member.ID = m.self
	}
	return batch(m.raft.KV(), registerOps(member))
}

// Admit adds a new member on the leader: Raft voter + registry + epoch bump, then returns synced list.
func (m *Membership) Admit(_ context.Context, member Member) ([]Member, error) {
	if !m.raft.IsLeader() {
		return nil, fmt.Errorf("not leader")
	}
	if member.ID == "" || member.Raft == "" || member.HTTP == "" {
		return nil, fmt.Errorf("member id, raft, and http are required")
	}
	has, err := m.raft.HasServer(member.ID)
	if err != nil {
		return nil, err
	}
	if !has {
		if err := m.raft.AddVoter(member.ID, member.Raft); err != nil {
			return nil, err
		}
	}
	if err := batch(m.raft.KV(), registerOps(member)); err != nil {
		return nil, err
	}
	if _, err := m.raft.KV().Incr(EpochKey); err != nil {
		return nil, err
	}
	return m.Sync(context.Background())
}

// Remove drops a member from Raft and the registry (leader only).
func (m *Membership) Remove(_ context.Context, id string) error {
	if !m.raft.IsLeader() {
		return fmt.Errorf("not leader")
	}
	if err := m.raft.RemoveVoter(id); err != nil {
		return err
	}
	if err := batch(m.raft.KV(), removeOps(id)); err != nil {
		return err
	}
	_, err := m.raft.KV().Incr(EpochKey)
	return err
}

// List returns all registered members from KV (replicated to every node).
func (m *Membership) List(_ context.Context) ([]Member, error) {
	return listMembers(m.raft.KV())
}

// ListHTTP returns registered node HTTP URLs.
func (m *Membership) ListHTTP(_ context.Context) ([]string, error) {
	return listHTTP(m.raft.KV())
}

// Sync returns the current full node list after membership changes.
func (m *Membership) Sync(_ context.Context) ([]Member, error) {
	return listMembers(m.raft.KV())
}
