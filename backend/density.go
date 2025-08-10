package main

import (
	"math"
)

const (
	// Bomb density field parameters (world generation)
	densityMean = 0.20
	sScale      = 0.20

	// Length scales in chunk units for bomb density noise
	broadLenChunks = 20
	localLenChunks = 2.5

	// Weights for bomb density noise
	aWeight = 0.6
	bWeight = 0.8

	// Active player scoring boost parameters
	// Radius in chunks around the action chunk to measure unique players
	activeRadiusChunks = 2 // looks at a 5x5 chunk window
	// Linear boost per unique nearby player (configured so 12 players -> 5x)
	activeBoostPerPlayer = (5.0 - 1.0) / 12.0
	// Cap the number of players contributing to the boost
	activeBoostPlayerCap = 12
	// Clamp active multiplier
	activeMinMultiplier = 1.0
	activeMaxMultiplier = 5.0
)

// Quintic fade 6t^5 - 15t^4 + 10t^3
func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

// gradAngle creates a deterministic gradient angle in [0,2π) from lattice (xi,yi) and seed
func gradAngle(xi, yi int64, seed uint64) float64 {
	const A = uint64(0x1F123BB5)
	const B = uint64(0x9E3779B1)
	const C = uint64(0x94D049BB)
	h := splitmix64(uint64(uint64(xi)*A + uint64(yi)*B + seed*C))
	const inv2p64 = 1.0 / 18446744073709551616.0 // 2^64
	return (float64(h) * inv2p64) * (2.0 * math.Pi)
}

func gradDot(xi, yi int64, x, y float64, seed uint64) float64 {
	a := gradAngle(xi, yi, seed)
	gx, gy := math.Cos(a), math.Sin(a)
	return gx*(x-float64(xi)) + gy*(y-float64(yi))
}

// gradientNoise2D: Perlin-style single-octave gradient noise with quintic interpolation
// Approximately zero-mean with range around [-1,1]
func gradientNoise2D(x, y, lengthScale float64, seed uint64) float64 {
	s := lengthScale
	xf := x / s
	yf := y / s

	x0 := math.Floor(xf)
	y0 := math.Floor(yf)
	x1 := x0 + 1
	y1 := y0 + 1
	tx := xf - x0
	ty := yf - y0
	u := fade(tx)
	v := fade(ty)

	n00 := gradDot(int64(x0), int64(y0), xf, yf, seed)
	n10 := gradDot(int64(x1), int64(y0), xf, yf, seed)
	n01 := gradDot(int64(x0), int64(y1), xf, yf, seed)
	n11 := gradDot(int64(x1), int64(y1), xf, yf, seed)

	nx0 := n00 + u*(n10-n00)
	nx1 := n01 + u*(n11-n01)
	return nx0 + v*(nx1-nx0)
}

// getChunkDensity returns the canonical per-chunk bomb density in [0,1]. Deterministic and cached.
func (s *Server) getChunkDensity(chunkID ChunkID) float64 {
	s.densityCacheMu.RLock()
	if d, ok := s.densityCache[chunkID]; ok {
		s.densityCacheMu.RUnlock()
		return d
	}
	s.densityCacheMu.RUnlock()

	cx := float64(chunkID.X)
	cy := float64(chunkID.Y)
	broad := gradientNoise2D(cx, cy, broadLenChunks, 42)
	local := gradientNoise2D(cx, cy, localLenChunks, 42^0xA5A5A5A5)

	// Combine independent unit-variance fields, then normalize to unit std
	combined := aWeight*broad + bWeight*local
	norm := math.Sqrt(aWeight*aWeight + bWeight*bWeight)
	if norm > 0 {
		combined /= norm
	}

	dens := densityMean + sScale*combined
	// Clamp to [0,1] for safety
	if dens < 0 {
		dens = 0
	} else if dens > 1 {
		dens = 1
	}

	s.densityCacheMu.Lock()
	s.densityCache[chunkID] = dens
	s.densityCacheMu.Unlock()
	return dens
}

// getActivePlayerMultiplier computes a score multiplier based on the density of
// active players (unique subscribers) within a small chunk radius around chunkID.
// Caller is expected to hold s.stateMu when calling this to ensure a consistent view.
func (s *Server) getActivePlayerMultiplier(chunkID ChunkID) float64 {
	// Collect unique player IDs across a (2*R+1)^2 chunk window
	unique := make(map[uint32]struct{})
	for dy := int64(-activeRadiusChunks); dy <= int64(activeRadiusChunks); dy++ {
		for dx := int64(-activeRadiusChunks); dx <= int64(activeRadiusChunks); dx++ {
			cid := ChunkID{X: chunkID.X + dx, Y: chunkID.Y + dy}
			if subs, ok := s.subs[cid]; ok {
				for pid := range subs {
					unique[pid] = struct{}{}
				}
			}
		}
	}

	n := len(unique)
	if n <= 0 {
		return activeMinMultiplier
	}

	// Linear boost per unique player, capped and clamped.
	if n > activeBoostPlayerCap {
		n = activeBoostPlayerCap
	}
	m := activeMinMultiplier + activeBoostPerPlayer*float64(n)
	if m < activeMinMultiplier {
		m = activeMinMultiplier
	}
	if m > activeMaxMultiplier {
		m = activeMaxMultiplier
	}
	return m
}

// getBombDensityMultiplier implements the density-based score multiplier as defined in
// scripts/python/score_multiplier.py. It maps density percent in [10,50] to [0.2x,10x]
// using a quintic smoothstep of t^q, with 1.0x at 21%.
func (s *Server) getBombDensityMultiplier(chunkID ChunkID) float64 {
	d := s.getChunkDensity(chunkID) // [0,1]
	dPct := d * 100.0
	const (
		dLo = 10.0
		dHi = 50.0
		yLo = 0.2
		yHi = 10.0
		q   = 1.1453726926484111 // chosen so f(21%) == 1.0
	)
	if dPct <= dLo {
		return yLo
	}
	if dPct >= dHi {
		return yHi
	}
	// Normalize to [0,1]
	t := (dPct - dLo) / (dHi - dLo)
	// s = smoothstep5(t^q) where smoothstep5 is 6t^5 - 15t^4 + 10t^3
	sVal := fade(math.Pow(t, q))
	return yLo + (yHi-yLo)*sVal
}

func (s *Server) getScoreMultiplier(chunkID ChunkID) float64 {
	active := s.getActivePlayerMultiplier(chunkID)
	dens := s.getBombDensityMultiplier(chunkID)
	return active * dens
}
