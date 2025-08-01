import { useState, useRef, useCallback } from "react";
import pako from "pako";
import { ms as PB } from "./gen/messages_pb.js";

const log = __DEV__ ? console.log.bind(console) : () => {};

export const CHUNK = 64;
const COMPRESS_THRESHOLD = 50;
const MINE_COUNT = 20;

function encodeMsg(msg) {
  const buf = PB.Msg.encode(msg).finish();
  if (__DEV__) {
    console.log("OUTGOING:", {
      raw: msg,
      serialized_size: buf.length,
      message_type: Object.keys(msg)[0],
    });
  }
  if (buf.length < COMPRESS_THRESHOLD) {
    return buf;
  }
  return pako.gzip(buf);
}

function decodeMsg(data) {
  let bytes = new Uint8Array(data);
  if (bytes.length > 2 && bytes[0] === 0x1f && bytes[1] === 0x8b) {
    bytes = pako.ungzip(bytes);
  }
  const decoded = PB.Msg.decode(bytes);
  if (__DEV__) {
    console.log("INCOMING:", {
      raw: decoded,
      compressed_size: data.byteLength,
      decompressed_size: bytes.length,
      message_type: Object.keys(decoded)[0],
    });
  }
  return decoded;
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

const isMine = (seed, x, y) => {
  const cellSeed = splitmix64(seed + BigInt(y * CHUNK + x));
  return Number(cellSeed % 100n) < MINE_COUNT;
};

// Convert world coordinates to chunk and local coordinates
const worldToChunk = (worldX, worldY) => {
  const chunkX = Math.floor(worldX / CHUNK);
  const chunkY = Math.floor(worldY / CHUNK);

  let localX = worldX - chunkX * CHUNK;
  let localY = worldY - chunkY * CHUNK;

  if (localX < 0) localX += CHUNK;
  if (localY < 0) localY += CHUNK;

  return { chunkX, chunkY, localX, localY };
};

export const useGameState = () => {
  const storedId = parseInt(localStorage.getItem("playerId") || "0", 10);
  const storedName = localStorage.getItem("username") || "";

  const [ws, setWs] = useState(null);
  const [connected, setConnected] = useState(false);
  const [leaderboard, setLeaderboard] = useState([]);
  const [playerId, setPlayerId] = useState(storedId);
  const [playerScore, setPlayerScore] = useState(0);
  const [username, setUsername] = useState(storedName);
  const [scorePopups, setScorePopups] = useState([]);
  const [tick, setTick] = useState(0);

  // Game state refs
  const seedCache = useRef(new Map());
  const subscribedChunks = useRef(new Set());
  const revealedCellsRef = useRef(new Map());
  const flaggedCellsRef = useRef(new Map());
  const playerFlagsRef  = useRef(new Map());

  const countAdjacentMines = useCallback(async (cx, cy, x, y) => {
    let count = 0;
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        if (dx === 0 && dy === 0) continue;
        let nx = x + dx;
        let ny = y + dy;
        let ncx = cx;
        let ncy = cy;
        if (nx < 0) {
          ncx--;
          nx += CHUNK;
        } else if (nx >= CHUNK) {
          ncx++;
          nx -= CHUNK;
        }
        if (ny < 0) {
          ncy--;
          ny += CHUNK;
        } else if (ny >= CHUNK) {
          ncy++;
          ny -= CHUNK;
        }

        const nSeed = seedCache.current.get(`${ncx},${ncy}`);
        if (!nSeed) continue;
        if (isMine(nSeed, nx, ny)) count++;
      }
    }
    return count;
  }, []);

  const ensureChunkSubscription = useCallback(
    (chunkX, chunkY) => {
      const key = `${chunkX},${chunkY}`;
      if (!subscribedChunks.current.has(key) && ws && connected) {
        subscribedChunks.current.add(key);
        ws.send(encodeMsg(PB.Msg.create({ subscribe: { chunkX, chunkY } })));
      }
    },
    [ws, connected],
  );

  const ensureChunkUnsubscription = useCallback(
    (chunkX, chunkY) => {
      const key = `${chunkX},${chunkY}`;
      if (subscribedChunks.current.has(key) && ws && connected) {
        subscribedChunks.current.delete(key);
        ws.send(encodeMsg(PB.Msg.create({ unsubscribe: { chunkX, chunkY } })));
      }
    },
    [ws, connected],
  );

  const floodFillReveal = useCallback(
    async (startWorldX, startWorldY) => {
      const toReveal = new Set();
      const visited = new Set();
      const queue = [{ worldX: startWorldX, worldY: startWorldY }];

      while (queue.length > 0) {
        const { worldX, worldY } = queue.shift();
        const coordKey = `${worldX},${worldY}`;

        if (visited.has(coordKey)) continue;
        visited.add(coordKey);

        const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;

        if (revealedCellsRef.current.has(cellKey)) continue;

        let seed = seedCache.current.get(`${chunkX},${chunkY}`);
        if (!seed) continue;

        if (isMine(seed, localX, localY)) continue;

        const adjacentMines = await countAdjacentMines(
          chunkX,
          chunkY,
          localX,
          localY,
        );

        toReveal.add({
          worldX,
          worldY,
          chunkX,
          chunkY,
          localX,
          localY,
          adjacentMines,
          cellKey,
        });

        if (adjacentMines === 0) {
          for (let dy = -1; dy <= 1; dy++) {
            for (let dx = -1; dx <= 1; dx++) {
              if (dx === 0 && dy === 0) continue;
              const neighborWorldX = worldX + dx;
              const neighborWorldY = worldY + dy;
              const neighborCoordKey = `${neighborWorldX},${neighborWorldY}`;

              if (!visited.has(neighborCoordKey)) {
                queue.push({ worldX: neighborWorldX, worldY: neighborWorldY });
              }
            }
          }
        }
      }

      for (const cell of toReveal) {
        const optimisticReveal = {
          chunkId: { X: cell.chunkX, Y: cell.chunkY },
          x: cell.localX,
          y: cell.localY,
          playerId: -1,
          isMine: false,
          adjacentMines: cell.adjacentMines,
        };

        revealedCellsRef.current.set(cell.cellKey, optimisticReveal);
        ensureChunkSubscription(cell.chunkX, cell.chunkY);
      }

      setTick((t) => t + 1);
      return toReveal.size;
    },
    [countAdjacentMines, ensureChunkSubscription],
  );

  const handleCellClick = useCallback(
    async (worldX, worldY, isRightClick = false) => {
      if (!ws || !connected) return;
      if (__DEV__) console.log("CELL CLICK:", { worldX, worldY, isRightClick });

      const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
      const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
      const flagKey = `${worldX},${worldY}`;

      if (isRightClick) {
        if (__DEV__)
          console.log("ATTEMPTING TO FLAG:", {
            chunkX,
            chunkY,
            localX,
            localY,
          });
        if (flaggedCellsRef.current.has(flagKey)) return;
        if (revealedCellsRef.current.has(cellKey)) return;

        ws.send(
          encodeMsg(
            PB.Msg.create({
              flag: { chunkId: { X: chunkX, Y: chunkY }, x: localX, y: localY },
            }),
          ),
        );
        setTick((t) => t + 1);
        return;
      }

      const revealedCell = revealedCellsRef.current.get(cellKey);

      // Chording logic
      if (
        revealedCell &&
        !revealedCell.isMine &&
        revealedCell.adjacentMines > 0
      ) {
        let flagCount = 0;
        const cellsToReveal = [];

        for (let dy = -1; dy <= 1; dy++) {
          for (let dx = -1; dx <= 1; dx++) {
            if (dx === 0 && dy === 0) continue;

            const neighborWorldX = worldX + dx;
            const neighborWorldY = worldY + dy;
            const neighborFlagKey = `${neighborWorldX},${neighborWorldY}`;

            const {
              chunkX: nChunkX,
              chunkY: nChunkY,
              localX: nLocalX,
              localY: nLocalY,
            } = worldToChunk(neighborWorldX, neighborWorldY);
            const neighborCellKey = `${nChunkX},${nChunkY},${nLocalX},${nLocalY}`;
            const neighborRevealed =
              revealedCellsRef.current.get(neighborCellKey);

            if (
              flaggedCellsRef.current.has(neighborFlagKey) ||
              (neighborRevealed && neighborRevealed.isMine)
            ) {
              flagCount++;
            } else if (!neighborRevealed) {
              cellsToReveal.push({
                worldX: neighborWorldX,
                worldY: neighborWorldY,
              });
            }
          }
        }

        if (flagCount === revealedCell.adjacentMines) {
          for (const cell of cellsToReveal) {
            handleCellClick(cell.worldX, cell.worldY, false);
          }
        }
        return;
      }

      ensureChunkSubscription(chunkX, chunkY);

      const key = `${chunkX},${chunkY},${localX},${localY}`;
      if (revealedCellsRef.current.has(key)) return;
      if (flaggedCellsRef.current.has(flagKey)) return;

      let seed = seedCache.current.get(`${chunkX},${chunkY}`);
      if (!seed) return;

      if (isMine(seed, localX, localY)) {
        const optimisticReveal = {
          chunkId: { X: chunkX, Y: chunkY },
          x: localX,
          y: localY,
          playerId: -1,
          isMine: true,
          adjacentMines: 0,
        };

        revealedCellsRef.current.set(key, optimisticReveal);
        setTick((t) => t + 1);

        ws.send(
          encodeMsg(
            PB.Msg.create({
              reveal: {
                chunkId: { X: chunkX, Y: chunkY },
                x: localX,
                y: localY,
              },
            }),
          ),
        );
        return;
      }

      const adjacent = await countAdjacentMines(chunkX, chunkY, localX, localY);

      if (adjacent === 0) {
        const revealedCount = await floodFillReveal(worldX, worldY);
        log(`Flood fill revealed ${revealedCount} cells`);

        ws.send(
          encodeMsg(
            PB.Msg.create({
              reveal: {
                chunkId: { X: chunkX, Y: chunkY },
                x: localX,
                y: localY,
                flow: true,
              },
            }),
          ),
        );
      } else {
        const optimisticReveal = {
          chunkId: { X: chunkX, Y: chunkY },
          x: localX,
          y: localY,
          playerId: -1,
          isMine: false,
          adjacentMines: adjacent,
        };

        revealedCellsRef.current.set(key, optimisticReveal);
        setTick((t) => t + 1);

        ws.send(
          encodeMsg(
            PB.Msg.create({
              reveal: {
                chunkId: { X: chunkX, Y: chunkY },
                x: localX,
                y: localY,
              },
            }),
          ),
        );
      }
    },
    [
      ws,
      connected,
      ensureChunkSubscription,
      countAdjacentMines,
      floodFillReveal,
    ],
  );

  const connectWs = useCallback(
    (nameInput, flagID) => {
      const wsUrl = `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;
      const websocket = new WebSocket(wsUrl);
      websocket.binaryType = "arraybuffer";

      websocket.onopen = () => {
        const msg = PB.Msg.create({
          hello: { playerId, name: nameInput, flagID },
        });
        websocket.send(encodeMsg(msg));
        setConnected(true);
        setWs(websocket);
      };

      websocket.onclose = () => {
        setConnected(false);
        setWs(null);
      };

      websocket.onmessage = (event) => {
        const m = decodeMsg(event.data);

        if (__DEV__) {
          console.log("PROCESSING MESSAGE:", {
            type: Object.keys(m)[0],
            payload: m,
          });
        }

        let data = {};
        if (m.welcome) {
          data = {
            type: "welcome",
            playerId: m.welcome.playerId,
            name: m.welcome.name,
            flagID: m.welcome.flagID,
            viewX: m.welcome.viewX,
            viewY: m.welcome.viewY,
          };
        } else if (m.chunkSync) {
          data = {
            type: "chunkSync",
            chunkId: m.chunkSync.chunkId,
            seed: m.chunkSync.seed,
            reveals: m.chunkSync.reveals,
            flags: m.chunkSync.flagGroups,
          };
        } else if (m.revealAck) {
          data = { type: "revealAck", ok: m.revealAck.ok };
        } else if (m.flagAck) {
          data = { type: "flagAck", ok: m.flagAck.ok };
        } else if (m.leaderboard) {
          data = {
            type: "leaderboard",
            version: m.leaderboard.version,
            entries: m.leaderboard.entries,
          };
        } else if (m.scoreUpdate) {
          data = {
            type: "scoreUpdate",
            score: m.scoreUpdate.score,
            delta: m.scoreUpdate.delta,
            worldX: m.scoreUpdate.worldX,
            worldY: m.scoreUpdate.worldY,
          };
        } else if (m.flag) {
          data = { ...m.flag, chunkId: m.flag.chunkId };
        } else if (m.reveal) {
          data = { ...m.reveal, chunkId: m.reveal.chunkId };
        }

        // Handle different message types
        if (data.type === "welcome") {
          setPlayerId(data.playerId);
          setUsername(data.name || "");
          playerFlagsRef.current.set(data.playerId, data.flagID);
          localStorage.setItem("playerId", data.playerId);
          localStorage.setItem("username", data.name || "");
          return;
        }

        if (data.type === "chunkSync") {
          const bytesToBig = (u8) =>
            new DataView(u8.buffer, u8.byteOffset, 8).getBigUint64(0, true);

          const key = `${data.chunkId.X},${data.chunkId.Y}`;
          seedCache.current.set(key, bytesToBig(data.seed));

          if (Array.isArray(data.flags)) {
            for (const group of data.flags) {
              for (const loc of group.locations) {
                // Get the chunk coordinates from the parent message
                const flagWorldX = data.chunkId.X * CHUNK + loc.x;
                const flagWorldY = data.chunkId.Y * CHUNK + loc.y;
                const flagKey = `${flagWorldX},${flagWorldY}`;

                // Set the flag on the grid using its flagID
                flaggedCellsRef.current.set(flagKey, group.flagID);
              }
            }
            setTick((t) => t + 1);
          }

          if (data.reveals instanceof Uint8Array) {
            const bits = [];
            let bit = 0;
            let idx = 0;
            for (const count of data.reveals) {
              for (let i = 0; i < count && idx < CHUNK * CHUNK; i++) {
                bits[idx++] = bit;
              }
              if (count !== 255) {
                bit ^= 1;
              }
            }
            for (let i = 0; i < bits.length; i++) {
              if (!bits[i]) continue;
              const x = i % CHUNK;
              const y = Math.floor(i / CHUNK);
              const seed = bytesToBig(data.seed);
              const cellIsMine = isMine(seed, x, y);
              let adjacentMines = 0;

              if (!cellIsMine) {
                for (let dy = -1; dy <= 1; dy++) {
                  for (let dx = -1; dx <= 1; dx++) {
                    if (dx === 0 && dy === 0) continue;
                    let nx = x + dx;
                    let ny = y + dy;
                    let ncx = data.chunkId.X;
                    let ncy = data.chunkId.Y;
                    if (nx < 0) {
                      ncx--;
                      nx += CHUNK;
                    } else if (nx >= CHUNK) {
                      ncx++;
                      nx -= CHUNK;
                    }
                    if (ny < 0) {
                      ncy--;
                      ny += CHUNK;
                    } else if (ny >= CHUNK) {
                      ncy++;
                      ny -= CHUNK;
                    }

                    const nSeed = seedCache.current.get(`${ncx},${ncy}`);
                    if (nSeed && isMine(nSeed, nx, ny)) adjacentMines++;
                  }
                }
              }

              const cellKey = `${data.chunkId.X},${data.chunkId.Y},${x},${y}`;
              revealedCellsRef.current.set(cellKey, {
                chunkId: data.chunkId,
                x,
                y,
                playerId: -1,
                isMine: cellIsMine,
                adjacentMines,
              });
            }
            setTick((t) => t + 1);
          }
        } else if (data.type === "leaderboard") {
          if (data.entries) {
            const list = data.entries
              .map((e) => {
                let num = e.score;
                if (num.endsWith("k")) {
                  num = parseFloat(num.slice(0, -1)) * 1000;
                } else if (num.endsWith("M")) {
                  num = parseFloat(num.slice(0, -1)) * 1000000;
                } else {
                  num = parseInt(num) || 0;
                }
                return { playerId: e.playerId, name: e.name || "", score: num };
              })
              .sort((a, b) => b.score - a.score);
            setLeaderboard(list);
          }
        } else if (data.type === "scoreUpdate") {
          if (typeof data.score === "number") {
            setPlayerScore(data.score);

            if (
              data.delta &&
              data.delta !== 0 &&
              (data.worldX !== 0 || data.worldY !== 0)
            ) {
              requestAnimationFrame(() => {
                const id = Math.random().toString(36).slice(2);
                setScorePopups((p) => [
                  ...p,
                  {
                    id,
                    worldX: data.worldX,
                    worldY: data.worldY,
                    delta: data.delta,
                  },
                ]);
                setTimeout(() => {
                  setScorePopups((p) => p.filter((s) => s.id !== id));
                }, 1000);
              });
            }
          }
        } else if (
          data.chunkId &&
          typeof data.x === "number" &&
          typeof data.y === "number"
        ) {
          if (typeof data.flagID === "number") {
            // Flag broadcast
            const flagWorldX = data.chunkId.X * CHUNK + data.x;
            const flagWorldY = data.chunkId.Y * CHUNK + data.y;
            const flagKey = `${flagWorldX},${flagWorldY}`;

            flaggedCellsRef.current.set(flagKey, data.flagID);
            playerFlagsRef.current.set(data.playerId, data.flagID);
            setTick((t) => t + 1);
          } else {
            // Reveal broadcast
            const { chunkX, chunkY, localX, localY } = worldToChunk(
              data.chunkId.X * CHUNK + data.x,
              data.chunkId.Y * CHUNK + data.y,
            );
            const cellKey = `${chunkX},${chunkY},${localX},${localY}`;

            const seed = seedCache.current.get(`${chunkX},${chunkY}`);
            if (seed) {
              const cellIsMine = isMine(seed, localX, localY);
              let adjacentMines = 0;

              if (!cellIsMine) {
                for (let dy = -1; dy <= 1; dy++) {
                  for (let dx = -1; dx <= 1; dx++) {
                    if (dx === 0 && dy === 0) continue;
                    let nx = localX + dx;
                    let ny = localY + dy;
                    let ncx = chunkX;
                    let ncy = chunkY;
                    if (nx < 0) {
                      ncx--;
                      nx += CHUNK;
                    } else if (nx >= CHUNK) {
                      ncx++;
                      nx -= CHUNK;
                    }
                    if (ny < 0) {
                      ncy--;
                      ny += CHUNK;
                    } else if (ny >= CHUNK) {
                      ncy++;
                      ny -= CHUNK;
                    }

                    const nSeed = seedCache.current.get(`${ncx},${ncy}`);
                    if (nSeed && isMine(nSeed, nx, ny)) adjacentMines++;
                  }
                }
              }

              revealedCellsRef.current.set(cellKey, {
                chunkId: data.chunkId,
                x: localX,
                y: localY,
                playerId: data.playerId,
                isMine: cellIsMine,
                adjacentMines,
              });

              setTick((t) => t + 1);
            }
          }
        }
      };

      return () => {
        websocket.close();
      };
    },
    [playerId],
  );

  const sendViewUpdate = useCallback(
    (viewX, viewY) => {
      if (ws && connected) {
        ws.send(
          encodeMsg(
            PB.Msg.create({
              viewUpdate: {
                viewX: Math.floor(viewX),
                viewY: Math.floor(viewY),
              },
            }),
          ),
        );
      }
    },
    [ws, connected],
  );

  return {
    // Connection state
    connected,
    playerId,
    playerScore,
    username,
    setUsername,

    // Game state
    leaderboard,
    scorePopups,
    tick,

    // Game state refs
    seedCache,
    subscribedChunks,
    revealedCellsRef,
    flaggedCellsRef,
    playerFlagsRef,

    // Actions
    handleCellClick,
    ensureChunkSubscription,
    ensureChunkUnsubscription,
    connectWs,
    sendViewUpdate,

    // Helpers
    worldToChunk,
    isMine,
  };
};
