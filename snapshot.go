package main

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func init() {
	// Register the concrete types once for gob so we don't have to do it each
	// encode().
	gob.Register(ChunkID{})
	gob.Register(ChunkBits{})
}

// PERSISTENCE CONSTANTS
const (
	// S3 configuration
	defaultBucketName = "infinite-minesweeper-snapshots"
	snapshotKey       = "snapshot.gob.gz"
	walKey            = "wal.log"

	// Timing
	walFlushInterval = 3 * time.Minute
	snapshotInterval = 30 * time.Minute
)

// WAL entry types
type WALEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "reveal" or "flag"
	Data      []byte    `json:"data"` // JSON encoded Reveal or Flag
	Sequence  uint64    `json:"sequence"`
}

// snapshotData is what actually gets serialized. Gob can handle maps with
// struct keys, so we keep the exact types.
type snapshotData struct {
	Chunks       map[ChunkID]*ChunkBits
	CellOwners   map[ChunkID]map[int]int32
	Flags        map[ChunkID]map[int]Flag
	Scores       map[int32]int32
	PlayerNames  map[int32]string
	PlayerColors map[int32]string
	NextPlayerID int32
}

func (s *Server) initAWS() error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"), // Default region, can be overridden by env
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %v", err)
	}

	s.s3Client = s3.New(sess)
	s.bucketName = os.Getenv("S3_BUCKET_NAME")
	if s.bucketName == "" {
		s.bucketName = defaultBucketName
	}

	log.Printf("Using S3 bucket: %s", s.bucketName)
	return nil
}

func (s *Server) writeWALEntry(entryType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("WAL: failed to marshal data: %v", err)
		return
	}

	s.walMutex.Lock()
	s.walSeq++
	entry := WALEntry{
		Timestamp: time.Now(),
		Type:      entryType,
		Data:      jsonData,
		Sequence:  s.walSeq,
	}
	s.walBuffer = append(s.walBuffer, entry)
	s.walMutex.Unlock()
}

func (s *Server) flushWAL() error {
	s.walMutex.Lock()
	if len(s.walBuffer) == 0 {
		s.walMutex.Unlock()
		return nil
	}

	entries := make([]WALEntry, len(s.walBuffer))
	copy(entries, s.walBuffer)
	s.walBuffer = s.walBuffer[:0] // Clear buffer
	s.walMutex.Unlock()

	// Read existing WAL from S3
	existingEntries, err := s.readWALFromS3()
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("failed to read existing WAL: %v", err)
	}

	// Append new entries
	allEntries := append(existingEntries, entries...)

	// Write back to S3
	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	for _, entry := range allEntries {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to encode WAL entry: %v", err)
		}
	}

	_, err = s.s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(walKey),
		Body:   strings.NewReader(buf.String()),
	})

	if err != nil {
		return fmt.Errorf("failed to upload WAL to S3: %v", err)
	}

	log.Printf("WAL: flushed %d entries to S3", len(entries))
	return nil
}

func (s *Server) readWALFromS3() ([]WALEntry, error) {
	result, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(walKey),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	var entries []WALEntry
	decoder := json.NewDecoder(result.Body)
	for {
		var entry WALEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to decode WAL entry: %v", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (s *Server) truncateWAL() error {
	// Simply delete the WAL file from S3 after successful snapshot
	_, err := s.s3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(walKey),
	})
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("failed to truncate WAL: %v", err)
	}
	return nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound")
}

func (s *Server) periodicWALFlush() {
	ticker := time.NewTicker(walFlushInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.flushWAL(); err != nil {
			log.Printf("[wal] flush error: %v", err)
		}
	}
}

func (s *Server) replayWAL() error {
	entries, err := s.readWALFromS3()
	if err != nil {
		if isS3NotFound(err) {
			return nil // No WAL to replay
		}
		return err
	}

	log.Printf("[wal] replaying %d WAL entries", len(entries))

	for _, entry := range entries {
		switch entry.Type {
		case "reveal":
			var reveal Reveal
			if err := json.Unmarshal(entry.Data, &reveal); err != nil {
				log.Printf("[wal] failed to unmarshal reveal: %v", err)
				continue
			}
			s.replayReveal(reveal)

		case "flag":
			var flag Flag
			if err := json.Unmarshal(entry.Data, &flag); err != nil {
				log.Printf("[wal] failed to unmarshal flag: %v", err)
				continue
			}
			s.replayFlag(flag)
		}

		// Update WAL sequence number
		if entry.Sequence > s.walSeq {
			s.walSeq = entry.Sequence
		}
	}

	return nil
}

