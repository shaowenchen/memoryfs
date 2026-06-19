package lifecycle

import (
	"sync"
	"sync/atomic"
)

// State represents node lifecycle state for rolling updates.
type State string

const (
	StateActive   State = "active"
	StateDraining State = "draining"
	StateDrained  State = "drained"
)

// Manager tracks node lifecycle during rolling updates.
type Manager struct {
	mu          sync.RWMutex
	state       State
	epoch       atomic.Uint64
	pendingDrain int32
}

// NewManager creates a lifecycle manager starting in active state.
func NewManager() *Manager {
	m := &Manager{state: StateActive}
	m.epoch.Store(1)
	return m
}

// State returns current lifecycle state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Epoch returns cluster epoch bumped on membership changes.
func (m *Manager) Epoch() uint64 { return m.epoch.Load() }

// BumpEpoch increments cluster epoch after topology change.
func (m *Manager) BumpEpoch() uint64 {
	return m.epoch.Add(1)
}

// StartDrain marks node as draining; rejects new chunk assignments.
func (m *Manager) StartDrain() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateActive {
		m.state = StateDraining
	}
}

// SetPendingDrain updates remaining chunks to migrate during drain.
func (m *Manager) SetPendingDrain(n int) {
	atomic.StoreInt32(&m.pendingDrain, int32(n))
}

// PendingDrain returns remaining chunks during drain.
func (m *Manager) PendingDrain() int {
	return int(atomic.LoadInt32(&m.pendingDrain))
}

// MarkDrained marks node fully drained and safe to stop.
func (m *Manager) MarkDrained() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateDrained
	atomic.StoreInt32(&m.pendingDrain, 0)
}

// Ready reactivates node after startup.
func (m *Manager) Ready() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateActive
	atomic.StoreInt32(&m.pendingDrain, 0)
}

// AcceptsChunks reports whether node accepts new chunk writes.
func (m *Manager) AcceptsChunks() bool {
	return m.State() == StateActive
}
