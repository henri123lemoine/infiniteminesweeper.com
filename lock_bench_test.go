package main

import (
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	rlockBudget = 100 * time.Nanosecond // p99 target for read‑lock
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
