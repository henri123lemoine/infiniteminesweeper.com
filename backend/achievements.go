package main

import (
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// PlayerStats mirrors pb.PlayerStats; it's the authoritative, mutable
// server-side copy that advancement thresholds are evaluated against.
type PlayerStats struct {
	CellsRevealed         uint64
	CorrectFlags          uint64
	ChunksFounded         uint64
	ChunksCleared         uint64
	CurrentSafeStreak     uint32
	MaxSafeStreak         uint32
	FurthestChunkDistance uint32
	HighDensityReveals    uint64
}

func (p *PlayerStats) toPB() *pb.PlayerStats {
	if p == nil {
		return &pb.PlayerStats{}
	}
	return &pb.PlayerStats{
		CellsRevealed:         p.CellsRevealed,
		CorrectFlags:          p.CorrectFlags,
		ChunksFounded:         p.ChunksFounded,
		ChunksCleared:         p.ChunksCleared,
		CurrentSafeStreak:     p.CurrentSafeStreak,
		MaxSafeStreak:         p.MaxSafeStreak,
		FurthestChunkDistance: p.FurthestChunkDistance,
		HighDensityReveals:    p.HighDensityReveals,
	}
}

// AchievementDef declares one advancement: a stable ID, an optional flag
// shape reward (nil = badge only), and the predicate that decides whether a
// given PlayerStats snapshot has earned it.
type AchievementDef struct {
	ID          string
	Name        string
	RewardShape *flagShape // nil = badge only, no flag reward
	Check       func(*PlayerStats) bool
}

// flagShape groups the color variants of one flag design. Wire-level flag IDs
// are per COLOR VARIANT (161 numeric sprite IDs across 17 shapes), so unlocks
// operate on whole shapes and grant every variant at once. Variant ID lists
// come from the generated frontend/src/assets/spritesheet.json.
type flagShape struct {
	name     string
	cost     uint32
	variants []uint32
}

func rangeIDs(lo, hi uint32) []uint32 {
	ids := make([]uint32, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		ids = append(ids, i)
	}
	return ids
}

var (
	shapeRectangle        = &flagShape{"Rectangle Flag", 5, rangeIDs(1, 10)}
	shapeTriangle         = &flagShape{"Triangle Flag", 5, rangeIDs(11, 20)}
	shapeGuidon           = &flagShape{"Guidon Flag", 5, rangeIDs(82, 91)}
	shapeWavyTriangle     = &flagShape{"Wavy Triangle Flag", 30, rangeIDs(102, 111)}
	shapeWavyRectangle    = &flagShape{"Wavy Rectangle Flag", 50, rangeIDs(92, 101)}
	shapeWavyGuidon       = &flagShape{"Wavy Guidon Flag", 50, rangeIDs(112, 121)}
	shapeVeryWavyTriangle = &flagShape{"Very Wavy Triangle Flag", 120, rangeIDs(122, 131)}
	shapeVeryWavyGuidon   = &flagShape{"Very Wavy Guidon Flag", 150, rangeIDs(132, 141)}
	shapeDownTriangle     = &flagShape{"Down Triangle Flag", 250, rangeIDs(21, 30)}
	shapeDownRectangle    = &flagShape{"Down Rectangle Flag", 300, rangeIDs(142, 151)}
	shapeDownPointyRect   = &flagShape{"Down Pointy Rectangle Flag", 400, rangeIDs(152, 161)}
	shapeDownGuidon       = &flagShape{"Down Guidon Flag", 450, rangeIDs(31, 40)}
	shapeBlackDownGuidon  = &flagShape{"Black Down Guidon Flag", 750, []uint32{61}}
	shapeBrokenRectangle  = &flagShape{"Broken Rectangle Flag", 1000, rangeIDs(41, 50)}
	shapeBrokenGuidon     = &flagShape{"Broken Guidon Flag", 1500, rangeIDs(51, 60)}
	shapeDragonEye        = &flagShape{"Dragon Eye Flag", 3000, rangeIDs(62, 71)}
	shapeDragon           = &flagShape{"Dragon Flag", 10000, rangeIDs(72, 81)}
)

var allFlagShapes = []*flagShape{
	shapeRectangle, shapeTriangle, shapeGuidon,
	shapeWavyTriangle, shapeWavyRectangle, shapeWavyGuidon,
	shapeVeryWavyTriangle, shapeVeryWavyGuidon,
	shapeDownTriangle, shapeDownRectangle, shapeDownPointyRect, shapeDownGuidon,
	shapeBlackDownGuidon, shapeBrokenRectangle, shapeBrokenGuidon,
	shapeDragonEye, shapeDragon,
}

// defaultFlagID is the fallback for a locked or invalid flag ID at join.
// Sprite IDs start at 1, so 0 is not a valid fallback.
var defaultFlagID = shapeRectangle.variants[0]

// Cost <= 5 (rectangle/triangle/guidon in all colors) is the free starter
// set: color is a player's minimap identity, so new players need options.
const freeFlagCostCeiling = 5

// paidFlagIDs: variant IDs not available to a fresh player; membership here
// is what makes isFlagUnlockedLocked gate a flag at all.
var paidFlagIDs = func() map[uint32]bool {
	m := make(map[uint32]bool)
	for _, sh := range allFlagShapes {
		if sh.cost <= freeFlagCostCeiling {
			continue
		}
		for _, id := range sh.variants {
			m[id] = true
		}
	}
	return m
}()

// Each chain tracks one stat in ascending difficulty. Every paid shape is
// rewarded by at least one achievement, at roughly cost-matched difficulty.
var achievementDefs = []AchievementDef{
	// Chunks founded (new territory claimed)
	{ID: "first_foothold", Name: "First Foothold", RewardShape: shapeWavyTriangle, Check: func(p *PlayerStats) bool { return p.ChunksFounded >= 1 }},
	{ID: "homesteader", Name: "Homesteader", RewardShape: shapeWavyGuidon, Check: func(p *PlayerStats) bool { return p.ChunksFounded >= 15 }},
	{ID: "frontier_baron", Name: "Frontier Baron", RewardShape: shapeDownRectangle, Check: func(p *PlayerStats) bool { return p.ChunksFounded >= 75 }},
	{ID: "empire_builder", Name: "Empire Builder", RewardShape: shapeBrokenRectangle, Check: func(p *PlayerStats) bool { return p.ChunksFounded >= 400 }},
	{ID: "continental", Name: "Continental", RewardShape: shapeDragon, Check: func(p *PlayerStats) bool { return p.ChunksFounded >= 2000 }},

	// Furthest chunk distance from origin (exploration)
	{ID: "wanderer", Name: "Wanderer", RewardShape: shapeWavyRectangle, Check: func(p *PlayerStats) bool { return p.FurthestChunkDistance >= 30 }},
	{ID: "pioneer", Name: "Pioneer", RewardShape: shapeVeryWavyTriangle, Check: func(p *PlayerStats) bool { return p.FurthestChunkDistance >= 150 }},
	{ID: "pathfinder", Name: "Pathfinder", RewardShape: shapeDownGuidon, Check: func(p *PlayerStats) bool { return p.FurthestChunkDistance >= 750 }},
	{ID: "edge_of_the_map", Name: "Edge of the Map", RewardShape: shapeDragonEye, Check: func(p *PlayerStats) bool { return p.FurthestChunkDistance >= 3000 }},

	// Chunks fully cleared (all 4096 cells revealed)
	{ID: "land_surveyor", Name: "Land Surveyor", RewardShape: shapeVeryWavyGuidon, Check: func(p *PlayerStats) bool { return p.ChunksCleared >= 1 }},
	{ID: "territory_controller", Name: "Territory Controller", RewardShape: shapeBlackDownGuidon, Check: func(p *PlayerStats) bool { return p.ChunksCleared >= 15 }},
	{ID: "regional_commander", Name: "Regional Commander", RewardShape: shapeBrokenGuidon, Check: func(p *PlayerStats) bool { return p.ChunksCleared >= 75 }},

	// Max consecutive mistake-free actions (skill/streak)
	{ID: "steady_hand", Name: "Steady Hand", RewardShape: shapeDownTriangle, Check: func(p *PlayerStats) bool { return p.MaxSafeStreak >= 150 }},
	{ID: "master_sweeper", Name: "Master Sweeper", RewardShape: shapeDownPointyRect, Check: func(p *PlayerStats) bool { return p.MaxSafeStreak >= 1500 }},
	{ID: "nerves_of_steel", Name: "Nerves of Steel", RewardShape: shapeDragonEye, Check: func(p *PlayerStats) bool { return p.MaxSafeStreak >= 7500 }},

	// Correct mine flags placed
	{ID: "deminer", Name: "Deminer", RewardShape: shapeWavyTriangle, Check: func(p *PlayerStats) bool { return p.CorrectFlags >= 250 }},
	{ID: "bomb_specialist", Name: "Bomb Specialist", RewardShape: shapeBrokenRectangle, Check: func(p *PlayerStats) bool { return p.CorrectFlags >= 2500 }},

	// Cells revealed in high-density (>=32%) chunks
	{ID: "into_the_fire", Name: "Into the Fire", RewardShape: shapeWavyGuidon, Check: func(p *PlayerStats) bool { return p.HighDensityReveals >= 100 }},
	{ID: "danger_seeker", Name: "Danger Seeker", RewardShape: shapeBrokenGuidon, Check: func(p *PlayerStats) bool { return p.HighDensityReveals >= 1000 }},

	// Total cells revealed (badge-only progression; capped off by a reward)
	{ID: "explorer", Name: "Explorer", Check: func(p *PlayerStats) bool { return p.CellsRevealed >= 2500 }},
	{ID: "cartographer", Name: "Cartographer", Check: func(p *PlayerStats) bool { return p.CellsRevealed >= 50000 }},
	{ID: "world_mapper", Name: "World Mapper", RewardShape: shapeDragon, Check: func(p *PlayerStats) bool { return p.CellsRevealed >= 500000 }},

	// rising_star / top_of_the_world skipped: rank triggers need re-evaluation
	// on other players' score changes, not just the actor's own stat deltas.

	// Meta: collect a spread of flag-rewarding achievements. Evaluated last so
	// it can see unlocks recorded earlier in the same evaluation pass.
	{ID: "collector", Name: "Collector", Check: nil}, // handled specially below
}

// bumpSafeStreakLocked records one more flawless action for pid, updating the
// running max. Caller must hold stateMu (write).
func (s *Server) bumpSafeStreakLocked(playerID uint32) {
	stats := s.statsForLocked(playerID)
	stats.CurrentSafeStreak++
	if stats.CurrentSafeStreak > stats.MaxSafeStreak {
		stats.MaxSafeStreak = stats.CurrentSafeStreak
	}
}

// resetSafeStreakLocked zeroes pid's current flawless-action streak (mine hit
// or wrong flag). Caller must hold stateMu (write).
func (s *Server) resetSafeStreakLocked(playerID uint32) {
	s.statsForLocked(playerID).CurrentSafeStreak = 0
}

// rebuildUnlockedFlagsLocked derives pid's unlocked flag-variant set from
// their unlocked achievements (unlockedFlags is never persisted directly).
// Caller must hold stateMu (write).
func (s *Server) rebuildUnlockedFlagsLocked(pid uint32) {
	unlocked := s.unlockedAdvancements[pid]
	var m map[uint32]bool
	for _, def := range achievementDefs {
		if def.RewardShape == nil || !unlocked[def.ID] {
			continue
		}
		if m == nil {
			m = make(map[uint32]bool)
		}
		for _, id := range def.RewardShape.variants {
			m[id] = true
		}
	}
	if m != nil {
		s.unlockedFlags[pid] = m
	} else {
		delete(s.unlockedFlags, pid)
	}
}

// isFlagUnlockedLocked: free flags are always usable, paid ones once
// unlocked; a currently-equipped flag stays usable (grandfathering).
func (s *Server) isFlagUnlockedLocked(pid, flagID uint32) bool {
	if !paidFlagIDs[flagID] {
		return true
	}
	if s.unlockedFlags[pid][flagID] {
		return true
	}
	return s.playerFlags[pid] == flagID
}

// buildAdvancementSyncLocked serializes the player's full advancement state,
// sent on join and after every new unlock. Caller must hold stateMu.
func (s *Server) buildAdvancementSyncLocked(playerID uint32) []byte {
	unlockedSet := s.unlockedAdvancements[playerID]
	unlockedIDs := make([]string, 0, len(unlockedSet))
	for id := range unlockedSet {
		unlockedIDs = append(unlockedIDs, id)
	}
	unlockedFlagSet := s.unlockedFlags[playerID]
	unlockedFlagIDs := make([]uint32, 0, len(unlockedFlagSet))
	for flagID := range unlockedFlagSet {
		unlockedFlagIDs = append(unlockedFlagIDs, flagID)
	}
	return mustProto(&pb.Msg{Payload: &pb.Msg_AdvancementSync{AdvancementSync: &pb.AdvancementSync{
		Stats:           s.playerStats[playerID].toPB(),
		UnlockedIds:     unlockedIDs,
		UnlockedFlagIds: unlockedFlagIDs,
	}}})
}

// evaluateAdvancementsLocked checks all achievement defs against the player's
// current stats, records any newly-crossed ones, and returns unlock messages
// for just the ones that are new this call. Caller must hold stateMu (write).
func (s *Server) evaluateAdvancementsLocked(playerID uint32) []*pb.AdvancementUnlocked {
	stats := s.playerStats[playerID]
	if stats == nil {
		return nil
	}

	unlocked := s.unlockedAdvancements[playerID]
	if unlocked == nil {
		unlocked = make(map[string]bool)
		s.unlockedAdvancements[playerID] = unlocked
	}

	var newUnlocks []*pb.AdvancementUnlocked

	unlock := func(id string, shape *flagShape) {
		if unlocked[id] {
			return
		}
		unlocked[id] = true
		var rewardFlagID uint32
		if shape != nil {
			if s.unlockedFlags[playerID] == nil {
				s.unlockedFlags[playerID] = make(map[uint32]bool)
			}
			// The representative (first) variant rides in the toast message.
			for _, id := range shape.variants {
				s.unlockedFlags[playerID][id] = true
			}
			rewardFlagID = shape.variants[0]
		}
		newUnlocks = append(newUnlocks, &pb.AdvancementUnlocked{Id: id, RewardFlagId: rewardFlagID})
	}

	for _, def := range achievementDefs {
		if def.Check == nil {
			continue // collector: evaluated after the main pass, below
		}
		if def.Check(stats) {
			unlock(def.ID, def.RewardShape)
		}
	}

	// collector: unlocked once >=10 flag-reward achievements are unlocked.
	const collectorThreshold = 10
	rewardUnlocks := 0
	for _, def := range achievementDefs {
		if def.RewardShape != nil && unlocked[def.ID] {
			rewardUnlocks++
		}
	}
	if rewardUnlocks >= collectorThreshold {
		unlock("collector", nil)
	}

	return newUnlocks
}
