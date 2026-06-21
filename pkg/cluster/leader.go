package cluster

import (
	"context"
	"log"
	"time"
)

// RunLeaderLoop registers the bootstrap leader and runs fn when leadership is acquired.
func RunLeaderLoop(ctx context.Context, m *Membership, self Member, fn func(context.Context) error) {
	var wasLeader bool
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		leader := m.IsLeader()
		if leader && !wasLeader {
			if err := m.RegisterSelf(self); err != nil {
				log.Printf("cluster: register self: %v", err)
			} else {
				log.Printf("cluster: leader registered %s", self.ID)
			}
			if err := m.ReconcileRaftPeers(ctx); err != nil {
				log.Printf("cluster: reconcile raft peers: %v", err)
			}
			if fn != nil {
				if err := fn(ctx); err != nil {
					log.Printf("cluster: leader ready hook: %v", err)
				}
			}
		}
		wasLeader = leader
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}
