package main

import (
	"time"

	"github.com/gorilla/websocket"
)

type ChunkID struct {
	X, Y int64
}

type ChunkBits [64]uint64

type Flag struct {
	FlagID uint32
}

type PlayerView struct {
	Chunk ChunkID
	Cell  uint32
	// Last region dimensions in chunks used for subscription window
	RectWChunks int
	RectHChunks int
}

type Player struct {
	ID          uint32
	Conn        *websocket.Conn
	Send        chan []byte
	Mailbox     chan func(*Player) // actor channel
	TokenBucket TokenBucket
	Name        string
	FlagID      uint32
	View        PlayerView

	// Leaderboard version already sent (protected by mailbox now)
	LastLBVersion uint64

	// Suspicion metrics (for admin dashboards, nothing enforced server‑side)
	SusRevealOverflow int // # of extra reveals processed beyond MaxRevealsPerMin

	// Scoring
	Score int32

	// outbound-drop counter (closes WS after 32 consecutive drops)
	dropMisses int

	done chan struct{} // closed when player is fully removed
}

// TokenBucket for rate limiting seed requests (200/min)
type TokenBucket struct {
	tokens     int
	lastRefill time.Time
}
