# Add Advancements System

## Overview

We need to implement a comprehensive achievements system to increase player engagement beyond just chasing high scores. This system will track various player actions and award coins that can be spent on cosmetic flag unlocks.

## Implementation Requirements

### Backend Infrastructure

- **Player Statistics Tracking**

  - Total playtime (milliseconds)
  - Cells revealed (individual clicks, not flood-fill cascades)
  - Flags placed correctly
  - Current safe-click streak (consecutive reveals without hitting mines)
  - Maximum safe-click streak achieved
  - Current flag-free streak (reveals without placing any flags)
  - Maximum flag-free streak achieved
  - Furthest chunk distance from origin (Manhattan distance)
  - Chunks fully cleared (all cells revealed or flagged)
  - Daily leaderboard positions
  - Mine explosion count and timestamps
  - Concurrent click detection (same cell clicked by multiple players within time window)

- **Achievement Definition System**

  - JSON/YAML configuration files for each achievement
  - Support for tiered achievements with multiple levels
  - Hidden achievements that don't appear until unlocked
  - Negative coin rewards for "punishment" achievements
  - Auto-ban triggers for certain achievements

- **Coin Economy**
  - Player coin balance (can go negative)
  - Transaction logging for coin awards
  - Integration with flag unlock system

### Frontend Components

- Achievements panel showing locked, in-progress, and completed achievements
- Progress bars for tiered achievements
- Toast notifications for unlocks
- Hidden achievements only visible after completion

## Achievement Categories

### Time-Based Progression

**Playtime Milestones**

- Boot Camp: 1 minute total (5 coins)
- Regular: 30 minutes (10 coins)
- Veteran: 2 hours (25 coins)
- Old Timer: 10 hours (60 coins)
- Mine Sweeping Legend: 50 hours (150 coins)

### Exploration & Discovery

**Cells Revealed**

- Scout: 100 cells (5 coins)
- Explorer: 1,000 cells (15 coins)
- Cartographer: 10,000 cells (40 coins)
- World Mapper: 100,000 cells (100 coins)
- Digital Columbus: 1,000,000 cells (250 coins)

**Flag Placement**

- Flag Bearer: 25 correct flags (5 coins)
- Demining Expert: 250 flags (20 coins)
- Bomb Specialist: 2,500 flags (50 coins)
- Mine Marshal: 25,000 flags (120 coins)

**Frontier Exploration**

- Wanderer: Reach distance 25 from origin (10 coins)
- Pioneer: Reach distance 100 (25 coins)
- Pathfinder: Reach distance 500 (75 coins)
- Edge Walker: Reach distance 1,000 (150 coins)

### Skill-Based Achievements

**Safe-Click Streaks**

- Steady Hand: 50 consecutive safe clicks (10 coins)
- Precision Player: 200 consecutive safe clicks (30 coins)
- Master Sweeper: 1,000 consecutive safe clicks (80 coins)
- Zen Master: 5,000 consecutive safe clicks (200 coins)

**Flag-Free Streaks**

- No Training Wheels: 100 reveals without placing flags (15 coins)
- Raw Instinct: 500 reveals without flags (50 coins)
- Flag Allergy: 2,000 reveals without flags (120 coins)

**Speed Challenges**

- Quick Draw: Reveal 100 safe cells in under 5 minutes (20 coins)
- Speed Demon: Reveal 200 safe cells in under 2 minutes (60 coins)
- Inhuman Reflexes: Reveal 300 safe cells in under 1 minute (150 coins + auto-ban)

### Chunk-Based Achievements

**Chunk Mastery**

- Land Surveyor: Clear 1 complete chunk (15 coins)
- Territory Controller: Clear 10 chunks (45 coins)
- Regional Commander: Clear 50 chunks (120 coins)
- Continental Overlord: Clear 200 chunks (300 coins)

### Leaderboard Performance

**Daily Rankings**

- Rising Star: Finish in top 100 on daily leaderboard (25 coins)
- Daily Dominator: Finish in top 10 (75 coins)
- King of the Hill: Finish #1 on daily leaderboard (200 coins)

### Multiplayer Interactions

**Competitive Elements**

- Photo Finish: Have your reveal rejected due to another player clicking the same cell simultaneously (20 coins)
- Claim Jumper: Successfully reveal a cell that another player attempted to reveal at the same time (15 coins)

### Hidden Achievements

**Lucky Discoveries**

- Lucky Seven: Reveal a cell showing "7" (30 coins)
- Maximum Danger: Reveal a cell showing "8" (50 coins)
- The Void: Discover and clear a chunk with zero mines (100 coins)
- Minefield From Hell: Discover a chunk where every cell contains a mine (150 coins)

**Score Milestones**

- High Roller: Reach 100,000 points (50 coins)
- Score Junkie: Reach 500,000 points (100 coins)
- The Untouchable: Reach 1,000,000 points (250 coins)
- Numbers Game: Score exactly 123,456 points (75 coins)

**Easter Eggs**

- Inner Circle: Set username matching friends list (Henri, Thomas, Johara, Jojo, Zo, Zainab, Jan Sakala, Joshua, Flavie, Jean, Nathalie, Xavier, Jodie, Josh, Matt, Nicholas, Nik, Sybil, Tony, Asha, Dany, Jeremy, Julie, Noah, Fraser, Massimo, Haydn, Raz, Tayla, Evelyn) (25 coins)
- Digital Archaeologist: Play the game for exactly 1,337 minutes total (42 coins)
- Pattern Seeker: Reveal cells that spell out "MINE" in adjacent positions (60 coins)

**Punishment Achievements**

- Butterfingers: Click 3 mines within 1 second (-10 coins)
- Mine Magnet: Hit 10 mines in a single session (-5 coins)
- Rage Quit Simulator: Close game within 10 seconds of hitting a mine (-1 coin)

### Collection & Progression

**Flag Unlocks**

- First Steps: Unlock your first cosmetic flag (10 coins)
- Collector: Unlock 5 different flags (25 coins)
- Flag Enthusiast: Unlock 10 different flags (50 coins)
- Completionist: Unlock all available flags (100 coins)

**Meta Achievements**

- Achievement Hunter: Unlock 10 achievements (20 coins)
- Overachiever: Unlock 25 achievements (75 coins)
- Perfect Game: Complete a 1,000-cell session with zero wrong flags and zero mine hits (200 coins)

## Technical Implementation Notes

### Achievement Tracking

- All statistics should be tracked server-side to prevent cheating
- Achievements should be evaluated on every relevant player action
- Use atomic transactions for coin awards to prevent duplication
- Implement rate limiting to prevent achievement spam

### Special Mechanics

- Concurrent click detection requires tracking RPC timestamps within 50ms windows
- Auto-ban system should trigger immediately upon "Inhuman Reflexes" unlock
- Hidden achievements should not appear in UI until unlocked
- Negative coin achievements should still show toast notifications but with different styling

### Data Storage

- Achievement definitions stored in configuration files for easy updates
- Player progress tracked in database with efficient indexing
- Achievement unlock history with timestamps for analytics

### Future Considerations

- Seasonal achievements for special events
- Achievement rarity statistics
- Social features showing friend achievements
- Achievement-based matchmaking or lobbies

This system should provide players with both short-term goals (daily achievements) and long-term progression paths while maintaining the core gameplay experience.
