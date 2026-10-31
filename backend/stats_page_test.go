package main

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newStatsTestServer() *Server {
	s := NewServer()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.chunks[ChunkID{X: 0, Y: 0}] = overviewTestChunk(1)
	s.chunks[ChunkID{X: 3, Y: -4}] = overviewTestChunk(2)
	s.totalRevealed = 1234567
	s.playerNames[1], s.scores[1] = "alice", 500
	s.playerNames[2], s.scores[2] = "<script>alert(1)</script>", 900
	s.playerNames[3], s.scores[3] = "", 10_000
	s.flags[ChunkID{X: 0, Y: 0}] = chunkFlags{{Cell: 1}, {Cell: 2}}
	s.rebuildOverviewLocked()
	return s
}

func TestStatsPageRendersEscapedLeaderboardAndTotals(t *testing.T) {
	s := newStatsTestServer()
	rec := httptest.NewRecorder()
	s.handleStatsPage(rec, httptest.NewRequest("GET", "/stats/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"1,234,567", "&lt;script&gt;alert(1)&lt;/script&gt;", `src="/stats/world.png"`, "Farthest from the center"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(body, "<script>alert(1)") {
		t.Error("player name was not escaped")
	}
	if strings.Contains(body, "10,000") {
		t.Error("unnamed player appeared on the leaderboard")
	}
	if i, j := strings.Index(body, "&lt;script&gt;"), strings.Index(body, "alice"); i < 0 || j < 0 || i > j {
		t.Error("leaderboard is not sorted by score")
	}
	if got := rec.Header().Get("Cache-Control"); got != statsCacheControl {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestStatsWorldMapIsValidPNG(t *testing.T) {
	s := newStatsTestServer()
	rec := httptest.NewRecorder()
	s.handleStatsPage(rec, httptest.NewRequest("GET", "/stats/world.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("code = %d, type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() == 0 || b.Dx() > statsMapMaxSide || b.Dy() > statsMapMaxSide {
		t.Fatalf("map size %v", b)
	}
}

func TestStatsPageWithoutWorldOmitsMap(t *testing.T) {
	s := NewServer()
	rec := httptest.NewRecorder()
	s.handleStatsPage(rec, httptest.NewRequest("GET", "/stats/", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "world.png") {
		t.Fatalf("code = %d, map referenced on an empty world", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleStatsPage(rec, httptest.NewRequest("GET", "/stats/world.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("world.png code = %d, want 404", rec.Code)
	}
}

func TestFormatThousands(t *testing.T) {
	cases := map[any]string{0: "0", 999: "999", 1000: "1,000", int32(-1234567): "-1,234,567", uint64(73444025): "73,444,025"}
	for in, want := range cases {
		if got := formatThousands(in); got != want {
			t.Errorf("formatThousands(%v) = %q, want %q", in, got, want)
		}
	}
}
