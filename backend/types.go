package main

import (
	"time"

	"github.com/gorilla/websocket"
)

type ChunkID struct {
	X, Y int32
}

type ChunkBits [64]uint64

type Reveal struct {
	ChunkID  ChunkID `json:"chunkId"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	PlayerID int32   `json:"playerId"`
}

type Flag struct {
	ChunkID  ChunkID `json:"chunkId"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	PlayerID int32   `json:"playerId"`
	FlagID   uint32  `json:"flagId"`
}

type Player struct {
	ID          int32
	Conn        *websocket.Conn
	Send        chan []byte
	Mailbox     chan func(*Player) // actor channel
	TokenBucket TokenBucket
	Name        string

	// Rate limiting
	FlagID            uint32
	RevealWindowStart time.Time
	RevealCount       int

	// Leaderboard version already sent (protected by mailbox now)
	LastLBVersion uint64

	// Suspicion metrics (for admin dashboards, nothing enforced server‑side)
	SusRevealOverflow int // # of extra reveals processed beyond MaxRevealsPerMin

	// Scoring
	FlagsInARow    int
	LastActionTime time.Time
	Score          int32

	// outbound-drop counter (closes WS after 32 consecutive drops)
	dropMisses int

	done chan struct{} // closed when player is fully removed
}

// TokenBucket for rate limiting seed requests (200/min)
type TokenBucket struct {
	tokens     int
	lastRefill time.Time
}
