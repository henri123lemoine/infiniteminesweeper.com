import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
} from "react";
import Minimap from "./Minimap.jsx";
import { useGameState, CHUNK } from "./useGameState.js";
import { CanvasRenderer } from "./CanvasRenderer.js";

function App() {
  const storedName = localStorage.getItem("username") || "";
  const [nameInput, setNameInput] = useState(storedName);

  const {
    connected,
    playerId,
    playerScore,
    username,
    setUsername,
    leaderboard,
    scorePopups,
    tick,
    setTick,
    seedCache,
    subscribedChunks,
    revealedCellsRef,
    flaggedCellsRef,
    playerColorsRef,
    handleCellClick,
    ensureChunkSubscription,
    ensureChunkUnsubscription,
    connectWs,
    sendViewUpdate,
    worldToChunk,
    isMine,
  } = useGameState();

  const canvasRef = useRef(null);
  const containerRef = useRef(null);

  // Camera/viewport state
  const rendererRef = useRef(new CanvasRenderer());
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
  const MINIMAP_SIZE = 200;
  const BASE_CELL_SIZE = 32;
  const CELL_SIZE = BASE_CELL_SIZE;

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

    if (!canvasRef.current || !containerRef.current) {
      return;
    }

    rendererRef.current.render({
      canvasRef,
      containerRef,
      viewRef,
      zoom,
      tick,
      CELL_SIZE,
      revealedCellsRef,
      flaggedCellsRef,
      worldToChunk,
      getNumberColor,
      playerColor,
    });
  }, [tick, zoom, getNumberColor, worldToChunk, playerColor]);

  // Subscribe to visible chunks
  const subscribeToVisibleChunks = useCallback(() => {
    if (!connected) return;

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

      if (connected) {
        sendViewUpdate(viewRef.current.x, viewRef.current.y);
      }
    },
    [
      isDragging,
      dragStart,
      screenToWorld,
      handleCellClick,
      commitViewRef,
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

      if (connected) {
        sendViewUpdate(viewRef.current.x, viewRef.current.y);
      }
    },
    [
      isDragging,
      dragStart,
      screenToWorld,
      handleCellClick,
      commitViewRef,
      connected,
    ],
  );

  useEffect(() => {
    if (!username) return;
    // Hook returns a cleanup fn
    const cleanup = connectWs(username, playerColor);
    return cleanup;
  }, [username, playerColor]);

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

  // Trigger canvas redraw when the game OR the viewport changes
  useEffect(() => {
    if (!renderRequestId.current) {
      renderRequestId.current = requestAnimationFrame(() => {
        renderRequestId.current = null;
        render();
      });
    }
  }, [render, tick, viewX, viewY, zoom]);

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

      <Minimap
        CHUNK={CHUNK}
        CELL_SIZE={CELL_SIZE}
        MINIMAP_SIZE={MINIMAP_SIZE}
        zoom={zoom}
        viewX={viewX}
        viewY={viewY}
        containerRef={containerRef}
        seedCache={seedCache}
        revealedCellsRef={revealedCellsRef}
        flaggedCellsRef={flaggedCellsRef}
        isMine={isMine}
        worldToChunk={worldToChunk}
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

export default App;
