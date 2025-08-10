package main

import (
	"math"
)

const (
	densityMean = 0.20
	sScale      = 0.20

	// Length scales in chunk units
	broadLenChunks = 20
	localLenChunks = 2.5

	// Weights
	aWeight = 0.6
	bWeight = 0.8
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

// getScoreMultiplier returns a small, controllable multiplier based on density (centered at 1.0).
func (s *Server) getScoreMultiplier(chunkID ChunkID) float64 {
	d := s.getChunkDensity(chunkID)
	// One-sigma above mean (~+0.0194) gives +k multiplier
	const k = 0.20 // conservative for now; tune via config if needed
	m := 1.0 + k*((d-densityMean)/sScale)
	if m < 0.75 {
		m = 0.75
	} else if m > 1.5 {
		m = 1.5
	}
	return m
}
