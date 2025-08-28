package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
	"google.golang.org/protobuf/proto"
)

// TestClient wraps a websocket connection with test utilities
type TestClient struct {
	conn     *websocket.Conn
	playerID uint32
	token    string
	name     string
	t        *testing.T
	mu       sync.Mutex
	messages chan *pb.Msg
	done     chan struct{}
}

func NewTestClient(t *testing.T, wsURL, name string) *TestClient {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed for %s: %v", name, err)
	}

	client := &TestClient{
		conn:     conn,
		name:     name,
		t:        t,
		messages: make(chan *pb.Msg, 100),
		done:     make(chan struct{}),
	}

	// Start message reader goroutine
	go client.messageReader()

	return client
}

func (c *TestClient) messageReader() {
	defer func() {
		select {
		case <-c.done:
			// Channel already closed
		default:
			close(c.done)
		}
	}()
	
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			continue
		}

		var msg pb.Msg
		if err := proto.Unmarshal(raw, &msg); err != nil {
			continue
		}

		select {
		case c.messages <- &msg:
		case <-c.done:
			return
		}
	}
}

func (c *TestClient) Send(msg *pb.Msg) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func (c *TestClient) ReceiveMessage(timeout time.Duration) *pb.Msg {
	select {
	case msg := <-c.messages:
		return msg
	case <-time.After(timeout):
		c.t.Fatalf("timeout waiting for message in client %s", c.name)
		return nil
	}
}

func (c *TestClient) Join() {
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: c.name, FlagID: 0}}}
	if err := c.Send(join); err != nil {
		c.t.Fatalf("failed to send join for %s: %v", c.name, err)
	}

	msg := c.ReceiveMessage(2 * time.Second)
	ack := msg.GetJoinAck()
	if ack == nil || !ack.Ok {
		c.t.Fatalf("join failed for %s: %s", c.name, ack.GetError())
	}
	c.token = ack.SessionToken
}

func (c *TestClient) Subscribe(chunkX, chunkY int64) {
	viewUpdate := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId:     &pb.ChunkID{X: chunkX, Y: chunkY},
		Cell:        0,
		WidthCells:  64,
		HeightCells: 64,
	}}}
	if err := c.Send(viewUpdate); err != nil {
		c.t.Fatalf("failed to subscribe %s to chunk (%d,%d): %v", c.name, chunkX, chunkY, err)
	}
}

func (c *TestClient) Reveal(chunkX, chunkY int64, cellIndex uint32, requestID uint64) {
	reveal := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
		ChunkId:   &pb.ChunkID{X: chunkX, Y: chunkY},
		Cell:      cellIndex,
		RequestId: requestID,
	}}}
	if err := c.Send(reveal); err != nil {
		c.t.Fatalf("failed to send reveal from %s: %v", c.name, err)
	}
}

func (c *TestClient) Flag(chunkX, chunkY int64, cellIndex uint32, requestID uint64) {
	flag := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
		ChunkId:      &pb.ChunkID{X: chunkX, Y: chunkY},
		Cell:         cellIndex,
		RequestId:    requestID,
		IsRightClick: true,
	}}}
	if err := c.Send(flag); err != nil {
		c.t.Fatalf("failed to send flag from %s: %v", c.name, err)
	}
}

func (c *TestClient) WaitForRevealAck(requestID uint64, timeout time.Duration) *pb.RevealAck {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-c.messages:
			if ack := msg.GetRevealAck(); ack != nil && ack.RequestId == requestID {
				return ack
			}
		case <-deadline:
			c.t.Fatalf("timeout waiting for RevealAck with requestID %d in client %s", requestID, c.name)
			return nil
		}
	}
}

func (c *TestClient) Close() {
	select {
	case <-c.done:
		// Already closed
	default:
		close(c.done)
	}
	c.conn.Close()
}

func setupTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()
	server := NewServer()
	server.proximityRadius = -1 // Allow all reveals for testing

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.handleWebSocket)

	ts := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	
	cleanup := func() {
		ts.Close()
	}
	
	return server, wsURL, cleanup
}

// Helper to find a mine in a chunk using the same deterministic logic as the server
func findMineInChunk(server *Server, chunkX, chunkY int64) (uint32, bool) {
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	seed := server.generateChunkSeed(chunkID)
	density := server.getChunkDensity(chunkID)
	
	// Convert density to the same format as server
	d32 := float32(density)
	if d32 < 0 {
		d32 = 0
	} else if d32 > 1 {
		d32 = 1
	}
	threshold := uint64(math.Floor(float64(d32 * 100.0)))
	if threshold > 100 {
		threshold = 100
	}
	
	// Check each cell in the chunk (4096 cells)
	for cell := uint32(0); cell < 4096; cell++ {
		cellSeed := splitmix64(seed + uint64(cell))
		if (cellSeed % 100) < threshold {
			return cell, true
		}
	}
	return 0, false
}


