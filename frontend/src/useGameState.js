import { useState, useRef, useCallback } from "react";
import pako from "pako";
import { ms as PB } from "./gen/messages_pb.js";

const log = __DEV__ ? console.log.bind(console) : () => {};

// Robust one-of detector: ignore numeric placeholders & legacy “payload”
const activeKey = (m) =>
  Object.keys(m).filter(
    (k) =>
      k !== "constructor" &&
      k !== "$type" &&
      k !== "payload" &&
      isNaN(Number(k)) &&
      m[k] != null
  )[0];

const normalizeChunkId = (cid = {}) => ({ X: cid.X ?? 0, Y: cid.Y ?? 0 });

export const CHUNK = 64;

function encodeMsg(msg) {
  if (msg && msg.payload && Object.keys(msg).length === 1) {
    msg = msg.payload;
  }

  // scrub BigInts
  const scrub = (o) => {
    if (o && typeof o === "object")
      for (const k of Object.keys(o)) {
        if (typeof o[k] === "bigint") o[k] = o[k].toString();
        else if (typeof o[k] === "object") scrub(o[k]);
      }
  };
  scrub(msg);

  const buf = PB.Msg.encode(PB.Msg.create(msg)).finish();
  if (__DEV__) {
    console.log("OUTGOING:", {
      raw: msg,
      serialized_size: buf.length,
      message_type: activeKey(msg),
    });
  }
  return pako.gzip(buf);
}

function decodeMsg(data) {
  let bytes = pako.ungzip(new Uint8Array(data));

  // Convert to plain JS so we can easily introspect keys
  const decodedPlain = PB.Msg.toObject(PB.Msg.decode(bytes), {
    longs: String,
    // Include default values so scalar fields like score=0 are preserved
    defaults: true,
  });

  const msg =
    decodedPlain.payload && Object.keys(decodedPlain).length === 1
      ? decodedPlain.payload
      : decodedPlain;

  if (__DEV__) {
    const k = activeKey(msg) ?? "<unknown>";
    console.log("INCOMING:", {
      raw: k !== "<unknown>" ? msg[k] : undefined,
      compressed_size: data.byteLength,
      decompressed_size: bytes.length,
      message_type: k,
    });
  }
  return msg;
}

// bytes decoding helper (protobufjs toObject gives base64 strings for bytes)
function b64ToU8(v) {
  if (!v) return new Uint8Array(0);
  if (v instanceof Uint8Array) return v;
  if (Array.isArray(v)) return new Uint8Array(v);
  if (typeof v === "string") {
    try {
      const bin = atob(v);
      const u8 = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i) & 0xff;
      return u8;
    } catch {
      return new Uint8Array(0);
    }
  }
  return new Uint8Array(0);
}

// Deterministic helpers
const splitmix64 = (state) => {
  state = (state + 0x9e3779b97f4a7c15n) & 0xffffffffffffffffn;
  state =
    ((state ^ (state >> 30n)) * 0xbf58476d1ce4e5b9n) & 0xffffffffffffffffn;
  state =
    ((state ^ (state >> 27n)) * 0x94d049bb133111ebn) & 0xffffffffffffffffn;
  return state ^ (state >> 31n);
};

const isMineWith = (seed, density, cell) => {
  // Validate seed
  if (typeof seed !== "bigint" || seed < 0n) return null;

  // Validate cell
  if (!Number.isInteger(cell) || cell < 0 || cell >= 4096) return null;

  // Validate density type
  if (typeof density !== "number" || !Number.isFinite(density)) return null;

  // Clamp density to [0, 1]
  const clampedDensity = Math.max(0, Math.min(1, density));

  const cellSeed = splitmix64(seed + BigInt(cell));
  const threshold = Math.floor(clampedDensity * 100);
  return Number(cellSeed % 100n) < threshold;
};

// Convert world coordinates to chunk and cell index coordinates
const worldToChunk = (worldX, worldY) => {
  const chunkX = Math.floor(worldX / CHUNK);
  const chunkY = Math.floor(worldY / CHUNK);

  let localX = worldX - chunkX * CHUNK;
  let localY = worldY - chunkY * CHUNK;

  if (localX < 0) localX += CHUNK;
  if (localY < 0) localY += CHUNK;

  const cell = localY * CHUNK + localX;
  return { chunkX, chunkY, cell };
};

