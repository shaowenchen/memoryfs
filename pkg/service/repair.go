package service

import (
	"sync"
	"time"
)

type repairJob struct {
	ChunkID   string
	Replicas  []string
	Attempts  int
	Enqueued  time.Time
	LastError string
}

// RepairQueue tracks chunks needing replica repair.
type RepairQueue struct {
	mu      sync.Mutex
	pending map[string]repairJob
}

// NewRepairQueue creates an empty repair queue.
func NewRepairQueue() *RepairQueue {
	return &RepairQueue{pending: make(map[string]repairJob)}
}

func (q *RepairQueue) Enqueue(chunkID string, replicas []string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.pending[chunkID]; ok {
		return
	}
	q.pending[chunkID] = repairJob{
		ChunkID:  chunkID,
		Replicas: append([]string(nil), replicas...),
		Enqueued: time.Now(),
	}
}

func (q *RepairQueue) Done(chunkID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.pending, chunkID)
}

func (q *RepairQueue) Fail(chunkID, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	job, ok := q.pending[chunkID]
	if !ok {
		return
	}
	job.Attempts++
	job.LastError = errMsg
	q.pending[chunkID] = job
}

func (q *RepairQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *RepairQueue) Snapshot() []repairJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]repairJob, 0, len(q.pending))
	for _, job := range q.pending {
		out = append(out, job)
	}
	return out
}

// RepairInfo exposes queue status for APIs.
type RepairInfo struct {
	Pending int         `json:"pending"`
	Jobs    []RepairJob `json:"jobs,omitempty"`
}

// RepairJob is a public view of a repair task.
type RepairJob struct {
	ChunkID   string    `json:"chunk_id"`
	Replicas  []string  `json:"replicas"`
	Attempts  int       `json:"attempts"`
	Enqueued  time.Time `json:"enqueued"`
	LastError string    `json:"last_error,omitempty"`
}

func (q *RepairQueue) Info(limit int) RepairInfo {
	jobs := q.Snapshot()
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	pub := make([]RepairJob, 0, len(jobs))
	for _, j := range jobs {
		pub = append(pub, RepairJob{
			ChunkID: j.ChunkID, Replicas: append([]string(nil), j.Replicas...),
			Attempts: j.Attempts, Enqueued: j.Enqueued, LastError: j.LastError,
		})
	}
	return RepairInfo{Pending: len(q.Snapshot()), Jobs: pub}
}
