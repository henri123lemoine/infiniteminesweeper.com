package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PlayerStats tracks various metrics used for achievements.
type PlayerStats struct {
	TotalPlaytime           time.Duration `json:"total_playtime" yaml:"total_playtime"`
	CellsRevealed           uint64        `json:"cells_revealed" yaml:"cells_revealed"`
	FlagsCorrect            uint64        `json:"flags_correct" yaml:"flags_correct"`
	SafeClickStreakCurrent  uint64        `json:"safe_click_streak_current" yaml:"safe_click_streak_current"`
	SafeClickStreakMax      uint64        `json:"safe_click_streak_max" yaml:"safe_click_streak_max"`
	FlagFreeStreakCurrent   uint64        `json:"flag_free_streak_current" yaml:"flag_free_streak_current"`
	FlagFreeStreakMax       uint64        `json:"flag_free_streak_max" yaml:"flag_free_streak_max"`
	FurthestChunkDistance   int64         `json:"furthest_chunk_distance" yaml:"furthest_chunk_distance"`
	ChunksCleared           uint64        `json:"chunks_cleared" yaml:"chunks_cleared"`
	DailyLeaderboardEntries uint64        `json:"daily_leaderboard_positions" yaml:"daily_leaderboard_positions"`
	MineExplosions          []time.Time   `json:"mine_explosions" yaml:"mine_explosions"`
	ConcurrentClicks        uint64        `json:"concurrent_clicks" yaml:"concurrent_clicks"`
}

// CoinTransaction records a change to a player's coin balance.
type CoinTransaction struct {
	Time   time.Time
	Amount int64
	Reason string
}

// PlayerCoins tracks a player's current balance and transaction history.
type PlayerCoins struct {
	Balance int64
	History []CoinTransaction
}

// AwardCoins adjusts the player's balance and logs the transaction.
func (p *Player) AwardCoins(amount int64, reason string) {
	p.Coins.Balance += amount
	p.Coins.History = append(p.Coins.History, CoinTransaction{
		Time:   time.Now(),
		Amount: amount,
		Reason: reason,
	})
}

// RecordCellReveal updates statistics for a safe cell reveal.
func (p *Player) RecordCellReveal() {
	p.Stats.CellsRevealed++
	p.Stats.SafeClickStreakCurrent++
	if p.Stats.SafeClickStreakCurrent > p.Stats.SafeClickStreakMax {
		p.Stats.SafeClickStreakMax = p.Stats.SafeClickStreakCurrent
	}
	p.Stats.FlagFreeStreakCurrent++
	if p.Stats.FlagFreeStreakCurrent > p.Stats.FlagFreeStreakMax {
		p.Stats.FlagFreeStreakMax = p.Stats.FlagFreeStreakCurrent
	}
}

// RecordFlagPlaced tracks a flag placement and resets the flag-free streak.
func (p *Player) RecordFlagPlaced(correct bool) {
	if correct {
		p.Stats.FlagsCorrect++
	}
	p.Stats.FlagFreeStreakCurrent = 0
}

// RecordMineExplosion tracks a mine hit and resets the safe-click streak.
func (p *Player) RecordMineExplosion() {
	p.Stats.MineExplosions = append(p.Stats.MineExplosions, time.Now())
	p.Stats.SafeClickStreakCurrent = 0
}

// AchievementLevel represents a tier within an achievement.
type AchievementLevel struct {
	Threshold uint64 `json:"threshold" yaml:"threshold"`
	Reward    int64  `json:"reward" yaml:"reward"`
}

// Achievement defines an achievement and its tiers.
type Achievement struct {
	ID          string             `json:"id" yaml:"id"`
	Name        string             `json:"name" yaml:"name"`
	Description string             `json:"description" yaml:"description"`
	Levels      []AchievementLevel `json:"levels" yaml:"levels"`
	Hidden      bool               `json:"hidden" yaml:"hidden"`
	AutoBan     bool               `json:"auto_ban" yaml:"auto_ban"`
}

// AchievementManager loads and stores achievement definitions.
type AchievementManager struct {
	Achievements map[string]Achievement
}

// LoadAchievements reads all JSON or YAML achievement definitions from a directory.
func LoadAchievements(dir string) (*AchievementManager, error) {
	mgr := &AchievementManager{Achievements: make(map[string]Achievement)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var a Achievement
		if ext == ".json" {
			err = json.Unmarshal(data, &a)
		} else {
			err = yaml.Unmarshal(data, &a)
		}
		if err != nil {
			return nil, err
		}
		if a.ID == "" {
			a.ID = strings.TrimSuffix(name, ext)
		}
		mgr.Achievements[a.ID] = a
	}
	return mgr, nil
}
