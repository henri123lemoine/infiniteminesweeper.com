package main

import "testing"

func TestDensityAndMultipliersRanges(t *testing.T) {
    s := NewServer()
    // probe several chunks
    samples := []ChunkID{{0,0}, {10,-5}, {-7,3}, {100,100}}
    for _, cid := range samples {
        d := s.getChunkDensity(cid)
        if d < 0 || d > 1 { t.Fatalf("density out of range: %v", d) }
        m := s.getBombDensityMultiplier(cid)
        if m < 0.2 || m > 10.0 { t.Fatalf("density multiplier out of range: %v", m) }
    }
}

func TestActivePlayerMultiplierIncreasesWithSubs(t *testing.T) {
    s := NewServer()
    cid := ChunkID{0,0}
    s.stateMu.Lock()
    s.subs[cid] = map[uint32]struct{}{1:{}}
    m1 := s.getActivePlayerMultiplier(cid)
    s.subs[cid][2] = struct{}{}
    m2 := s.getActivePlayerMultiplier(cid)
    s.stateMu.Unlock()
    if !(m2 >= m1) { t.Fatalf("expected multiplier to not decrease: %v -> %v", m1, m2) }
}

