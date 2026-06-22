package chunk

import (
	"fmt"
	"hash/fnv"
	"sort"
)

// TargetRole identifies a target's position within a chain.
type TargetRole int

const (
	RoleHead   TargetRole = iota // newest writes enter here
	RoleMiddle                   // propagates HEAD -> TAIL
	RoleTail                     // last copy in the chain
)

func (r TargetRole) String() string {
	switch r {
	case RoleHead:
		return "HEAD"
	case RoleTail:
		return "TAIL"
	default:
		return "MIDDLE"
	}
}

// Target is one replica of a chain, hosted on one node.
type Target struct {
	NodeURL string
	Role    TargetRole
}

// Chain is an ordered list of targets (HEAD first, TAIL last). All targets in
// a chain must reside on different nodes.
type Chain struct {
	ID      uint32
	Targets []Target
}

// Head returns the HEAD target.
func (c *Chain) Head() Target { return c.Targets[0] }

// Tail returns the TAIL target.
func (c *Chain) Tail() Target { return c.Targets[len(c.Targets)-1] }

// NextOf returns the next-in-chain after the given node, or empty if it is TAIL.
func (c *Chain) NextOf(nodeURL string) (Target, bool) {
	for i, t := range c.Targets {
		if t.NodeURL == nodeURL && i+1 < len(c.Targets) {
			return c.Targets[i+1], true
		}
	}
	return Target{}, false
}

// NodeURLs returns all target node URLs in chain order (HEAD first).
func (c *Chain) NodeURLs() []string {
	out := make([]string, len(c.Targets))
	for i, t := range c.Targets {
		out[i] = t.NodeURL
	}
	return out
}

// Contains reports whether nodeURL hosts a target of this chain.
func (c *Chain) Contains(nodeURL string) bool {
	for _, t := range c.Targets {
		if t.NodeURL == nodeURL {
			return true
		}
	}
	return false
}

// ChainTable is the set of chains usable by the cluster. Chunks are placed on
// chains; chains not listed in the table are inactive.
type ChainTable struct {
	Chains []Chain
}

// BuildChainTable derives a deterministic chain table from the sorted node
// list. The number of chains equals len(nodes) and each chain has rf targets
// on distinct nodes, rotated so every node hosts an equal share of HEADs.
func BuildChainTable(nodes []string, rf int) (*ChainTable, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}
	if rf <= 0 {
		rf = 1
	}
	if rf > len(nodes) {
		rf = len(nodes)
	}
	sorted := append([]string(nil), nodes...)
	sort.Strings(sorted)

	chains := make([]Chain, len(sorted))
	for i := range sorted {
		targets := make([]Target, 0, rf)
		for j := 0; j < rf; j++ {
			role := RoleMiddle
			switch j {
			case 0:
				role = RoleHead
			case rf - 1:
				role = RoleTail
			}
			targets = append(targets, Target{
				NodeURL: sorted[(i+j)%len(sorted)],
				Role:    role,
			})
		}
		chains[i] = Chain{ID: uint32(i), Targets: targets}
	}
	return &ChainTable{Chains: chains}, nil
}

// SelectChain maps a chunkID to a chain via FNV-1a hash.
func (t *ChainTable) SelectChain(chunkID string) (*Chain, error) {
	if t == nil || len(t.Chains) == 0 {
		return nil, fmt.Errorf("chain table empty")
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(chunkID))
	idx := int(h.Sum32() % uint32(len(t.Chains)))
	return &t.Chains[idx], nil
}

// ChainFor is a convenience that builds a chain table and selects for chunkID.
func ChainFor(nodes []string, chunkID string, rf int) (*Chain, error) {
	table, err := BuildChainTable(nodes, rf)
	if err != nil {
		return nil, err
	}
	return table.SelectChain(chunkID)
}
