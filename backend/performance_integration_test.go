//go:build integration
// +build integration

package main

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestLoadWithManyClients tests server performance under high client load
func TestLoadWithManyClients(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	const numClients = 50
	const actionsPerClient = 20

	clients := make([]*TestClient, numClients)
	var wg sync.WaitGroup

	// Start time measurement
	startTime := time.Now()

	// Create and connect all clients concurrently
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(clientNum int) {
			defer wg.Done()

			client := NewTestClient(t, wsURL, fmt.Sprintf("load%d_%d_%d", clientNum, time.Now().UnixNano()%10000, clientNum*1000+int(time.Now().UnixMicro()%1000)))
			clients[clientNum] = client

			// Join
			client.Join()

			// Subscribe to different chunks to spread load
			chunkX := int64(clientNum % 10)
			chunkY := int64(clientNum / 10)
			client.Subscribe(chunkX, chunkY)

			// Perform random reveals
			for action := 0; action < actionsPerClient; action++ {
				cellIndex := uint32((clientNum*actionsPerClient + action) % 4096)
				requestID := uint64(clientNum*1000 + action)

				client.Reveal(chunkX, chunkY, cellIndex, requestID)

				// Small delay to avoid overwhelming
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Cleanup clients
	for _, client := range clients {
		if client != nil {
			client.Close()
		}
	}

	elapsed := time.Since(startTime)
	totalActions := numClients * actionsPerClient
	actionsPerSecond := float64(totalActions) / elapsed.Seconds()

	t.Logf("Load test completed:")
	t.Logf("  Clients: %d", numClients)
	t.Logf("  Actions per client: %d", actionsPerClient)
	t.Logf("  Total actions: %d", totalActions)
	t.Logf("  Time elapsed: %v", elapsed)
	t.Logf("  Actions/second: %.2f", actionsPerSecond)

	// Performance assertions
	if elapsed > 30*time.Second {
		t.Errorf("load test took too long: %v (expected < 30s)", elapsed)
	}

	if actionsPerSecond < 50 {
		t.Errorf("throughput too low: %.2f actions/sec (expected > 50)", actionsPerSecond)
	}
}

// TestMemoryUsageUnderLoad verifies memory doesn't grow excessively
func TestMemoryUsageUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory test in short mode")
	}

	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	// Create many chunks to simulate memory usage
	const numClients = 20
	const chunksPerClient = 10

	clients := make([]*TestClient, numClients)

	// Initial memory snapshot
	server.stateMu.RLock()
	initialChunks := len(server.chunks)
	initialSubs := 0
	for _, subs := range server.playerSubs {
		initialSubs += len(subs)
	}
	server.stateMu.RUnlock()

	// Create clients and spread them across many chunks
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientNum int) {
			defer wg.Done()

			client := NewTestClient(t, wsURL, fmt.Sprintf("mem%d_%d_%d", clientNum, time.Now().UnixNano()%10000, clientNum*2000+int(time.Now().UnixMicro()%1000)))
			clients[clientNum] = client
			client.Join()

			// Subscribe to multiple chunks
			for chunk := 0; chunk < chunksPerClient; chunk++ {
				chunkX := int64(clientNum*chunksPerClient + chunk)
				chunkY := int64(clientNum % 5) // Spread across Y axis
				client.Subscribe(chunkX, chunkY)

				// Make a reveal to populate the chunk
				client.Reveal(chunkX, chunkY, uint32(chunk*10), uint64(clientNum*1000+chunk))
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond) // Let server process everything

	// Final memory snapshot
	server.stateMu.RLock()
	finalChunks := len(server.chunks)
	finalSubs := 0
	for _, subs := range server.playerSubs {
		finalSubs += len(subs)
	}
	server.stateMu.RUnlock()

	t.Logf("Memory usage test:")
	t.Logf("  Initial chunks: %d, Final chunks: %d", initialChunks, finalChunks)
	t.Logf("  Initial subscriptions: %d, Final subscriptions: %d", initialSubs, finalSubs)
	t.Logf("  Chunks created: %d", finalChunks-initialChunks)
	t.Logf("  Expected max chunks: %d", numClients*chunksPerClient)

	// Cleanup
	for _, client := range clients {
		if client != nil {
			client.Close()
		}
	}

	// Reasonable bounds checking
	expectedMaxChunks := numClients * chunksPerClient
	if finalChunks-initialChunks > expectedMaxChunks*2 {
		t.Errorf("too many chunks created: %d (expected <= %d)",
			finalChunks-initialChunks, expectedMaxChunks*2)
	}
}