export const useGameState = () => {
  const initialName = "";

  const [ws, setWs] = useState(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef(null);
  const connectedRef = useRef(false);
  const isReconnectingRef = useRef(false);
  const lastViewSentRef = useRef({
    startX: null,
    startY: null,
    w: 0,
    h: 0,
    at: 0,
  });
  const MIN_VIEW_SEND_INTERVAL_MS = 220;
  const [leaderboard, setLeaderboard] = useState([]);
  const [playerScore, setPlayerScore] = useState(0);
  const [userRank, setUserRank] = useState(0);
  const [username, setUsername] = useState(initialName);
  const [scorePopups, setScorePopups] = useState([]);
  const [hintPopups, setHintPopups] = useState([]);
  const [tick, setTick] = useState(0);
  const [updateError, setUpdateError] = useState("");
  const [joinError, setJoinError] = useState("");
  // Advancements: lifetime stats + unlock sets synced on join, toasts on unlock
  const [advStats, setAdvStats] = useState(null);
  const [unlockedAdvIds, setUnlockedAdvIds] = useState([]);
  const [unlockedFlagIds, setUnlockedFlagIds] = useState([]);
  const [advToasts, setAdvToasts] = useState([]);
  const [serverFlagID, setServerFlagID] = useState(null);
  const [updateSuccess, setUpdateSuccess] = useState(false);
  // Server-suggested spawn location (world cell coordinates)
  const serverSpawnRef = useRef(null);
  // Active nearby players: Map<playerId, {x, y, targetX, targetY, flagId}>
  const activePlayersRef = useRef(new Map());

  // Minimap streaming store and helpers
  const minimapTilesRef = useRef(new Map()); // key "x,y" -> { version, data: Uint8Array, canvas, resolution }
  // Union of active subscriptions actually sent to the server
  const minimapActiveSubsRef = useRef(new Set());
  // Desired subscriptions per logical source (e.g. 'hud', 'overlay')
  const minimapDesiredBySourceRef = useRef(new Map()); // sourceKey -> Set("x,y")
  // Track current resolution preference to detect changes
  const currentResolutionRef = useRef(64);
  // 0..9: board background, mines, numbers; 10..19: 10 flag colors
  const minimapPalette = new Uint32Array(256);
  // base board colors
  minimapPalette[0] = 0xffc0c0c0; // unseen
  // Use very dark gray for mines so pure black can be reserved for a flag color
  minimapPalette[1] = 0xff202020; // mine (dark gray)
  minimapPalette[2] = 0xffe0e0e0; // empty
  minimapPalette[3] = 0xffe9ecff; // n1
  minimapPalette[4] = 0xffe9ffea; // n2
  minimapPalette[5] = 0xffffe9ea; // n3
  minimapPalette[6] = 0xffececff; // n4
  minimapPalette[7] = 0xfffff0ea; // n5
  minimapPalette[8] = 0xffd4fff2; // n6
  minimapPalette[9] = 0xfff0e4ff; // n7+ (slightly purple for contrast)
  // 10 flag buckets must align with (flagID % 10) color mapping:
  // 0: light_gray, 1: red, 2: green, 3: blue, 4: yellow,
  // 5: orange, 6: purple, 7: cyan, 8: pink, 9: dark_gray (black)
  const flagColors = [
    0xffdcdcdc, // light_gray
    0xffe31c1c, // red
    0xff22c55e, // green
    0xff2166f3, // blue
    0xfff59e0b, // yellow
    0xffff6b2c, // orange
    0xffa855f7, // purple
    0xff06b6d4, // cyan
    0xffff69b4, // pink
    0xff000000, // dark_gray / black
  ];
  for (let i = 0; i < 10; i++) minimapPalette[10 + i] = flagColors[i];

  const minimapGetCanvas = useCallback((key) => {
    const rec = minimapTilesRef.current.get(key);
    if (!rec) return null;
    if (rec.canvas) return rec.canvas;
    const resolution = rec.resolution || CHUNK; // fallback to CHUNK for legacy tiles
    const c =
      typeof OffscreenCanvas !== "undefined"
        ? new OffscreenCanvas(resolution, resolution)
        : (() => {
            const el = document.createElement("canvas");
            el.width = resolution;
            el.height = resolution;
            return el;
          })();
    rec.canvas = c;
    return c;
  }, []);

  const minimapDrawFull = useCallback((key) => {
    const rec = minimapTilesRef.current.get(key);
    if (!rec) return;
    const c = minimapGetCanvas(key);
    const ctx = c.getContext("2d");
    const resolution = rec.resolution || CHUNK; // fallback to CHUNK for legacy tiles
    const img = ctx.createImageData(resolution, resolution);
    const dst = img.data;
    const data = rec.data;
    let di = 0;
    for (let i = 0; i < data.length; i++) {
      const idx = data[i] & 0xff;
      const color = minimapPalette[idx] >>> 0;
      const a = (color >>> 24) & 0xff;
      const r = (color >>> 16) & 0xff;
      const g = (color >>> 8) & 0xff;
      const b = color & 0xff;
      dst[di++] = r;
      dst[di++] = g;
      dst[di++] = b;
      dst[di++] = a;
    }
    ctx.putImageData(img, 0, 0);
  }, []);

  const minimapDrawRect = useCallback((key, x, y, w, h) => {
    const rec = minimapTilesRef.current.get(key);
    if (!rec) return;
    const c = minimapGetCanvas(key);
    const ctx = c.getContext("2d");
    const resolution = rec.resolution || CHUNK;
    const img = ctx.getImageData(x, y, w, h);
    const dst = img.data;
    let di = 0;
    for (let row = 0; row < h; row++) {
      const base = (y + row) * resolution + x;
      for (let col = 0; col < w; col++) {
        const idx = rec.data[base + col] & 0xff;
        const color = minimapPalette[idx] >>> 0;
        const a = (color >>> 24) & 0xff;
        const r = (color >>> 16) & 0xff;
        const g = (color >>> 8) & 0xff;
        const b = color & 0xff;
        dst[di++] = r;
        dst[di++] = g;
        dst[di++] = b;
        dst[di++] = a;
      }
    }
    ctx.putImageData(img, x, y);
  }, []);

  // Game state refs
  const seedCache = useRef(new Map());
  const densityCache = useRef(new Map());
  const revealedCellsRef = useRef(new Map());
  const flaggedCellsRef = useRef(new Map());
  const chunkVersionRef = useRef(new Map()); // "cx,cy" -> monotonically increasing version

  const bumpChunkVersion = useCallback((cx, cy) => {
    const k = `${cx},${cy}`;
    const v = (chunkVersionRef.current.get(k) || 0) + 1;
    chunkVersionRef.current.set(k, v);
  }, []);
  const playerFlagsRef = useRef(new Map());
  const optimisticActions = useRef(new Map());
  // Cells revealed before all neighbor seeds arrived (adjacentMines === -1),
  // chunkKey -> Set(cellIdx); recomputed when an adjacent seed shows up,
  // otherwise they'd render blank forever.
  const pendingAdjacencyRef = useRef(new Map());

  const registerPendingAdjacency = useCallback((chunkKey, cell) => {
    let set = pendingAdjacencyRef.current.get(chunkKey);
    if (!set) {
      set = new Set();
      pendingAdjacencyRef.current.set(chunkKey, set);
    }
    set.add(cell);
  }, []);

  // Request seeds and densities for adjacent chunks (pre-emptive caching)
  const requestAdjacentSeeds = useCallback(
    (cx, cy) => {
      if (!ws || !connected) return;

      const adjacentChunks = [];
      const alreadyRequested = new Set();

      // Check all 8 adjacent chunks around the given chunk
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          if (dx === 0 && dy === 0) continue; // Skip the center chunk itself
          const adjX = cx + dx;
          const adjY = cy + dy;
          const chunkKey = `${adjX},${adjY}`;

          // Only request if we don't already have the seed cached
          if (
            !seedCache.current.has(chunkKey) &&
            !alreadyRequested.has(chunkKey)
          ) {
            adjacentChunks.push({ X: adjX, Y: adjY });
            alreadyRequested.add(chunkKey);
          }
        }
      }

      if (adjacentChunks.length > 0 && ws.readyState === WebSocket.OPEN) {
        ws.send(
          encodeMsg({
            seedRequest: {
              chunkIds: adjacentChunks,
            },
          })
        );
      }
    },
    [ws, connected]
  );

  const countAdjacentMines = useCallback((cx, cy, cell) => {
    const x = cell % CHUNK;
    const y = Math.floor(cell / CHUNK);
    const worldX = cx * CHUNK + x;
    const worldY = cy * CHUNK + y;

    let count = 0;
    let hasIncompleteData = false;

    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        if (dx === 0 && dy === 0) continue;
        const {
          chunkX: ncx,
          chunkY: ncy,
          cell: ncell,
        } = worldToChunk(worldX + dx, worldY + dy);
        const chunkKey = `${ncx},${ncy}`;
        const nSeed = seedCache.current.get(chunkKey);
        const nDensity = densityCache.current.get(chunkKey);

        if (!nSeed || nDensity == null) {
          hasIncompleteData = true;
          continue;
        }
        if (isMineWith(nSeed, nDensity, ncell)) count++;
      }
    }

    // Return -1 to indicate incomplete data, which the caller can render as "?"
    return hasIncompleteData ? -1 : count;
  }, []);

  // Recompute any pending-adjacency cells in the 3x3 chunk neighborhood of
  // (cx, cy) — called when a new seed/density arrives for that chunk.
  const recomputeAdjacencyAround = useCallback(
    (cx, cy) => {
      let changed = false;
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          const ncx = cx + dx;
          const ncy = cy + dy;
          const chunkKey = `${ncx},${ncy}`;
          const set = pendingAdjacencyRef.current.get(chunkKey);
          if (!set || set.size === 0) continue;
          const seed = seedCache.current.get(chunkKey);
          const d = densityCache.current.get(chunkKey);
          if (seed == null || d == null) continue;
          let chunkChanged = false;
          for (const cell of Array.from(set)) {
            const isMineVal = !!isMineWith(seed, d, cell);
            const adjacent = isMineVal ? 0 : countAdjacentMines(ncx, ncy, cell);
            if (adjacent === -1) continue; // some neighbor seed still missing
            const cellKey = `${chunkKey},${cell}`;
            const prev = revealedCellsRef.current.get(cellKey);
            if (prev) {
              revealedCellsRef.current.set(cellKey, {
                ...prev,
                isMine: isMineVal,
                adjacentMines: adjacent,
              });
              chunkChanged = true;
            }
            set.delete(cell);
          }
          if (set.size === 0) pendingAdjacencyRef.current.delete(chunkKey);
          if (chunkChanged) {
            bumpChunkVersion(ncx, ncy);
            changed = true;
          }
        }
      }
      return changed;
    },
    [countAdjacentMines, bumpChunkVersion]
  );

  const applyChunkSync = useCallback(
    (data) => {
      const { chunkId, seed, reveals, flagGroups: fgRaw, density } = data;
      const { X, Y } = normalizeChunkId(chunkId);
      const chunkKey = `${X},${Y}`;
      const seedBigInt = new DataView(
        seed.buffer,
        seed.byteOffset,
        8
      ).getBigUint64(0, true);
      seedCache.current.set(chunkKey, seedBigInt);
      if (typeof density === "number")
        densityCache.current.set(chunkKey, density);

      const flagGroups = Array.isArray(fgRaw) ? fgRaw : [];
      for (const group of flagGroups) {
        const { flagID, cells } = group;
        let cellList = [];
        if (cells && typeof cells === "object") {
          if (Array.isArray(cells.cells)) {
            cellList = cells.cells;
          } else if (Array.isArray(cells)) {
            cellList = cells;
          }
        }
        for (const cell of cellList) {
          const localX = cell % CHUNK;
          const localY = Math.floor(cell / CHUNK);
          const flagWorldX = X * CHUNK + localX;
          const flagWorldY = Y * CHUNK + localY;
          flaggedCellsRef.current.set(`${flagWorldX},${flagWorldY}`, flagID);
        }
      }

      const view = new DataView(
        reveals.buffer,
        reveals.byteOffset,
        reveals.byteLength
      );
      for (let i = 0; i < CHUNK * CHUNK; i++) {
        const wordIndex = Math.floor(i / CHUNK);
        const bitIndex = i % CHUNK;
        if (
          (view.getBigUint64(wordIndex * 8, true) &
            (1n << BigInt(bitIndex))) !==
          0n
        ) {
          const cellKey = `${X},${Y},${i}`;
          const d = densityCache.current.get(chunkKey);
          const isMineVal = d != null ? isMineWith(seedBigInt, d, i) : false;
          const adjacent = isMineVal ? 0 : countAdjacentMines(X, Y, i);
          if (adjacent === -1) registerPendingAdjacency(chunkKey, i);
          revealedCellsRef.current.set(cellKey, {
            isMine: isMineVal,
            adjacentMines: adjacent,
          });
        }
      }
      // Any change to this chunk's content should bump its version
      bumpChunkVersion(X, Y);

      // This chunk's seed may complete the data neighbors were waiting on
      recomputeAdjacencyAround(X, Y);

      // Pre-emptively request seeds for adjacent chunks to prevent cache misses
      requestAdjacentSeeds(X, Y);
    },
    [
      countAdjacentMines,
      bumpChunkVersion,
      requestAdjacentSeeds,
      registerPendingAdjacency,
      recomputeAdjacencyAround,
    ]
  );

  // Count flags around a (world-coord) cell
  const countAdjacentFlags = useCallback((worldX, worldY) => {
    let c = 0;
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        if (dx === 0 && dy === 0) continue;
        if (flaggedCellsRef.current.has(`${worldX + dx},${worldY + dy}`)) c++;
      }
    }
    return c;
  }, []);

  // Check if a chord operation is valid by comparing placed flags + revealed mines to expected adjacent mines
  const canChord = useCallback(
    (worldX, worldY) => {
      const placedFlags = countAdjacentFlags(worldX, worldY);
      let revealedMines = 0;

      // Count revealed mines in the 8 adjacent cells
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          if (dx === 0 && dy === 0) continue;
          const wx = worldX + dx,
            wy = worldY + dy;
          // Ignore flagged neighbors when counting revealed mines to avoid double counting
          if (flaggedCellsRef.current.has(`${wx},${wy}`)) continue;
          const { chunkX: cx, chunkY: cy, cell: c } = worldToChunk(wx, wy);
          const key = `${cx},${cy},${c}`;
          const data = revealedCellsRef.current.get(key);
          if (data?.isMine) revealedMines++;
        }
      }

      const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
      const expectedMines = countAdjacentMines(chunkX, chunkY, cell);

      // If we have incomplete data (expectedMines = -1), prevent chording for safety
      if (expectedMines < 0) return false;

      return placedFlags + revealedMines === expectedMines;
    },
    [countAdjacentFlags, countAdjacentMines]
  );

  // Send throttled view updates with viewport size in world cells
  const sendViewUpdate = useRef(
    (() => {
      let raf = null;
      let pending = null;
      return (chunkX, chunkY, cell, widthCells, heightCells) => {
        pending = { chunkX, chunkY, cell, widthCells, heightCells };
        if (raf) return;
        raf = requestAnimationFrame(() => {
          raf = null;
          const p = pending;
          pending = null;
          const s = wsRef.current;
          const isConn = connectedRef.current;
          if (!p) return;

          // Compute intended chunk-rect (mirror server logic)
          const chunksWide = Math.ceil(p.widthCells / CHUNK) + 2;
          const chunksHigh = Math.ceil(p.heightCells / CHUNK) + 2;
          const halfW = Math.floor(chunksWide / 2);
          const halfH = Math.floor(chunksHigh / 2);
          const startX = p.chunkX - halfW;
          const startY = p.chunkY - halfH;

          // Skip if identical rect and within min interval
          const now = performance.now();
          const last = lastViewSentRef.current;
          const sameRect =
            last.startX === startX &&
            last.startY === startY &&
            last.w === chunksWide &&
            last.h === chunksHigh;
          if (sameRect && now - last.at < MIN_VIEW_SEND_INTERVAL_MS) {
            if (__DEV__)
              console.log("viewUpdate coalesced: same rect + within interval");
            return;
          }
          // Update last before send to avoid bursts
          lastViewSentRef.current = {
            startX,
            startY,
            w: chunksWide,
            h: chunksHigh,
            at: now,
          };

          if (!s || !isConn || s.readyState !== WebSocket.OPEN) {
            if (__DEV__)
              console.log("viewUpdate skip: not connected or ws not open", {
                hasWs: !!s,
                isConn,
                readyState: s?.readyState,
              });
            return;
          }
          if (__DEV__) console.log("OUTGOING(viewUpdate)", p);
          s.send(
            encodeMsg({
              viewUpdate: {
                chunkId: { X: p.chunkX, Y: p.chunkY },
                cell: p.cell,
                widthCells: p.widthCells,
                heightCells: p.heightCells,
              },
            })
          );
        });
      };
    })()
  );

  // Return true if all chunks intersecting a Chebyshev radius (default 2) are known (we have their seed)
  const isRadiusFullyKnown = useCallback(
    (worldX, worldY, radius = 2) => {
      const seenChunks = new Set();
      for (let dy = -radius; dy <= radius; dy++) {
        for (let dx = -radius; dx <= radius; dx++) {
          const { chunkX: cx, chunkY: cy } = worldToChunk(
            worldX + dx,
            worldY + dy
          );
          seenChunks.add(`${cx},${cy}`);
        }
      }
      for (const key of seenChunks) {
        if (!seedCache.current.has(key)) return false;
      }
      return true;
    },
    [seedCache]
  );

  // Proximity rule: block only if we KNOW there are no revealed cells within Chebyshev distance <= 2
  // of the target. If the neighborhood isn't fully known, allow and send anyway.
  const isWithinTwoOfRevealed = useCallback(
    (worldX, worldY) => {
      // allow first actions when we have no reveals at all
      if (revealedCellsRef.current.size === 0) return true;
      // if any neighbor within 2 is revealed, allow
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) {
          const {
            chunkX: cx,
            chunkY: cy,
            cell,
          } = worldToChunk(worldX + dx, worldY + dy);
          if (revealedCellsRef.current.has(`${cx},${cy},${cell}`)) return true;
        }
      }
      // if we don't fully know the radius, allow (send anyway)
      if (!isRadiusFullyKnown(worldX, worldY, 2)) return true;
      // fully known and nothing revealed nearby => block
      return false;
    },
    [isRadiusFullyKnown]
  );

  const pushHintPopup = useCallback((worldX, worldY, message) => {
    const id = Math.random().toString(36).slice(2);
    setHintPopups((p) => [...p, { id, worldX, worldY, message }]);
    setTimeout(() => {
      setHintPopups((p) => p.filter((h) => h.id !== id));
    }, 1500);
  }, []);

  const handleCellClick = useCallback(
    (worldX, worldY, isRightClick = false, isChord = false) => {
      if (!ws || !connected) return;

      const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
      const cellKey = `${chunkX},${chunkY},${cell}`;
      const flagKey = `${worldX},${worldY}`;
      const chunkKey = `${chunkX},${chunkY}`;

      const revealedCell = revealedCellsRef.current.get(cellKey);
      if (isChord && !revealedCell) return;
      if (!isChord && revealedCell) return;
      if (flaggedCellsRef.current.has(flagKey)) {
        // Left-clicking a flag asks the server whose it is
        if (!isRightClick && !isChord && ws.readyState === WebSocket.OPEN) {
          ws.send(
            encodeMsg({
              cellOwnerRequest: { chunkId: { X: chunkX, Y: chunkY }, cell },
            })
          );
        }
        return;
      }

      // Enforce two-cell proximity for non-chord actions
      if (!isChord && !isWithinTwoOfRevealed(worldX, worldY)) {
        pushHintPopup(
          worldX,
          worldY,
          "Stay near the island — reveal within 2 cells of explored area"
        );
        return;
      }

      const requestId = Date.now().toString();
      const optimisticChanges = new Map();
      optimisticActions.current.set(requestId, optimisticChanges);
      let didLocalMutation = false; // track if we really changed stuff

      const recordChange = (cKey, change) => {
        if (!optimisticChanges.has(cKey)) {
          optimisticChanges.set(cKey, { reveals: new Set(), flags: new Set() });
        }
        if (change.type === "reveal")
          optimisticChanges.get(cKey).reveals.add(change.cell);
        if (change.type === "flag")
          optimisticChanges.get(cKey).flags.add(change.cell);
        didLocalMutation = true;
        // bump the chunk version for this optimistic mutation
        const [cx, cy] = cKey.split(",").map(Number);
        bumpChunkVersion(cx, cy);
      };

      if (isRightClick) {
        const myFlagId = playerFlagsRef.current.get(username);
        const seed = seedCache.current.get(chunkKey);
        const d = densityCache.current.get(chunkKey);
        // Always send flag placement requests to the server
        didLocalMutation = true;
        // If we know the seed and this is NOT a mine, suppress optimistic flag but still send the request.
        if (seed && !isMineWith(seed, d, cell)) {
          // Don't set optimistic flag for non-mines
        } else if (myFlagId !== undefined) {
          // Only optimistic-flag when unknown or when it is actually a mine
          flaggedCellsRef.current.set(flagKey, myFlagId);
          recordChange(chunkKey, { type: "flag", cell });
        }
      } else if (isChord) {
        // Only chord if flags == adjacentMines
        if (!canChord(worldX, worldY)) return;

        // BFS queue seeded with all adjacent cells
        const queue = [];
        const visited = new Set();
        for (let dy = -1; dy <= 1; dy++) {
          for (let dx = -1; dx <= 1; dx++) {
            if (dx === 0 && dy === 0) continue;
            queue.push({ x: worldX + dx, y: worldY + dy });
          }
        }

        while (queue.length) {
          const { x: wx, y: wy } = queue.shift();
          const flagKey = `${wx},${wy}`;
          if (flaggedCellsRef.current.has(flagKey)) continue;

          const { chunkX: cx, chunkY: cy, cell: cidx } = worldToChunk(wx, wy);
          const chunkKey = `${cx},${cy}`;
          const cellKey = `${cx},${cy},${cidx}`;
          if (revealedCellsRef.current.has(cellKey) || visited.has(cellKey))
            continue;
          visited.add(cellKey);

          const seed = seedCache.current.get(chunkKey);
          const d = densityCache.current.get(chunkKey);
          if (!seed || d == null) continue;

          // reveal
          const isM = isMineWith(seed, d, cidx);
          const adj = isM ? 0 : countAdjacentMines(cx, cy, cidx);
          revealedCellsRef.current.set(cellKey, {
            isMine: isM,
            adjacentMines: adj,
          });
          recordChange(chunkKey, { type: "reveal", cell: cidx });

          // if zero, expand around it (only if we have complete data)
          if (!isM && adj === 0) {
            for (let dy2 = -1; dy2 <= 1; dy2++) {
              for (let dx2 = -1; dx2 <= 1; dx2++) {
                if (dx2 === 0 && dy2 === 0) continue;
                queue.push({ x: wx + dx2, y: wy + dy2 });
              }
            }
          }
        }
      } else {
        // Standard reveal
        const seed = seedCache.current.get(chunkKey);
        const d = densityCache.current.get(chunkKey);
        if (seed && d != null) {
          if (isMineWith(seed, d, cell)) {
            revealedCellsRef.current.set(cellKey, {
              isMine: true,
              adjacentMines: 0,
            });
          } else {
            const adjacent = countAdjacentMines(chunkX, chunkY, cell);
            // Store -1 for incomplete data, which renderers can display as "?"
            revealedCellsRef.current.set(cellKey, {
              isMine: false,
              adjacentMines: adjacent,
            });
          }
          recordChange(chunkKey, { type: "reveal", cell });
        }
      }

      setTick((t) => t + 1);

      if (!didLocalMutation) return; // chord / click revealed nothing → abort

      if (__DEV__) {
        console.log("OPTIMISTIC-START", {
          requestId,
          worldX,
          worldY,
          isRightClick,
          isChord,
        });
      }

      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          encodeMsg({
            reveal: {
              requestId,
              chunkId: { X: chunkX, Y: chunkY },
              cell,
              isRightClick,
              isChord,
            },
          })
        );
      }
    },
    [ws, connected, countAdjacentMines, countAdjacentFlags, canChord, username]
  );

  const connectWebSocket = useCallback(() => {
    // Prevent multiple concurrent connection attempts
    if (isReconnectingRef.current) {
      return () => {};
    }

    isReconnectingRef.current = true;
    const wsUrl = `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;
    const websocket = new WebSocket(wsUrl);
    websocket.binaryType = "arraybuffer";
    let didJoinAck = false;

    websocket.onopen = () => {
      // Start in spectator mode - no initial message needed
      // Connection starts as spectator automatically
      setWs(websocket);
      wsRef.current = websocket;
      connectedRef.current = true;
      isReconnectingRef.current = false;

      // Note: setConnected(true) is only called after successful join
      // On fresh connection, server has no minimap subs; clear local active set
      try {
        minimapActiveSubsRef.current.clear();
      } catch {}
      // Reconcile any desired minimap subscriptions accumulated before open
      const resubscribeAll = () => {
        try {
          const s = wsRef.current;
          if (!s || s.readyState !== WebSocket.OPEN) return;
          const union = new Set();
          for (const set of minimapDesiredBySourceRef.current.values()) {
            for (const k of set) union.add(k);
          }
          if (union.size) {
            const tiles = Array.from(union).map((k) => {
              const [x, y] = k.split(",").map(Number);
              return { x, y };
            });
            s.send(encodeMsg({ minimapSubscribe: { tiles } }));
            minimapActiveSubsRef.current = new Set(union);
          }
        } catch {}
      };
      // Run after layout to also allow initial viewport computation
      requestAnimationFrame(resubscribeAll);
      // Kick an initial view update on the next frame once the DOM is ready
      requestAnimationFrame(() => {
        try {
          // Use DOM-based helper to compute current viewport in world cells
          const root = document.querySelector("#root")?.firstElementChild;
          if (root) {
            const width = root.clientWidth;
            const height = root.clientHeight;
            const worldWidthCells = Math.ceil(width / 1);
            const worldHeightCells = Math.ceil(height / 1);
            const centerWorldX = Math.floor(
              (viewRef.current.x + width / 2) / 1
            );
            const centerWorldY = Math.floor(
              (viewRef.current.y + height / 2) / 1
            );
            const { chunkX, chunkY, cell } = worldToChunk(
              centerWorldX,
              centerWorldY
            );
            sendViewUpdate.current(
              chunkX,
              chunkY,
              cell,
              worldWidthCells,
              worldHeightCells
            );
          }
        } catch {}
      });

      // Auto-rejoin if we have stored credentials (handles reconnection after disconnect)
      const storedUsername = localStorage.getItem("username");
      const storedToken = localStorage.getItem("session_token");
      const storedFlagID = localStorage.getItem("flagID");
      if (storedUsername && storedToken && storedFlagID) {
        // Small delay to ensure connection is fully established
        setTimeout(() => {
          if (websocket.readyState === WebSocket.OPEN) {
            websocket.send(
              encodeMsg({
                join: {
                  sessionToken: storedToken,
                  name: storedUsername,
                  flagID: Number(storedFlagID) || 0,
                },
              })
            );
          }
        }, 100);
      }
    };

    websocket.onclose = (event) => {
      setConnected(false);
      setWs(null);
      wsRef.current = null;
      connectedRef.current = false;
      isReconnectingRef.current = false;
      try {
        minimapActiveSubsRef.current.clear();
      } catch {}
      optimisticActions.current.clear();
      revealedCellsRef.current.clear();
      flaggedCellsRef.current.clear();
      playerFlagsRef.current.clear();
      pendingAdjacencyRef.current.clear();
      // If server rejected the join (e.g., invalid or taken username),
      // reset username so the join dialog is shown again.
      if (!didJoinAck) {
        setUsername("");
      }
    };

    websocket.onmessage = (event) => {
      const msg = decodeMsg(event.data);
      const type = activeKey(msg);
      const data = msg[type];

      if (type === "joinAck") {
        didJoinAck = true;
        if (data.ok) {
          // Successful join - transition to player mode
          localStorage.setItem("session_token", data.sessionToken);
          localStorage.setItem("username", data.name || "");
          localStorage.setItem("score", String(data.score ?? 0));
          localStorage.setItem("flagID", String(data.flagID));
          playerFlagsRef.current.set(data.name, data.flagID);
          setUsername(data.name || "");
          setPlayerScore(data.score ?? 0);
          if (data.userRank) setUserRank(Number(data.userRank));
          setServerFlagID(data.flagID); // Sync server's authoritative flagID
          setJoinError(""); // Clear any previous join errors
          setConnected(true); // Now we're fully connected as a player

          // Send a view update after successful join
          requestAnimationFrame(() => {
            try {
              const root = document.querySelector("#root")?.firstElementChild;
              if (root) {
                const width = root.clientWidth;
                const height = root.clientHeight;
                const worldWidthCells = Math.ceil(width / 1);
                const worldHeightCells = Math.ceil(height / 1);
                const centerWorldX = Math.floor(
                  (viewRef.current.x + width / 2) / 1
                );
                const centerWorldY = Math.floor(
                  (viewRef.current.y + height / 2) / 1
                );
                const { chunkX, chunkY, cell } = worldToChunk(
                  centerWorldX,
                  centerWorldY
                );
                sendViewUpdate.current(
                  chunkX,
                  chunkY,
                  cell,
                  worldWidthCells,
                  worldHeightCells
                );
              }
            } catch {}
          });
        } else {
          // Join failed - stay in spectator mode but notify user
          console.error("Join failed:", data.error);
          setJoinError(data.error || "Join failed");
          // Clear username to force UI back to join dialog
          setUsername("");
          localStorage.removeItem("username");
          localStorage.removeItem("session_token"); // Also clear invalid session
        }
      } else if (type === "updateAck") {
        if (data.ok) {
          // Profile update successful
          localStorage.setItem("username", data.name || "");
          localStorage.setItem("flagID", String(data.flagID));
          setUsername(data.name || "");
          playerFlagsRef.current.set(data.name, data.flagID);
          setServerFlagID(data.flagID); // Sync server's authoritative flagID
          setUpdateError(""); // Clear any previous errors
          setUpdateSuccess(true); // Signal successful update
        } else {
          setUpdateError(data.error || "Profile update failed");
          console.error("Profile update failed:", data.error);
        }
      } else if (type === "spawnHint") {
        // Server-suggested spawn location for spectators
        if (data.chunkId != null) {
          const cid = data.chunkId;
          const cell = Number(data.cell || 0) | 0;
          const localX = cell % CHUNK;
          const localY = Math.floor(cell / CHUNK);
          const spawnX = (Number(cid.X) | 0) * CHUNK + localX;
          const spawnY = (Number(cid.Y) | 0) * CHUNK + localY;
          serverSpawnRef.current = { x: spawnX, y: spawnY, at: Date.now() };
        }
      } else if (type === "playerPositions") {
        // Update active player positions
        const oldPlayers = activePlayersRef.current;
        const newPlayers = new Map();
        for (const p of data.players || []) {
          if (p.chunkId != null) {
            const cid = p.chunkId;
            const cell = Number(p.cell || 0) | 0;
            const localX = cell % CHUNK;
            const localY = Math.floor(cell / CHUNK);
            const targetX = (Number(cid.X) | 0) * CHUNK + localX;
            const targetY = (Number(cid.Y) | 0) * CHUNK + localY;

            const existing = oldPlayers.get(p.playerId);
            let x, y;
            if (existing) {
              x = existing.x;
              y = existing.y;
            } else {
              x = targetX;
              y = targetY;
            }

            newPlayers.set(p.playerId, {
              x,
              y,
              targetX,
              targetY,
              flagId: p.flagId || 0,
            });
          }
        }
        activePlayersRef.current = newPlayers;
      } else if (type === "chunkSync") {
        applyChunkSync(data);
        setTick((t) => t + 1);
      } else if (type === "chunkRegionSync") {
        const region = PB.ChunkRegion.toObject(
          PB.ChunkRegion.decode(data.chunks),
          { defaults: false }
        );
        for (const cs of region.chunks || []) {
          applyChunkSync(cs);
        }
        setTick((t) => t + 1);
      } else if (type === "minimapFullTile") {
        const tr = data.tile || {};
        const key = `${tr.x},${tr.y}`;
        const buf = b64ToU8(data.data);
        const resolution = data.resolution || CHUNK; // fallback for legacy tiles

        // Check if this is a resolution change - if so, force canvas regeneration
        const existingRec = minimapTilesRef.current.get(key);
        const rec = {
          version: Number(data.version) || 0,
          data: new Uint8Array(buf),
          resolution,
        };

        // If resolution changed, clear old canvas to force regeneration at new size
        if (
          existingRec &&
          existingRec.resolution !== resolution &&
          existingRec.canvas
        ) {
          delete existingRec.canvas;
        }

        minimapTilesRef.current.set(key, rec);
        minimapDrawFull(key);
      } else if (type === "minimapTileDelta") {
        const tr = data.tile || {};
        const key = `${tr.x},${tr.y}`;
        let rec = minimapTilesRef.current.get(key);
        const resolution = data.resolution || CHUNK; // fallback for legacy tiles

        // If we haven't received a full tile yet, bootstrap an unseen tile and apply delta
        if (!rec) {
          rec = {
            version: 0,
            data: new Uint8Array(resolution * resolution),
            resolution,
          };
          minimapTilesRef.current.set(key, rec);
          // draw initial unseen background for this tile
          minimapDrawFull(key);
        } else if (rec.resolution !== resolution) {
          // Resolution changed - replace with new tile at new resolution
          if (rec.canvas) {
            delete rec.canvas;
          }
          rec = {
            version: 0,
            data: new Uint8Array(resolution * resolution),
            resolution,
          };
          minimapTilesRef.current.set(key, rec);
          minimapDrawFull(key);
        }
        const incVersion = Number(data.version) || 0;
        if (incVersion <= (rec.version || 0)) return; // idempotent
        const rects = Array.isArray(data.rects) ? data.rects : [];
        for (const r of rects) {
          const x = r.x | 0,
            y = r.y | 0,
            w = r.w | 0,
            h = r.h | 0;
          const bytes = b64ToU8(r.rows || []);
          let src = 0;
          for (let row = 0; row < h; row++) {
            const base = (y + row) * resolution + x;
            rec.data.set(bytes.subarray(src, src + w), base);
            src += w;
          }
          minimapDrawRect(key, x, y, w, h);
        }
        rec.version = incVersion;
      } else if (type === "revealAck") {
        // `requestId` arrives as string (because we set `longs:"String"`).
        const reqRaw = data.requestId ?? data.request_id;
        if (reqRaw == null) {
          if (__DEV__) console.error("RevealAck without requestId", data);
          return;
        }
        const requestId = String(reqRaw);
        const { ok, scoreUpdate, userRank } = data;

        const optimisticAction = optimisticActions.current.get(requestId);
        if (!optimisticAction) return;

        // 1) Revert *all* optimistic changes for this action
        for (const [chunkKey, changes] of optimisticAction.entries()) {
          // revert reveals
          changes.reveals.forEach((cell) => {
            revealedCellsRef.current.delete(`${chunkKey},${cell}`);
          });

          // revert flags
          changes.flags.forEach((cell) => {
            // chunkKey is "cx,cy"
            const [cx, cy] = chunkKey.split(",").map(Number);
            const lx = cell % CHUNK;
            const ly = Math.floor(cell / CHUNK);
            const worldX = cx * CHUNK + lx;
            const worldY = cy * CHUNK + ly;
            flaggedCellsRef.current.delete(`${worldX},${worldY}`);
          });
          // bump version for reverted optimistic changes
          const [bx, by] = chunkKey.split(",").map(Number);
          bumpChunkVersion(bx, by);
        }

        // 2) Clean up optimistic record
        optimisticActions.current.delete(requestId);

        // 3) If the server accepted, apply the canonical change
        if (ok) {
          // protobufjs flattens one-of’s, so we look for the fields directly
          const outcomeType = data.revealedCells
            ? "revealedCells"
            : data.flaggedCell
              ? "flaggedCell"
              : null;
          if (!outcomeType) {
            if (__DEV__) console.warn("RevealAck with unknown outcome", data);
            return;
          }
          const outcome = data[outcomeType];
          // scoreUpdate.chunkId is `{}` when X=0 & Y=0 because pbjs drops
          // default-valued fields.  Re-hydrate it so the rest of the code
          // doesn’t choke.
          const { X, Y } = normalizeChunkId(scoreUpdate.chunkId);
          const primaryChunkKey = `${X},${Y}`;

          if (outcomeType === "revealedCells") {
            // guard against missing or non-array cells
            const cells = Array.isArray(outcome.cells) ? outcome.cells : [];
            for (const cell of cells) {
              const cellKey = `${primaryChunkKey},${cell}`;
              const seed = seedCache.current.get(primaryChunkKey);
              const d = densityCache.current.get(primaryChunkKey);
              const prevCell = revealedCellsRef.current.get(cellKey);
              const isMineVal =
                seed && d != null
                  ? isMineWith(seed, d, cell)
                  : (prevCell?.isMine ?? false);
              let adjacent = isMineVal ? 0 : countAdjacentMines(X, Y, cell);
              if (adjacent === -1 && prevCell && prevCell.adjacentMines >= 0) {
                adjacent = prevCell.adjacentMines;
              }
              if (adjacent === -1)
                registerPendingAdjacency(primaryChunkKey, cell);
              revealedCellsRef.current.set(cellKey, {
                isMine: isMineVal,
                adjacentMines: adjacent,
              });
            }
            bumpChunkVersion(X, Y);
          } else if (outcomeType === "flaggedCell") {
            const cellIdx =
              outcome &&
              typeof outcome === "object" &&
              Number.isFinite(outcome.cell)
                ? outcome.cell
                : outcome;
            const lx = cellIdx % CHUNK;
            const ly = Math.floor(cellIdx / CHUNK);
            const worldX = X * CHUNK + lx;
            const worldY = Y * CHUNK + ly;
            flaggedCellsRef.current.set(
              `${worldX},${worldY}`,
              playerFlagsRef.current.get(username)
            );
            // Also mark the cell as revealed (content suppressed) for continuity/compression-aware logic
            const cellKey = `${primaryChunkKey},${cellIdx}`;
            revealedCellsRef.current.set(cellKey, {
              isMine: false,
              adjacentMines: 0,
              isFlagged: true,
            });
            bumpChunkVersion(X, Y);
          }

          // update score + popup
          setPlayerScore(scoreUpdate.score);
          if (userRank) {
            setUserRank(userRank);
          }
          if (scoreUpdate.delta !== 0) {
            const cell = scoreUpdate.cell;
            const lx = cell % CHUNK;
            const ly = Math.floor(cell / CHUNK);
            const worldX = X * CHUNK + lx;
            const worldY = Y * CHUNK + ly;
            const id = Math.random().toString(36).slice(2);
            setScorePopups((p) => [
              ...p,
              { id, worldX, worldY, delta: scoreUpdate.delta },
            ]);
            setTimeout(
              () => setScorePopups((p) => p.filter((s) => s.id !== id)),
              1000
            );
          }
        }

        // force a re-render
        setTick((t) => t + 1);
      } else if (type === "chunkUpdateBroadcast") {
        const { chunkId, ...update } = data;
        const { X, Y } = normalizeChunkId(chunkId);
        const chunkKey = `${X},${Y}`;
        const updateType = data.revealedCells
          ? "revealedCells"
          : data.flaggedCell
            ? "flaggedCell"
            : null;
        if (!updateType) return; // Or log an error
        const updateData = update[updateType];

        // Flood-fill can spill into a never-seen chunk; fetch its seed so
        // the pending-adjacency recompute can fill in real numbers.
        if (
          !seedCache.current.has(chunkKey) &&
          websocket.readyState === WebSocket.OPEN
        ) {
          websocket.send(
            encodeMsg({ seedRequest: { chunkIds: [{ X, Y }] } })
          );
        }

        for (const [reqId, action] of optimisticActions.current.entries()) {
          if (action.has(chunkKey)) {
            action.delete(chunkKey);
            if (action.size === 0) optimisticActions.current.delete(reqId);
          }
        }

        if (updateType === "revealedCells") {
          // guard against missing or non-array cells
          const cells = Array.isArray(updateData.cells) ? updateData.cells : [];
          for (const cell of cells) {
            const cellKey = `${chunkKey},${cell}`;
            const lX = cell % CHUNK,
              lY = Math.floor(cell / CHUNK);
            const worldKey = `${X * CHUNK + lX},${Y * CHUNK + lY}`;

            const seed = seedCache.current.get(chunkKey);
            const d = densityCache.current.get(chunkKey);
            const prevCell = revealedCellsRef.current.get(cellKey);
            const isMineVal =
              seed && d != null
                ? isMineWith(seed, d, cell)
                : (prevCell?.isMine ?? false);
            let adjacent = isMineVal ? 0 : countAdjacentMines(X, Y, cell);
            // Never downgrade an already-known number to "incomplete"
            if (adjacent === -1 && prevCell && prevCell.adjacentMines >= 0) {
              adjacent = prevCell.adjacentMines;
            }
            if (adjacent === -1) registerPendingAdjacency(chunkKey, cell);

            // Only clear a flag if this cell is NOT a mine. If it's a mine, keep (or later set) the flag.
            if (!isMineVal) {
              flaggedCellsRef.current.delete(worldKey);
            }

            // Mark revealed; if it's currently flagged, carry that through for rendering suppression
            const isFlagged = flaggedCellsRef.current.has(worldKey);
            revealedCellsRef.current.set(cellKey, {
              isMine: isMineVal,
              adjacentMines: adjacent,
              isFlagged,
            });
          }
          bumpChunkVersion(X, Y);
        } else if (updateType === "flaggedCell") {
          const lX = updateData.cell % CHUNK,
            lY = Math.floor(updateData.cell / CHUNK);
          const wX = X * CHUNK + lX,
            wY = Y * CHUNK + lY;
          flaggedCellsRef.current.set(`${wX},${wY}`, updateData.flagID ?? 0);
          // Also mark as revealed (content suppressed)
          const cellKey = `${chunkKey},${updateData.cell}`;
          revealedCellsRef.current.set(cellKey, {
            isMine: false,
            adjacentMines: 0,
            isFlagged: true,
          });
          bumpChunkVersion(X, Y);
        }

        setTick((t) => t + 1);
      } else if (type === "advancementSync") {
        const stats = data.stats || {};
        // uint64 fields arrive as strings (longs: String); normalize
        const norm = {};
        for (const [k, v] of Object.entries(stats)) norm[k] = Number(v) || 0;
        setAdvStats(norm);
        setUnlockedAdvIds(
          Array.isArray(data.unlockedIds) ? data.unlockedIds : []
        );
        setUnlockedFlagIds(
          Array.isArray(data.unlockedFlagIds)
            ? data.unlockedFlagIds.map(Number)
            : []
        );
      } else if (type === "advancementUnlocked") {
        const id = data.id;
        if (!id) return;
        const rewardFlagId = Number(data.rewardFlagId) || 0;
        setUnlockedAdvIds((prev) =>
          prev.includes(id) ? prev : [...prev, id]
        );
        if (rewardFlagId) {
          setUnlockedFlagIds((prev) =>
            prev.includes(rewardFlagId) ? prev : [...prev, rewardFlagId]
          );
        }
        const toastId = Math.random().toString(36).slice(2);
        setAdvToasts((prev) => [...prev, { toastId, id, rewardFlagId }]);
        setTimeout(() => {
          setAdvToasts((prev) => prev.filter((t) => t.toastId !== toastId));
        }, 5000);
      } else if (type === "cellOwnerResponse") {
        const { X, Y } = normalizeChunkId(data.chunkId);
        const cell = Number(data.cell || 0) | 0;
        const lx = cell % CHUNK;
        const ly = Math.floor(cell / CHUNK);
        const name = data.name || "";
        pushHintPopup(
          X * CHUNK + lx,
          Y * CHUNK + ly,
          name ? `⚑ ${name}` : "⚑ placed before names were tracked"
        );
      } else if (type === "seedResponse") {
        // Handle seed response for pre-emptive caching
        const seeds = Array.isArray(data.seeds) ? data.seeds : [];
        let adjacencyChanged = false;
        for (const seedData of seeds) {
          if (seedData.chunkId && seedData.seed && seedData.density != null) {
            const { X, Y } = normalizeChunkId(seedData.chunkId);
            const chunkKey = `${X},${Y}`;
            // Convert base64 seed to bytes if needed
            const seedBytes = b64ToU8(seedData.seed);
            const seedBigInt = new DataView(
              seedBytes.buffer,
              seedBytes.byteOffset,
              8
            ).getBigUint64(0, true);
            seedCache.current.set(chunkKey, seedBigInt);
            densityCache.current.set(chunkKey, seedData.density);
            if (recomputeAdjacencyAround(X, Y)) adjacencyChanged = true;
          }
        }
        if (adjacencyChanged) setTick((t) => t + 1);
      } else if (type === "leaderboard") {
        const entries = Array.isArray(data.entries) ? data.entries : [];
        // De-duplicate by name on the client defensively; keep the highest score
        const byName = new Map();
        for (const e of entries) {
          const prev = byName.get(e.name);
          if (!prev || (e.score ?? 0) > (prev.score ?? 0))
            byName.set(e.name, e);
        }
        // Use stable sorting to match server-side behavior
        const uniqueEntries = Array.from(byName.values()).sort((a, b) => {
          if (a.score !== b.score) {
            return b.score - a.score;
          }
          return a.name.localeCompare(b.name);
        });
        setLeaderboard(uniqueEntries);
        playerFlagsRef.current.clear();
        for (const entry of uniqueEntries) {
          playerFlagsRef.current.set(entry.name, entry.flagID);
        }
      }
    };

    return () => websocket.close();
  }, [
    countAdjacentMines,
    registerPendingAdjacency,
    recomputeAdjacencyAround,
    pushHintPopup,
  ]);

  // Join the game by sending a Join message (transitions from SPECTATOR to PLAYER)
  const joinGame = useCallback(
    (nameInput, flagID) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) {
        console.error("Cannot join: WebSocket not connected");
        return;
      }

      const sessionToken = localStorage.getItem("session_token") || "";
      ws.send(
        encodeMsg({
          join: {
            sessionToken,
            name: nameInput,
            flagID,
          },
        })
      );
    },
    [ws]
  );

  // Update profile while in PLAYER state
  const updateProfile = useCallback(
    (nameInput, flagID) => {
      if (!ws || ws.readyState !== WebSocket.OPEN || !connected) {
        console.error("Cannot update profile: not connected as player");
        return;
      }

      setUpdateError(""); // Clear any previous errors
      setUpdateSuccess(false); // Clear any previous success
      ws.send(
        encodeMsg({
          updateProfile: {
            name: nameInput,
            flagID,
          },
        })
      );
    },
    [ws, connected]
  );

  const disconnect = useCallback(() => {
    try {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.close();
      }
    } catch {
      // ignore
    }
  }, [ws]);

  // Handle page visibility changes for mobile reconnection
  const handleVisibilityChange = useCallback(() => {
    if (document.visibilityState === "visible") {
      // Page became visible - check if we need to reconnect
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
        // Only reconnect if we have stored credentials (user was previously connected)
        const storedUsername = localStorage.getItem("username");
        const storedToken = localStorage.getItem("session_token");
        if (storedUsername && storedToken) {
          if (__DEV__)
            console.log("Page became visible - attempting reconnection");
          connectWebSocket();
        }
      }
    }
  }, [connectWebSocket]);

  // Expose wsRef to window for testing
  if (__DEV__ && typeof window !== "undefined") {
    window.wsRef = wsRef;
  }

  return {
    connected,
    playerScore,
    userRank,
    username,
    setUsername,
    leaderboard,
    scorePopups,
    hintPopups,
    tick,
    setTick,
    seedCache,
    revealedCellsRef,
    flaggedCellsRef,
    playerFlagsRef,
    chunkVersionRef,
    handleCellClick,
    connectWebSocket,
    joinGame,
    updateProfile,
    updateError,
    updateSuccess,
    joinError,
    serverFlagID,
    disconnect,
    // Advancements
    advStats,
    unlockedAdvIds,
    unlockedFlagIds,
    advToasts,
    worldToChunk,
    // expose caches for minimap and other consumers
    densityCache,
    // expose throttled view-update sender to App.jsx
    sendViewUpdateRef: sendViewUpdate,
    // expose visibility handler for App.jsx to set up listener
    handleVisibilityChange,
    // Minimap streaming
    minimapTilesRef,
    updateMinimapSubscriptions: useCallback(
      (
        centerWorldX,
        centerWorldY,
        widthCells,
        heightCells,
        marginTiles = 1,
        sourceKey = "default",
        resolution = 64
      ) => {
        const s = wsRef.current;
        if (!s || s.readyState !== WebSocket.OPEN) return;

        // Check if resolution changed significantly - request new tiles but keep old ones visible
        const prevResolution = currentResolutionRef.current;
        if (resolution !== prevResolution) {
          currentResolutionRef.current = resolution;

          // Force re-subscription of all active tiles at new resolution
          // But keep the old tiles visible until new ones arrive
          if (minimapActiveSubsRef.current.size > 0) {
            const allTiles = Array.from(minimapActiveSubsRef.current).map(
              (k) => {
                const [x, y] = k.split(",").map(Number);
                return { x, y };
              }
            );
            s.send(
              encodeMsg({ minimapSubscribe: { tiles: allTiles, resolution } })
            );
          }
        }

        const tilesWide = Math.ceil(widthCells / CHUNK) + marginTiles * 2;
        const tilesHigh = Math.ceil(heightCells / CHUNK) + marginTiles * 2;
        const centerChunkX = Math.floor(centerWorldX / CHUNK);
        const centerChunkY = Math.floor(centerWorldY / CHUNK);
        const halfW = Math.floor(tilesWide / 2);
        const halfH = Math.floor(tilesHigh / 2);
        const startX = centerChunkX - halfW;
        const startY = centerChunkY - halfH;

        // Compute desired set for this source (always, even if socket closed)
        const desiredForSource = new Set();
        for (let y = 0; y < tilesHigh; y++) {
          for (let x = 0; x < tilesWide; x++) {
            desiredForSource.add(`${startX + x},${startY + y}`);
          }
        }
        minimapDesiredBySourceRef.current.set(sourceKey, desiredForSource);

        // Compute union of all sources
        const union = new Set();
        for (const set of minimapDesiredBySourceRef.current.values()) {
          for (const k of set) union.add(k);
        }

        // Diff against active set we have on the server
        const toAdd = [];
        const toDel = [];
        for (const k of union)
          if (!minimapActiveSubsRef.current.has(k)) toAdd.push(k);
        for (const k of minimapActiveSubsRef.current)
          if (!union.has(k)) toDel.push(k);

        if (toAdd.length) {
          const tiles = toAdd.map((k) => {
            const [x, y] = k.split(",").map(Number);
            return { x, y };
          });
          s.send(encodeMsg({ minimapSubscribe: { tiles, resolution } }));
          toAdd.forEach((k) => minimapActiveSubsRef.current.add(k));
        }
        if (toDel.length) {
          const tiles = toDel.map((k) => {
            const [x, y] = k.split(",").map(Number);
            return { x, y };
          });
          s.send(encodeMsg({ minimapUnsubscribe: { tiles } }));
          toDel.forEach((k) => minimapActiveSubsRef.current.delete(k));
        }
      },
      []
    ),
    clearMinimapSubscriptionsFor: useCallback((sourceKey = "default") => {
      // Remove this source's desired set locally first
      minimapDesiredBySourceRef.current.delete(sourceKey);

      const s = wsRef.current;
      if (!s || s.readyState !== WebSocket.OPEN) return; // will reconcile on next open/call

      // Recompute union
      const union = new Set();
      for (const set of minimapDesiredBySourceRef.current.values()) {
        for (const k of set) union.add(k);
      }
      // Anything active but not in union should be unsubscribed
      const toDel = [];
      for (const k of minimapActiveSubsRef.current)
        if (!union.has(k)) toDel.push(k);
      if (toDel.length) {
        const tiles = toDel.map((k) => {
          const [x, y] = k.split(",").map(Number);
          return { x, y };
        });
        s.send(encodeMsg({ minimapUnsubscribe: { tiles } }));
        toDel.forEach((k) => minimapActiveSubsRef.current.delete(k));
      }
    }, []),
    // Server-suggested spawn location
    serverSpawnRef,
    // Nearby player positions for rendering
    activePlayersRef,
  };
};
