package main

import "encoding/gob"

func init() {
	// Register the concrete types once for gob so we don't have to do it each
	// encode().
	gob.Register(ChunkID{})
	gob.Register(ChunkBits{})
}
