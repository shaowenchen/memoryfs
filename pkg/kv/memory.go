package kv

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
)

// MemoryKV is an in-memory thread-safe KV store.
type MemoryKV struct {
	mu     sync.RWMutex
	str    map[string][]byte
	hash   map[string]map[string][]byte
	sets   map[string]map[string]struct{}
	incrs  map[string]uint64
}

// NewMemoryKV creates an empty in-memory KV store.
func NewMemoryKV() *MemoryKV {
	return &MemoryKV{
		str:   make(map[string][]byte),
		hash:  make(map[string]map[string][]byte),
		sets:  make(map[string]map[string]struct{}),
		incrs: make(map[string]uint64),
	}
}

func (m *MemoryKV) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.str[key]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneBytes(v), nil
}

func (m *MemoryKV) Set(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.str[key] = cloneBytes(value)
	return nil
}

func (m *MemoryKV) Del(keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.str, key)
		delete(m.hash, key)
		delete(m.sets, key)
		delete(m.incrs, key)
	}
	return nil
}

func (m *MemoryKV) Exists(key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.str[key]
	if ok {
		return true, nil
	}
	_, ok = m.hash[key]
	if ok {
		return true, nil
	}
	_, ok = m.sets[key]
	if ok {
		return true, nil
	}
	_, ok = m.incrs[key]
	return ok, nil
}

func (m *MemoryKV) HGet(key, field string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.hash[key]
	if !ok {
		return nil, ErrNotFound
	}
	v, ok := h[field]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneBytes(v), nil
}

func (m *MemoryKV) HSet(key, field string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hash[key]
	if !ok {
		h = make(map[string][]byte)
		m.hash[key] = h
	}
	h[field] = cloneBytes(value)
	return nil
}

func (m *MemoryKV) HGetAll(key string) (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.hash[key]
	if !ok {
		return map[string][]byte{}, nil
	}
	out := make(map[string][]byte, len(h))
	for k, v := range h {
		out[k] = cloneBytes(v)
	}
	return out, nil
}

func (m *MemoryKV) HDel(key string, fields ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hash[key]
	if !ok {
		return nil
	}
	for _, field := range fields {
		delete(h, field)
	}
	if len(h) == 0 {
		delete(m.hash, key)
	}
	return nil
}

func (m *MemoryKV) HExists(key, field string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.hash[key]
	if !ok {
		return false, nil
	}
	_, ok = h[field]
	return ok, nil
}

func (m *MemoryKV) HLen(key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.hash[key]
	if !ok {
		return 0, nil
	}
	return int64(len(h)), nil
}

func (m *MemoryKV) Incr(key string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrs[key]++
	v := m.incrs[key]
	m.str[key] = []byte(strconv.FormatUint(v, 10))
	return v, nil
}

func (m *MemoryKV) SAdd(key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sets[key]
	if !ok {
		s = make(map[string]struct{})
		m.sets[key] = s
	}
	for _, member := range members {
		if member != "" {
			s[member] = struct{}{}
		}
	}
	return nil
}

func (m *MemoryKV) SRem(key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sets[key]
	if !ok {
		return nil
	}
	for _, member := range members {
		delete(s, member)
	}
	if len(s) == 0 {
		delete(m.sets, key)
	}
	return nil
}

func (m *MemoryKV) SMembers(key string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sets[key]
	if !ok {
		return nil, nil
	}
	out := make([]string, 0, len(s))
	for member := range s {
		out = append(out, member)
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemoryKV) Batch(ops []Op) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range ops {
		if err := m.applyLocked(op); err != nil {
			return err
		}
	}
	return nil
}

func (m *MemoryKV) applyLocked(op Op) error {
	switch op.Type {
	case OpSet:
		m.str[op.Key] = cloneBytes(op.Value)
	case OpDel:
		delete(m.str, op.Key)
		delete(m.hash, op.Key)
		delete(m.sets, op.Key)
		delete(m.incrs, op.Key)
	case OpHSet:
		h, ok := m.hash[op.Key]
		if !ok {
			h = make(map[string][]byte)
			m.hash[op.Key] = h
		}
		h[op.Field] = cloneBytes(op.Value)
	case OpHDel:
		if h, ok := m.hash[op.Key]; ok {
			delete(h, op.Field)
			if len(h) == 0 {
				delete(m.hash, op.Key)
			}
		}
	case OpIncr:
		m.incrs[op.Key]++
		v := m.incrs[op.Key]
		m.str[op.Key] = []byte(strconv.FormatUint(v, 10))
	case OpSAdd:
		s, ok := m.sets[op.Key]
		if !ok {
			s = make(map[string]struct{})
			m.sets[op.Key] = s
		}
		if op.Member != "" {
			s[op.Member] = struct{}{}
		}
	case OpSRem:
		if s, ok := m.sets[op.Key]; ok {
			delete(s, op.Member)
			if len(s) == 0 {
				delete(m.sets, op.Key)
			}
		}
	default:
		return fmt.Errorf("unknown op type: %s", op.Type)
	}
	return nil
}

type snapshotData struct {
	Str   map[string][]byte            `json:"str"`
	Hash  map[string]map[string][]byte `json:"hash"`
	Sets  map[string][]string          `json:"sets"`
	Incrs map[string]uint64            `json:"incrs"`
}

func (m *MemoryKV) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data := snapshotData{
		Str:   make(map[string][]byte, len(m.str)),
		Hash:  make(map[string]map[string][]byte, len(m.hash)),
		Sets:  make(map[string][]string, len(m.sets)),
		Incrs: make(map[string]uint64, len(m.incrs)),
	}
	for k, v := range m.str {
		data.Str[k] = cloneBytes(v)
	}
	for k, h := range m.hash {
		cp := make(map[string][]byte, len(h))
		for f, v := range h {
			cp[f] = cloneBytes(v)
		}
		data.Hash[k] = cp
	}
	for k, s := range m.sets {
		members := make([]string, 0, len(s))
		for member := range s {
			members = append(members, member)
		}
		sort.Strings(members)
		data.Sets[k] = members
	}
	for k, v := range m.incrs {
		data.Incrs[k] = v
	}
	return json.Marshal(data)
}

func (m *MemoryKV) Restore(raw []byte) error {
	var data snapshotData
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.str = data.Str
	m.hash = data.Hash
	m.sets = make(map[string]map[string]struct{}, len(data.Sets))
	for k, members := range data.Sets {
		s := make(map[string]struct{}, len(members))
		for _, member := range members {
			s[member] = struct{}{}
		}
		m.sets[k] = s
	}
	m.incrs = data.Incrs
	return nil
}

func (m *MemoryKV) Close() error { return nil }

func cloneBytes(v []byte) []byte {
	if v == nil {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}
