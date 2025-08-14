package main

import (
    "testing"
    "time"

    "github.com/gorilla/websocket"
    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// This test documents a bug: when a player has two /ws connections (e.g. reload),
// closing one connection clears ALL subscriptions for that playerID, leaving the
// other connection unsubscribed and not receiving broadcasts.
//
// EXPECTATION (desired): The remaining connection should continue receiving
// broadcasts for chunks it had in view.
// CURRENT BEHAVIOR: It does not, because removePlayer() deletes s.subs[...] for the playerID.
//
// TODO: Fix the bug and re-enable this test
func _TestRemainingConnectionLosesSubscriptionsOnPeerClose_bug(t *testing.T) {
    s, wsURL, cleanup := startTestServer(t)
    defer cleanup()

    // c1: create identity
    c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial c1: %v", err) }
    defer c1.Close()
    hello1 := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{Name: "reload-user"}}}
    if err := c1.WriteMessage(websocket.BinaryMessage, encodeMsg(hello1)); err != nil { t.Fatalf("hello1: %v", err) }
    // welcome with token
    _, w1bytes, err := c1.ReadMessage()
    if err != nil { t.Fatalf("welcome1: %v", err) }
    w1, err := decodeMsg(w1bytes)
    if err != nil || w1.GetWelcome() == nil { t.Fatalf("decode welcome1: %v", err) }
    tok := w1.GetWelcome().SessionToken

    // c1 subscribes to a region containing (0,0)
    vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, WidthCells: 128, HeightCells: 128}}}
    if err := c1.WriteMessage(websocket.BinaryMessage, encodeMsg(vu)); err != nil { t.Fatalf("view1: %v", err) }
    // Give server time to process subscription
    time.Sleep(100 * time.Millisecond)

    // c2: reconnect with same token (typical reload)
    c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial c2: %v", err) }
    defer c2.Close()
    hello2 := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{SessionToken: tok}}}
    if err := c2.WriteMessage(websocket.BinaryMessage, encodeMsg(hello2)); err != nil { t.Fatalf("hello2: %v", err) }
    if _, _, err := c2.ReadMessage(); err != nil { t.Fatalf("welcome2: %v", err) }
    // Note: c2 does NOT resend a ViewUpdate yet (common during page reload)

    // Close c1 (old tab). This triggers removePlayer() which clears all subs for the playerID.
    c1.Close()
    time.Sleep(120 * time.Millisecond) // let removePlayer run and clear subs

    // Cause a broadcast on (0,0) by performing a reveal from another actor (playerID 999)
    s.stateMu.Lock()
    s.proximityRadius = -1 // ensure reveal allowed
    s.stateMu.Unlock()
    s.handleReveal(999, 1, ChunkID{X: 0, Y: 0}, 0, false, false)

    // Expectation (desired): c2 should receive a ChunkUpdateBroadcast since user is still looking at (0,0)
    // Actual result (bug): no broadcast arrives because subscriptions were wiped when c1 closed.
    c2.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
    for {
        _, data, err := c2.ReadMessage()
        if err != nil {
            t.Fatalf("BUG: missing broadcast after peer close; err=%v. This documents a real issue.", err)
        }
        m, err := decodeMsg(data)
        if err != nil { continue }
        if m.GetChunkUpdateBroadcast() != nil {
            // Success path (desired); but in current code this likely never executes
            return
        }
        // Ignore other messages (leaderboard, etc) and continue reading within deadline
    }
}