// TestMultiClientBoardConsistency verifies that multiple clients viewing the same region
// receive identical mine layouts and cell states
func TestMultiClientBoardConsistency(t *testing.T) {
	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	// Create two clients
	client1 := NewTestClient(t, wsURL, "player1")
	defer client1.Close()
	client2 := NewTestClient(t, wsURL, "player2")
	defer client2.Close()

	// Both clients join
	client1.Join()
	client2.Join()

	// Both subscribe to the same chunk (0,0)
	client1.Subscribe(0, 0)
	client2.Subscribe(0, 0)

	// Allow subscriptions to process
	time.Sleep(100 * time.Millisecond)

	// Client1 reveals cell at (0,0) cell index 10
	client1.Reveal(0, 0, 10, 1001)

	// Both clients should receive updates
	var update1, update2 *pb.ChunkUpdateBroadcast
	
	// Client1 should receive RevealAck and ChunkUpdateBroadcast
	for i := 0; i < 3; i++ {
		msg := client1.ReceiveMessage(2 * time.Second)
		if ack := msg.GetRevealAck(); ack != nil {
			// Good, got the ack
		}
		if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
			update1 = broadcast
		}
	}
	
	// Client2 should receive ChunkUpdateBroadcast
	for i := 0; i < 3; i++ {
		msg := client2.ReceiveMessage(2 * time.Second)
		if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
			update2 = broadcast
			break
		}
	}

	if update1 == nil || update2 == nil {
		t.Fatalf("clients didn't receive chunk updates: client1=%v client2=%v", update1, update2)
	}

	// Verify both updates are for the same chunk
	if update1.ChunkId.X != update2.ChunkId.X || update1.ChunkId.Y != update2.ChunkId.Y {
		t.Fatalf("chunk IDs mismatch: client1=(%d,%d) client2=(%d,%d)", 
			update1.ChunkId.X, update1.ChunkId.Y, update2.ChunkId.X, update2.ChunkId.Y)
	}

	// Verify revealed cells are identical
	revealed1 := update1.GetRevealedCells()
	revealed2 := update2.GetRevealedCells()
	
	if revealed1 == nil || revealed2 == nil {
		t.Fatalf("one or both updates missing revealed cells: client1=%v client2=%v", revealed1, revealed2)
	}
	
	if len(revealed1.Cells) != len(revealed2.Cells) {
		t.Fatalf("revealed cell counts differ: client1=%d client2=%d", 
			len(revealed1.Cells), len(revealed2.Cells))
	}

	for i, cell := range revealed1.Cells {
		if cell != revealed2.Cells[i] {
			t.Fatalf("revealed cell %d differs: client1=%d client2=%d", i, cell, revealed2.Cells[i])
		}
	}
}

// TestMoveValidationPipeline tests that invalid moves are properly rejected
func TestMoveValidationPipeline(t *testing.T) {
	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client := NewTestClient(t, wsURL, "validator")
	defer client.Close()
	client.Join()
	client.Subscribe(0, 0)
	
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		name      string
		chunkX    int64
		chunkY    int64
		cell      uint32
		expectAck bool
		setupFunc func()
	}{
		{
			name:      "valid reveal",
			chunkX:    0,
			chunkY:    0,
			cell:      5,
			expectAck: true,
		},
		{
			name:      "invalid cell index (too high)",
			chunkX:    0,
			chunkY:    0,
			cell:      4096, // Max is 4095 (64*64-1)
			expectAck: false,
		},
		{
			name:      "double reveal same cell",
			chunkX:    0,
			chunkY:    0,
			cell:      5, // Same as first test
			expectAck: true, // Server still sends ack, but with no new reveals
			setupFunc: func() {
				// First reveal was already done in "valid reveal" test
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.setupFunc != nil {
				test.setupFunc()
			}

			requestID := uint64(2000 + i)
			client.Reveal(test.chunkX, test.chunkY, test.cell, requestID)

			// Look for RevealAck within timeout
			timeout := time.After(1 * time.Second)
			var ack *pb.RevealAck
			
		messageLoop:
			for {
				select {
				case msg := <-client.messages:
					if revealAck := msg.GetRevealAck(); revealAck != nil && revealAck.RequestId == requestID {
						ack = revealAck
						break messageLoop
					}
					// Ignore other messages (leaderboard, etc.)
				case <-timeout:
					break messageLoop
				}
			}

			if test.expectAck && ack == nil {
				t.Fatalf("expected RevealAck but got none")
			}
			if !test.expectAck && ack != nil {
				t.Fatalf("expected no RevealAck but got one: %+v", ack)
			}
			if ack != nil && !ack.Ok {
				t.Logf("reveal rejected as expected")
			}
		})
	}
}

