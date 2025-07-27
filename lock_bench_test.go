package main

import (
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	rlockBudget = 200 * time.Nanosecond // p99 target for read‑lock
	wlockBudget = 2 * time.Microsecond  // p99 target for write‑lock
)

func init() {
	// Production runs single‑core; benchmarks should match.
	runtime.GOMAXPROCS(1)
}

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	idx := int(float64(len(d))*p) - 1
	if idx < 0 {
		idx = 0
	}
	return d[idx]
}

func BenchmarkLockHoldTimes(b *testing.B) {
	var mu sync.RWMutex

	b.Run("RLock", func(b *testing.B) {
		b.ReportAllocs()
		lats := make([]time.Duration, b.N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			mu.RLock()
			mu.RUnlock()
			lats[i] = time.Since(start)
		}
		if p99 := percentile(lats, 0.99); p99 > rlockBudget {
			b.Fatalf("p99 RLock latency %v exceeds budget %v", p99, rlockBudget)
		}
	})

	b.Run("WLock", func(b *testing.B) {
		b.ReportAllocs()
		lats := make([]time.Duration, b.N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			mu.Lock()
			mu.Unlock()
			lats[i] = time.Since(start)
		}
		if p99 := percentile(lats, 0.99); p99 > wlockBudget {
			b.Fatalf("p99 WLock latency %v exceeds budget %v", p99, wlockBudget)
		}
	})
}

func BenchmarkReveal(b *testing.B) {
	srv := NewServer()
	baseChunk := ChunkID{X: 0, Y: 0}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		localX, localY := 0, 0
		for pb.Next() {
			srv.reveal(1, baseChunk, localX, localY)

			// Advance to avoid hammering the same bit in the chunk.
			localX++
			if localX == ChunkSize {
				localX = 0
				localY++
				if localY == ChunkSize {
					localY = 0
				}
			}
		}
	})
}

// BenchmarkRevealHeavy hammers a large area of the board and reports extra
// heap growth per iteration so you can spot regressions quickly with
// `go test -run=^$ -bench RevealHeavy -benchmem`.
func BenchmarkRevealHeavy(b *testing.B) {
	srv := NewServer()

	// Load a 256 × 256 chunk grid (~65 k chunks) so that every reveal below
	// touches an already‑allocated chunk node instead of silently short‑circuiting.
	const grid = 256
	for x := -grid / 2; x < grid/2; x++ {
		for y := -grid / 2; y < grid/2; y++ {
			// Ensure chunk is loaded by revealing a dummy cell
			srv.reveal(1, ChunkID{X: int32(x), Y: int32(y)}, 0, 0)
		}
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	b.ReportAllocs()
	b.ResetTimer()

	base := ChunkID{X: 0, Y: 0}

	// Pre‑choose 100 chunks we’ll fully reveal exactly once.
	full := make([]ChunkID, 100)
	for i := range full {
		full[i] = ChunkID{
			X: int32(rand.Intn(grid) - grid/2),
			Y: int32(rand.Intn(grid) - grid/2),
		}
	}

	for i := 0; i < b.N; i++ {
		// stage A: first 100 iters => full‑chunk reveal
		if i < len(full) {
			c := full[i]
			for x := 0; x < ChunkSize; x++ {
				for y := 0; y < ChunkSize; y++ {
					srv.reveal(1, c, x, y)
				}
			}
			continue
		}

		// stage B: organic spray across the board
		for j := 0; j < 30; j++ { // 30 different chunks
			c := ChunkID{
				X: int32(rand.Intn(grid) - grid/2),
				Y: int32(rand.Intn(grid) - grid/2),
			}
			for k := 0; k < 200; k++ { // 200 random cells per chunk
				x := rand.Intn(ChunkSize)
				y := rand.Intn(ChunkSize)
				srv.reveal(1, c, x, y)
			}
		}

		// ping the origin chunk now and then to keep that code path hot.
		if i%250 == 0 {
			srv.reveal(1, base, rand.Intn(ChunkSize), rand.Intn(ChunkSize))
		}
	}

	b.StopTimer()
	runtime.ReadMemStats(&after)
	// Extra heap per op (not captured by -benchmem's per‑op numbers).
	extra := float64(after.Alloc-before.Alloc) / float64(b.N)
	b.ReportMetric(extra, "bytes/op_extra")
}
