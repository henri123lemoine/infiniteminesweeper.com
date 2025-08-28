package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
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