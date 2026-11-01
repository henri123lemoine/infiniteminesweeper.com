package main

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestTokenBucketRefillAndPartialTake(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newTokenBucket(rateLimit{capacity: 10, refillEvery: 100 * time.Millisecond})

	if got := b.takeUpTo(t0, 25); got != 10 {
		t.Fatalf("first take got %d, want 10", got)
	}
	if b.take(t0) {
		t.Fatal("empty bucket should refuse")
	}
	if got := b.takeUpTo(t0.Add(350*time.Millisecond), 10); got != 3 {
		t.Fatalf("after 350ms got %d, want 3", got)
	}
	// Leftover 50ms carries into the next refill.
	if !b.take(t0.Add(400 * time.Millisecond)) {
		t.Fatal("expected a token at 400ms")
	}
	// Long idle refills to capacity, never beyond.
	if got := b.takeUpTo(t0.Add(time.Hour), 100); got != 10 {
		t.Fatalf("after idle got %d, want 10", got)
	}
}

func dialAndJoin(t *testing.T, wsURL string, join *pb.Join) (*websocket.Conn, *pb.JoinAck) {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(&pb.Msg{Payload: &pb.Msg_Join{Join: join}})); err != nil {
		t.Fatalf("join: %v", err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for joinAck: %v", err)
		}
		if m, err := decodeMsg(data); err == nil && m.GetJoinAck() != nil {
			c.SetReadDeadline(time.Time{})
			return c, m.GetJoinAck()
		}
	}
}

func TestSecondLoginDisplacesFirst(t *testing.T) {
	s, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	c1, ack1 := dialAndJoin(t, wsURL, &pb.Join{Name: "solo-user"})
	defer c1.Close()
	c2, ack2 := dialAndJoin(t, wsURL, &pb.Join{SessionToken: ack1.SessionToken})
	defer c2.Close()
	if !ack2.Ok || ack2.Name != "solo-user" {
		t.Fatalf("second login should resume identity, got %+v", ack2)
	}

	c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, _, err := c1.ReadMessage()
		if err == nil {
			continue
		}
		if !websocket.IsCloseError(err, closeDisplacedCode) {
			t.Fatalf("first connection closed with %v, want code %d", err, closeDisplacedCode)
		}
		break
	}

	// The displaced connection's teardown must not wipe the new one's subs.
	time.Sleep(100 * time.Millisecond)
	vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{ChunkId: &pb.ChunkID{}, WidthCells: 128, HeightCells: 128}}}
	if err := c2.WriteMessage(websocket.BinaryMessage, encodeMsg(vu)); err != nil {
		t.Fatalf("view: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	s.playersMu.RLock()
	conns := 0
	for id, set := range s.players {
		if id < 1<<31 { // skip spectator IDs
			conns += len(set)
		}
	}
	s.playersMu.RUnlock()
	if conns != 1 {
		t.Fatalf("player has %d live connections, want 1", conns)
	}

	s.handleReveal(999, 1, ChunkID{}, 0, false, false)
	c2.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, data, err := c2.ReadMessage()
		if err != nil {
			t.Fatalf("expected broadcast on new connection: %v", err)
		}
		if m, err := decodeMsg(data); err == nil && m.GetChunkUpdateBroadcast() != nil {
			return
		}
	}
}

func TestRevealRateLimitRejectsExcess(t *testing.T) {
	_, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	c, _ := dialAndJoin(t, wsURL, &pb.Join{Name: "spammer"})
	defer c.Close()

	const sent = 60
	for i := range sent {
		r := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
			RequestId: uint64(i + 1),
			ChunkId:   &pb.ChunkID{X: int64(i), Y: 500},
			Cell:      0,
		}}}
		if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(r)); err != nil {
			t.Fatalf("reveal %d: %v", i, err)
		}
	}

	rejected := 0
	acks := 0
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for acks < sent {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read after %d acks: %v", acks, err)
		}
		m, err := decodeMsg(data)
		if err != nil || m.GetRevealAck() == nil {
			continue
		}
		acks++
		if ack := m.GetRevealAck(); !ack.Ok {
			if !ack.RateLimited {
				t.Fatalf("rejected ack %d missing rate_limited", ack.RequestId)
			}
			rejected++
		}
	}
	// A few tokens may refill while the burst is in flight.
	if rejected < 10 || rejected > 20 {
		t.Fatalf("rejected %d of %d reveals, want roughly %d", rejected, sent, sent-defaultActionLimit.capacity)
	}
}

