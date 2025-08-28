# Integration Tests for Infinite Minesweeper

This document describes the comprehensive integration test suite for the infinite multiplayer minesweeper game.

## Overview

The integration tests verify the complete client-server interaction pipeline, from WebSocket connections through game state synchronization. They test the core architectural patterns:

- **WebSocket Connection Lifecycle**: Connection establishment, authentication, and cleanup
- **Board State Consistency**: Multi-client view synchronization and deterministic mine generation  
- **Move Validation Pipeline**: Server-side validation and error handling
- **Real-time Updates**: Broadcast distribution and optimistic update reconciliation
- **Performance & Scalability**: Load testing and resource management

## Test Structure

### Backend Tests (`backend/multiplayer_integration_test.go`)

**Core Integration Tests:**
- `TestMultiClientBoardConsistency` - Verifies multiple clients viewing same region receive identical mine layouts
- `TestMoveValidationPipeline` - Tests server rejection of invalid moves (out-of-bounds cells, double reveals, etc.)
- `TestConcurrentReveals` - Multiple clients revealing simultaneously 
- `TestRateLimiting` - Rapid-fire request throttling
- `TestReconnectionStateSync` - State synchronization after client reconnection

**Test Infrastructure:**
- `TestClient` struct wrapping WebSocket with test utilities
- Automatic message compression/decompression handling
- Built-in timeout and error handling
- Message queuing for async verification

### Performance Tests (`backend/performance_integration_test.go`)

**Load & Stress Tests:**
- `TestLoadWithManyClients` - 50 concurrent clients performing 20 actions each
- `TestMemoryUsageUnderLoad` - Memory growth verification under sustained load
- `TestChunkGenerationPerformance` - Measures chunk creation throughput
- `TestWebSocketThroughput` - Raw message handling capacity
- `TestConcurrentChunkAccess` - Thread safety under high contention

**Benchmarks:**
- `BenchmarkRevealProcessing` - Core reveal logic performance
- `BenchmarkChunkSubscription` - Subscription management overhead

### Frontend Tests (`frontend/tests/integration-tests.mjs`)

**Client-Side Integration:**
- WebSocket connection lifecycle management
- Join/authentication flow verification
- Chunk subscription and region sync handling
- Reveal action request/response cycle
- Multi-client coordination testing
- Rapid message handling stress test

**Mock Infrastructure:**
- `MockWebSocketClient` - Test WebSocket wrapper with compression support
- `TestRunner` - Assertion framework with concise reporting
- Configurable timeouts and async message handling

## Key Test Patterns

### Multi-Client State Consistency

```javascript
// Two clients subscribe to same chunk
client1.Subscribe(0, 0)
client2.Subscribe(0, 0)

// Client1 reveals cell
client1.Reveal(0, 0, 10, requestID)

// Both receive identical updates
const update1 = await client1.receiveChunkUpdate()
const update2 = await client2.receiveChunkUpdate()
assert(deepEqual(update1.revealedCells, update2.revealedCells))
```

### Invalid Move Rejection

```javascript
const tests = [
  { cell: 5, expectAck: true },           // Valid reveal
  { cell: 4096, expectAck: false },       // Invalid cell index  
  { cell: 5, expectAck: true },           // Double reveal (allowed, no new reveals)
]
```

### Concurrent Access Testing

```javascript
// All clients reveal different cells simultaneously  
const promises = clients.map((client, i) => 
  client.Reveal(0, 0, i + 10, requestID + i)
)
await Promise.all(promises)

// Verify each received their own RevealAck
```

### Performance Verification

```javascript
const startTime = Date.now()
// Perform N operations
const elapsed = Date.now() - startTime
const throughput = operations / (elapsed / 1000)

assert(throughput > minExpectedThroughput)
assert(elapsed < maxExpectedTime)
```

## Running Tests

### Quick Start

```bash
# Run all integration tests
./scripts/run-integration-tests.sh

# Backend only  
./scripts/run-integration-tests.sh --backend-only

# Skip performance tests
./scripts/run-integration-tests.sh --skip-performance

# Verbose output with custom timeout
./scripts/run-integration-tests.sh -v -t 120s
```

### Manual Execution

