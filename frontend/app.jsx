import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
} from "react";
import ReactDOM from "react-dom";

// DEV_MODE is set by esbuild in build.js
const log = DEV_MODE ? console.log.bind(console) : () => {};

const PB = protobuf.roots["default"].ms;
const COMPRESS_THRESHOLD = 100;
function encodeMsg(msg) {
  const buf = PB.Msg.encode(msg).finish();
  if (DEV_MODE) {
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
  if (DEV_MODE) {
    console.log("INCOMING:", {
      raw: decoded,
      compressed_size: data.byteLength,
      decompressed_size: bytes.length,
      message_type: Object.keys(decoded)[0],
    });
  }
  return decoded;
}

function App() {
  const storedId = parseInt(localStorage.getItem("playerId") || "0", 10);
  const storedName = localStorage.getItem("username") || "";

  const [ws, setWs] = useState(null);
  const [connected, setConnected] = useState(false);
  const [leaderboard, setLeaderboard] = useState([]);
  const [playerId, setPlayerId] = useState(storedId);
  const [playerScore, setPlayerScore] = useState(0);
  const [username, setUsername] = useState(storedName);
  const [nameInput, setNameInput] = useState(storedName);
  const canvasRef = useRef(null);
  const containerRef = useRef(null);
  const canvasSizeRef = useRef({ w: 0, h: 0, dpr: 1 });

  // Camera/viewport state
  const storedViewX = parseInt(sessionStorage.getItem("viewX") || "0", 10);
  const storedViewY = parseInt(sessionStorage.getItem("viewY") || "0", 10);
  const [viewX, setViewX] = useState(storedViewX);
  const [viewY, setViewY] = useState(storedViewY);
  const viewRef = useRef({ x: storedViewX, y: storedViewY });
  const rafRef = useRef(null);

  const commitViewRef = useCallback(() => {
    setViewX(viewRef.current.x);
    setViewY(viewRef.current.y);
  }, []);

  const scheduleViewUpdate = useCallback(
    (x, y) => {
      viewRef.current.x = x;
      viewRef.current.y = y;
      if (!rafRef.current) {
        rafRef.current = requestAnimationFrame(() => {
          rafRef.current = null;
          commitViewRef();
        });
      }
    },
    [commitViewRef],
  );
  const [zoom, setZoom] = useState(1);
  const handleZoom = useCallback(
    (delta) => {
      setZoom((z) => {
        const newZoom = Math.min(Math.max(z + delta, 0.5), 3);
        const container = containerRef.current;
        if (container) {
          const centerX = (viewRef.current.x + container.clientWidth / 2) / z;
          const centerY = (viewRef.current.y + container.clientHeight / 2) / z;
          scheduleViewUpdate(
            newZoom * centerX - container.clientWidth / 2,
            newZoom * centerY - container.clientHeight / 2,
          );
        }
        return newZoom;
      });
    },
    [scheduleViewUpdate],
  );
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({
    x: 0,
    y: 0,
    viewX: 0,
    viewY: 0,
  });
  const dragTimeoutRef = useRef(null);

  // Rendering optimization refs
  const lastRenderTime = useRef(0);
  const renderRequestId = useRef(null);

  // Game state
  const seedCache = useRef(new Map());
  const subscribedChunks = useRef(new Set());
  const revealedCellsRef = useRef(new Map());
  const flaggedCellsRef = useRef(new Map()); // worldX,worldY -> {color: string, playerId: number}
  const playerColorsRef = useRef(new Map());
  const [scorePopups, setScorePopups] = useState([]);
  const [tick, setTick] = useState(0);

  // Leaderboard visibility and number formatting
  const [leaderboardVisible, setLeaderboardVisible] = useState(true);

  // Player color state
  const [playerColor, setPlayerColor] = useState(
    localStorage.getItem("playerColor") || "#FF0000",
  );

  // Score popup color function
  /*
  const getScoreColor = useCallback((delta) => {
    if (delta > 0) {
      // Green for positive scores, more intense for higher values
      const intensity = Math.min(Math.abs(delta) / 20, 1); // Scale 0-1 based on delta
      const green = Math.floor(100 + intensity * 155); // 100-255 range
      return `rgb(0, ${green}, 0)`;
    } else if (delta < 0) {
      // Red for negative scores, more intense for larger losses
      const intensity = Math.min(Math.abs(delta) / 100, 1); // Scale 0-1 based on delta (bombs are -100)
      const red = Math.floor(150 + intensity * 105); // 150-255 range
      return `rgb(${red}, 0, 0)`;
    }
    return "#666"; // Gray for zero delta (shouldn't happen)
  }, []);
  */
  const getScoreColor = useCallback((delta) => {
    if (delta > 0) return "#fff";
    if (delta < 0) return "#f00";
    return "#666"; // shouldn't happen
  }, []);

  // Minimap state
  const minimapCanvasRef = useRef(null);
  const [minimapChunks, setMinimapChunks] = useState(3); // 3x3 by default

  const toggleMinimapSize = useCallback(() => {
    setMinimapChunks((c) => (c === 1 ? 3 : c === 3 ? 5 : c === 5 ? 7 : 1));
  }, []);

  const toggleLeaderboard = useCallback(() => {
    setLeaderboardVisible((v) => !v);
  }, []);
  const formatScore = useCallback((score) => {
    // Format scores into human‑friendly strings, e.g. 1.2k or 1.5M
    if (score >= 1000000) {
      const val = (score / 1000000).toFixed(1).replace(/\.0$/, "");
      return `${val}M`;
    }
    if (score >= 1000) {
      const val = (score / 1000).toFixed(1).replace(/\.0$/, "");
      return `${val}k`;
    }
    return String(score);
  }, []);

  // Color wheel component
  const ColorWheel = useCallback(({ value, onChange }) => {
    const [isDragging, setIsDragging] = useState(false);
    const wheelRef = useRef(null);

    const hexToHsv = (hex) => {
      const r = parseInt(hex.slice(1, 3), 16) / 255;
      const g = parseInt(hex.slice(3, 5), 16) / 255;
      const b = parseInt(hex.slice(5, 7), 16) / 255;

      const max = Math.max(r, g, b);
      const min = Math.min(r, g, b);
      const diff = max - min;

      let h = 0;
      if (diff !== 0) {
        if (max === r) h = (60 * ((g - b) / diff) + 360) % 360;
        else if (max === g) h = (60 * ((b - r) / diff) + 120) % 360;
        else h = (60 * ((r - g) / diff) + 240) % 360;
      }

      const s = max === 0 ? 0 : diff / max;
      const v = max;

      return [h, s, v];
    };

    const hsvToHex = (h, s, v) => {
      const c = v * s;
      const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
      const m = v - c;

      let r, g, b;
      if (h < 60) [r, g, b] = [c, x, 0];
      else if (h < 120) [r, g, b] = [x, c, 0];
      else if (h < 180) [r, g, b] = [0, c, x];
      else if (h < 240) [r, g, b] = [0, x, c];
      else if (h < 300) [r, g, b] = [x, 0, c];
      else [r, g, b] = [c, 0, x];

      r = Math.round((r + m) * 255);
      g = Math.round((g + m) * 255);
      b = Math.round((b + m) * 255);

      return `#${r.toString(16).padStart(2, "0")}${g.toString(16).padStart(2, "0")}${b.toString(16).padStart(2, "0")}`;
    };

    const [h, s, v] = hexToHsv(value);

    const handleMouseDown = (e) => {
      setIsDragging(true);
      handleMouseMove(e);
    };

    const handleMouseMove = (e) => {
      if (!isDragging && e.type === "mousemove") return;

      const rect = wheelRef.current?.getBoundingClientRect();
      if (!rect) return;

      const centerX = rect.width / 2;
      const centerY = rect.height / 2;
      const x = e.clientX - rect.left - centerX;
      const y = e.clientY - rect.top - centerY;

      const angle = ((Math.atan2(y, x) * 180) / Math.PI + 90 + 360) % 360;
      const distance = Math.min(Math.sqrt(x * x + y * y), centerX - 10);
      const saturation = distance / (centerX - 10);

      const newColor = hsvToHex(angle, saturation, 0.9);
      onChange(newColor);
    };

    const handleMouseUp = () => {
      setIsDragging(false);
    };

    useEffect(() => {
      if (isDragging) {
        document.addEventListener("mousemove", handleMouseMove);
        document.addEventListener("mouseup", handleMouseUp);
        return () => {
          document.removeEventListener("mousemove", handleMouseMove);
          document.removeEventListener("mouseup", handleMouseUp);
        };
      }
    }, [isDragging]);

    const wheelStyle = {
      width: 150,
      height: 150,
      borderRadius: "50%",
      background: `conic-gradient(
        hsl(0, 100%, 50%), hsl(60, 100%, 50%), hsl(120, 100%, 50%),
        hsl(180, 100%, 50%), hsl(240, 100%, 50%), hsl(300, 100%, 50%), hsl(0, 100%, 50%)
      ), radial-gradient(circle, transparent 0%, white 100%)`,
      position: "relative",
      cursor: "crosshair",
      margin: "10px auto",
    };

    const knobX = Math.cos(((h - 90) * Math.PI) / 180) * s * 65 + 75;
    const knobY = Math.sin(((h - 90) * Math.PI) / 180) * s * 65 + 75;

    return (
      <div style={{ textAlign: "center" }}>
        <div ref={wheelRef} style={wheelStyle} onMouseDown={handleMouseDown}>
          <div
            style={{
              position: "absolute",
              left: knobX - 8,
              top: knobY - 8,
              width: 16,
              height: 16,
              borderRadius: "50%",
              backgroundColor: value,
              border: "2px solid white",
              boxShadow: "0 0 3px rgba(0,0,0,0.5)",
              pointerEvents: "none",
            }}
          />
        </div>
        <div
          style={{
            width: 30,
            height: 30,
            backgroundColor: value,
            border: "2px solid #ccc",
            borderRadius: 4,
            margin: "10px auto",
            boxShadow: "0 2px 4px rgba(0,0,0,0.1)",
          }}
        />
      </div>
    );
  }, []);

  // Constants
  const CHUNK = 64;
  const MINE_COUNT = 20;
  const MINIMAP_SIZE = 200;
  const BASE_CELL_SIZE = 32;
  const CELL_SIZE = BASE_CELL_SIZE; // Remove zoom from CELL_SIZE calculation

  // Deterministic helpers
  const splitmix64 = useCallback((state) => {
    state = (state + 0x9e3779b97f4a7c15n) & 0xffffffffffffffffn;
    state =
      ((state ^ (state >> 30n)) * 0xbf58476d1ce4e5b9n) & 0xffffffffffffffffn;
    state =
      ((state ^ (state >> 27n)) * 0x94d049bb133111ebn) & 0xffffffffffffffffn;
    return state ^ (state >> 31n);
  }, []);

  const isMine = useCallback(
    (seed, x, y) => {
      const cellSeed = splitmix64(seed + BigInt(y * CHUNK + x));
      return Number(cellSeed % 100n) < MINE_COUNT;
    },
    [splitmix64],
  );

  // Convert screen coordinates to world coordinates
  const screenToWorld = useCallback(
    (screenX, screenY) => {
      const container = containerRef.current;
      if (!container) return { x: 0, y: 0 };

      const rect = container.getBoundingClientRect();
      const canvasX = (screenX - rect.left) / zoom;
      const canvasY = (screenY - rect.top) / zoom;

      // Convert canvas pixels to world coordinates
      const worldX = Math.floor((canvasX + viewRef.current.x) / CELL_SIZE);
      const worldY = Math.floor((canvasY + viewRef.current.y) / CELL_SIZE);

      return { x: worldX, y: worldY };
    },
    [zoom, CELL_SIZE],
  );

  // Convert world coordinates to chunk and local coordinates
  const worldToChunk = useCallback((worldX, worldY) => {
    const chunkX = Math.floor(worldX / CHUNK);
    const chunkY = Math.floor(worldY / CHUNK);

    // Simpler local coordinate calculation
    let localX = worldX - chunkX * CHUNK;
    let localY = worldY - chunkY * CHUNK;

    // Ensure positive local coordinates
    if (localX < 0) localX += CHUNK;
    if (localY < 0) localY += CHUNK;

    return { chunkX, chunkY, localX, localY };
  }, []);

  // Add getNumberColor for Minesweeper number coloring
  const getNumberColor = useCallback((num) => {
    const colors = {
      1: "#0000FF",
      2: "#008000",
      3: "#FF0000",
      4: "#000080",
      5: "#800000",
      6: "#008080",
      7: "#000000",
      8: "#808080",
    };
    return colors[num] || "#000000";
  }, []);

  const countAdjacentMines = useCallback(
    async (cx, cy, x, y) => {
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
    },
    [isMine],
  );

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

  // Flood fill reveal for cells with 0 adjacent mines
  const floodFillReveal = useCallback(
    async (startWorldX, startWorldY) => {
      const toReveal = new Set();
      const visited = new Set();
      const queue = [{ worldX: startWorldX, worldY: startWorldY }];

      // BFS to find all connected 0-mine cells
      while (queue.length > 0) {
        const { worldX, worldY } = queue.shift();
        const coordKey = `${worldX},${worldY}`;

        if (visited.has(coordKey)) continue;
        visited.add(coordKey);

        const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;

        // Skip if already revealed
        if (revealedCellsRef.current.has(cellKey)) continue;

        // Ensure we have the seed for this chunk
        let seed = seedCache.current.get(`${chunkX},${chunkY}`);
        if (!seed) {
          // Skip chunks that don't have seeds yet
          continue;
        }

        // Skip if it's a mine
        if (isMine(seed, localX, localY)) continue;

        // Count adjacent mines
        const adjacentMines = await countAdjacentMines(
          chunkX,
          chunkY,
          localX,
          localY,
        );

        // Add to reveal list
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

        // If this cell has 0 adjacent mines, add its neighbors to the queue
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

      // Create optimistic reveals and send to server
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

        ws.send(
          encodeMsg(
            PB.Msg.create({
              reveal: {
                chunkId: { X: cell.chunkX, Y: cell.chunkY },
                x: cell.localX,
                y: cell.localY,
              },
            }),
          ),
        );
      }

      setTick((t) => t + 1);
      return toReveal.size;
    },
    [ws, worldToChunk, isMine, countAdjacentMines, ensureChunkSubscription],
  );

  // Handle cell click
  const handleCellClick = useCallback(
    async (worldX, worldY, isRightClick = false) => {
      if (!ws || !connected) return;
      if (DEV_MODE)
        console.log("CELL CLICK:", { worldX, worldY, isRightClick });

      const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
      const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
      const flagKey = `${worldX},${worldY}`;

      // Handle right-click for flagging
      if (isRightClick) {
        if (DEV_MODE)
          console.log("ATTEMPTING TO FLAG:", {
            chunkX,
            chunkY,
            localX,
            localY,
          });
        // Don't flag if already flagged
        if (flaggedCellsRef.current.has(flagKey)) return;

        // Don't flag revealed cells
        if (revealedCellsRef.current.has(cellKey)) return;

        // Send flag request to server
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

      // Handle left-click: check if it's chording or normal reveal
      const revealedCell = revealedCellsRef.current.get(cellKey);

      // If clicking on a revealed cell with a number, try chording
      if (
        revealedCell &&
        !revealedCell.isMine &&
        revealedCell.adjacentMines > 0
      ) {
        // Count flags and mines in 3x3 area
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

            // Count flags and revealed mines
            if (
              flaggedCellsRef.current.has(neighborFlagKey) ||
              (neighborRevealed && neighborRevealed.isMine)
            ) {
              flagCount++;
            } else if (!neighborRevealed) {
              // This cell can be revealed
              cellsToReveal.push({
                worldX: neighborWorldX,
                worldY: neighborWorldY,
              });
            }
          }
        }

        // Only chord if flag count matches the number
        if (flagCount === revealedCell.adjacentMines) {
          // Reveal all unflagged, unrevealed neighbors
          for (const cell of cellsToReveal) {
            handleCellClick(cell.worldX, cell.worldY, false);
          }
        }
        return;
      }

      ensureChunkSubscription(chunkX, chunkY);

      const key = `${chunkX},${chunkY},${localX},${localY}`;
      if (revealedCellsRef.current.has(key)) return;

      // Don't reveal flagged cells
      if (flaggedCellsRef.current.has(flagKey)) return;

      let seed = seedCache.current.get(`${chunkX},${chunkY}`);
      if (!seed) {
        // Seed not available yet, user needs to wait for chunk sync
        return;
      }

      // If it's a mine, just reveal the single cell
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

      // Check adjacent mines for this cell
      const adjacent = await countAdjacentMines(chunkX, chunkY, localX, localY);

      // If it has 0 adjacent mines, do flood fill
      if (adjacent === 0) {
        const revealedCount = await floodFillReveal(worldX, worldY);
        log(`Flood fill revealed ${revealedCount} cells`);
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
      worldToChunk,
      ensureChunkSubscription,
      isMine,
      countAdjacentMines,
      floodFillReveal,
    ],
  );

  // 3D Cell Drawing Functions
  const draw3DCell = (ctx, x, y, size, isRevealed) => {
    const borderWidth = Math.max(1, size * 0.08);
    const innerBorderWidth = Math.max(0.5, size * 0.04);

    if (isRevealed) {
      // Revealed cell - flat appearance
      ctx.fillStyle = "#e0e0e0";
      ctx.fillRect(x, y, size, size);

      // Subtle inset border
      ctx.fillStyle = "rgba(0, 0, 0, 0.15)";
      ctx.fillRect(x, y, size, borderWidth);
      ctx.fillRect(x, y, borderWidth, size);

      ctx.fillStyle = "rgba(255, 255, 255, 0.3)";
      ctx.fillRect(
        x + size - borderWidth,
        y + borderWidth,
        borderWidth,
        size - borderWidth,
      );
      ctx.fillRect(
        x + borderWidth,
        y + size - borderWidth,
        size - borderWidth,
        borderWidth,
      );
    } else {
      // Unrevealed cell - raised 3D appearance
      ctx.fillStyle = "#c0c0c0";
      ctx.fillRect(
        x + borderWidth,
        y + borderWidth,
        size - 2 * borderWidth,
        size - 2 * borderWidth,
      );

      // Top and left highlights
      ctx.fillStyle = "#d4d4d4";
      ctx.fillRect(x, y, size, borderWidth);
      ctx.fillRect(x, y, borderWidth, size);

      ctx.fillStyle = "rgba(212, 212, 212, 0.6)";
      ctx.fillRect(
        x + borderWidth,
        y + borderWidth,
        size - 2 * borderWidth,
        innerBorderWidth,
      );
      ctx.fillRect(
        x + borderWidth,
        y + borderWidth,
        innerBorderWidth,
        size - 2 * borderWidth,
      );

      // Bottom and right shadows
      ctx.fillStyle = "#808080";
      ctx.fillRect(
        x + borderWidth,
        y + size - borderWidth,
        size - borderWidth,
        borderWidth,
      );
      ctx.fillRect(
        x + size - borderWidth,
        y + borderWidth,
        borderWidth,
        size - borderWidth,
      );

      ctx.fillStyle = "rgba(128, 128, 128, 0.6)";
      const innerShadowOffset = borderWidth + innerBorderWidth;
      ctx.fillRect(
        x + innerShadowOffset,
        y + size - innerShadowOffset,
        size - 2 * innerShadowOffset,
        innerBorderWidth,
      );
      ctx.fillRect(
        x + size - innerShadowOffset,
        y + innerShadowOffset,
        innerBorderWidth,
        size - 2 * innerShadowOffset,
      );

      // Corner highlights/shadows
      ctx.fillStyle = "#d4d4d4";
      ctx.fillRect(x, y, borderWidth, borderWidth);

      ctx.fillStyle = "#606060";
      ctx.fillRect(
        x + size - borderWidth,
        y + size - borderWidth,
        borderWidth,
        borderWidth,
      );
    }
  };

  // Content Drawing Functions
  const drawMine = (ctx, x, y, size) => {
    const mineScale = 0.8;
    const centerX = x + size / 2;
    const centerY = y + size / 2;
    const radius = size * 0.25 * mineScale;

    // Mine body
    ctx.fillStyle = "#2a2a2a";
    ctx.beginPath();
    ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
    ctx.fill();

    // Mine spikes
    ctx.strokeStyle = "#2a2a2a";
    ctx.lineWidth = Math.max(1, size * 0.06 * mineScale);
    const spikeLength = radius * 0.8;

    for (let i = 0; i < 8; i++) {
      const angle = (i * Math.PI) / 4;
      const startX = centerX + Math.cos(angle) * radius * 0.6;
      const startY = centerY + Math.sin(angle) * radius * 0.6;
      const endX = centerX + Math.cos(angle) * (radius + spikeLength);
      const endY = centerY + Math.sin(angle) * (radius + spikeLength);

      ctx.beginPath();
      ctx.moveTo(startX, startY);
      ctx.lineTo(endX, endY);
      ctx.stroke();
    }

    // Highlight on mine
    ctx.fillStyle = "#5a5a5a";
    ctx.beginPath();
    ctx.arc(
      centerX - radius * 0.3,
      centerY - radius * 0.3,
      radius * 0.3,
      0,
      Math.PI * 2,
    );
    ctx.fill();
  };

  const drawNumber = (ctx, x, y, size, number, getNumberColor) => {
    const fontSize = Math.max(8, size * 0.5);
    ctx.font = `bold ${fontSize}px monospace`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillStyle = getNumberColor(number);

    // Add subtle shadow
    ctx.shadowColor = "rgba(0, 0, 0, 0.2)";
    ctx.shadowBlur = 1;
    ctx.shadowOffsetX = 0.5;
    ctx.shadowOffsetY = 0.5;

    ctx.fillText(number.toString(), x + size / 2, y + size / 2 + 1.5);
    ctx.shadowColor = "transparent";
  };

  const drawFlag = (ctx, x, y, size, flagColor) => {
    const poleX = x + size * 0.175;
    const poleTop = y + size * 0.15;
    const poleHeight = size * 0.75;
    const poleWidth = size * 0.08;
    const flagWidth = size * 0.6;
    const flagHeight = size * 0.4;
    const poleOutline = Math.max(1, size * 0.04);
    const flagOutline = Math.max(1, size * 0.04);

    // Draw pole with black outline
    ctx.fillStyle = "#333";
    ctx.fillRect(poleX, poleTop, poleWidth, poleHeight);
    ctx.save();
    ctx.strokeStyle = "#000";
    ctx.lineWidth = poleOutline;
    ctx.strokeRect(poleX, poleTop, poleWidth, poleHeight);
    ctx.restore();

    // Draw flag with player color and black outline
    ctx.fillStyle = flagColor;
    ctx.fillRect(poleX + poleWidth, poleTop, flagWidth, flagHeight);
    ctx.save();
    ctx.strokeStyle = "#000";
    ctx.lineWidth = flagOutline;
    ctx.strokeRect(poleX + poleWidth, poleTop, flagWidth, flagHeight);
    ctx.restore();
  };

  // Cell Rendering Logic
  const renderCell = (
    ctx,
    screenX,
    screenY,
    cellSize,
    cellData,
    isRevealed,
    getNumberColor,
  ) => {
    // Always draw the base cell first
    draw3DCell(ctx, screenX, screenY, cellSize, isRevealed);

    // If not revealed, we're done (content will be handled separately for flags)
    if (!isRevealed) return;

    // Draw revealed cell content
    if (cellData.isMine) {
      drawMine(ctx, screenX, screenY, cellSize);
    } else if (cellData.adjacentMines > 0) {
      drawNumber(
        ctx,
        screenX,
        screenY,
        cellSize,
        cellData.adjacentMines,
        getNumberColor,
      );
    }
  };

  // Main Canvas Rendering
  const render = useCallback(() => {
    // Throttle rendering to 120fps max
    const now = performance.now();
    if (now - lastRenderTime.current < 8.33) {
      if (!renderRequestId.current) {
        renderRequestId.current = requestAnimationFrame(() => {
          renderRequestId.current = null;
          render();
        });
      }
      return;
    }
    lastRenderTime.current = now;

    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext("2d");
    const width = container.clientWidth;
    const height = container.clientHeight;
    const dpr = window.devicePixelRatio || 1;

    // Only resize canvas when necessary
    if (
      width !== canvasSizeRef.current.w ||
      height !== canvasSizeRef.current.h ||
      dpr !== canvasSizeRef.current.dpr
    ) {
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      canvasSizeRef.current = { w: width, h: height, dpr };
    }

    // Apply zoom and DPR scaling
    ctx.setTransform(dpr * zoom, 0, 0, dpr * zoom, 0, 0);

    // Clear background
    ctx.fillStyle = "#c0c0c0";
    ctx.fillRect(
      -viewRef.current.x / zoom,
      -viewRef.current.y / zoom,
      width / zoom,
      height / zoom,
    );

    // Calculate visible world coordinates
    const startWorldX = Math.floor(viewRef.current.x / CELL_SIZE);
    const startWorldY = Math.floor(viewRef.current.y / CELL_SIZE);
    const endWorldX = Math.ceil((viewRef.current.x + width / zoom) / CELL_SIZE);
    const endWorldY = Math.ceil(
      (viewRef.current.y + height / zoom) / CELL_SIZE,
    );

    // Render all cells in the visible area
    for (let worldY = startWorldY; worldY <= endWorldY; worldY++) {
      for (let worldX = startWorldX; worldX <= endWorldX; worldX++) {
        const screenX = worldX * CELL_SIZE - viewRef.current.x;
        const screenY = worldY * CELL_SIZE - viewRef.current.y;

        const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
        const cellData = revealedCellsRef.current.get(cellKey);
        const isRevealed = cellData !== undefined;

        // Render the cell
        renderCell(
          ctx,
          screenX,
          screenY,
          CELL_SIZE,
          cellData,
          isRevealed,
          getNumberColor,
        );
      }
    }

    // Render flags on top of unrevealed cells
    flaggedCellsRef.current.forEach((flagData, flagKey) => {
      const [worldX, worldY] = flagKey.split(",").map(Number);
      const screenX = worldX * CELL_SIZE - viewRef.current.x;
      const screenY = worldY * CELL_SIZE - viewRef.current.y;

      // Skip if not visible
      if (
        screenX + CELL_SIZE < 0 ||
        screenX > width / zoom ||
        screenY + CELL_SIZE < 0 ||
        screenY > height / zoom
      )
        return;

      // Don't draw flag if cell is revealed
      const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
      const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
      if (revealedCellsRef.current.has(cellKey)) return;

      const flagColor = flagData?.color || playerColor;
      drawFlag(ctx, screenX, screenY, CELL_SIZE, flagColor);
    });
  }, [tick, zoom, getNumberColor, worldToChunk, playerColor, draw3DCell]);

  // Minimap rendering
  const renderMinimap = useCallback(() => {
    const canvas = minimapCanvasRef.current;
    if (!canvas) return;

    const cellsPerSide = CHUNK * minimapChunks;
    canvas.width = cellsPerSide;
    canvas.height = cellsPerSide;
    canvas.style.width = `${MINIMAP_SIZE}px`;
    canvas.style.height = `${MINIMAP_SIZE}px`;

    const ctx = canvas.getContext("2d");

    // Center chunk based on view
    const gameCanvas = canvasRef.current;
    const container = containerRef.current;
    const width = container?.clientWidth || 0;
    const height = container?.clientHeight || 0;
    const centerWorldX = Math.floor(
      (viewRef.current.x + width / 2 / zoom) / CELL_SIZE,
    );
    const centerWorldY = Math.floor(
      (viewRef.current.y + height / 2 / zoom) / CELL_SIZE,
    );

    // Calculate minimap center in pixels
    const minimapCenterX = cellsPerSide / 2;
    const minimapCenterY = cellsPerSide / 2;

    // Calculate world coordinate range to display
    const worldStartX = centerWorldX - Math.floor(cellsPerSide / 2);
    const worldStartY = centerWorldY - Math.floor(cellsPerSide / 2);

    // Clear canvas
    ctx.fillStyle = "#808080";
    ctx.fillRect(0, 0, cellsPerSide, cellsPerSide);

    // Render each pixel of the minimap
    for (let py = 0; py < cellsPerSide; py++) {
      for (let px = 0; px < cellsPerSide; px++) {
        const worldX = worldStartX + px;
        const worldY = worldStartY + py;

        const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
        const flagKey = `${worldX},${worldY}`;
        const seed = seedCache.current.get(`${chunkX},${chunkY}`);

        let color = "#909090"; // Default unrevealed color
        const flag = flaggedCellsRef.current.get(flagKey);
        if (flag) {
          color = flag.color || "#ff0000";
        } else if (revealedCellsRef.current.has(cellKey)) {
          const cell = revealedCellsRef.current.get(cellKey);
          if (cell.isMine)
            color = "#333333"; // Lighter than full black
          else {
            const n = cell.adjacentMines;
            if (n === 0) color = "#e0e0e0";
            else if (n === 1)
              color = "#d0d0ff"; // Light blue tint
            else if (n === 2)
              color = "#d0ffd0"; // Light green tint
            else if (n === 3)
              color = "#ffd0d0"; // Light red tint
            else if (n === 4)
              color = "#d0d0d0"; // Light navy tint
            else if (n === 5)
              color = "#f0d0d0"; // Light maroon tint
            else if (n === 6)
              color = "#d0f0f0"; // Light cyan tint
            else color = "#c0c0c0"; // Light gray
          }
        } else if (seed && isMine(seed, localX, localY)) {
          // Unrevealed mine
          color = "#909090";
        }

        ctx.fillStyle = color;
        ctx.fillRect(px, py, 1, 1);
      }
    }

    // Draw fixed viewport indicator in center of minimap
    if (width > 0 && height > 0) {
      // Calculate viewport size in world cells
      const viewWidthCells = Math.ceil(width / zoom / CELL_SIZE);
      const viewHeightCells = Math.ceil(height / zoom / CELL_SIZE);

      // Draw viewport box centered on minimap
      const boxLeft = minimapCenterX - viewWidthCells / 2;
      const boxTop = minimapCenterY - viewHeightCells / 2;

      ctx.strokeStyle = "rgba(200, 200, 200, 0.8)";
      ctx.lineWidth = 1;
      ctx.strokeRect(boxLeft, boxTop, viewWidthCells, viewHeightCells);
    }
  }, [
    viewX,
    viewY,
    tick,
    minimapChunks,
    worldToChunk,
    isMine,
    zoom,
    CELL_SIZE,
  ]);

  // Subscribe to visible chunks
  const subscribeToVisibleChunks = useCallback(() => {
    if (!ws || !connected) return;

    const container = containerRef.current;
    if (!container) return;

    const width = container.clientWidth;
    const height = container.clientHeight;
    const startWorldX = Math.floor(viewRef.current.x / CELL_SIZE);
    const startWorldY = Math.floor(viewRef.current.y / CELL_SIZE);
    const endWorldX = Math.ceil((viewRef.current.x + width / zoom) / CELL_SIZE);
    const endWorldY = Math.ceil(
      (viewRef.current.y + height / zoom) / CELL_SIZE,
    );

    const startChunkX = Math.floor(startWorldX / CHUNK) - 1;
    const startChunkY = Math.floor(startWorldY / CHUNK) - 1;
    const endChunkX = Math.ceil(endWorldX / CHUNK) + 1;
    const endChunkY = Math.ceil(endWorldY / CHUNK) + 1;

    const visibleNow = new Set();

    for (let chunkY = startChunkY; chunkY <= endChunkY; chunkY++) {
      for (let chunkX = startChunkX; chunkX <= endChunkX; chunkX++) {
        const key = `${chunkX},${chunkY}`;
        visibleNow.add(key);
        ensureChunkSubscription(chunkX, chunkY);
      }
    }

    // Un‑subscribe chunks that scrolled off (>1 ring outside viewport)
    subscribedChunks.current.forEach((key) => {
      if (!visibleNow.has(key)) {
        const [cx, cy] = key.split(",").map(Number);
        ensureChunkUnsubscription(cx, cy);
      }
    });
  }, [
    ws,
    connected,
    zoom,
    ensureChunkSubscription,
    ensureChunkUnsubscription,
    CELL_SIZE,
  ]);

  // Mouse event handlers
  const handleMouseDown = useCallback(
    (e) => {
      e.preventDefault(); // Prevent context menu and other default behaviors

      if (e.button === 2) {
        // Right click for flag
        const { x: worldX, y: worldY } = screenToWorld(e.clientX, e.clientY);
        handleCellClick(worldX, worldY, true);
        return;
      }

      if (e.button !== 0) return;

      setDragStart({
        x: e.clientX,
        y: e.clientY,
        viewX: viewRef.current.x,
        viewY: viewRef.current.y,
      });

      // Set a timeout to enable dragging after a delay
      dragTimeoutRef.current = setTimeout(() => {
        setIsDragging(true);
      }, 150); // delay before considering it a drag
    },
    [screenToWorld, handleCellClick],
  );

  const handleMouseMove = useCallback(
    (e) => {
      if (!dragStart.x) return; // No mouse down recorded

      const dx = Math.abs(e.clientX - dragStart.x);
      const dy = Math.abs(e.clientY - dragStart.y);

      // If mouse moved significantly, immediately enable dragging
      if (dx > 50 || dy > 50) {
        if (dragTimeoutRef.current) {
          clearTimeout(dragTimeoutRef.current);
          dragTimeoutRef.current = null;
        }
        setIsDragging(true);
      }

      if (isDragging) {
        const deltaX = (e.clientX - dragStart.x) / zoom;
        const deltaY = (e.clientY - dragStart.y) / zoom;

        scheduleViewUpdate(dragStart.viewX - deltaX, dragStart.viewY - deltaY);
      }
    },
    [isDragging, dragStart, zoom, scheduleViewUpdate],
  );

  const handleMouseUp = useCallback(
    (e) => {
      if (dragTimeoutRef.current) {
        clearTimeout(dragTimeoutRef.current);
        dragTimeoutRef.current = null;
      }

      if (!isDragging && dragStart.x) {
        const { x: worldX, y: worldY } = screenToWorld(e.clientX, e.clientY);
        const isRight = e.button === 2 || (e.button === 0 && e.ctrlKey);
        handleCellClick(worldX, worldY, isRight);
      }

      setIsDragging(false);
      setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });

      // Cancel any pending RAF and commit final view
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
        commitViewRef();
      }

      if (ws && connected) {
        ws.send(
          encodeMsg(
            PB.Msg.create({
              viewUpdate: {
                viewX: Math.floor(viewRef.current.x),
                viewY: Math.floor(viewRef.current.y),
              },
            }),
          ),
        );
      }
    },
    [
      isDragging,
      dragStart,
      screenToWorld,
      handleCellClick,
      commitViewRef,
      ws,
      connected,
    ],
  );

  // Handle context menu (prevent default right-click menu)
  const handleContextMenu = useCallback((e) => {
    e.preventDefault();
  }, []);

  // Touch event handlers for mobile
  const handleTouchStart = useCallback(
    (e) => {
      if (e.touches.length === 1) {
        const touch = e.touches[0];
        e.preventDefault(); // Prevent text selection
        setDragStart({
          x: touch.clientX,
          y: touch.clientY,
          viewX: viewRef.current.x,
          viewY: viewRef.current.y,
        });

        // Start long press timer for flagging
        dragTimeoutRef.current = setTimeout(() => {
          // This is a long press - place a flag
          const { x: worldX, y: worldY } = screenToWorld(
            touch.clientX,
            touch.clientY,
          );
          handleCellClick(worldX, worldY, true); // true = right click (flag)

          // Clear drag start to prevent any further actions
          setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });
        }, 200); // long press duration
      }
    },
    [screenToWorld, handleCellClick],
  );

  const handleTouchMove = useCallback(
    (e) => {
      e.preventDefault(); // Prevent scrolling
      if (e.touches.length === 1 && dragStart.x) {
        const touch = e.touches[0];
        const dx = Math.abs(touch.clientX - dragStart.x);
        const dy = Math.abs(touch.clientY - dragStart.y);

        // If touch moved significantly, cancel long press and enable dragging
        if (dx > 10 || dy > 10) {
          if (dragTimeoutRef.current) {
            clearTimeout(dragTimeoutRef.current);
            dragTimeoutRef.current = null;
          }
          setIsDragging(true);
        }

        if (isDragging) {
          const deltaX = (touch.clientX - dragStart.x) / zoom;
          const deltaY = (touch.clientY - dragStart.y) / zoom;

          scheduleViewUpdate(
            dragStart.viewX - deltaX,
            dragStart.viewY - deltaY,
          );
        }
      }
    },
    [isDragging, dragStart, zoom, scheduleViewUpdate],
  );

  const handleTouchEnd = useCallback(
    (e) => {
      // Clear the long press timeout
      if (dragTimeoutRef.current) {
        clearTimeout(dragTimeoutRef.current);
        dragTimeoutRef.current = null;
      }

      if (!isDragging && dragStart.x && e.changedTouches.length === 1) {
        // This was a short tap, not a drag or long press - reveal cell
        const touch = e.changedTouches[0];
        const { x: worldX, y: worldY } = screenToWorld(
          touch.clientX,
          touch.clientY,
        );
        handleCellClick(worldX, worldY, false); // false = left click (reveal)
      }

      setIsDragging(false);
      setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });

      // Cancel any pending RAF and commit final view
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
        commitViewRef();
      }

      if (ws && connected) {
        ws.send(
          encodeMsg(
            PB.Msg.create({
              viewUpdate: {
                viewX: Math.floor(viewRef.current.x),
                viewY: Math.floor(viewRef.current.y),
              },
            }),
          ),
        );
      }
    },
    [
      isDragging,
      dragStart,
      screenToWorld,
      handleCellClick,
      commitViewRef,
      ws,
      connected,
    ],
  );

  // WebSocket setup
  const connectWs = useCallback(() => {
    const wsUrl = `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;
    const websocket = new WebSocket(wsUrl);
    websocket.binaryType = "arraybuffer";

    websocket.onopen = () => {
      const msg = PB.Msg.create({
        hello: { playerId: playerId, name: nameInput, color: playerColor },
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

      // Additional debug logging for message processing
      if (DEV_MODE) {
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
          color: m.welcome.color,
          viewX: m.welcome.viewX,
          viewY: m.welcome.viewY,
        };
      } else if (m.chunkSync) {
        data = {
          type: "chunkSync",
          chunkId: m.chunkSync.chunkId,
          seed: m.chunkSync.seed,
          reveals: m.chunkSync.reveals,
          flags: m.chunkSync.flags,
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

      if (data.type === "welcome") {
        setPlayerId(data.playerId);
        setUsername(data.name || "");
        localStorage.setItem("playerId", data.playerId);
        localStorage.setItem("username", data.name || "");
        scheduleViewUpdate(data.viewX || 0, data.viewY || 0);
        sessionStorage.setItem("viewX", String(data.viewX || 0));
        sessionStorage.setItem("viewY", String(data.viewY || 0));
        return;
      }

      if (data.type === "chunkSync") {
        // Helper to convert Uint8Array (protobuf bytes) to BigInt
        const bytesToBig = (u8) =>
          new DataView(u8.buffer, u8.byteOffset, 8).getBigUint64(0, true);

        const key = `${data.chunkId.X},${data.chunkId.Y}`;
        seedCache.current.set(key, bytesToBig(data.seed));

        // Handle flags from server
        if (Array.isArray(data.flags)) {
          for (const flag of data.flags) {
            const flagWorldX = flag.chunkId.X * CHUNK + flag.x;
            const flagWorldY = flag.chunkId.Y * CHUNK + flag.y;
            const flagKey = `${flagWorldX},${flagWorldY}`;

            flaggedCellsRef.current.set(flagKey, {
              color: flag.color,
              playerId: flag.playerId,
            });
          }
          setTick((t) => t + 1);
        }

        if (Array.isArray(data.reveals)) {
          for (const cell of data.reveals) {
            const seed = bytesToBig(data.seed);
            const cellIsMine = isMine(seed, cell.x, cell.y);
            let adjacentMines = 0;

            if (!cellIsMine) {
              // Calculate adjacent mines synchronously since we have the seed
              for (let dy = -1; dy <= 1; dy++) {
                for (let dx = -1; dx <= 1; dx++) {
                  if (dx === 0 && dy === 0) continue;
                  let nx = cell.x + dx;
                  let ny = cell.y + dy;
                  let ncx = cell.chunkId.X;
                  let ncy = cell.chunkId.Y;
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

            const cellKey = `${cell.chunkId.X},${cell.chunkId.Y},${cell.x},${cell.y}`;
            revealedCellsRef.current.set(cellKey, {
              ...cell,
              isMine: cellIsMine,
              adjacentMines,
            });
          }
          setTick((t) => t + 1);
        }
      } else if (data.type === "flagAck") {
        // Simple acknowledgment - just indicates success or failure
        if (!data.ok) {
          // Flag failed - could show user feedback here if needed
          console.log("Flag failed");
        }
      } else if (data.type === "revealAck") {
        if (!data.ok) {
          // Optimistic reveal lost → resync this chunk
          const key = `${data.chunkId.X},${data.chunkId.Y},${data.x},${data.y}`;
          revealedCellsRef.current.delete(key);

          // Force a re‑subscribe to get authoritative state
          ws.send(
            encodeMsg(
              PB.Msg.create({
                subscribe: { chunkX: data.chunkId.X, chunkY: data.chunkId.Y },
              }),
            ),
          );

          setTick((t) => t + 1);
        }
      } else if (
        data.chunkId &&
        typeof data.x === "number" &&
        typeof data.y === "number" &&
        data.color
      ) {
        // This is a flag broadcast from server
        const flagWorldX = data.chunkId.X * CHUNK + data.x;
        const flagWorldY = data.chunkId.Y * CHUNK + data.y;
        const flagKey = `${flagWorldX},${flagWorldY}`;

        flaggedCellsRef.current.set(flagKey, {
          color: data.color,
          playerId: data.playerId,
        });
        playerColorsRef.current.set(data.playerId, data.color);

        setTick((t) => t + 1);
      } else if (
        data.chunkId &&
        typeof data.x === "number" &&
        typeof data.y === "number" &&
        typeof data.playerId === "number" &&
        !data.color
      ) {
        // This is a reveal broadcast from server (e.g., from wrong flag)
        const { chunkX, chunkY, localX, localY } = worldToChunk(
          data.chunkId.X * CHUNK + data.x,
          data.chunkId.Y * CHUNK + data.y,
        );
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;

        // Get the seed for this chunk to determine if it's a mine
        const seed = seedCache.current.get(`${chunkX},${chunkY}`);
        if (seed) {
          const cellIsMine = isMine(seed, localX, localY);
          let adjacentMines = 0;

          if (!cellIsMine) {
            // Calculate adjacent mines
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

          // Only show popup if there's a delta and valid coordinates (not initial score)
          if (
            data.delta &&
            data.delta !== 0 &&
            (data.worldX !== 0 || data.worldY !== 0)
          ) {
            // Batch score popup updates to prevent blocking
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
      }
    };

    return () => {
      websocket.close();
    };
  }, [playerId, nameInput, playerColor, isMine]);

  useEffect(() => {
    if (!username) return;

    // Small delay to ensure state is stable
    const timeoutId = setTimeout(() => {
      const cleanup = connectWs();
      return cleanup;
    }, 100);

    return () => clearTimeout(timeoutId);
  }, [username]);

  // Subscribe to visible chunks when view changes
  useEffect(() => {
    subscribeToVisibleChunks();
  }, [subscribeToVisibleChunks]);

  // Sync viewRef to sessionStorage
  useEffect(() => {
    viewRef.current.x = viewX;
    viewRef.current.y = viewY;
    sessionStorage.setItem("viewX", String(viewX));
    sessionStorage.setItem("viewY", String(viewY));
  }, [viewX, viewY]);

  // Cleanup RAF on unmount
  useEffect(() => {
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
      if (renderRequestId.current)
        cancelAnimationFrame(renderRequestId.current);
    };
  }, []);

  // Trigger re-render when game state changes
  useEffect(() => {
    if (!renderRequestId.current) {
      renderRequestId.current = requestAnimationFrame(() => {
        renderRequestId.current = null;
        render();
      });
    }
  }, [render, tick]);

  useEffect(() => {
    renderMinimap();
  }, [renderMinimap, viewX, viewY]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      setTick((t) => t + 1);
    };

    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const topPlayers = useMemo(() => {
    return leaderboard.slice(0, 10);
  }, [leaderboard]);

  // Calculate current world position for debugging
  const centerRect = canvasRef.current?.getBoundingClientRect();
  const container = containerRef.current;
  const containerWidth = container?.clientWidth || 0;
  const containerHeight = container?.clientHeight || 0;
  const centerWorldX = Math.floor(
    (viewX + containerWidth / 2 / zoom) / CELL_SIZE,
  );
  const centerWorldY = Math.floor(
    (viewY + containerHeight / 2 / zoom) / CELL_SIZE,
  );
  const { chunkX: centerChunkX, chunkY: centerChunkY } = worldToChunk(
    centerWorldX,
    centerWorldY,
  );

  return (
    <div className="game-container">
      {!username && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.5)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 20,
          }}
        >
          <div
            style={{
              background: "white",
              padding: 20,
              borderRadius: 8,
              textAlign: "center",
              maxWidth: 300,
            }}
          >
            <h3>Enter username</h3>
            <input
              value={nameInput}
              onChange={(e) => setNameInput(e.target.value)}
              style={{
                padding: 8,
                marginBottom: 10,
                borderRadius: 4,
                border: "1px solid #ccc",
                width: "80%",
              }}
              placeholder="Your name"
            />
            <div style={{ margin: "15px 0" }}>
              <div style={{ marginBottom: "10px", fontWeight: "bold" }}>
                Choose your color:
              </div>
              <ColorWheel
                value={playerColor}
                onChange={(color) => {
                  setPlayerColor(color);
                  localStorage.setItem("playerColor", color);
                }}
              />
            </div>
            <button
              onClick={() => {
                if (nameInput.trim()) {
                  setUsername(nameInput.trim());
                }
              }}
              style={{
                padding: "10px 20px",
                backgroundColor: "#4CAF50",
                color: "white",
                border: "none",
                borderRadius: 4,
                cursor: "pointer",
                fontSize: 16,
              }}
              disabled={!nameInput.trim()}
            >
              Join Game
            </button>
          </div>
        </div>
      )}

      {/* Player Score Display */}
      <div className="player-score">Score: {playerScore}</div>

      <div className="header">
        <div className="status">
          <div
            className={`connection-status ${connected ? "connected" : "disconnected"}`}
          >
            {connected ? "Connected" : "Disconnected"}
          </div>
        </div>
      </div>

      <div className="board-container">
        <div
          ref={containerRef}
          className={`canvas-container ${isDragging ? "dragging" : ""}`}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
          onContextMenu={handleContextMenu}
          onTouchStart={handleTouchStart}
          onTouchMove={handleTouchMove}
          onTouchEnd={handleTouchEnd}
          style={{ touchAction: "none" }} // Prevent default touch behaviors
        >
          <canvas ref={canvasRef} id="game-canvas" />

          {scorePopups.map((p) => {
            const x = p.worldX * CELL_SIZE * zoom - viewX;
            const y = p.worldY * CELL_SIZE * zoom - viewY;
            return (
              <div
                key={p.id}
                className="score-popup"
                style={{
                  left: x,
                  top: y,
                  color: getScoreColor(p.delta),
                  fontWeight: "bold",
                  textShadow: "1px 1px 2px rgba(0,0,0,0.8)",
                }}
              >
                {p.delta > 0 ? `+${p.delta}` : p.delta}
              </div>
            );
          })}

          {DEV_MODE && (
            <div className="coordinates-debug">
              <div
                className={`connection-status ${connected ? "connected" : "disconnected"}`}
              >
                {connected ? "Connected" : "Disconnected"}
              </div>
              View: ({Math.round(viewRef.current.x)},{" "}
              {Math.round(viewRef.current.y)})<br />
              Center: ({centerWorldX}, {centerWorldY})<br />
              Chunk: ({centerChunkX}, {centerChunkY})
            </div>
          )}
        </div>
      </div>

      <canvas
        ref={minimapCanvasRef}
        className="minimap"
        onClick={toggleMinimapSize}
      />

      <div className="leaderboard">
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <h3 style={{ margin: 0 }}>Leaderboard</h3>
          <button onClick={toggleLeaderboard}>
            {leaderboardVisible ? "Hide" : "Show"}
          </button>
        </div>
        {leaderboardVisible && (
          <>
            {topPlayers.length > 0 ? (
              <ol>
                {topPlayers.map((p) => (
                  <li key={p.playerId}>
                    <span
                      className="lb-flag"
                      style={{
                        backgroundColor:
                          playerColorsRef.current.get(p.playerId) || "#ccc",
                      }}
                    />
                    {p.name ? p.name : `Player ${p.playerId}`}:{" "}
                    {formatScore(p.score)}
                  </li>
                ))}
              </ol>
            ) : (
              <p>No players yet</p>
            )}
          </>
        )}
      </div>
      <div className="zoom-controls">
        <button onClick={() => handleZoom(0.25)}>+</button>
        <button onClick={() => handleZoom(-0.25)}>-</button>
      </div>
    </div>
  );
}

ReactDOM.render(<App />, document.getElementById("root"));