func TestViewUpdateTrimmedToChunkBudget(t *testing.T) {
	s, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	budget := rateLimit{capacity: 500, refillEvery: time.Millisecond}
	s.chunkLimit = budget

	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Max-size view: 56x56 = 3136 chunks, far more than the burst.
	vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId: &pb.ChunkID{}, WidthCells: 40 * ChunkSize, HeightCells: 40 * ChunkSize,
	}}}
	subCount := func() int {
		s.stateMu.RLock()
		defer s.stateMu.RUnlock()
		for _, set := range s.playerSubs {
			return len(set)
		}
		return 0
	}

	if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(vu)); err != nil {
		t.Fatalf("view: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	first := subCount()
	if first < budget.capacity || first > budget.capacity+200 {
		t.Fatalf("first view subscribed %d chunks, want about %d", first, budget.capacity)
	}
	s.stateMu.RLock()
	for _, set := range s.playerSubs {
		if _, ok := set[ChunkID{}]; !ok {
			t.Error("center chunk should be loaded first")
		}
	}
	s.stateMu.RUnlock()

	// Resending the unchanged view, as the client's heartbeat does, finishes
	// the load as the bucket refills.
	for range 50 {
		if subCount() == 56*56 {
			return
		}
		if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(vu)); err != nil {
			t.Fatalf("view: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("view never finished loading: %d of %d chunks", subCount(), 56*56)
}

func TestRepeatedRateLimitingSuspends(t *testing.T) {
	s := NewServer()
	s.actionLimit = rateLimit{capacity: 1, refillEvery: time.Hour}
	s.suspension = suspensionPolicy{strikes: 3, resetAfter: time.Second, duration: time.Hour}
	t0 := time.Unix(1000, 0)

	if ok, _ := s.takeAction(1, t0); !ok {
		t.Fatal("first action should pass")
	}
	// A quiet gap between strikes resets the streak.
	s.takeAction(1, t0)
	s.takeAction(1, t0)
	if _, until := s.takeAction(1, t0.Add(2*time.Second)); !until.IsZero() {
		t.Fatal("streak should have reset after the gap")
	}
	s.takeAction(1, t0.Add(2*time.Second))
	_, until := s.takeAction(1, t0.Add(2*time.Second))
	if want := t0.Add(2*time.Second + time.Hour); !until.Equal(want) {
		t.Fatalf("suspended until %v, want %v", until, want)
	}
	if _, ok := s.suspendedUntil(1, t0.Add(30*time.Minute)); !ok {
		t.Fatal("should still be suspended")
	}
	if _, ok := s.suspendedUntil(1, t0.Add(2*time.Hour)); ok {
		t.Fatal("suspension should have expired")
	}
	if _, ok := s.suspendedUntil(2, t0); ok {
		t.Fatal("other players are unaffected")
	}
}

func TestSuspendedPlayerIsDisconnectedAndCannotRejoin(t *testing.T) {
	s, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	s.suspension = suspensionPolicy{strikes: 5, resetAfter: time.Minute, duration: time.Hour}

	c, ack := dialAndJoin(t, wsURL, &pb.Join{Name: "botter"})
	defer c.Close()
	for i := range defaultActionLimit.capacity + 10 {
		r := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
			RequestId: uint64(i + 1),
			ChunkId:   &pb.ChunkID{X: int64(i), Y: 500},
		}}}
		if err := c.WriteMessage(websocket.BinaryMessage, encodeMsg(r)); err != nil {
			break
		}
	}
	expectClose := func(c *websocket.Conn) {
		t.Helper()
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			_, _, err := c.ReadMessage()
			if err == nil {
				continue
			}
			if !websocket.IsCloseError(err, closeSuspendedCode) {
				t.Fatalf("closed with %v, want code %d", err, closeSuspendedCode)
			}
			return
		}
	}
	expectClose(c)

	c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c2.Close()
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: ack.SessionToken}}}
	if err := c2.WriteMessage(websocket.BinaryMessage, encodeMsg(join)); err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	expectClose(c2)
}

func TestActionBudgetReleasedOnceFresh(t *testing.T) {
	s := NewServer()
	s.actionLimit = rateLimit{capacity: 2, refillEvery: 100 * time.Millisecond}
	s.suspension = suspensionPolicy{strikes: 2, resetAfter: time.Second, duration: time.Hour}
	t0 := time.Unix(1000, 0)
	has := func(id uint32) bool {
		s.actionBucketsMu.Lock()
		defer s.actionBucketsMu.Unlock()
		return s.actionBuckets[id] != nil
	}

	s.takeAction(1, t0)
	s.releaseActionBudget(1, t0)
	if !has(1) {
		t.Fatal("a partly spent budget must survive disconnect")
	}
	s.releaseActionBudget(1, t0.Add(time.Second))
	if has(1) {
		t.Fatal("a refilled budget should be released")
	}

	for range 4 {
		s.takeAction(2, t0)
	}
	s.releaseActionBudget(2, t0.Add(30*time.Minute))
	if !has(2) {
		t.Fatal("a suspension must survive disconnect")
	}
	s.releaseActionBudget(2, t0.Add(2*time.Hour))
	if has(2) {
		t.Fatal("an expired suspension should be released")
	}
}

func TestActionBudgetReleaseRetriesAfterRefill(t *testing.T) {
	s := NewServer()
	s.actionLimit = rateLimit{capacity: 1, refillEvery: 20 * time.Millisecond}
	s.takeAction(1, time.Now())
	s.releaseActionBudget(1, time.Now())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.actionBucketsMu.Lock()
		gone := s.actionBuckets[1] == nil
		s.actionBucketsMu.Unlock()
		if gone {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("budget was never released after refilling")
}
