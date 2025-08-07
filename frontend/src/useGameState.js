import { useState, useRef, useCallback } from "react";
import pako from "pako";
import { ms as PB } from "./gen/messages_pb.js";

const log = __DEV__ ? console.log.bind(console) : () => {};

// Robust one-of detector: ignore numeric placeholders & legacy “payload”
const activeKey = (m) =>
  Object.keys(m)
    .filter(
      (k) =>
        k !== "constructor" &&
        k !== "$type" &&
        k !== "payload" &&
        isNaN(Number(k)) &&
        m[k] != null
    )[0];

const normalizeChunkId = (cid = {}) => ({ X: cid.X ?? 0, Y: cid.Y ?? 0 });

export const CHUNK = 64;
const MINE_COUNT = 20;

function encodeMsg(msg) {
  // unwrap legacy `{ payload:{ … } }` objects
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
    defaults: false,
  });

  // Un-wrap `{ payload:{ … } }` envelope (older builds)
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

// Deterministic helpers
const splitmix64 = (state) => {
  state = (state + 0x9e3779b97f4a7c15n) & 0xffffffffffffffffn;
  state =
    ((state ^ (state >> 30n)) * 0xbf58476d1ce4e5b9n) & 0xffffffffffffffffn;
  state =
    ((state ^ (state >> 27n)) * 0x94d049bb133111ebn) & 0xffffffffffffffffn;
  return state ^ (state >> 31n);
};

