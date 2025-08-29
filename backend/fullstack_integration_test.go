//go:build integration
// +build integration

package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// TestFullStackIntegration spins up a real server, connects 10 WebSocket
// clients, exercises contention + unique reveals, and checks that:
//
//   - exactly one client won the contention race
//   - every client succeeded on its unique cell
//   - the leaderboard has a single top scorer
func TestFullStackIntegration(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	const clients = 10
	conns := make([]*websocket.Conn, clients)

	// Phase 0: connect & handshake
	for i := 0; i < clients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns[i] = conn

		join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: fmt.Sprintf("p%d", i)}}}
		if err := conn.WriteMessage(websocket.BinaryMessage, encodeMsg(join)); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("joinAck %d: %v", i, err)
		}
		m, err := decodeMsg(data)
		if err != nil {
			t.Fatalf("decode joinAck %d: %v", i, err)
		}
		if m.GetJoinAck() == nil {
			t.Fatalf("unexpected joinAck %d", i)
		}

		// drain first leaderboard so it doesn't confuse later reads
		if _, err := readLeaderboard(conn, 2*time.Second); err != nil {
			t.Fatalf("initial lb %d: %v", i, err)
		}
	}

	// Phase 1: contention reveal – all hit (1,1) concurrently
	start := make(chan struct{})
	var wg sync.WaitGroup
	okFlags := make([]bool, clients)

	for idx, conn := range conns {
		wg.Add(1)
		go func(i int, c *websocket.Conn) {
			defer wg.Done()
			<-start
			reveal := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 1}}}
			if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(reveal)); err != nil {
				t.Errorf("write contention %d: %v", i, err)
				return
			}
			ok, err := waitForRevealAck(c, 2*time.Second)
			if err != nil {
				t.Errorf("ack contention %d: %v", i, err)
				return
			}
			okFlags[i] = ok
			t.Logf("contention[%d] ok=%v", i, ok)
		}(idx, conn)
	}
	close(start)
	wg.Wait()

	// exactly one winner expected
	contenders := 0
	for _, ok := range okFlags {
		if ok {
			contenders++
		}
	}
	if contenders != 1 {
		t.Fatalf("expected exactly 1 contention winner, got %d", contenders)
	}

	// Phase 2: each client reveals in its own chunk to avoid prior flood-fill overlap
	successes := 0
	for i, conn := range conns {
		chunk := &pb.ChunkID{X: int64(1 + i), Y: 0}
		cell := uint32(0)
		reveal := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{ChunkId: chunk, Cell: cell}}}
		if err := conn.WriteMessage(websocket.BinaryMessage, encodeMsg(reveal)); err != nil {
			t.Fatalf("write unique %d: %v", i, err)
		}
		ok, err := waitForRevealAck(conn, 2*time.Second)
		if err != nil {
			t.Fatalf("ack unique %d: %v", i, err)
		}
		if ok {
			successes++
		}
		t.Logf("unique[%d] chunk=(%d,%d) cell=%d ok=%v", i, chunk.X, chunk.Y, cell, ok)
	}

	if successes != clients {
		t.Fatalf("expected %d successful unique reveals, got %d", clients, successes)
	}

	// Phase 3: verify leaderboard (force immediate rebuild)
	srv.stateMu.Lock()
	srv.buildLeaderboardUnsafe()
	srv.stateMu.Unlock()

	srv.stateMu.RLock()
	var max int32
	for _, sc := range srv.scores {
		if sc > max {
			max = sc
		}
	}
	maxTied := 0
	for _, sc := range srv.scores {
		if sc == max {
			maxTied++
		}
	}
	srv.stateMu.RUnlock()

	t.Logf("max score=%d shared_by=%d", max, maxTied)
	if maxTied != 1 {
		t.Fatalf("expected a single top scorer, got %d", maxTied)
	}

	// cleanup
	for _, c := range conns {
		c.Close()
	}
}