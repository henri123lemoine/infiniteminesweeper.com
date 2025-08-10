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
import FlagSelector from "./FlagSelector.jsx";
import meta from "./assets/spritesheet.json";

if (__DEV__) console.log("dev mode!");
else console.log("production mode!");

function App() {
  const storedName = localStorage.getItem("username") || "";
  const [nameInput, setNameInput] = useState(storedName);
  const [hotspotInfo, setHotspotInfo] = useState(null);
  const [activeTab, setActiveTab] = useState("play"); // play | leaderboard | advancements
  const [fullLeaderboard, setFullLeaderboard] = useState(null);
  const [lbLoading, setLbLoading] = useState(false);
  const [joinError, setJoinError] = useState("");

  const {
    connected,
    playerScore,
    userRank,
    username,
    setUsername,
    disconnect,
    leaderboard,
    scorePopups,
    hintPopups,
    tick,
    setTick,
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
    sendViewUpdateRef,
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
  const guestCleanupRef = useRef(null);
  const userMovedRef = useRef(false);

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
  const handleWheel = useCallback(
    (e) => {
      e.preventDefault();
      userMovedRef.current = true;

      const container = containerRef.current;
      if (!container) return;

      const rect = container.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;

      const MIN_ZOOM = 0.1;
      const MAX_ZOOM = 5;

      // Smooth exponential zoom; negative deltaY -> zoom in
      const zoomFactor = Math.exp(-e.deltaY * 0.001);

      setZoom((prevZoom) => {
        const targetZoom = Math.min(
          Math.max(prevZoom * zoomFactor, MIN_ZOOM),
          MAX_ZOOM,
        );

        // Anchor the zoom on the mouse position for intuitive behavior
        const worldX = viewRef.current.x + mouseX / prevZoom;
        const worldY = viewRef.current.y + mouseY / prevZoom;
        const newViewX = worldX - mouseX / targetZoom;
        const newViewY = worldY - mouseY / targetZoom;
        scheduleViewUpdate(newViewX, newViewY);

        return targetZoom;
      });
    },
    [containerRef, scheduleViewUpdate],
  );

  // Attach non-passive wheel listener to prevent page scroll while zooming
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener("wheel", handleWheel, { passive: false });
    return () => el.removeEventListener("wheel", handleWheel);
  }, [handleWheel]);
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0, viewX: 0, viewY: 0 });
  const dragTimeoutRef = useRef(null);

  // Rendering optimization refs
  const lastRenderTime = useRef(0);
  const renderRequestId = useRef(null);

  // Leaderboard visibility and number formatting
  const [leaderboardVisible, setLeaderboardVisible] = useState(true);
  const [helpOpen, setHelpOpen] = useState(false);

  // Build numeric sprite ID list for the 'flag' category only
  const FLAG_IDS = useMemo(
    () =>
      Object.keys(meta.frames)
        .filter(
          (k) => !Number.isNaN(Number(k)) && meta.frames[k]?.category === "flag",
        )
        .map((k) => Number(k))
        .sort((a, b) => a - b),
    [],
  );

  // Player flag state (migrate legacy index-based value → stable numeric ID)
  const [flagID, setFlagID] = useState(() => {
    const raw = localStorage.getItem("flagID");
    const parsed = Number(raw);
    // Fallback to first available flag if missing/invalid
    const fallback = FLAG_IDS[0] ?? 0;
    if (!Number.isFinite(parsed)) return fallback;
    // If value is a valid flag sprite ID, keep it.
    if (FLAG_IDS.includes(parsed)) return parsed;
    // Legacy case: treat small integer as index into flag ID list.
    if (parsed >= 0 && parsed < FLAG_IDS.length) return FLAG_IDS[parsed];
    return fallback;
  });

  // Ensure we always store numeric IDs (not array indices)
  useEffect(() => {
    if (!Number.isFinite(flagID)) return;
    const stored = Number(localStorage.getItem("flagID"));
    if (stored !== flagID) localStorage.setItem("flagID", String(flagID));
  }, [flagID]);

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
      flagID,
    });
  }, [tick, zoom, getNumberColor, worldToChunk, flagID]);

  // Instead of per-chunk subscribes, send a viewUpdate proportional to viewport
  const sendViewportUpdate = useCallback(() => {
    if (!connected) return;
    requestAnimationFrame(() => {
      const container = containerRef.current;
      const width = container?.clientWidth || window.innerWidth || 0;
      const height = container?.clientHeight || window.innerHeight || 0;

      const worldWidthCells = Math.ceil(width / zoom / CELL_SIZE);
      const worldHeightCells = Math.ceil(height / zoom / CELL_SIZE);

      const centerWorldX = Math.floor((viewRef.current.x + (width / zoom) / 2) / CELL_SIZE);
      const centerWorldY = Math.floor((viewRef.current.y + (height / zoom) / 2) / CELL_SIZE);
      const { chunkX, chunkY, cell } = worldToChunk(centerWorldX, centerWorldY);

      if (typeof sendViewUpdateRef?.current === 'function') {
        sendViewUpdateRef.current(chunkX, chunkY, cell, worldWidthCells, worldHeightCells);
      }
    });
  }, [connected, zoom, CELL_SIZE, worldToChunk]);

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
      userMovedRef.current = true;

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
        userMovedRef.current = true;
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
        const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${cell}`;
        const isRevealed = revealedCellsRef.current.has(cellKey);

        handleCellClick(worldX, worldY, false, isRevealed);
      }

      setIsDragging(false);
      setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });

      // Cancel any pending RAF and commit final view
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
        commitViewRef();
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
        userMovedRef.current = true;
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
          userMovedRef.current = true;
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
        // This was a short tap, not a drag or long press - reveal cell or chord
        const touch = e.changedTouches[0];
        const { x: worldX, y: worldY } = screenToWorld(
          touch.clientX,
          touch.clientY,
        );
        const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${cell}`;
        const isRevealed = revealedCellsRef.current.has(cellKey);

        // Pass true for isChord if the cell is already revealed
        handleCellClick(worldX, worldY, false, isRevealed);
      }

      setIsDragging(false);
      setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });

      // Cancel any pending RAF and commit final view
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
        commitViewRef();
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

  // Keyboard panning with Arrow keys (and optional WASD)
  useEffect(() => {
    const handleKeyDown = (e) => {
      // Ignore if typing into inputs or contentEditable elements
      const tag = (e.target && e.target.tagName) ? e.target.tagName.toLowerCase() : "";
      if (tag === "input" || tag === "textarea" || tag === "select") return;
      if (e.target && e.target.isContentEditable) return;

      let dxCells = 0;
      let dyCells = 0;
      // Base step in cells; hold Shift for a larger step
      const base = 8;
      const step = e.shiftKey ? base * 4 : base;
      switch (e.key) {
        case "ArrowLeft":
        case "a":
        case "A":
          dxCells = -step;
          break;
        case "ArrowRight":
        case "d":
        case "D":
          dxCells = step;
          break;
        case "ArrowUp":
        case "w":
        case "W":
          dyCells = -step;
          break;
        case "ArrowDown":
        case "s":
        case "S":
          dyCells = step;
          break;
        default:
          return; // not a navigation key we care about
      }

      e.preventDefault(); // prevent page scroll
      userMovedRef.current = true;
      const dxPx = dxCells * CELL_SIZE;
      const dyPx = dyCells * CELL_SIZE;
      // ArrowRight increases viewX to move camera right; ArrowDown increases viewY
      scheduleViewUpdate(viewRef.current.x + dxPx, viewRef.current.y + dyPx);
    };

    window.addEventListener("keydown", handleKeyDown, { passive: false });
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [CELL_SIZE, scheduleViewUpdate]);

  useEffect(() => {
    if (!username) return;
    const cleanup = connectWs(username, Number(flagID));
    return cleanup;
  }, [username, flagID, connectWs]);

  // Send view-based updates when view changes
  useEffect(() => {
    sendViewportUpdate();
  }, [sendViewportUpdate, viewX, viewY]);

  // Also send once on connect/zoom change and on resize
  useEffect(() => {
    if (connected) sendViewportUpdate();
  }, [connected, sendViewportUpdate, zoom]);

  useEffect(() => {
    const onResize = () => sendViewportUpdate();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [sendViewportUpdate]);

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

  // Pre-load assets
  useEffect(() => {
    CanvasRenderer.initSprites();
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

  // Center camera helper
  const centerCameraOnWorld = useCallback((worldX, worldY) => {
    const container = containerRef.current;
    if (!container) return;
    const centerX = container.clientWidth / 2;
    const centerY = container.clientHeight / 2;
    const newViewX = worldX * CELL_SIZE - centerX / zoom;
    const newViewY = worldY * CELL_SIZE - centerY / zoom;
    scheduleViewUpdate(newViewX, newViewY);
  }, [CELL_SIZE, zoom, scheduleViewUpdate]);

  // Fetch hotspot to showcase activity on landing
  useEffect(() => {
    let canceled = false;
    fetch("/hotspot")
      .then(r => (r.ok ? r.json() : null))
      .then(data => {
        if (canceled || !data) return;
        setHotspotInfo(data);
        // Only auto-center if the user hasn't moved the camera yet
        if (userMovedRef.current) return;
        const chunkWorldX = data.X * CHUNK + CHUNK / 2;
        const chunkWorldY = data.Y * CHUNK + CHUNK / 2;
        requestAnimationFrame(() => centerCameraOnWorld(chunkWorldX, chunkWorldY));
      })
      .catch(() => {});
    return () => {
      canceled = true;
    };
  }, [centerCameraOnWorld]);

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
  const container = containerRef.current || { clientWidth: 0, clientHeight: 0 };
  const containerWidth = container.clientWidth;
  const containerHeight = container.clientHeight;
  const centerWorldX = Math.floor(
    (viewX + containerWidth / 2 / zoom) / CELL_SIZE,
  );
  const centerWorldY = Math.floor(
    (viewY + containerHeight / 2 / zoom) / CELL_SIZE,
  );
  const { chunkX: centerChunkX, chunkY: centerChunkY, cell: centerCell } = worldToChunk(
    centerWorldX,
    centerWorldY,
  );

  return (
    <div className="game-container">
      {/* Home button to reopen overlay */}
      <button
        onClick={() => {
          // Disconnect any existing real session and go back to spectate landing
          try { disconnect(); } catch {}
          // Clear stored username to trigger overlay
          localStorage.removeItem("username");
          setUsername("");
        }}
        style={{
          position: "fixed",
          top: 52, // below the score chip
          left: "50%",
          transform: "translateX(-50%)",
          zIndex: 21,
          background: "#fff",
          border: "1px solid #ccc",
          borderRadius: 6,
          padding: "6px 10px",
          cursor: "pointer",
          boxShadow: "0 1px 2px rgba(0,0,0,0.1)",
        }}
      >
        Home
      </button>
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
              maxWidth: 860,
              width: "90vw",
              height: "90vh",
              overflow: "auto",
            }}
          >
            <h1 style={{ marginTop: 0 }}>Infinite Minesweeper</h1>
            <p style={{ margin: "8px 0 16px", color: "#555" }}>
              Discover an endless world together. You are currently spectating live explorers.
              {hotspotInfo && hotspotInfo.count > 0 && (
                <>
                  {" "}There are <b>{hotspotInfo.count}</b> players near the hotspot.
                </>
              )}
            </p>

            {/* Tabs */}
            <div style={{ display: "flex", gap: 8, justifyContent: "center", marginBottom: 16 }}>
              {[
                { k: "play", label: "Play" },
                { k: "leaderboard", label: "Leaderboard" },
                { k: "advancements", label: "Advancements" },
              ].map((t) => (
                <button
                  key={t.k}
                  onClick={() => {
                    setActiveTab(t.k);
                    if (t.k === "leaderboard" && fullLeaderboard == null && !lbLoading) {
                      setLbLoading(true);
                      fetch("/leaderboard")
                        .then((r) => (r.ok ? r.json() : []))
                        .then((rows) => setFullLeaderboard(Array.isArray(rows) ? rows : []))
                        .catch(() => setFullLeaderboard([]))
                        .finally(() => setLbLoading(false));
                    }
                  }}
                  style={{
                    padding: "6px 10px",
                    borderRadius: 6,
                    border: activeTab === t.k ? "2px solid #1976d2" : "1px solid #ccc",
                    background: activeTab === t.k ? "#e3f2fd" : "#fafafa",
                    cursor: "pointer",
                  }}
                >
                  {t.label}
                </button>
              ))}
            </div>

            {activeTab === "play" && (
              <>
                <h3>Choose a name to join</h3>
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
                  maxLength={20}
                  pattern="[A-Za-z0-9_-]{1,20}"
                  title="Use 1-20 characters: letters, numbers, underscores, or hyphens"
                />
                <button
                  onClick={() => {
                    const trimmedName = nameInput.trim();
                    const chosen =
                      trimmedName || `User${Math.floor(Math.random() * 100000).toString().padStart(5, "0")}`;
                    const token = localStorage.getItem("session_token");
                    // If we already have a session, persist rename server-side first
                    if (token) {
                      fetch("/profile/update", {
                        method: "POST",
                        headers: {
                          "Content-Type": "application/json",
                          "X-Session-Token": token,
                        },
                        body: JSON.stringify({ name: chosen }),
                      })
                        .then((r) => r.ok ? r.json() : Promise.reject(r))
                        .then(() => {
                          localStorage.setItem("username", chosen);
                          setUsername(chosen);
                        })
                        .catch(() => {
                          setUsername(chosen); // Fallback: will reconnect and get rejected/accepted
                        });
                      return;
                    }
                    // Close guest connection before joining
                    if (guestCleanupRef.current) {
                      try { guestCleanupRef.current(); } catch {}
                      guestCleanupRef.current = null;
                    }
                    setUsername(chosen);
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
                >
                  Join Game
                </button>
                <div style={{ margin: "15px 0" }}>
                  <div style={{ marginBottom: "10px", fontWeight: "bold" }}>Choose your flag:</div>
                  <FlagSelector
                    value={flagID}
                    onChange={async (id) => {
                      setFlagID(id);
                      localStorage.setItem("flagID", id);
                      const token = localStorage.getItem("session_token");
                      if (token) {
                        try {
                          await fetch("/profile/update", {
                            method: "POST",
                            headers: {
                              "Content-Type": "application/json",
                              "X-Session-Token": token,
                            },
                            body: JSON.stringify({ flagID: id }),
                          });
                        } catch {}
                      }
                    }}
                  />
                </div>
                {joinError && (
                  <div style={{ color: "#c00", marginTop: 8 }}>{joinError}</div>
                )}
                <div style={{ fontSize: 12, color: "#777" }}>You can pan and zoom to explore even before joining.</div>
              </>
            )}

            {activeTab === "leaderboard" && (
              <div style={{ textAlign: "left", maxWidth: 720, margin: "0 auto" }}>
                {lbLoading && <p>Loading leaderboard…</p>}
                {!lbLoading && fullLeaderboard && fullLeaderboard.length === 0 && <p>No players yet.</p>}
                {!lbLoading && Array.isArray(fullLeaderboard) && fullLeaderboard.length > 0 && (
                  <ol>
                    {fullLeaderboard.slice(0, 200).map((row) => (
                      <li key={row.name} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <canvas
                          ref={(c) => {
                            if (!c) return;
                            const ctx = c.getContext("2d");
                            const cssSize = 20;
                            const dpr = window.devicePixelRatio || 1;
                            c.width = Math.round(cssSize * dpr);
                            c.height = Math.round(cssSize * dpr);
                            c.style.width = `${cssSize}px`;
                            c.style.height = `${cssSize}px`;
                            ctx.imageSmoothingEnabled = false;
                            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
                            rendererRef.current
                              .drawSprite(ctx, row.flagID ?? 0, 0, 0, cssSize, cssSize)
                              .catch(console.error);
                          }}
                          style={{ imageRendering: "pixelated" }}
                        />
                        <span style={{ flex: 1 }}>{row.name}</span>
                        <b>{formatScore(row.score ?? 0)}</b>
                      </li>
                    ))}
                  </ol>
                )}
              </div>
            )}

            {activeTab === "advancements" && (
              <div style={{ color: "#666" }}>
                Coming soon: personal milestones, streaks, and discoveries.
              </div>
            )}
          </div>
        </div>
      )}

      {/* Player Score Display */}
      <div className="player-score">Score: {playerScore ?? 0}</div>

      <div className="header">
        <div className="status">
          <div
            className={`connection-status ${connected ? "connected" : "disconnected"}`}
          >
            {connected ? "Connected" : "Disconnected"}
          </div>
        </div>
      </div>

      {/* Help & Scoring dropdown */}
      <div className="help-dropdown">
        <button className="help-button" onClick={() => setHelpOpen((v) => !v)}>
          {helpOpen ? "Close" : "Help & Scoring"}
        </button>
        {helpOpen && (
          <div className="help-content">
            <h3 style={{ marginTop: 0 }}>Scoring</h3>
            <ul style={{ paddingLeft: 18, marginTop: 8 }}>
              <li>Reveal a hidden cell: +1</li>
              <li>Place a flag on a mine: +10</li>
              <li>Wrong flag: -20</li>
              <li>Hit a mine: -100 (ouch!)</li>
              <li>Your total score never goes below 0</li>
            </ul>
            <h4 style={{ marginBottom: 6 }}>Learn Minesweeper</h4>
            <a
              href="https://www.youtube.com/watch?v=ytKOmS8vJng"
              target="_blank"
              rel="noopener noreferrer"
              style={{ fontWeight: 600 }}
            >
              Watch on YouTube
            </a>
          </div>
        )}
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
            const vx = viewRef.current.x;
            const vy = viewRef.current.y;
            const x = (p.worldX * CELL_SIZE - vx) * zoom;
            const y = (p.worldY * CELL_SIZE - vy) * zoom;
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

          {hintPopups.map((h) => {
            const vx = viewRef.current.x;
            const vy = viewRef.current.y;
            const x = (h.worldX * CELL_SIZE - vx) * zoom;
            const y = (h.worldY * CELL_SIZE - vy) * zoom;
            return (
              <div
                key={h.id}
                style={{
                  position: "absolute",
                  left: x,
                  top: y - 20,
                  padding: "2px 6px",
                  background: "rgba(0,0,0,0.7)",
                  color: "#fff",
                  borderRadius: 4,
                  fontSize: 12,
                  pointerEvents: "none",
                  whiteSpace: "nowrap",
                }}
              >
                {h.message}
              </div>
            );
          })}

          {__DEV__ && (
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
            alignItems: "center",
          }}
        >
          <h3 style={{ margin: 0 }}>Leaderboard</h3>
        </div>
        {topPlayers.length > 0 ? (
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {topPlayers.map((p, index) => (
              <li
                key={p.name}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  marginBottom: 2,
                }}
              >
                <span style={{ display: "flex", alignItems: "center" }}>
                  <canvas
                    ref={(c) => {
                      if (!c) return;
                      const ctx = c.getContext("2d");
                      const cssSize = 25; // CSS pixels
                      const dpr = window.devicePixelRatio || 1;
                      c.width = Math.round(cssSize * dpr);
                      c.height = Math.round(cssSize * dpr);
                      c.style.width = `${cssSize}px`;
                      c.style.height = `${cssSize}px`;
                      ctx.imageSmoothingEnabled = false;
                      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
                      rendererRef.current
                        .drawSprite(
                          ctx,
                          playerFlagsRef.current.get(p.name) ?? 0,
                          0,
                          0,
                          cssSize,
                          cssSize,
                        )
                        .catch(console.error);
                    }}
                    key={`flag-${p.name}-${playerFlagsRef.current.get(p.name) ?? 0}`}
                    style={{ marginRight: 6, verticalAlign: "middle", imageRendering: "pixelated" }}
                  />
                  {p.name}
                </span>
                <span style={{ fontWeight: "bold" }}>{formatScore(p.score ?? 0)}</span>
              </li>
            ))}
            {userRank > 10 && (
              <>
                <li style={{
                  borderTop: "1px solid #ccc",
                  marginTop: 4,
                  paddingTop: 4,
                  marginBottom: 2
                }}>
                  <span style={{ fontSize: "12px", color: "#666" }}>
                    You: #{userRank} ({formatScore(playerScore)})
                  </span>
                </li>
              </>
            )}
          </ul>
        ) : (
          <p>No players yet</p>
        )}
      </div>
    </div>
  );
}

export default App;
