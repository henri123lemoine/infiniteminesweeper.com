package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAchievements(t *testing.T) {
	dir := t.TempDir()
	jsonData := `{"id":"playtime","name":"Boot Camp","levels":[{"threshold":60000,"reward":5}]}`
	if err := os.WriteFile(filepath.Join(dir, "playtime.json"), []byte(jsonData), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	yamlData := "id: butterfingers\nname: Butterfingers\nhidden: true\nlevels:\n - threshold: 3\n   reward: -10\n"
	if err := os.WriteFile(filepath.Join(dir, "butterfingers.yaml"), []byte(yamlData), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	mgr, err := LoadAchievements(dir)
	if err != nil {
		t.Fatalf("LoadAchievements: %v", err)
	}
	if len(mgr.Achievements) != 2 {
		t.Fatalf("expected 2 achievements, got %d", len(mgr.Achievements))
	}
	if mgr.Achievements["playtime"].Levels[0].Reward != 5 {
		t.Fatalf("unexpected reward: %v", mgr.Achievements["playtime"].Levels[0].Reward)
	}
	if !mgr.Achievements["butterfingers"].Hidden {
		t.Fatalf("expected butterfingers to be hidden")
	}
}

func TestAwardCoins(t *testing.T) {
	var p Player
	p.AwardCoins(10, "test")
	p.AwardCoins(-3, "spend")
	if p.Coins.Balance != 7 {
		t.Fatalf("expected balance 7, got %d", p.Coins.Balance)
	}
	if len(p.Coins.History) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(p.Coins.History))
	}
}
