// Static advancement definitions, mirroring backend/achievements.go.
// The server is authoritative for unlock evaluation; this table only drives
// display. Chains group the levels of one pursuit (Founder I..V etc.);
// `stat` names the PlayerStats field that measures progress, and
// `rewardFlagId` is the representative variant of the flag shape a level
// unlocks (0 = badge only).

export const CHAINS = [
  {
    key: "founder",
    title: "Founder",
    desc: "Be the first to reveal a cell in untouched chunks",
    stat: "chunksFounded",
    levels: [
      { id: "first_foothold", name: "First Foothold", threshold: 1, rewardFlagId: 102 },
      { id: "homesteader", name: "Homesteader", threshold: 15, rewardFlagId: 112 },
      { id: "frontier_baron", name: "Frontier Baron", threshold: 75, rewardFlagId: 142 },
      { id: "empire_builder", name: "Empire Builder", threshold: 400, rewardFlagId: 41 },
      { id: "continental", name: "Continental", threshold: 2000, rewardFlagId: 72 },
    ],
  },
  {
    key: "range",
    title: "Expedition",
    desc: "Reach far-away chunks (distance from the origin)",
    stat: "furthestChunkDistance",
    levels: [
      { id: "wanderer", name: "Wanderer", threshold: 30, rewardFlagId: 92 },
      { id: "pioneer", name: "Pioneer", threshold: 150, rewardFlagId: 122 },
      { id: "pathfinder", name: "Pathfinder", threshold: 750, rewardFlagId: 31 },
      { id: "edge_of_the_map", name: "Edge of the Map", threshold: 3000, rewardFlagId: 62 },
    ],
  },
  {
    key: "surveyor",
    title: "Surveyor",
    desc: "Fully clear chunks — all 4,096 cells revealed or flagged",
    stat: "chunksCleared",
    levels: [
      { id: "land_surveyor", name: "Land Surveyor", threshold: 1, rewardFlagId: 132 },
      { id: "territory_controller", name: "Territory Controller", threshold: 15, rewardFlagId: 61 },
      { id: "regional_commander", name: "Regional Commander", threshold: 75, rewardFlagId: 51 },
    ],
  },
  {
    key: "streak",
    title: "Flawless",
    desc: "Consecutive mistake-free actions (best streak)",
    stat: "maxSafeStreak",
    levels: [
      { id: "steady_hand", name: "Steady Hand", threshold: 150, rewardFlagId: 21 },
      { id: "master_sweeper", name: "Master Sweeper", threshold: 1500, rewardFlagId: 152 },
      { id: "nerves_of_steel", name: "Nerves of Steel", threshold: 7500, rewardFlagId: 67 },
    ],
  },
  {
    key: "deminer",
    title: "Deminer",
    desc: "Mines correctly flagged",
    stat: "correctFlags",
    levels: [
      { id: "deminer", name: "Deminer", threshold: 250, rewardFlagId: 107 },
      { id: "bomb_specialist", name: "Bomb Specialist", threshold: 2500, rewardFlagId: 46 },
    ],
  },
  {
    key: "danger",
    title: "Danger Zone",
    desc: "Safe reveals inside dense minefields",
    stat: "highDensityReveals",
    levels: [
      { id: "into_the_fire", name: "Into the Fire", threshold: 100, rewardFlagId: 117 },
      { id: "danger_seeker", name: "Danger Seeker", threshold: 1000, rewardFlagId: 56 },
    ],
  },
  {
    key: "cartography",
    title: "Cartography",
    desc: "Lifetime cells revealed",
    stat: "cellsRevealed",
    levels: [
      { id: "explorer", name: "Explorer", threshold: 2500, rewardFlagId: 0 },
      { id: "cartographer", name: "Cartographer", threshold: 50000, rewardFlagId: 0 },
      { id: "world_mapper", name: "World Mapper", threshold: 500000, rewardFlagId: 77 },
    ],
  },
  {
    key: "collector",
    title: "Collector",
    desc: "Flag rewards earned from other advancements",
    stat: null,
    levels: [
      { id: "collector", name: "Collector", threshold: 10, rewardFlagId: 0 },
    ],
  },
];

export const ACHIEVEMENTS = CHAINS.flatMap((c) =>
  c.levels.map((l) => ({ ...l, stat: c.stat, chain: c.key, hasReward: l.rewardFlagId > 0 }))
);

export const achievementById = new Map(ACHIEVEMENTS.map((a) => [a.id, a]));
