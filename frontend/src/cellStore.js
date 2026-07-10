// Per-chunk typed-array cell state. One byte per cell replaces a string-keyed
// Map entry holding an object per cell — dense merged-world regions hold
// millions of cells, and the Map version dominated client memory and GC.
//
// Byte layout: bit7 revealed, bit6 mine, bit5 flagged-carry (render
// suppression for revealed-but-flagged cells), bits0-4 adjacency 0..8 with
// 31 meaning "unknown yet" (neighbor seeds still missing).
export const CELL_REVEALED = 0x80;
export const CELL_MINE = 0x40;
export const CELL_FLAG_CARRY = 0x20;
export const ADJ_MASK = 0x1f;
export const ADJ_UNKNOWN = 0x1f;

export const packCell = (isMine, adjacent, flagCarry = false) =>
  CELL_REVEALED |
  (isMine ? CELL_MINE : 0) |
  (flagCarry ? CELL_FLAG_CARRY : 0) |
  (adjacent < 0 ? ADJ_UNKNOWN : adjacent & ADJ_MASK);

export const cellAdjacency = (packed) => {
  const a = packed & ADJ_MASK;
  return a === ADJ_UNKNOWN ? -1 : a;
};

const CELLS_PER_CHUNK = 64 * 64;

export class CellStore {
  constructor() {
    this.chunks = new Map(); // "cx,cy" -> Uint8Array(4096)
    this.everRevealed = false;
  }

  chunk(chunkKey) {
    return this.chunks.get(chunkKey) || null;
  }

  ensure(chunkKey) {
    let st = this.chunks.get(chunkKey);
    if (!st) {
      st = new Uint8Array(CELLS_PER_CHUNK);
      this.chunks.set(chunkKey, st);
    }
    this.everRevealed = true;
    return st;
  }

  get(cx, cy, cell) {
    const st = this.chunks.get(`${cx},${cy}`);
    return st ? st[cell] : 0;
  }

  set(cx, cy, cell, packed) {
    this.ensure(`${cx},${cy}`)[cell] = packed;
  }

  clearCell(cx, cy, cell) {
    const st = this.chunks.get(`${cx},${cy}`);
    if (st) st[cell] = 0;
  }

  clear() {
    this.chunks.clear();
    this.everRevealed = false;
  }

  evictFarChunks(centerCx, centerCy, keepRadius) {
    evictFarKeys(this.chunks, centerCx, centerCy, keepRadius);
  }
}

export class FlagStore {
  constructor() {
    this.chunks = new Map();
  }

  chunk(chunkKey) {
    return this.chunks.get(chunkKey) || null;
  }

  _loc(worldKey) {
    const comma = worldKey.indexOf(",");
    const wx = +worldKey.slice(0, comma);
    const wy = +worldKey.slice(comma + 1);
    const cx = Math.floor(wx / 64);
    const cy = Math.floor(wy / 64);
    return [`${cx},${cy}`, (wy - cy * 64) * 64 + (wx - cx * 64)];
  }

  get(worldKey) {
    const [chunkKey, cell] = this._loc(worldKey);
    return this.chunks.get(chunkKey)?.get(cell);
  }

  has(worldKey) {
    const [chunkKey, cell] = this._loc(worldKey);
    return this.chunks.get(chunkKey)?.has(cell) || false;
  }

  set(worldKey, flagID) {
    const [chunkKey, cell] = this._loc(worldKey);
    let flags = this.chunks.get(chunkKey);
    if (!flags) {
      flags = new Map();
      this.chunks.set(chunkKey, flags);
    }
    flags.set(cell, flagID);
    return this;
  }

  delete(worldKey) {
    const [chunkKey, cell] = this._loc(worldKey);
    const flags = this.chunks.get(chunkKey);
    if (!flags) return false;
    const deleted = flags.delete(cell);
    if (flags.size === 0) this.chunks.delete(chunkKey);
    return deleted;
  }

  clear() {
    this.chunks.clear();
  }

  evictFarChunks(centerCx, centerCy, keepRadius) {
    evictFarKeys(this.chunks, centerCx, centerCy, keepRadius);
  }
}

// Drop entries of a "cx,cy"-keyed Map farther than keepRadius (Chebyshev,
// in chunks) from the viewport center. The server re-syncs chunks when they
// re-enter the subscription window, so eviction is safe for anything
// comfortably outside it; radius must stay well above the subscription margin.
export function evictFarKeys(map, centerCx, centerCy, keepRadius) {
  for (const key of map.keys()) {
    const comma = key.indexOf(",");
    const cx = +key.slice(0, comma);
    const cy = +key.slice(comma + 1);
    if (
      Math.max(Math.abs(cx - centerCx), Math.abs(cy - centerCy)) > keepRadius
    ) {
      map.delete(key);
    }
  }
}