const isMine = (seed, cell) => {
  if (!Number.isFinite(cell)) return false;
  const cellSeed = splitmix64(seed + BigInt(Math.trunc(cell)));
  return Number(cellSeed % 100n) < MINE_COUNT;
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
  const storedName = localStorage.getItem("username") || "";

  const [ws, setWs] = useState(null);
  const [connected, setConnected] = useState(false);
  const [leaderboard, setLeaderboard] = useState([]);
  const [playerScore, setPlayerScore] = useState(0);
  const [username, setUsername] = useState(storedName);
  const [scorePopups, setScorePopups] = useState([]);
  const [tick, setTick] = useState(0);

  // Game state refs
  const seedCache = useRef(new Map());
  const subscribedChunks = useRef(new Set());
  const subscriptionOrder = useRef([]);
  const revealedCellsRef = useRef(new Map());
  const flaggedCellsRef = useRef(new Map());
  const playerFlagsRef = useRef(new Map());
  const optimisticActions = useRef(new Map());

  const countAdjacentMines = useCallback((cx, cy, cell) => {
    const x = cell % CHUNK;
    const y = Math.floor(cell / CHUNK);
    const worldX = cx * CHUNK + x;
    const worldY = cy * CHUNK + y;

    let count = 0;
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        if (dx === 0 && dy === 0) continue;
        const {
          chunkX: ncx,
          chunkY: ncy,
          cell: ncell,
        } = worldToChunk(worldX + dx, worldY + dy);
        const nSeed = seedCache.current.get(`${ncx},${ncy}`);
        if (!nSeed) continue;
        if (isMine(nSeed, ncell)) count++;
      }
    }
    return count;
  }, []);

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
  const canChord = useCallback((worldX, worldY) => {
    const placedFlags = countAdjacentFlags(worldX, worldY);
    let revealedMines = 0;

    // Count revealed mines in the 8 adjacent cells
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        if (dx === 0 && dy === 0) continue;
        const wx = worldX + dx, wy = worldY + dy;
        const { chunkX: cx, chunkY: cy, cell: c } = worldToChunk(wx, wy);
        const key = `${cx},${cy},${c}`;
        const data = revealedCellsRef.current.get(key);
        if (data?.isMine) revealedMines++;
      }
    }

    const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
    const expectedMines = countAdjacentMines(chunkX, chunkY, cell);

    return placedFlags + revealedMines === expectedMines;
  }, [countAdjacentFlags, countAdjacentMines]);

  const ensureChunkSubscription = useCallback(
    (chunkX, chunkY) => {
      const key = `${chunkX},${chunkY}`;
      if (
        !subscribedChunks.current.has(key) &&
        ws &&
        connected &&
        ws.readyState === WebSocket.OPEN
      ) {
        // FIFO with hard cap
        subscribedChunks.current.add(key);
        subscriptionOrder.current.push(key);

        while (subscriptionOrder.current.length > 50) {
          const oldest = subscriptionOrder.current.shift();
          subscribedChunks.current.delete(oldest);
          const [ox, oy] = oldest.split(",").map(Number);
          ws.send(encodeMsg({ unsubscribe: { chunkId: { X: ox, Y: oy } } }));
        }
        ws.send(encodeMsg({ subscribe: { chunkId: { X: chunkX, Y: chunkY } } }));
      }
    },
    [ws, connected],
  );

  const ensureChunkUnsubscription = useCallback(
    (chunkX, chunkY) => {
      const key = `${chunkX},${chunkY}`;
      if (subscribedChunks.current.has(key) && ws && connected) {
        subscribedChunks.current.delete(key);
        subscriptionOrder.current = subscriptionOrder.current.filter(
          (k) => k !== key,
        );
        ws.send(
          encodeMsg({ unsubscribe: { chunkId: { X: chunkX, Y: chunkY } } }),
        );
      }
    },
    [ws, connected],
  );

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
      if (flaggedCellsRef.current.has(flagKey)) return;

      // Only allow interactions near already revealed cells to encourage
      // connected exploration. Client-side guard mirrors server validation.
      let nearRevealed = false;
      for (let dy = -2; dy <= 2 && !nearRevealed; dy++) {
        for (let dx = -2; dx <= 2; dx++) {
          const { chunkX: nx, chunkY: ny, cell: ncell } = worldToChunk(worldX + dx, worldY + dy);
          if (revealedCellsRef.current.has(`${nx},${ny},${ncell}`)) {
            nearRevealed = true;
            break;
          }
        }
      }
      if (!nearRevealed) return;

      const requestId = Date.now().toString();
      const optimisticChanges = new Map();
      optimisticActions.current.set(requestId, optimisticChanges);
      let didLocalMutation = false;          // track if we really changed stuff

      const recordChange = (cKey, change) => {
        if (!optimisticChanges.has(cKey)) {
            optimisticChanges.set(cKey, { reveals: new Set(), flags: new Set() });
        }
        if (change.type === "reveal") optimisticChanges.get(cKey).reveals.add(change.cell);
        if (change.type === "flag")   optimisticChanges.get(cKey).flags.add(change.cell);
        didLocalMutation = true;
      };

      if (isRightClick) {
        const myFlagId = playerFlagsRef.current.get(username);
        if (myFlagId !== undefined) {
          flaggedCellsRef.current.set(flagKey, myFlagId);
          recordChange(chunkKey, {type: 'flag', cell});
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
          if (revealedCellsRef.current.has(cellKey) || visited.has(cellKey)) continue;
          visited.add(cellKey);

          const seed = seedCache.current.get(chunkKey);
          if (!seed) continue;

          // reveal
          const isM = isMine(seed, cidx);
          const adj = isM ? 0 : countAdjacentMines(cx, cy, cidx);
          revealedCellsRef.current.set(cellKey, { isMine: isM, adjacentMines: adj });
          recordChange(chunkKey, { type: 'reveal', cell: cidx });

          // if zero, expand around it
          if (!isM && adj === 0) {
            for (let dy2 = -1; dy2 <= 1; dy2++) {
              for (let dx2 = -1; dx2 <= 1; dx2++) {
                if (dx2 === 0 && dy2 === 0) continue;
                queue.push({ x: wx + dx2, y: wy + dy2 });
              }
            }
          }
        }
      } else { // Standard reveal
        const seed = seedCache.current.get(chunkKey);
        if (seed) {
          if (isMine(seed, cell)) {
            revealedCellsRef.current.set(cellKey, { isMine: true, adjacentMines: 0 });
          } else {
            const adjacent = countAdjacentMines(chunkX, chunkY, cell);
            revealedCellsRef.current.set(cellKey, { isMine: false, adjacentMines: adjacent });
          }
          recordChange(chunkKey, { type: 'reveal', cell });
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
        ws.send(encodeMsg({ reveal: {
            requestId,
            chunkId: { X: chunkX, Y: chunkY },
            cell,
            isRightClick,
            isChord,
        }}));
      }
    },
    [ws, connected, countAdjacentMines, countAdjacentFlags, canChord, username],
  );

  const connectWs = useCallback(
    (nameInput, flagID) => {
      const wsUrl = `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;
      const websocket = new WebSocket(wsUrl);
      websocket.binaryType = "arraybuffer";

      websocket.onopen = () => {
        const sessionToken = localStorage.getItem("session_token") || "";
        websocket.send(encodeMsg({ hello: { sessionToken, name: nameInput, flagID } }));
        setConnected(true);
        setWs(websocket);
      };

      websocket.onclose = () => {
        setConnected(false);
        setWs(null);
        subscribedChunks.current.clear();
        optimisticActions.current.clear();
        revealedCellsRef.current.clear();
        flaggedCellsRef.current.clear();
        playerFlagsRef.current.clear();
      };

      websocket.onmessage = (event) => {
        const msg = decodeMsg(event.data);
        const type = activeKey(msg);
        const data = msg[type];

        if (type === "welcome") {
          localStorage.setItem("session_token", data.sessionToken);
          localStorage.setItem("username", data.name || "");
          localStorage.setItem("score", String(data.score));
          localStorage.setItem("flagID", String(data.flagID));
          playerFlagsRef.current.set(data.name, data.flagID);
          setUsername(data.name || "");
          setPlayerScore(data.score);
        } else if (type === "chunkSync") {
          const { chunkId, seed, reveals, flagGroups: fgRaw } = data;
          const { X, Y } = normalizeChunkId(chunkId);
          const chunkKey = `${X},${Y}`;
          const seedBigInt = new DataView(seed.buffer, seed.byteOffset, 8).getBigUint64(0, true);
          seedCache.current.set(chunkKey, seedBigInt);

          // `flagGroups` might be `undefined` when the chunk has no flags
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

          const view = new DataView(reveals.buffer, reveals.byteOffset, reveals.byteLength);
          for (let i = 0; i < CHUNK * CHUNK; i++) {
            const wordIndex = Math.floor(i / CHUNK);
            const bitIndex = i % CHUNK;
            if ((view.getBigUint64(wordIndex * 8, true) & (1n << BigInt(bitIndex))) !== 0n) {
              const cellKey = `${X},${Y},${i}`;
              const isMineVal = isMine(seedBigInt, i);
              const adjacent = isMineVal ? 0 : countAdjacentMines(X, Y, i);
              revealedCellsRef.current.set(cellKey, { isMine: isMineVal, adjacentMines: adjacent });
            }
          }
          setTick(t => t + 1);
        } else if (type === "revealAck") {
          // `requestId` arrives as string (because we set `longs:"String"`).
          const reqRaw = data.requestId ?? data.request_id;
          if (reqRaw == null) {
            if (__DEV__) console.error("RevealAck without requestId", data);
            return;
          }
          const requestId = String(reqRaw);
          const { ok, scoreUpdate } = data;

          const optimisticAction = optimisticActions.current.get(requestId);
          if (!optimisticAction) return;

          // 1) Revert *all* optimistic changes for this action
          for (const [chunkKey, changes] of optimisticAction.entries()) {
            // revert reveals
            changes.reveals.forEach(cell => {
              revealedCellsRef.current.delete(`${chunkKey},${cell}`);
            });

            // revert flags
            changes.flags.forEach(cell => {
              // chunkKey is "cx,cy"
              const [cx, cy] = chunkKey.split(",").map(Number);
              const lx = cell % CHUNK;
              const ly = Math.floor(cell / CHUNK);
              const worldX = cx * CHUNK + lx;
              const worldY = cy * CHUNK + ly;
              flaggedCellsRef.current.delete(`${worldX},${worldY}`);
            });
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
                const isMineVal = seed ? isMine(seed, cell) : false;
                const adjacent = isMineVal
                  ? 0
                  : countAdjacentMines(X, Y, cell);
                revealedCellsRef.current.set(cellKey, {
                  isMine: isMineVal,
                  adjacentMines: adjacent,
                });
              }
            } else if (outcomeType === "flaggedCell") {
              const cell = outcome; // single uint32
              const lx = cell % CHUNK;
              const ly = Math.floor(cell / CHUNK);
              const worldX = X * CHUNK + lx;
              const worldY = Y * CHUNK + ly;
              flaggedCellsRef.current.set(
                `${worldX},${worldY}`,
                playerFlagsRef.current.get(username)
              );
            }

            // update score + popup
            setPlayerScore(scoreUpdate.score);
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
            const updateType = data.revealedCells ? "revealedCells" : data.flaggedCell ? "flaggedCell" : null;
            if (!updateType) return; // Or log an error
            const updateData = update[updateType];

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
                    // Remove any optimistic flag before revealing
                    const lX = cell % CHUNK, lY = Math.floor(cell / CHUNK);
                    flaggedCellsRef.current.delete(
                        `${X * CHUNK + lX},${Y * CHUNK + lY}`
                    );

                    const seed = seedCache.current.get(chunkKey);
                    const isMineVal = seed ? isMine(seed, cell) : false;
                    const adjacent = isMineVal
                        ? 0
                        : countAdjacentMines(X, Y, cell);
                    revealedCellsRef.current.set(cellKey, { isMine: isMineVal, adjacentMines: adjacent });
                }
            } else if (updateType === "flaggedCell") {
                const lX = updateData.cell % CHUNK, lY = Math.floor(updateData.cell / CHUNK);
                const wX = X * CHUNK + lX, wY = Y * CHUNK + lY;
                flaggedCellsRef.current.set(`${wX},${wY}`, updateData.flagID ?? 0);
            }

            setTick(t => t+1);
        } else if (type === "leaderboard") {
            const entries = Array.isArray(data.entries) ? data.entries : [];
            setLeaderboard(entries.length ? entries.sort((a, b) => b.score - a.score) : []);
            playerFlagsRef.current.clear();
            for (const entry of entries) {
                playerFlagsRef.current.set(entry.name, entry.flagID);
            }
        }
      };

      return () => websocket.close();
    },
    [countAdjacentMines, username],
  );

  return {
    connected,
    playerScore,
    username,
    setUsername,
    leaderboard,
    scorePopups,
    tick,
    seedCache,
    subscribedChunks,
    revealedCellsRef,
    flaggedCellsRef,
    playerFlagsRef,
    handleCellClick,
    ensureChunkSubscription,
    ensureChunkUnsubscription,
    connectWs,
    worldToChunk,
    isMine,
  };
};
