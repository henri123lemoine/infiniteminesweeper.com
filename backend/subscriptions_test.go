package main

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestViewUpdateSubscriptionsAndLRU(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	// Shrink capacity to make eviction easy to observe
	srv.stateMu.Lock()
	srv.maxPlayerSubs = 40
	srv.stateMu.Unlock()

	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// handshake
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "SubTester"}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(join)); err != nil {
		t.Fatalf("write join: %v", err)
	}
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatalf("joinAck: %v", err)
	}
	// drain one leaderboard if present (best-effort)
	_ = func() error { _, _, err := c.ReadMessage(); return err }()

	// send a view covering 2x2 chunks → 6x6 rect with prefetch margin
	vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId: &pb.ChunkID{X: 0, Y: 0},
		Cell:    0, WidthCells: 128, HeightCells: 128,
	}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(vu)); err != nil {
		t.Fatalf("view1: %v", err)
	}

	// allow readPump to process
	time.Sleep(100 * time.Millisecond)

	// fetch the single playerID
	srv.playersMu.RLock()
	var pid uint32
	for k := range srv.players {
		pid = k
		break
	}
	srv.playersMu.RUnlock()
	if pid == 0 {
		t.Fatalf("no player registered")
	}

	srv.stateMu.RLock()
	subCount := len(srv.playerSubs[pid])
	srv.stateMu.RUnlock()
	if subCount == 0 || subCount > 36 {
		t.Fatalf("unexpected subs after view: %d (want 1..36)", subCount)
	}

	// Move view far away: 8x8 rect => 64 wanted but capacity=40, and all
	// old subs around (0,0) are outside the retention window
	vu2 := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId: &pb.ChunkID{X: 50, Y: -50},
		Cell:    0, WidthCells: 256, HeightCells: 256,
	}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(vu2)); err != nil {
		t.Fatalf("view2: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	srv.stateMu.RLock()
	subCount2 := len(srv.playerSubs[pid])
	staleFar := 0
	for cid := range srv.playerSubs[pid] {
		if cid.X < 30 {
			staleFar++
		}
	}
	srv.stateMu.RUnlock()
	if subCount2 == 0 || subCount2 > srv.maxPlayerSubs {
		t.Fatalf("LRU not enforced: subs=%d cap=%d", subCount2, srv.maxPlayerSubs)
	}
	if staleFar > 0 {
		t.Fatalf("far subs not pruned: %d chunks near old view remain", staleFar)
	}
}