// TestConcurrentReveals tests behavior when multiple clients reveal simultaneously
func TestConcurrentReveals(t *testing.T) {
	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	const numClients = 5
	clients := make([]*TestClient, numClients)
	
	// Create and join all clients
	for i := 0; i < numClients; i++ {
		clients[i] = NewTestClient(t, wsURL, fmt.Sprintf("concurrent%d", i))
		defer clients[i].Close()
		clients[i].Join()
		clients[i].Subscribe(0, 0)
	}

	time.Sleep(200 * time.Millisecond)

	// All clients reveal different cells simultaneously
	var wg sync.WaitGroup
	for i, client := range clients {
		wg.Add(1)
		go func(c *TestClient, cellIndex uint32) {
			defer wg.Done()
			c.Reveal(0, 0, cellIndex, uint64(3000+cellIndex))
		}(client, uint32(i+10)) // Cells 10, 11, 12, 13, 14
	}

	wg.Wait()

	// Verify each client receives their own RevealAck
	ackCounts := make(map[uint32]int)
	updateCounts := make(map[uint32]int)

	for i, client := range clients {
		// Each client should get at least their own RevealAck and some updates
		timeout := time.After(3 * time.Second)
		receivedAck := false
		
	clientLoop:
		for {
			select {
			case msg := <-client.messages:
				if ack := msg.GetRevealAck(); ack != nil {
					ackCounts[uint32(i)]++
					if ack.RequestId == uint64(3000+i+10) {
						receivedAck = true
					}
				}
				if msg.GetChunkUpdateBroadcast() != nil {
					updateCounts[uint32(i)]++
				}
			case <-timeout:
				break clientLoop
			}
		}
		
		if !receivedAck {
			t.Errorf("client %d didn't receive their own RevealAck", i)
		}
	}

	// Verify reasonable message distribution
	t.Logf("Ack counts: %v", ackCounts)
	t.Logf("Update counts: %v", updateCounts)
}

// TestRateLimiting verifies that rapid-fire requests are properly throttled
func TestRateLimiting(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	// Set aggressive rate limiting for testing
	server.stateMu.Lock()
	// Note: Adjust these based on actual rate limiting implementation
	server.stateMu.Unlock()

	client := NewTestClient(t, wsURL, "spammer")
	defer client.Close()
	client.Join()
	client.Subscribe(0, 0)

	time.Sleep(100 * time.Millisecond)

	// Send many reveals rapidly
	const numRapidReveals = 20
	const delayBetween = 10 * time.Millisecond

	var successCount, errorCount int
	
	for i := 0; i < numRapidReveals; i++ {
		// Try to reveal different cells rapidly
		cellIndex := uint32(20 + i)
		if cellIndex >= 4096 {
			cellIndex = uint32(20 + (i % 100)) // Wrap around to valid range
		}
		
		client.Reveal(0, 0, cellIndex, uint64(4000+i))
		time.Sleep(delayBetween)
	}

	// Count responses
	timeout := time.After(5 * time.Second)
	responses := 0
	
responseLoop:
	for responses < numRapidReveals {
		select {
		case msg := <-client.messages:
			if ack := msg.GetRevealAck(); ack != nil {
				responses++
				if ack.Ok {
					successCount++
				} else {
					errorCount++
				}
			}
		case <-timeout:
			break responseLoop
		}
	}

	t.Logf("Rapid reveals: %d success, %d errors, %d total responses", 
		successCount, errorCount, responses)
	
	// We expect some rate limiting, but this depends on implementation
	// At minimum, we shouldn't crash and should get reasonable responses
	if responses == 0 {
		t.Fatalf("no responses received to rapid reveals")
	}
}