func (s *Server) replayReveal(reveal Reveal) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Get or create chunk
	chunk, exists := s.chunks[reveal.ChunkID]
	if !exists {
		chunk = &ChunkBits{}
		s.chunks[reveal.ChunkID] = chunk
	}

	// Set the bit (mark as revealed)
	bitIndex := reveal.Y*ChunkSize + reveal.X
	wordIndex := bitIndex / 64
	bitOffset := bitIndex % 64
	chunk[wordIndex] |= 1 << bitOffset

	// Track who revealed it
	if s.cellOwners[reveal.ChunkID] == nil {
		s.cellOwners[reveal.ChunkID] = make(map[int]int32)
	}
	s.cellOwners[reveal.ChunkID][bitIndex] = reveal.PlayerID
}

func (s *Server) replayFlag(flag Flag) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	bitIndex := flag.Y*ChunkSize + flag.X
	if s.flags[flag.ChunkID] == nil {
		s.flags[flag.ChunkID] = make(map[int]Flag)
	}
	s.flags[flag.ChunkID][bitIndex] = flag
}

func (s *Server) initPersistence() {
	if err := s.initAWS(); err != nil {
		log.Fatalf("Failed to initialize AWS: %v", err)
	}

	if err := s.loadSnapshotFromS3(); err != nil {
		log.Printf("[snapshot] no previous snapshot loaded from S3: %v (starting fresh)", err)
	} else {
		log.Printf("[snapshot] loaded snapshot from S3")

		// Replay WAL entries after loading snapshot
		if err := s.replayWAL(); err != nil {
			log.Printf("[wal] failed to replay WAL: %v", err)
		} else {
			log.Printf("[wal] successfully replayed WAL")
		}
	}

	go s.periodicSnapshotLoop() // fire‑and‑forget
	go s.periodicWALFlush()     // flush WAL every 30 seconds
}

func (s *Server) saveSnapshotToS3() error {
	s.stateMu.RLock()
	data := snapshotData{
		Chunks:       s.chunks,
		CellOwners:   s.cellOwners,
		Flags:        s.flags,
		Scores:       s.scores,
		PlayerNames:  s.playerNames,
		PlayerColors: s.playerColors,
		NextPlayerID: s.nextPlayerID,
	}
	s.stateMu.RUnlock()

	// Create compressed snapshot in memory
	var buf strings.Builder
	gz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(&data); err != nil {
		return fmt.Errorf("failed to encode snapshot: %v", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %v", err)
	}

	// Upload to S3
	_, err := s.s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(snapshotKey),
		Body:   strings.NewReader(buf.String()),
	})
	if err != nil {
		return fmt.Errorf("failed to upload snapshot to S3: %v", err)
	}

	// Truncate WAL after successful snapshot
	if err := s.truncateWAL(); err != nil {
		log.Printf("[wal] failed to truncate WAL after snapshot: %v", err)
		// Don't fail the snapshot operation for this
	}

	return nil
}

func (s *Server) loadSnapshotFromS3() error {
	result, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(snapshotKey),
	})
	if err != nil {
		return fmt.Errorf("failed to download snapshot from S3: %v", err)
	}
	defer result.Body.Close()

	gz, err := gzip.NewReader(result.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	dec := gob.NewDecoder(gz)
	var data snapshotData
	if err := dec.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode snapshot: %v", err)
	}

	s.stateMu.Lock()
	s.chunks = data.Chunks
	s.cellOwners = data.CellOwners
	if data.Flags != nil {
		s.flags = data.Flags
	} else {
		s.flags = make(map[ChunkID]map[int]Flag)
	}
	s.scores = data.Scores
	if data.PlayerNames != nil {
		s.playerNames = data.PlayerNames
	} else {
		s.playerNames = make(map[int32]string)
	}
	if data.PlayerColors != nil {
		s.playerColors = data.PlayerColors
	} else {
		s.playerColors = make(map[int32]string)
	}
	if data.NextPlayerID != 0 {
		s.nextPlayerID = data.NextPlayerID
	}
	s.lbDirty = true // force rebuild of leaderboard on first tick
	s.stateMu.Unlock()
	return nil
}

func (s *Server) periodicSnapshotLoop() {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.saveSnapshotToS3(); err != nil {
			log.Printf("[snapshot] save error: %v", err)
		} else {
			log.Printf("[snapshot] saved to S3")
		}
	}
}
