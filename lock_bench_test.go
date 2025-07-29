package main

import (
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
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

		p50 := percentile(lats, 0.50)
		p99 := percentile(lats, 0.99)
		b.ReportMetric(float64(p50.Nanoseconds()), "p50_ns")
		b.ReportMetric(float64(p99.Nanoseconds()), "p99_ns")

		b.Logf("RLock p50: %v, p99: %v", p50, p99)
		if p99 > 1*time.Microsecond {
			b.Logf("WARNING: High RLock latency may impact read performance")
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

		p50 := percentile(lats, 0.50)
		p99 := percentile(lats, 0.99)
		b.ReportMetric(float64(p50.Nanoseconds()), "p50_ns")
		b.ReportMetric(float64(p99.Nanoseconds()), "p99_ns")

		b.Logf("WLock p50: %v, p99: %v", p50, p99)
		if p99 > 10*time.Microsecond {
			b.Logf("WARNING: High WLock latency may impact write performance")
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

	// Post-benchmark analysis
	nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.Logf("%.1f ns/op (%.1f million ops/sec)", nsPerOp, 1000/nsPerOp)
	if nsPerOp > 200 {
		b.Logf("WARNING: Slow reveals may cause gameplay lag")
	}
}

func BenchmarkRevealHeavy(b *testing.B) {
	srv := NewServer()

	// Pre-allocate chunks to avoid measuring allocation overhead
	const grid = 64 // Smaller grid for more reasonable test times
	chunks := make([]ChunkID, 0, grid*grid)
	for x := -grid / 2; x < grid/2; x++ {
		for y := -grid / 2; y < grid/2; y++ {
			chunk := ChunkID{X: int32(x), Y: int32(y)}
			chunks = append(chunks, chunk)
			// Touch each chunk once to ensure it's allocated
			srv.reveal(1, chunk, 0, 0)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Track memory more accurately
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	for i := 0; i < b.N; i++ {
		// Pick 10 random chunks and reveal 50 cells each
		for j := 0; j < 10; j++ {
			chunk := chunks[rand.Intn(len(chunks))]
			for k := 0; k < 50; k++ {
				x := rand.Intn(ChunkSize)
				y := rand.Intn(ChunkSize)
				srv.reveal(1, chunk, x, y)
			}
		}
	}

	b.StopTimer()
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Report heap growth per operation (handle GC timing issues)
	if b.N > 0 && m2.Alloc >= m1.Alloc {
		heapGrowth := float64(m2.Alloc-m1.Alloc) / float64(b.N)
		b.ReportMetric(heapGrowth, "heap_bytes/op")

		totalOps := b.N * 10 * 50
		b.Logf("Processed %d reveals across %d chunks", totalOps, len(chunks))
		if heapGrowth > 1024*1024 {
			b.Logf("WARNING: High heap growth %.1f MB/iter", heapGrowth/(1024*1024))
		}
	} else {
		b.Logf("Heap measurement unreliable due to GC timing")
	}
}

// New benchmark: measure reveal performance on fresh vs existing chunks
func BenchmarkRevealFreshVsExisting(b *testing.B) {
	srv := NewServer()

	b.Run("FreshChunk", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Each reveal hits a new chunk
			chunk := ChunkID{X: int32(i), Y: int32(i)}
			srv.reveal(1, chunk, 0, 0)
		}

		// Analysis for chunk allocation cost
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		b.Logf("Fresh chunk: %.0f μs/op", nsPerOp/1000)
		if nsPerOp > 100000 {
			b.Logf("WARNING: Slow chunk allocation may cause lag")
		}
	})

	b.Run("ExistingChunk", func(b *testing.B) {
		// Pre-create the chunk
		chunk := ChunkID{X: 0, Y: 0}
		srv.reveal(1, chunk, 0, 0)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			x := i % ChunkSize
			y := (i / ChunkSize) % ChunkSize
			srv.reveal(1, chunk, x, y)
		}

		// Analysis for cached access
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		b.Logf("Cached chunk: %.1f ns/op", nsPerOp)
		if nsPerOp > 100 {
			b.Logf("WARNING: Unexpectedly slow cache access")
		}
	})
}
