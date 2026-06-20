package cli

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// BenchmarkOptions configures storage throughput tests.
type BenchmarkOptions struct {
	Seed       string
	URIPrefix  string
	Token      string
	AutoPrefix bool
	Size       int
	Writes     int
	Reads      int
	Workers    int
	Prefix     string
	Cleanup    bool
}

// BenchmarkResult holds measured throughput.
type BenchmarkResult struct {
	ChunkSize int
	Writes    int
	Reads     int
	Workers   int
	WriteMBs  float64
	ReadMBs   float64
	WriteAvg  time.Duration
	ReadAvg   time.Duration
}

// RunBenchmark executes chunk write/read throughput tests against a node.
func RunBenchmark(ctx context.Context, opt BenchmarkOptions, w io.Writer) (*BenchmarkResult, error) {
	if opt.Size <= 0 {
		opt.Size = 4 << 20
	}
	if opt.Writes <= 0 {
		opt.Writes = 50
	}
	if opt.Reads <= 0 {
		opt.Reads = 50
	}
	if opt.Workers <= 0 {
		opt.Workers = 4
	}
	if opt.Prefix == "" {
		opt.Prefix = "bench"
	}

	prefix := opt.URIPrefix
	if opt.AutoPrefix && prefix == "" {
		prefix = DetectPrefix(ctx, opt.Seed, opt.Token)
	}
	c := NewClient(opt.Seed, prefix, opt.Token)

	payload := make([]byte, opt.Size)
	_, _ = rand.Read(payload)

	ids := make([]string, opt.Writes)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s_%d_%d", opt.Prefix, time.Now().UnixNano(), i)
	}

	writeAcc := &latencyAcc{}
	writeStart := time.Now()
	if err := runWorkers(ctx, opt.Workers, opt.Writes, func(i int) error {
		start := time.Now()
		if err := c.PutChunk(ctx, ids[i], payload); err != nil {
			return err
		}
		writeAcc.Add(time.Since(start))
		return nil
	}); err != nil {
		return nil, err
	}
	writeDur := time.Since(writeStart)

	readCount := min(opt.Reads, len(ids))
	readAcc := &latencyAcc{}
	readStart := time.Now()
	if err := runWorkers(ctx, opt.Workers, readCount, func(i int) error {
		start := time.Now()
		got, err := c.GetChunk(ctx, ids[i%len(ids)])
		if err != nil {
			return err
		}
		if len(got) != len(payload) {
			return fmt.Errorf("chunk %s size mismatch: got %d want %d", ids[i%len(ids)], len(got), len(payload))
		}
		readAcc.Add(time.Since(start))
		return nil
	}); err != nil {
		return nil, err
	}
	readDur := time.Since(readStart)

	res := &BenchmarkResult{
		ChunkSize: opt.Size,
		Writes:    opt.Writes,
		Reads:     readCount,
		Workers:   opt.Workers,
	}
	if writeDur > 0 {
		res.WriteMBs = float64(int64(opt.Writes)*int64(opt.Size)) / writeDur.Seconds() / (1 << 20)
	}
	if readDur > 0 {
		res.ReadMBs = float64(int64(res.Reads)*int64(opt.Size)) / readDur.Seconds() / (1 << 20)
	}
	if opt.Writes > 0 {
		res.WriteAvg = time.Duration(writeAcc.Load() / int64(opt.Writes))
	}
	if res.Reads > 0 {
		res.ReadAvg = time.Duration(readAcc.Load() / int64(res.Reads))
	}

	printBenchmark(w, opt, prefix, res)

	if opt.Cleanup {
		for _, id := range ids {
			_ = c.DeleteChunk(ctx, id)
		}
	}
	return res, nil
}

type latencyAcc struct{ v atomic.Int64 }

func (a *latencyAcc) Add(d time.Duration) { a.v.Add(int64(d)) }
func (a *latencyAcc) Load() int64         { return a.v.Load() }

func runWorkers(ctx context.Context, workers, total int, fn func(int) error) error {
	if total == 0 {
		return nil
	}
	if workers > total {
		workers = total
	}
	jobs := make(chan int)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for i := range jobs {
			if err := fn(i); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}

	wg.Add(workers)
	for range workers {
		go worker()
	}
	go func() {
		for i := range total {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- i:
			}
		}
		close(jobs)
	}()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
}

func printBenchmark(w io.Writer, opt BenchmarkOptions, prefix string, res *BenchmarkResult) {
	_, _ = fmt.Fprintf(w, "MemoryFS Benchmark\n")
	_, _ = fmt.Fprintf(w, "Seed:    %s\n", opt.Seed)
	if prefix != "" {
		_, _ = fmt.Fprintf(w, "Prefix:  %s\n", prefix)
	}
	_, _ = fmt.Fprintf(w, "Chunk:   %s   Workers: %d   Writes: %d   Reads: %d\n\n",
		FormatBytes(int64(res.ChunkSize)), res.Workers, res.Writes, res.Reads)
	_, _ = fmt.Fprintf(w, "Write:   %.1f MiB/s   avg %s/op\n", res.WriteMBs, res.WriteAvg.Round(time.Millisecond))
	_, _ = fmt.Fprintf(w, "Read:    %.1f MiB/s   avg %s/op\n", res.ReadMBs, res.ReadAvg.Round(time.Millisecond))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
