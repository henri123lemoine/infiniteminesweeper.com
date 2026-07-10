// Unit tests for the packed cell store
import {
  CellStore,
  FlagStore,
  packCell,
  cellAdjacency,
  CELL_REVEALED,
  CELL_MINE,
  CELL_FLAG_CARRY,
  evictFarKeys,
} from "../src/cellStore.js";

let failures = 0;
const check = (name, cond) => {
  if (!cond) {
    console.error(`FAIL: ${name}`);
    failures++;
  } else {
    console.log(`OK  : ${name}`);
  }
};

// packing roundtrips
for (let adj = 0; adj <= 8; adj++) {
  const p = packCell(false, adj);
  check(`adjacency ${adj} roundtrip`, cellAdjacency(p) === adj && (p & CELL_REVEALED) && !(p & CELL_MINE));
}
check("unknown adjacency (-1) roundtrip", cellAdjacency(packCell(false, -1)) === -1);
check("mine pack", (packCell(true, 0) & CELL_MINE) !== 0);
check("flag carry pack", (packCell(false, 0, true) & CELL_FLAG_CARRY) !== 0);
check("zero byte is unrevealed", (0 & CELL_REVEALED) === 0);

// store behavior
const s = new CellStore();
check("empty get is 0", s.get(3, -4, 100) === 0);
check("everRevealed starts false", !s.everRevealed);
s.set(3, -4, 100, packCell(true, 0));
check("set/get", (s.get(3, -4, 100) & CELL_MINE) !== 0);
check("everRevealed after set", s.everRevealed);
s.clearCell(3, -4, 100);
check("clearCell", s.get(3, -4, 100) === 0);

// eviction keeps near, drops far
const s2 = new CellStore();
s2.set(0, 0, 0, packCell(false, 1));
s2.set(30, 0, 0, packCell(false, 1));
s2.set(-3, 2, 5, packCell(false, 2));
s2.evictFarChunks(0, 0, 24);
check("evict keeps near chunk", s2.get(0, 0, 0) !== 0);
check("evict keeps near negative chunk", s2.get(-3, 2, 5) !== 0);
check("evict drops far chunk", s2.get(30, 0, 0) === 0);

const m = new Map([["0,0", 1], ["100,-100", 2]]);
evictFarKeys(m, 0, 0, 24);
check("evictFarKeys on plain map", m.has("0,0") && !m.has("100,-100"));

const flags = new FlagStore();
flags.set("0,0", 7);
flags.set("-1,-1", 8);
flags.set("4096,0", 9);
check("flag get/has", flags.get("0,0") === 7 && flags.has("-1,-1"));
check("negative flag chunk", flags.chunk("-1,-1")?.get(4095) === 8);
flags.delete("0,0");
check("flag delete removes empty chunk", !flags.has("0,0") && flags.chunk("0,0") === null);
flags.evictFarChunks(0, 0, 32);
check("flag eviction", flags.has("-1,-1") && !flags.has("4096,0"));

if (failures) process.exit(1);
console.log("All cellStore checks passed.");