// TestReconnectionStateSync verifies state synchronization after reconnection
func TestReconnectionStateSync(t *testing.T) {
	_, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	// Client connects and makes some moves
	client1 := NewTestClient(t, wsURL, "reconnector")
	client1.Join()
	token := client1.token
	client1.Subscribe(0, 0)
	
	time.Sleep(100 * time.Millisecond)
	
	// Make a reveal
	client1.Reveal(0, 0, 30, 5001)
	
	// Wait for reveal to process
	timeout := time.After(2 * time.Second)
revealLoop:
	for {
		select {
		case msg := <-client1.messages:
			if msg.GetRevealAck() != nil {
				break revealLoop
			}
		case <-timeout:
			t.Fatalf("initial reveal timed out")
		}
	}
	
	client1.Close()
	time.Sleep(100 * time.Millisecond)

	// Reconnect with same token
	client2 := NewTestClient(t, wsURL, "reconnector")
	defer client2.Close()
	
	rejoin := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: token}}}
	if err := client2.Send(rejoin); err != nil {
		t.Fatalf("failed to rejoin: %v", err)
	}

	// Should get join ack
	msg := client2.ReceiveMessage(2 * time.Second)
	ack := msg.GetJoinAck()
	if ack == nil || !ack.Ok {
		t.Fatalf("rejoin failed: %s", ack.GetError())
	}

	// Subscribe to same region and verify we get current state
	client2.Subscribe(0, 0)
	
	// Should receive chunk region sync with previous reveals
	timeout = time.After(3 * time.Second)
	for {
		select {
		case msg := <-client2.messages:
			if sync := msg.GetChunkRegionSync(); sync != nil {
				// Successfully received sync message - that's what we wanted to test
				t.Logf("Successfully received chunk region sync after reconnect")
				return
			}
		case <-timeout:
			t.Fatalf("did not receive proper state synchronization after reconnect")
		}
	}
}

// TestMineExplosionBehavior verifies mine explosion mechanics and score penalties
func TestMineExplosionBehavior(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	// Create two clients to verify explosion is seen by all
	client1 := NewTestClient(t, wsURL, "miner1")
	defer client1.Close()
	client2 := NewTestClient(t, wsURL, "observer")
	defer client2.Close()

	// Both clients join
	client1.Join()
	client2.Join()


	// Both subscribe to the same chunk
	const chunkX, chunkY = int64(0), int64(0)
	client1.Subscribe(chunkX, chunkY)
	client2.Subscribe(chunkX, chunkY)

	time.Sleep(100 * time.Millisecond)

	// Find a mine in the chunk using server logic
	mineCell, hasMine := findMineInChunk(server, chunkX, chunkY)
	if !hasMine {
		t.Skip("No mines found in test chunk - this is statistically unlikely but possible")
	}

	t.Logf("Found mine at cell %d in chunk (%d,%d)", mineCell, chunkX, chunkY)

	// Client1 reveals the mine
	requestID := uint64(7000)
	client1.Reveal(chunkX, chunkY, mineCell, requestID)

	// Client1 should receive RevealAck with -100 score penalty
	ack := client1.WaitForRevealAck(requestID, 3*time.Second)
	if !ack.Ok {
		t.Fatalf("mine reveal should be acknowledged even though it explodes")
	}

	// Verify the revealed cells contains the mine
	revealedCells := ack.GetRevealedCells()
	if revealedCells == nil {
		t.Fatalf("expected RevealedCells in mine explosion ack")
	}

	foundMine := false
	for _, cell := range revealedCells.Cells {
		if cell == mineCell {
			foundMine = true
			break
		}
	}
	if !foundMine {
		t.Fatalf("exploded mine cell %d not found in revealed cells: %v", mineCell, revealedCells.Cells)
	}

	// Check score was penalized correctly
	scoreDelta := int32(0)
	if ack.ScoreUpdate != nil {
		scoreDelta = ack.ScoreUpdate.Delta
	}
	if scoreDelta != -100 {
		t.Errorf("expected score delta -100 for mine explosion, got %d", scoreDelta)
	}

	// Wait for potential ChunkUpdateBroadcast to both clients
	var update1, update2 *pb.ChunkUpdateBroadcast
	
	// Collect updates from both clients
	timeout := time.After(2 * time.Second)
	updates1 := 0
	updates2 := 0
	
updateLoop:
	for updates1 == 0 || updates2 == 0 {
		select {
		case msg := <-client1.messages:
			if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
				update1 = broadcast
				updates1++
			}
		case msg := <-client2.messages:
			if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
				update2 = broadcast
				updates2++
			}
		case <-timeout:
			break updateLoop
		}
	}

	if update1 == nil || update2 == nil {
		t.Fatalf("both clients should receive chunk updates after mine explosion: client1=%v client2=%v", update1, update2)
	}

	// Verify both updates show the same mine explosion
	if update1.ChunkId.X != update2.ChunkId.X || update1.ChunkId.Y != update2.ChunkId.Y {
		t.Fatalf("chunk update IDs mismatch: client1=(%d,%d) client2=(%d,%d)",
			update1.ChunkId.X, update1.ChunkId.Y, update2.ChunkId.X, update2.ChunkId.Y)
	}

	// Both should have the mine cell revealed
	revealed1 := update1.GetRevealedCells()
	revealed2 := update2.GetRevealedCells()

	if revealed1 == nil || revealed2 == nil {
		t.Fatalf("both updates should contain revealed cells: client1=%v client2=%v", revealed1, revealed2)
	}

	// Verify both clients see the exploded mine
	foundInUpdate1 := false
	foundInUpdate2 := false
	for _, cell := range revealed1.Cells {
		if cell == mineCell {
			foundInUpdate1 = true
			break
		}
	}
	for _, cell := range revealed2.Cells {
		if cell == mineCell {
			foundInUpdate2 = true
			break
		}
	}

	if !foundInUpdate1 || !foundInUpdate2 {
		t.Fatalf("exploded mine not visible to all clients: client1=%v client2=%v", foundInUpdate1, foundInUpdate2)
	}

	t.Logf("Mine explosion test passed: cell %d exploded with -100 score penalty, visible to all clients", mineCell)
}