// TestChunkGenerationPerformance measures chunk generation speed
func TestChunkGenerationPerformance(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client := NewTestClient(t, wsURL, "perftest")
	defer client.Close()
	client.Join()

	const numChunks = 100
	startTime := time.Now()

	// Force generation of many chunks by subscribing to them
	for i := 0; i < numChunks; i++ {
		chunkX := int64(i % 10)
		chunkY := int64(i / 10)

		client.Subscribe(chunkX, chunkY)

		// Make a reveal to ensure chunk is fully generated
		client.Reveal(chunkX, chunkY, 0, uint64(i+1000))
		
		// Brief delay to allow server to process the reveal
		time.Sleep(2 * time.Millisecond)
	}
	
	// Allow additional time for all operations to complete
	time.Sleep(100 * time.Millisecond)

	elapsed := time.Since(startTime)
	chunksPerSecond := float64(numChunks) / elapsed.Seconds()

	t.Logf("Chunk generation performance:")
	t.Logf("  Chunks generated: %d", numChunks)
	t.Logf("  Time elapsed: %v", elapsed)
	t.Logf("  Chunks/second: %.2f", chunksPerSecond)

	// Performance expectations
	if elapsed > 5*time.Second {
		t.Errorf("chunk generation too slow: %v (expected < 5s)", elapsed)
	}

	if chunksPerSecond < 20 {
		t.Errorf("chunk generation rate too low: %.2f chunks/sec (expected > 20)", chunksPerSecond)
	}

	// Verify chunks were actually created
	server.stateMu.RLock()
	finalChunkCount := len(server.chunks)
	server.stateMu.RUnlock()

	if finalChunkCount < numChunks {
		t.Errorf("expected at least %d chunks, got %d", numChunks, finalChunkCount)
	}
}

// TestWebSocketThroughput measures raw message throughput
func TestWebSocketThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throughput test in short mode")
	}

	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client := NewTestClient(t, wsURL, "throughput")
	defer client.Close()
	client.Join()
	client.Subscribe(0, 0)

	const numMessages = 500
	const batchSize = 10

	startTime := time.Now()

	// Send messages in batches to avoid overwhelming
	for batch := 0; batch < numMessages/batchSize; batch++ {
		var batchWg sync.WaitGroup
		batchWg.Add(batchSize)

		for i := 0; i < batchSize; i++ {
			go func(msgNum int) {
				defer batchWg.Done()
				cellIndex := uint32(msgNum % 4096)
				requestID := uint64(batch*batchSize + msgNum + 2000)
				client.Reveal(0, 0, cellIndex, requestID)
			}(batch*batchSize + i)
		}

		batchWg.Wait()
		time.Sleep(10 * time.Millisecond) // Small delay between batches
	}

	elapsed := time.Since(startTime)
	messagesPerSecond := float64(numMessages) / elapsed.Seconds()

	t.Logf("WebSocket throughput test:")
	t.Logf("  Messages sent: %d", numMessages)
	t.Logf("  Time elapsed: %v", elapsed)
	t.Logf("  Messages/second: %.2f", messagesPerSecond)

	// Performance expectations
	if elapsed > 10*time.Second {
		t.Errorf("throughput test took too long: %v (expected < 10s)", elapsed)
	}

	if messagesPerSecond < 50 {
		t.Errorf("message throughput too low: %.2f msg/sec (expected > 50)", messagesPerSecond)
	}
}

// TestConcurrentChunkAccess tests thread safety under high concurrency
func TestConcurrentChunkAccess(t *testing.T) {
	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	const numGoroutines = 20
	const accessesPerGoroutine = 50

	// All goroutines will access the same chunk to maximize contention
	const targetChunkX, targetChunkY = int64(0), int64(0)

	var wg sync.WaitGroup
	startTime := time.Now()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(routineNum int) {
			defer wg.Done()

			client := NewTestClient(t, wsURL, fmt.Sprintf("concurrent%d_%d_%d", routineNum, time.Now().UnixNano()%10000, routineNum*3000+int(time.Now().UnixMicro()%1000)))
			defer client.Close()

			client.Join()
			client.Subscribe(targetChunkX, targetChunkY)

			// Perform many accesses to the same chunk
			for access := 0; access < accessesPerGoroutine; access++ {
				cellIndex := uint32((routineNum*accessesPerGoroutine + access) % 4096)
				requestID := uint64(routineNum*1000 + access + 3000)

				client.Reveal(targetChunkX, targetChunkY, cellIndex, requestID)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	totalAccesses := numGoroutines * accessesPerGoroutine
	accessesPerSecond := float64(totalAccesses) / elapsed.Seconds()

	t.Logf("Concurrent chunk access test:")
	t.Logf("  Goroutines: %d", numGoroutines)
	t.Logf("  Accesses per goroutine: %d", accessesPerGoroutine)
	t.Logf("  Total accesses: %d", totalAccesses)
	t.Logf("  Time elapsed: %v", elapsed)
	t.Logf("  Accesses/second: %.2f", accessesPerSecond)

	// Should complete without deadlocks or race conditions
	if elapsed > 15*time.Second {
		t.Errorf("concurrent access test took too long: %v (expected < 15s)", elapsed)
	}
}

// BenchmarkRevealProcessing benchmarks the core reveal processing
func BenchmarkRevealProcessing(b *testing.B) {
	_, wsURL, cleanup := setupTestServer(&testing.T{})
	defer cleanup()

	client := NewTestClient(&testing.T{}, wsURL, "benchmark")
	defer client.Close()
	client.Join()
	client.Subscribe(0, 0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cellIndex := uint32(i % 4096)
		requestID := uint64(i + 5000)
		client.Reveal(0, 0, cellIndex, requestID)
	}
}

// BenchmarkChunkSubscription benchmarks subscription management
func BenchmarkChunkSubscription(b *testing.B) {
	_, wsURL, cleanup := setupTestServer(&testing.T{})
	defer cleanup()

	client := NewTestClient(&testing.T{}, wsURL, "subbench")
	defer client.Close()
	client.Join()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		chunkX := int64(i % 100)
		chunkY := int64((i / 100) % 100)
		client.Subscribe(chunkX, chunkY)
	}
}

