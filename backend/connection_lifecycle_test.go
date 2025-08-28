package main

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// This test verifies the fix for a connection lifecycle bug: when a player has
// two /ws connections (e.g. reload), closing one connection should NOT clear
// subscriptions for that playerID, leaving the remaining connection to continue
// receiving broadcasts for chunks it had in view.
func TestRemainingConnectionKeepsSubscriptionsOnPeerClose(t *testing.T) {
	s, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	// c1: create identity
	c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial c1: %v", err)
	}
	defer c1.Close()
	join1 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "reload-user"}}}
	if err := c1.WriteMessage(websocket.BinaryMessage, encodeMsg(join1)); err != nil {
		t.Fatalf("join1: %v", err)
	}
	// joinAck with token
	_, j1bytes, err := c1.ReadMessage()
	if err != nil {
		t.Fatalf("joinAck1: %v", err)
	}
	j1, err := decodeMsg(j1bytes)
	if err != nil || j1.GetJoinAck() == nil {
		t.Fatalf("decode joinAck1: %v", err)
	}
	tok := j1.GetJoinAck().SessionToken

	// c1 subscribes to a region containing (0,0)
	vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, WidthCells: 128, HeightCells: 128}}}
	if err := c1.WriteMessage(websocket.BinaryMessage, encodeMsg(vu)); err != nil {
		t.Fatalf("view1: %v", err)
	}
	// Give server time to process subscription
	time.Sleep(100 * time.Millisecond)

	// c2: reconnect with same token (typical reload)
	c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial c2: %v", err)
	}
	defer c2.Close()
	join2 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: tok}}}
	if err := c2.WriteMessage(websocket.BinaryMessage, encodeMsg(join2)); err != nil {
		t.Fatalf("join2: %v", err)
	}
	if _, _, err := c2.ReadMessage(); err != nil {
		t.Fatalf("joinAck2: %v", err)
	}
	// Note: c2 does NOT resend a ViewUpdate yet (common during page reload)

	// Close c1 (old tab). This triggers removePlayer() which clears all subs for the playerID.
	c1.Close()
	time.Sleep(120 * time.Millisecond) // let removePlayer run and clear subs

	// Cause a broadcast on (0,0) by performing a reveal from another actor (playerID 999)
	s.stateMu.Lock()
	s.proximityRadius = -1 // ensure reveal allowed
	s.stateMu.Unlock()
	s.handleReveal(999, 1, ChunkID{X: 0, Y: 0}, 0, false, false)

	// Expected behavior: c2 should receive a ChunkUpdateBroadcast since user is still looking at (0,0)
	// With the fix: subscriptions are preserved when other connections remain active.
	c2.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	for {
		_, data, err := c2.ReadMessage()
		if err != nil {
			t.Fatalf("Expected broadcast after peer close, but timed out: %v", err)
		}
		m, err := decodeMsg(data)
		if err != nil {
			continue
		}
		if m.GetChunkUpdateBroadcast() != nil {
			// Success! The fix works - subscriptions were preserved after peer close
			return
		}
		// Ignore other messages (leaderboard, etc) and continue reading within deadline
	}
}
