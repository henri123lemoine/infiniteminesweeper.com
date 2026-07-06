package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHotspot(t *testing.T) {
	s := NewServer()
	s.stateMu.Lock()
	s.subs[ChunkID{X: 3, Y: -2}] = map[uint32]struct{}{1: {}, 2: {}}
	s.subs[ChunkID{X: 10, Y: 0}] = map[uint32]struct{}{1: {}}
	s.stateMu.Unlock()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hotspot", nil)
	s.handleHotspot(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp struct {
		X, Y  int64
		Count int
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.X != 3 || resp.Y != -2 || resp.Count != 2 {
		t.Fatalf("got %+v; want X=3 Y=-2 Count=2", resp)
	}
}

func TestHandleLeaderboardHTTP(t *testing.T) {
	s := NewServer()
	s.stateMu.Lock()
	s.scores[1] = 100
	s.playerNames[1] = "A"
	s.playerFlags[1] = 1
	s.scores[2] = 300
	s.playerNames[2] = "B"
	s.playerFlags[2] = 2
	s.scores[3] = 200
	s.playerNames[3] = "A" // duplicate name with worse score; should be deduped
	s.playerFlags[3] = 3
	s.stateMu.Unlock()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	s.handleLeaderboardHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var out []struct {
		Name   string
		Score  int32
		FlagID uint32
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].Name != "B" || out[0].Score != 300 {
		t.Fatalf("first=%+v", out[0])
	}
	// Duplicate name "A" should keep the higher of 100 and 200 => 200
	if out[1].Name != "A" || out[1].Score != 200 {
		t.Fatalf("second=%+v", out[1])
	}
}

func TestHandleProfileUpdate(t *testing.T) {
	s := NewServer()
	// seed a player + token
	s.stateMu.Lock()
	pid := s.nextPlayerID
	s.nextPlayerID++
	token := "tok123"
	s.sessionTokens[token] = pid
	s.playerNames[pid] = "Old"
	s.playerFlags[pid] = 5
	s.scores[pid] = 42
	s.stateMu.Unlock()

	body := bytes.NewBuffer(nil)
	// flag 15 is a free starter-tier (Triangle) variant; paid flags are now
	// gated behind advancements and would be silently clamped here.
	io.WriteString(body, `{"name":"New","flagID":15}`)
	req := httptest.NewRequest(http.MethodPost, "/profile/update", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Token", token)
	rr := httptest.NewRecorder()
	s.handleProfileUpdate(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp struct {
		OK       bool   `json:"ok"`
		Name     string `json:"name"`
		FlagID   uint32 `json:"flagID"`
		Score    int32  `json:"score"`
		PlayerID uint32 `json:"playerID"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !resp.OK || resp.Name != "New" || resp.FlagID != 15 || resp.Score != 42 || resp.PlayerID != pid {
		t.Fatalf("bad resp: %+v", resp)
	}
}