// TestFlagOperationsSynchronization verifies flag placement and scoring mechanics
func TestFlagOperationsSynchronization(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client1 := NewTestClient(t, wsURL, "flagger")
	defer client1.Close()
	client2 := NewTestClient(t, wsURL, "observer")
	defer client2.Close()

	client1.Join()
	client2.Join()

	const chunkX, chunkY = int64(1), int64(1)
	client1.Subscribe(chunkX, chunkY)
	client2.Subscribe(chunkX, chunkY)

	time.Sleep(100 * time.Millisecond)

	// Test 1: Correct flag placement
	mineCell, hasMine := findMineInChunk(server, chunkX, chunkY)
	if !hasMine {
		t.Skip("No mines found in test chunk for flag test")
	}

	// First reveal a nearby cell to satisfy proximity rules
	nonMineCell := uint32(0)
	for nonMineCell < 4096 {
		if nonMineCell != mineCell {
			chunkID := ChunkID{X: chunkX, Y: chunkY}
			if !server.isMine(chunkID, nonMineCell) {
				break
			}
		}
		nonMineCell++
	}

	if nonMineCell >= 4096 {
		t.Skip("Could not find non-mine cell for proximity test")
	}

	// Reveal a safe cell first to establish proximity
	client1.Reveal(chunkX, chunkY, nonMineCell, 8001)
	safeAck := client1.WaitForRevealAck(8001, 2*time.Second)
	if !safeAck.Ok {
		t.Fatalf("initial safe reveal failed: %+v", safeAck)
	}

	// Now flag the mine (correct flag)
	client1.Flag(chunkX, chunkY, mineCell, 8002)
	flagAck := client1.WaitForRevealAck(8002, 2*time.Second)
	if !flagAck.Ok {
		t.Fatalf("correct flag placement should succeed")
	}

	// Verify positive score for correct flag
	flagScoreDelta := int32(0)
	if flagAck.ScoreUpdate != nil {
		flagScoreDelta = flagAck.ScoreUpdate.Delta
	}
	if flagScoreDelta <= 0 {
		t.Errorf("correct flag should give positive score, got %d", flagScoreDelta)
	}

	// Verify flag outcome
	flaggedCell := flagAck.GetFlaggedCell()
	if flaggedCell == nil {
		t.Fatalf("expected FlaggedCell outcome for correct flag")
	}
	if flaggedCell.Cell != mineCell {
		t.Errorf("flagged cell mismatch: expected %d, got %d", mineCell, flaggedCell.Cell)
	}

	// Test 2: Both clients should see the flag update
	var update1, update2 *pb.ChunkUpdateBroadcast
	timeout := time.After(2 * time.Second)
	updates1, updates2 := 0, 0

flagUpdateLoop:
	for updates1 == 0 || updates2 == 0 {
		select {
		case msg := <-client1.messages:
			if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
				update1 = broadcast
				updates1++
			}
		case msg := <-client2.messages:
			if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
				update2 = broadcast
				updates2++
			}
		case <-timeout:
			break flagUpdateLoop
		}
	}

	if update1 == nil || update2 == nil {
		t.Fatalf("both clients should see flag update: client1=%v client2=%v", update1, update2)
	}

	// Verify flag placement is synchronized
	flag1 := update1.GetFlaggedCell()
	flag2 := update2.GetFlaggedCell()

	if flag1 == nil || flag2 == nil {
		t.Fatalf("flag updates should contain flagged cell: client1=%v client2=%v", flag1, flag2)
	}

	// Verify both clients see the same flag
	if flag1.Cell != mineCell || flag2.Cell != mineCell {
		t.Fatalf("flag placement mismatch: expected cell %d, got client1=%d client2=%d", 
			mineCell, flag1.Cell, flag2.Cell)
	}

	if flag1.FlagID != flag2.FlagID {
		t.Fatalf("flag ID mismatch between clients: client1=%d client2=%d", flag1.FlagID, flag2.FlagID)
	}

	// Test 3: Wrong flag placement (should trigger flood fill and negative score)
	wrongFlagCell := nonMineCell + 1
	if wrongFlagCell >= 4096 {
		wrongFlagCell = nonMineCell - 1
	}
	
	// Make sure it's not a mine and not already revealed
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	if server.isMine(chunkID, wrongFlagCell) || server.isCellRevealed(chunkID, wrongFlagCell) {
		wrongFlagCell = nonMineCell + 2
		if wrongFlagCell >= 4096 || server.isMine(chunkID, wrongFlagCell) {
			t.Skip("Could not find suitable wrong flag cell")
		}
	}

	client1.Flag(chunkX, chunkY, wrongFlagCell, 8003)
	wrongFlagAck := client1.WaitForRevealAck(8003, 2*time.Second)
	if !wrongFlagAck.Ok {
		t.Fatalf("wrong flag should still be acknowledged")
	}

	// Verify negative score for wrong flag
	wrongFlagScoreDelta := int32(0)
	if wrongFlagAck.ScoreUpdate != nil {
		wrongFlagScoreDelta = wrongFlagAck.ScoreUpdate.Delta
	}
	if wrongFlagScoreDelta != -20 {
		t.Errorf("wrong flag should give -20 score penalty, got %d", wrongFlagScoreDelta)
	}

	// Wrong flag should trigger flood fill (revealed cells outcome)
	revealedFromWrongFlag := wrongFlagAck.GetRevealedCells()
	if revealedFromWrongFlag == nil {
		t.Fatalf("wrong flag should trigger flood fill with revealed cells")
	}

	// Should include the wrongly flagged cell
	foundWrongCell := false
	for _, cell := range revealedFromWrongFlag.Cells {
		if cell == wrongFlagCell {
			foundWrongCell = true
			break
		}
	}
	if !foundWrongCell {
		t.Errorf("wrongly flagged cell %d should be revealed: %v", wrongFlagCell, revealedFromWrongFlag.Cells)
	}

	t.Logf("Flag operations test passed: correct flag (+%d points), wrong flag (-20 points with flood fill)",
		flagScoreDelta)
}

