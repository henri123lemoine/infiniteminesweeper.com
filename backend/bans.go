package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

const (
	bansFileName       = "bans.txt"
	autoBansFileName   = "bans-auto.txt"
	bansReloadInterval = 15 * time.Second
	banCookieName      = "ims_b"
)

// banList is immutable once published; swap the whole pointer to change it.
type banList struct {
	players  map[uint32]struct{}
	prefixes []netip.Prefix
}

func (b *banList) bannedPlayer(id uint32) bool {
	if b == nil {
		return false
	}
	_, ok := b.players[id]
	return ok
}

func (b *banList) matchIP(ip netip.Addr) (netip.Prefix, bool) {
	if b == nil || !ip.IsValid() {
		return netip.Prefix{}, false
	}
	ip = ip.Unmap()
	for _, p := range b.prefixes {
		if p.Contains(ip) {
			return p, true
		}
	}
	return netip.Prefix{}, false
}

func (b *banList) bannedIP(ip netip.Addr) bool {
	_, ok := b.matchIP(ip)
	return ok
}

// stillBanned reports whether a ban cookie's entry is still listed, so
// deleting a line from the ban files also lifts the cookie.
func (b *banList) stillBanned(entry string) bool {
	if b == nil {
		return false
	}
	if id, ok := strings.CutPrefix(entry, "p"); ok {
		n, err := strconv.ParseUint(id, 10, 32)
		return err == nil && b.bannedPlayer(uint32(n))
	}
	p, err := netip.ParsePrefix(entry)
	if err != nil {
		return false
	}
	for _, q := range b.prefixes {
		if q == p {
			return true
		}
	}
	return false
}

// parseBanList reads one entry per line: a player ID, an IP, or a CIDR.
// Anything after '#' is a comment.
func parseBanList(r io.Reader, into *banList) error {
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if id, err := strconv.ParseUint(line, 10, 32); err == nil {
			into.players[uint32(id)] = struct{}{}
		} else if p, err := netip.ParsePrefix(line); err == nil {
			into.prefixes = append(into.prefixes, p.Masked())
		} else if a, err := netip.ParseAddr(line); err == nil {
			a = a.Unmap()
			into.prefixes = append(into.prefixes, netip.PrefixFrom(a, a.BitLen()))
		} else {
			log.Printf("[bans] line %d: ignoring unrecognized entry %q", n, line)
		}
	}
	return sc.Err()
}

func newBanList() *banList { return &banList{players: make(map[uint32]struct{})} }

func (s *Server) bans() *banList { return s.banned.Load() }

func (s *Server) isPlayerBanned(id uint32) bool { return s.bans().bannedPlayer(id) }

// clientIP trusts Fly-Client-IP only on Fly, where the edge proxy sets it.
func clientIP(r *http.Request) netip.Addr {
	if os.Getenv("FLY_APP_NAME") != "" {
		if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("Fly-Client-IP"))); err == nil {
			return a.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

func (s *Server) loadBans() error {
	s.banFileMu.Lock()
	defer s.banFileMu.Unlock()
	return s.loadBansLocked()
}

func (s *Server) loadBansLocked() error {
	b := newBanList()
	found := false
	for _, name := range []string{bansFileName, autoBansFileName} {
		f, err := os.Open(filepath.Join(s.dataDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		err = parseBanList(f, b)
		f.Close()
		if err != nil {
			return err
		}
		found = true
	}
	if !found {
		b = nil
	}
	s.banned.Store(b)
	return nil
}

// autoBanIP bans the address a banned identity just showed up from. IPv6
// clients rotate within their /64, so the whole /64 goes.
func (s *Server) autoBanIP(ip netip.Addr, reason string) {
	if !ip.IsValid() || ip.IsLoopback() {
		return
	}
	s.banFileMu.Lock()
	defer s.banFileMu.Unlock()
	if s.bans().bannedIP(ip) {
		return
	}
	bits := 32
	if ip.Is6() {
		bits = 64
	}
	p, _ := ip.Prefix(bits)
	line := fmt.Sprintf("%s # evaded %s, %s\n", p, reason, time.Now().UTC().Format(time.RFC3339))
	f, err := os.OpenFile(filepath.Join(s.dataDir, autoBansFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("[bans] auto-ban %s failed: %v", p, err)
		return
	}
	_, err = f.WriteString(line)
	f.Close()
	if err != nil {
		log.Printf("[bans] auto-ban %s failed: %v", p, err)
		return
	}
	log.Printf("[bans] auto-banned %s (evaded %s)", p, reason)
	if err := s.loadBansLocked(); err != nil {
		log.Printf("[bans] reload after auto-ban failed: %v", err)
	}
}

// checkBan returns the ban entry covering this request: a listed IP, or a
// ban cookie whose entry is still listed (which also bans the new IP).
func (s *Server) checkBan(r *http.Request) (entry string, banned bool) {
	b := s.bans()
	if b == nil {
		return "", false
	}
	ip := clientIP(r)
	if p, ok := b.matchIP(ip); ok {
		return p.String(), true
	}
	if c, err := r.Cookie(banCookieName); err == nil && b.stillBanned(c.Value) {
		s.autoBanIP(ip, c.Value)
		return c.Value, true
	}
	return "", false
}

func banCookie(entry string) *http.Cookie {
	return &http.Cookie{
		Name:     banCookieName,
		Value:    entry,
		Path:     "/",
		MaxAge:   10 * 365 * 24 * 3600,
		HttpOnly: true,
		Secure:   os.Getenv("FLY_APP_NAME") != "",
		SameSite: http.SameSiteLaxMode,
	}
}

// banGate serves the ban page on every path for banned visitors. /ws is
// passed through so handleWebSocket can tell a live client to reload.
func (s *Server) banGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			next.ServeHTTP(w, r)
			return
		}
		entry, banned := s.checkBan(r)
		if !banned {
			next.ServeHTTP(w, r)
			return
		}
		http.SetCookie(w, banCookie(entry))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, banPageHTML)
	})
}