```bash
# Backend integration tests
go test -v ./backend -run TestMultiClient -timeout 30s

# Performance tests  
go test -v ./backend -run TestLoad -timeout 60s

# Frontend tests (requires running server)
cd frontend && node tests/integration-tests.mjs

# Benchmarks
go test -bench=. -benchmem ./backend
```

### Environment Variables

```bash
export TEST_TIMEOUT=60s          # Test timeout
export VERBOSE=true              # Detailed output  
export SKIP_PERFORMANCE=true     # Skip perf tests
export BACKEND_ONLY=true         # Backend tests only
export FRONTEND_ONLY=true        # Frontend tests only
```

## Test Configuration

### Server Setup

Tests use `startTestServerWithFSM()` which:
- Disables proximity radius restrictions (`proximityRadius = -1`)
- Starts minimal leaderboard broadcaster
- Provides proper cleanup handling
- Returns WebSocket URL for client connections

### Performance Expectations

**Throughput Targets:**
- 50+ actions/second under load
- 20+ chunks/second generation  
- 50+ messages/second WebSocket throughput

**Latency Targets:**
- < 30s for 50-client load test
- < 5s for 100-chunk generation
- < 10s for 500-message throughput test

**Resource Limits:**
- Memory growth bounded by expected chunk count
- No file descriptor leaks
- Proper connection cleanup

## Architecture Coverage

### Tested Components

**Backend (`backend/`):**
- `server.go` - Core state management and WebSocket handling
- `game.go` - Reveal logic, flood-fill, and scoring  
- `websocket.go` - Connection lifecycle and message routing
- Protocol buffer message serialization/compression

**Frontend (`frontend/src/`):**
- `useGameState.js` - WebSocket client and state management
- Message encoding/decoding utilities
- Optimistic update reconciliation

**Protocol (`proto/messages.proto`):**
- All client-server message types
- State transition validation (SPECTATOR ↔ PLAYER)
- Chunk subscription management

### Integration Points

1. **WebSocket Protocol Stack**: Raw TCP → WebSocket → Gzip → Protocol Buffers → Game Logic
2. **State Synchronization**: Client Optimistic Updates ↔ Server Authority ↔ Broadcast Distribution  
3. **Chunk Management**: Subscription → Generation → Caching → Cleanup
4. **Authentication Flow**: Connection → Spectator Mode → Join → Player Mode → Session Management

## Debugging Failed Tests

### Common Issues

**Connection Failures:**
- Check server startup in test logs
- Verify port availability (WebSocket upgrade)
- Ensure proper cleanup between tests

**Message Timing:**  
- Increase timeout for slow CI environments
- Add debug logging to trace message flow
- Check for race conditions in concurrent tests

**State Inconsistency:**
- Verify deterministic chunk generation (seed consistency)
- Check subscription management (proper add/remove)
- Validate message ordering (RevealAck before ChunkUpdateBroadcast)

### Debug Logging

```bash
# Enable debug logs
go test -v ./backend -run TestName 2>&1 | grep -E "(Client|Player|Server)"

# Add temporary debug prints
t.Logf("Debug: received message type %T", msg.GetPayload())
```

## Future Enhancements

### Additional Test Scenarios

- **Network Partitions**: Client disconnect/reconnect during active gameplay
- **Database Integration**: WAL persistence and snapshot recovery testing  
- **Security Testing**: Rate limiting, input validation, session hijacking
- **Browser Compatibility**: Cross-browser WebSocket behavior
- **Mobile Testing**: Touch interactions and connection stability

### Test Infrastructure Improvements

- **Parallel Test Execution**: Independent test server instances
- **Test Data Generation**: Reproducible game state fixtures
- **Visual Regression Testing**: Canvas rendering verification  
- **Chaos Engineering**: Random failure injection
- **CI/CD Integration**: Automated test execution and reporting

## Contributing

When adding new integration tests:

1. Follow the established `TestClient` pattern for backend tests
2. Use `MockWebSocketClient` for frontend testing
3. Include both positive and negative test cases
4. Add performance tests for new features affecting throughput
5. Update this documentation with new test scenarios

The integration test suite is critical for maintaining system reliability as the codebase evolves. Comprehensive coverage ensures that client-server interactions work correctly across all supported scenarios.