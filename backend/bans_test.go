package main

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestParseBanList(t *testing.T) {
	b := newBanList()
	err := parseBanList(strings.NewReader(`
# comment
1234   # example
203.0.113.7
2001:db8:1234::/48
not-an-entry
`), b)
	if err != nil {
		t.Fatal(err)
	}
	if !b.bannedPlayer(1234) || b.bannedPlayer(1235) {
		t.Fatalf("player bans wrong: %v", b.players)
	}
	cases := map[string]bool{
		"203.0.113.7":        true,
		"::ffff:203.0.113.7": true,
		"203.0.113.8":        false,
		"2001:db8:1234:5::1": true,
		"2001:db8:1235::1":   false,
	}
	for s, want := range cases {
		if got := b.bannedIP(netip.MustParseAddr(s)); got != want {
			t.Errorf("bannedIP(%s)=%v want %v", s, got, want)
		}
	}
	var nilList *banList
	if nilList.bannedPlayer(1) || nilList.bannedIP(netip.MustParseAddr("1.2.3.4")) || nilList.stillBanned("p1") {
		t.Fatal("nil list must ban nothing")
	}
}

func TestClientIPTrustsFlyHeaderOnlyOnFly(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("Fly-Client-IP", "203.0.113.7")

	t.Setenv("FLY_APP_NAME", "")
	if got := clientIP(r).String(); got != "10.0.0.1" {
		t.Fatalf("off Fly got %s, want RemoteAddr", got)
	}
	t.Setenv("FLY_APP_NAME", "infiniteminesweeper")
	if got := clientIP(r).String(); got != "203.0.113.7" {
		t.Fatalf("on Fly got %s, want Fly-Client-IP", got)
	}
}

func writeBans(t *testing.T, srv *Server, body string) {
	t.Helper()
	if srv.dataDir == "data" {
		srv.dataDir = t.TempDir()
	}
	if err := os.WriteFile(filepath.Join(srv.dataDir, bansFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := srv.loadBans(); err != nil {
		t.Fatal(err)
	}
}

func autoBans(t *testing.T, srv *Server) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(srv.dataDir, autoBansFileName))
	return string(b)
}

// joinAs returns the first JoinAck or Banned the server sends.
func joinAs(t *testing.T, wsURL string, hdr http.Header, token, name string) *pb.Msg {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: token, Name: name}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(join)); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if m := decodePBMsg(t, data); m.GetJoinAck() != nil || m.GetBanned() != nil {
			return m
		}
	}
}

func onFlyFrom(t *testing.T, ip string) http.Header {
	t.Setenv("FLY_APP_NAME", "test")
	return http.Header{"Fly-Client-IP": {ip}}
}

func TestBannedPlayerRejoinGetsBanPageAndIPBan(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	hdr := onFlyFrom(t, "203.0.113.9")

	first := joinAs(t, wsURL, hdr, "", "Tester").GetJoinAck()
	if first == nil || !first.Ok {
		t.Fatalf("initial join failed: %v", first)
	}
	srv.stateMu.RLock()
	pid := srv.sessionTokens[first.SessionToken]
	srv.stateMu.RUnlock()

	writeBans(t, srv, strconv.FormatUint(uint64(pid), 10)+"\n")
	m := joinAs(t, wsURL, hdr, first.SessionToken, "Tester")
	if m.GetBanned() == nil || !strings.Contains(m.GetBanned().Html, "personal project") {
		t.Fatalf("banned rejoin got %v", m)
	}
	if !strings.Contains(autoBans(t, srv), "203.0.113.9/32") {
		t.Fatalf("rejoin IP not auto-banned: %q", autoBans(t, srv))
	}
}

func TestBannedIPGetsBannedMessageAndCookie(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	writeBans(t, srv, "203.0.113.7\n")

	c, resp, err := websocket.DefaultDialer.Dial(wsURL, onFlyFrom(t, "203.0.113.7"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if !strings.Contains(resp.Header.Get("Set-Cookie"), banCookieName+"=203.0.113.7/32") {
		t.Fatalf("no ban cookie on upgrade: %q", resp.Header.Get("Set-Cookie"))
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if decodePBMsg(t, data).GetBanned() == nil {
		t.Fatal("first message was not Banned")
	}
}

func TestBanGate(t *testing.T) {
	srv := NewServer()
	srv.dataDir = t.TempDir()
	writeBans(t, srv, "42\n203.0.113.7\n")
	t.Setenv("FLY_APP_NAME", "test")
	gate := srv.banGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("game"))
	}))
	get := func(ip string, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/how-to-play/", nil)
		r.Header.Set("Fly-Client-IP", ip)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: banCookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, r)
		return w
	}

	if w := get("198.51.100.1", ""); w.Code != http.StatusOK || w.Body.String() != "game" {
		t.Fatalf("clean visitor blocked: %d", w.Code)
	}
	w := get("203.0.113.7", "")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Bots and automated play") {
		t.Fatalf("banned IP not shown ban page: %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), banCookieName+"=203.0.113.7/32") {
		t.Fatalf("ban page did not set cookie: %q", w.Header().Get("Set-Cookie"))
	}

	// A banned browser on a new network bans that network too, IPv6 by /64.
	if w := get("2001:db8:aa:bb::5", "p42"); w.Code != http.StatusForbidden {
		t.Fatalf("cookie evasion not blocked: %d", w.Code)
	}
	if !strings.Contains(autoBans(t, srv), "2001:db8:aa:bb::/64 # evaded p42") {
		t.Fatalf("evading IP not auto-banned: %q", autoBans(t, srv))
	}
	if w := get("2001:db8:aa:bb::99", ""); w.Code != http.StatusForbidden {
		t.Fatalf("auto-banned /64 not blocked: %d", w.Code)
	}

	// Forged or stale cookies ban nothing.
	if w := get("198.51.100.2", "p43"); w.Code != http.StatusOK {
		t.Fatalf("cookie for unlisted player blocked: %d", w.Code)
	}
	writeBans(t, srv, "203.0.113.7\n")
	if w := get("198.51.100.3", "p42"); w.Code != http.StatusOK {
		t.Fatalf("unbanned player's cookie still blocks: %d", w.Code)
	}
}

func TestBannedPlayerHiddenFromLeaderboards(t *testing.T) {
	srv := NewServer()
	srv.scores[1], srv.playerNames[1] = 1000, "cheater"
	srv.scores[2], srv.playerNames[2] = 10, "honest"
	writeBans(t, srv, "1\n")

	srv.stateMu.Lock()
	defer srv.stateMu.Unlock()
	for _, e := range srv.collectLeaderboardLocked() {
		if e.Name == "cheater" {
			t.Fatal("banned player on HTTP leaderboard")
		}
	}
	if r := srv.getUserRankUnsafe(10); r != 1 {
		t.Fatalf("rank=%d want 1 with the banned score excluded", r)
	}
}