// refuseBannedSocket upgrades just long enough to deliver Banned and the
// cookie, so an open game tab swaps itself for the ban page.
func (s *Server) refuseBannedSocket(w http.ResponseWriter, r *http.Request, entry string) {
	conn, err := s.upgrader.Upgrade(w, r, http.Header{"Set-Cookie": {banCookie(entry).String()}})
	if err != nil {
		return
	}
	_ = conn.WriteMessage(websocket.BinaryMessage, bannedMsg())
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "banned"), time.Now().Add(time.Second))
	conn.Close()
}

func bannedMsg() []byte {
	return mustProto(&pb.Msg{Payload: &pb.Msg_Banned{Banned: &pb.Banned{Html: banPageHTML}}})
}

// runBanReloader picks up edits to the ban files without a restart and kicks
// anyone the new list covers.
func (s *Server) runBanReloader() {
	var last string
	for {
		var sig strings.Builder
		for _, name := range []string{bansFileName, autoBansFileName} {
			if fi, err := os.Stat(filepath.Join(s.dataDir, name)); err == nil {
				fmt.Fprintf(&sig, "%s:%d:%d;", name, fi.ModTime().UnixNano(), fi.Size())
			}
		}
		if sig.String() != last {
			last = sig.String()
			if err := s.loadBans(); err != nil {
				log.Printf("[bans] reload failed, keeping previous list: %v", err)
			} else {
				b := s.bans()
				np, ni := 0, 0
				if b != nil {
					np, ni = len(b.players), len(b.prefixes)
				}
				log.Printf("[bans] loaded %d player IDs, %d IP ranges", np, ni)
				s.kickBanned()
			}
		}
		time.Sleep(bansReloadInterval)
	}
}

func (s *Server) kickBanned() {
	b := s.bans()
	if b == nil {
		return
	}
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for id, set := range s.players {
		for p := range set {
			if b.bannedPlayer(id) || b.bannedIP(p.IP) {
				log.Printf("[bans] kicking ID=%d IP=%s", id, p.IP)
				s.sendBannedAndClose(p)
			}
		}
	}
}

// sendBannedAndClose queues Banned and closes shortly after so the write
// pump can flush it.
func (s *Server) sendBannedAndClose(p *Player) {
	select {
	case p.Send <- bannedMsg():
	default:
	}
	time.AfterFunc(time.Second, func() {
		if p.Conn != nil {
			p.Conn.Close()
		}
	})
}

const banPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Banned</title>
<style>
  :root { color-scheme: dark; }
  html, body { height: 100%; margin: 0; }
  body {
    display: flex; align-items: center; justify-content: center;
    background: #14161a; color: #e8e8e8;
    font: 17px/1.55 system-ui, -apple-system, "Segoe UI", sans-serif;
    padding: 0 20px;
  }
  main { max-width: 34em; }
  h1 { font-size: 1.6em; margin: 0 0 .6em; }
  p { margin: 0 0 1em; color: #c4c4c4; }
</style>
</head>
<body>
<main>
  <h1>You've been banned</h1>
  <p>Infinite Minesweeper is a personal project. It runs on servers I pay for with my own money.</p>
  <p>Bots and automated play are not allowed. This account, browser, and network have been banned for using them.</p>
</main>
</body>
</html>
`