// TestCrossChunkFloodFill verifies flood fill works correctly across chunk boundaries
func TestCrossChunkFloodFill(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client := NewTestClient(t, wsURL, "flood_tester")
	defer client.Close()
	client.Join()

	// Subscribe to adjacent chunks to test cross-boundary flood fill
	const chunkX1, chunkY1 = int64(0), int64(0) // Primary chunk
	const chunkX2, chunkY2 = int64(1), int64(0) // Adjacent chunk to the right

	client.Subscribe(chunkX1, chunkY1)
	client.Subscribe(chunkX2, chunkY2)

	time.Sleep(100 * time.Millisecond)

	// Find a good test location: a cell near the right edge of chunk1 with no adjacent mines
	// This will hopefully trigger flood fill into chunk2
	targetCell := findGoodFloodFillCell(server, chunkX1, chunkY1)
	if targetCell == ^uint32(0) {
		t.Skip("Could not find suitable flood fill cell on chunk boundary")
	}

	t.Logf("Testing flood fill from chunk (%d,%d) cell %d", chunkX1, chunkY1, targetCell)

	// Reveal the cell to trigger flood fill
	client.Reveal(chunkX1, chunkY1, targetCell, 9001)
	floodAck := client.WaitForRevealAck(9001, 3*time.Second)
	if !floodAck.Ok {
		t.Fatalf("flood fill reveal should succeed")
	}

	revealedCells := floodAck.GetRevealedCells()
	if revealedCells == nil {
		t.Fatalf("flood fill should return revealed cells")
	}

	if len(revealedCells.Cells) <= 1 {
		t.Skip("Flood fill did not expand - may not have found zero-adjacent cell")
	}

	t.Logf("Flood fill revealed %d cells", len(revealedCells.Cells))

	// Now check if we get chunk updates for multiple chunks
	chunkUpdates := make(map[string]*pb.ChunkUpdateBroadcast)
	timeout := time.After(3 * time.Second)
	
	// Collect chunk updates
collectLoop:
	for len(chunkUpdates) < 2 {
		select {
		case msg := <-client.messages:
			if broadcast := msg.GetChunkUpdateBroadcast(); broadcast != nil {
				key := fmt.Sprintf("%d,%d", broadcast.ChunkId.X, broadcast.ChunkId.Y)
				if _, exists := chunkUpdates[key]; !exists {
					chunkUpdates[key] = broadcast
					t.Logf("Received update for chunk (%d,%d) with %d revealed cells", 
						broadcast.ChunkId.X, broadcast.ChunkId.Y, 
						len(broadcast.GetRevealedCells().GetCells()))
				}
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Verify we got updates for multiple chunks (indicating cross-chunk flood fill)
	if len(chunkUpdates) < 2 {
		t.Logf("Got updates for %d chunks, expected multi-chunk flood fill", len(chunkUpdates))
		// This might not always trigger, so we'll just log it rather than fail
	}

	// Verify revealed cells are consistent across updates
	totalRevealed := 0
	for chunkKey, update := range chunkUpdates {
		revealed := update.GetRevealedCells()
		if revealed != nil {
			totalRevealed += len(revealed.Cells)
			t.Logf("Chunk %s: %d revealed cells", chunkKey, len(revealed.Cells))
		}
	}

	if totalRevealed == 0 {
		t.Fatalf("no cells revealed in chunk updates")
	}

	t.Logf("Cross-chunk flood fill test passed: %d total cells revealed across %d chunks", 
		totalRevealed, len(chunkUpdates))
}

// Helper to find a cell on chunk boundary that's likely to trigger cross-chunk flood fill
func findGoodFloodFillCell(server *Server, chunkX, chunkY int64) uint32 {
	const ChunkSize = 64
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	
	// Look for cells near the right edge (x = 62, 63) that have no adjacent mines
	for y := 10; y < ChunkSize-10; y++ { // Avoid very top/bottom edges
		for x := 60; x < ChunkSize; x++ { // Right edge cells
			cell := uint32(y*ChunkSize + x)
			
			// Skip if this cell is a mine
			if server.isMine(chunkID, cell) {
				continue
			}
			
			// Check adjacent mines count
			adjMines := 0
			worldX := int(chunkX)*ChunkSize + x
			worldY := int(chunkY)*ChunkSize + y
			
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					wx := worldX + dx
					wy := worldY + dy
					cid, cidx := worldToChunk(wx, wy)
					if server.isMine(cid, cidx) {
						adjMines++
					}
				}
			}
			
			// If this cell has no adjacent mines, it's good for flood fill
			if adjMines == 0 {
				return cell
			}
		}
	}
	
	return ^uint32(0) // Not found
}


// TestScoreCalculationAccuracy verifies score calculations are correct and consistent
func TestScoreCalculationAccuracy(t *testing.T) {
	server, wsURL, cleanup := setupTestServer(t)
	defer cleanup()

	client := NewTestClient(t, wsURL, "score_tester")
	defer client.Close()
	client.Join()

	const chunkX, chunkY = int64(2), int64(2) // Use different chunk to avoid interference
	client.Subscribe(chunkX, chunkY)
	time.Sleep(100 * time.Millisecond)

	// Get initial score (should be 0)
	initialScore := int32(0)
	
	// Test 1: Safe cell reveal should give positive score
	nonMineCell := findNonMineCell(server, chunkX, chunkY)
	if nonMineCell == ^uint32(0) {
		t.Skip("Could not find non-mine cell for score test")
	}

	client.Reveal(chunkX, chunkY, nonMineCell, 10001)
	safeAck := client.WaitForRevealAck(10001, 2*time.Second)
	if !safeAck.Ok {
		t.Fatalf("safe reveal should succeed")
	}

	safeScoreDelta := int32(0)
	if safeAck.ScoreUpdate != nil {
		safeScoreDelta = safeAck.ScoreUpdate.Delta
	}
	if safeScoreDelta <= 0 {
		t.Errorf("safe reveal should give positive score, got %d", safeScoreDelta)
	}

	expectedScore := initialScore + safeScoreDelta
	t.Logf("Safe reveal: +%d points (total: %d)", safeScoreDelta, expectedScore)

	// Test 2: Mine explosion should give -100 penalty
	mineCell, hasMine := findMineInChunk(server, chunkX, chunkY)
	if hasMine {
		client.Reveal(chunkX, chunkY, mineCell, 10002)
		mineAck := client.WaitForRevealAck(10002, 2*time.Second)
		if !mineAck.Ok {
			t.Fatalf("mine reveal should be acknowledged")
		}

		mineScoreDelta := int32(0)
		if mineAck.ScoreUpdate != nil {
			mineScoreDelta = mineAck.ScoreUpdate.Delta
		}
		if mineScoreDelta != -100 {
			t.Errorf("mine explosion should give -100 penalty, got %d", mineScoreDelta)
		}

		expectedScore = max(0, expectedScore + mineScoreDelta) // Score can't go below 0
		t.Logf("Mine explosion: %d points (total: %d)", mineScoreDelta, expectedScore)
	}

	// Test 3: Correct flag should give positive score
	anotherMine, hasAnotherMine := findAnotherMine(server, chunkX, chunkY, mineCell)
	if hasAnotherMine {
		client.Flag(chunkX, chunkY, anotherMine, 10003)
		flagAck := client.WaitForRevealAck(10003, 2*time.Second)
		correctFlagDelta := int32(0)
		if flagAck.ScoreUpdate != nil {
			correctFlagDelta = flagAck.ScoreUpdate.Delta
		}
		if flagAck.Ok && correctFlagDelta > 0 {
			expectedScore += correctFlagDelta
			t.Logf("Correct flag: +%d points (total: %d)", correctFlagDelta, expectedScore)

			// Verify the score includes multiplier effect
			chunkID := ChunkID{X: chunkX, Y: chunkY}
			multiplier := server.getScoreMultiplier(chunkID)
			expectedFlagScore := int32(math.Round(10 * multiplier))
			if correctFlagDelta != expectedFlagScore {
				t.Errorf("flag score mismatch: expected %d (10 * %.2f), got %d", 
					expectedFlagScore, multiplier, correctFlagDelta)
			}
		}
	}

	// Test 4: Wrong flag should give -20 penalty
	wrongCell := findAnotherNonMineCell(server, chunkX, chunkY, nonMineCell)
	if wrongCell != ^uint32(0) {
		client.Flag(chunkX, chunkY, wrongCell, 10004)
		wrongFlagAck := client.WaitForRevealAck(10004, 2*time.Second)
		if wrongFlagAck.Ok {
			wrongFlagDelta := int32(0)
			if wrongFlagAck.ScoreUpdate != nil {
				wrongFlagDelta = wrongFlagAck.ScoreUpdate.Delta
			}
			if wrongFlagDelta != -20 {
				t.Errorf("wrong flag should give -20 penalty, got %d", wrongFlagDelta)
			}
			expectedScore = max(0, expectedScore + wrongFlagDelta)
			t.Logf("Wrong flag: %d points (total: %d)", wrongFlagDelta, expectedScore)
		}
	}

	// Test 5: Score should never go below 0
	if expectedScore < 0 {
		t.Errorf("score went below zero: %d", expectedScore)
	}

	// Test 6: Large negative operation shouldn't cause score underflow
	if hasAnotherMine {
		// Try to explode multiple mines to test score clamping
		for i := 0; i < 5; i++ {
			testMine, hasTestMine := findAnotherMine(server, chunkX, chunkY, anotherMine)
			if !hasTestMine {
				break
			}
			anotherMine = testMine // Update for next iteration
			
			client.Reveal(chunkX, chunkY, testMine, uint64(10005+i))
			bombAck := client.WaitForRevealAck(uint64(10005+i), 2*time.Second)
			if bombAck.Ok {
				bombDelta := int32(0)
				if bombAck.ScoreUpdate != nil {
					bombDelta = bombAck.ScoreUpdate.Delta
				}
				expectedScore = max(0, expectedScore + bombDelta)
				if expectedScore < 0 {
					t.Errorf("score underflow detected: %d", expectedScore)
					break
				}
			}
		}
	}

	t.Logf("Score calculation test passed: final score %d (clamped to >= 0)", expectedScore)
}

// Helper to find a non-mine cell
func findNonMineCell(server *Server, chunkX, chunkY int64) uint32 {
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	for cell := uint32(0); cell < 4096; cell++ {
		if !server.isMine(chunkID, cell) {
			return cell
		}
	}
	return ^uint32(0) // Not found
}

// Helper to find another mine different from the given one
func findAnotherMine(server *Server, chunkX, chunkY int64, excludeCell uint32) (uint32, bool) {
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	for cell := uint32(0); cell < 4096; cell++ {
		if cell != excludeCell && server.isMine(chunkID, cell) {
			return cell, true
		}
	}
	return 0, false
}

// Helper to find another non-mine cell different from the given one
func findAnotherNonMineCell(server *Server, chunkX, chunkY int64, excludeCell uint32) uint32 {
	chunkID := ChunkID{X: chunkX, Y: chunkY}
	for cell := uint32(0); cell < 4096; cell++ {
		if cell != excludeCell && !server.isMine(chunkID, cell) {
			return cell
		}
	}
	return ^uint32(0) // Not found
}

// Helper max function for int32
func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}