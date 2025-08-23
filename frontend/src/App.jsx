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
import { initSprites, getFlagIds } from "./sprites/index.js";

function App() {
  const storedName = localStorage.getItem("username") || "";
  const [nameInput, setNameInput] = useState(storedName);
  const [hotspotInfo, setHotspotInfo] = useState(null);
  const [activeTab, setActiveTab] = useState("play"); // play | leaderboard | advancements
  const [showMinimapOverlay, setShowMinimapOverlay] = useState(false);
  const [fullLeaderboard, setFullLeaderboard] = useState(null);
  const [lbLoading, setLbLoading] = useState(false);
  const lbLoadingRef = useRef(false);
  const [lbFollowMe, setLbFollowMe] = useState(true);
  const myLbRowRef = useRef(null);
  const [validationError, setValidationError] = useState("");
  const [isRenameAttempt, setIsRenameAttempt] = useState(false);

  const {
    connected,
    playerScore,
    userRank,
    username,
    setUsername,
    connectWebSocket,
    joinGame,
    updateProfile,
    updateError,
    updateSuccess,
    joinError,
    serverFlagID,
    disconnect,
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
    worldToChunk,
    densityCache,
    sendViewUpdateRef,
    updateMinimapSubscriptions,
     clearMinimapSubscriptionsFor,
    minimapTilesRef,
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
  // Zoom: use ref for frame-accurate math/rendering, state for UI/re-renders
  // Start with mobile-friendly zoom level
  const initialZoom = useMemo(() => {
    const isMobile = window.innerWidth <= 768 || window.innerHeight <= 768;
    return isMobile ? 0.7 : 1; // Much more zoomed out on mobile
  }, []);
  const zoomRef = useRef(initialZoom);
  const rafRef = useRef(null);
  const guestCleanupRef = useRef(null);
  const userMovedRef = useRef(false);
  const userHasDraggedRef = useRef(false);

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
  const [zoom, setZoom] = useState(initialZoom);
  const [mainViewMoveToken, setMainViewMoveToken] = useState(0);
  const bumpMainViewMove = useCallback(() => setMainViewMoveToken((t) => t + 1), []);
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
      const zoomFactor = Math.exp(-e.deltaY * 0.0007);

      const prevZoom = zoomRef.current;
      const targetZoom = Math.min(Math.max(prevZoom * zoomFactor, MIN_ZOOM), MAX_ZOOM);

      // Anchor the zoom on the mouse position for intuitive behavior
      const worldX = viewRef.current.x + mouseX / prevZoom;
      const worldY = viewRef.current.y + mouseY / prevZoom;
      const newViewX = worldX - mouseX / targetZoom;
      const newViewY = worldY - mouseY / targetZoom;

      // Update ref immediately for in-frame math/rendering; state for UI/re-render
      zoomRef.current = targetZoom;
      scheduleViewUpdate(newViewX, newViewY);
      setZoom(targetZoom);
      bumpMainViewMove();
    },
    [containerRef, scheduleViewUpdate, bumpMainViewMove]
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
  const [showHomeOverlay, setShowHomeOverlay] = useState(true);
  const lbRefreshTimerRef = useRef(null);

  // Build numeric sprite ID list for the 'flag' category only
  const FLAG_IDS = useMemo(() => getFlagIds(), []);

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
  const getScoreColor = useCallback((delta) => {
    if (delta > 0) return "#fff";
    if (delta < 0) return "#f00";
    return "#666"; // shouldn't happen
  }, []);

  // Centralized home leaderboard fetcher (stable; guarded by ref to avoid re-runs)
  const fetchHomeLeaderboard = useCallback(() => {
    if (lbLoadingRef.current) return;
    lbLoadingRef.current = true;
    setLbLoading(true);
    fetch("/leaderboard")
      .then((r) => (r.ok ? r.json() : []))
      .then((rows) => setFullLeaderboard(Array.isArray(rows) ? rows : []))
      .catch(() => setFullLeaderboard([]))
      .finally(() => {
        lbLoadingRef.current = false;
        setLbLoading(false);
      });
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

  const formatFullScore = useCallback((score) => {
    // Format scores with commas for readability, e.g. 128,318,481
    return score.toLocaleString();
  }, []);

  // Constants
  const computeMinimapSize = useCallback(() => {
    const shortSide = Math.min(window.innerWidth || 0, window.innerHeight || 0);
    const isMobile = window.innerWidth <= 768 || window.innerHeight <= 768;
    const target = shortSide * (isMobile ? 0.18 : 0.22); // Smaller percentage on mobile
    return Math.round(Math.max(isMobile ? 120 : 160, Math.min(360, target))); // Smaller min size on mobile
  }, []);
  const [MINIMAP_SIZE, setMINIMAP_SIZE] = useState(() => computeMinimapSize());
  const BASE_CELL_SIZE = 32;
  const CELL_SIZE = BASE_CELL_SIZE;

  // Convert screen coordinates to world coordinates
  const screenToWorld = useCallback(
    (screenX, screenY) => {
      const container = containerRef.current;
      if (!container) return { x: 0, y: 0 };

      const rect = container.getBoundingClientRect();
      const z = zoomRef.current;
      const canvasX = (screenX - rect.left) / z;
      const canvasY = (screenY - rect.top) / z;

      // Convert canvas pixels to world coordinates
      const worldX = Math.floor((canvasX + viewRef.current.x) / CELL_SIZE);
      const worldY = Math.floor((canvasY + viewRef.current.y) / CELL_SIZE);

      return { x: worldX, y: worldY };
    },
    [CELL_SIZE],
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
      zoom: zoomRef.current,
      tick,
      CELL_SIZE,
      revealedCellsRef,
      flaggedCellsRef,
      chunkVersionRef,
      worldToChunk,
      getNumberColor,
      flagID,
    });
  }, [tick, getNumberColor, worldToChunk, flagID]);

  // Compute current viewport center and size in world cells
  const getViewportInfo = useCallback(() => {
    const container = containerRef.current;
    const width = container?.clientWidth || window.innerWidth || 0;
    const height = container?.clientHeight || window.innerHeight || 0;
    const z = zoomRef.current;
    const worldWidthCells = Math.ceil(width / z / CELL_SIZE);
    const worldHeightCells = Math.ceil(height / z / CELL_SIZE);
    const centerWorldX = Math.floor((viewRef.current.x + (width / z) / 2) / CELL_SIZE);
    const centerWorldY = Math.floor((viewRef.current.y + (height / z) / 2) / CELL_SIZE);
    return { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY };
  }, [CELL_SIZE]);

  // Send authoritative viewport update to the server (chunks for gameplay)
  const sendViewportUpdate = useCallback(() => {
    if (!connected && typeof sendViewUpdateRef?.current !== 'function') return;
    requestAnimationFrame(() => {
      const { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY } = getViewportInfo();
      const { chunkX, chunkY, cell } = worldToChunk(centerWorldX, centerWorldY);
      if (typeof sendViewUpdateRef?.current === 'function') {
        sendViewUpdateRef.current(chunkX, chunkY, cell, worldWidthCells, worldHeightCells);
      }
    });
  }, [connected, worldToChunk, getViewportInfo]);

  // Send minimap streaming subscriptions matching the current viewport
  const sendMinimapViewportUpdate = useCallback(() => {
    if (typeof updateMinimapSubscriptions !== 'function') return;
    requestAnimationFrame(() => {
      const { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY } = getViewportInfo();
      updateMinimapSubscriptions(centerWorldX, centerWorldY, worldWidthCells, worldHeightCells, 1, 'viewport');
    });
  }, [getViewportInfo, updateMinimapSubscriptions]);

  // Mouse event handlers
  const handleMouseDown = useCallback(
    (e) => {
      e.preventDefault(); // Prevent context menu and other default behaviors
      if (!connected) return;
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
      if (!connected || !dragStart.x) return; // No mouse down recorded or not joined

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
        userHasDraggedRef.current = true;
        const z = zoomRef.current;
        const deltaX = (e.clientX - dragStart.x) / z;
        const deltaY = (e.clientY - dragStart.y) / z;

        scheduleViewUpdate(dragStart.viewX - deltaX, dragStart.viewY - deltaY);
        bumpMainViewMove();
      }
    },
    [isDragging, dragStart, scheduleViewUpdate, bumpMainViewMove],
  );

  const handleMouseUp = useCallback(
    (e) => {
      if (!connected) return;
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

  // Touch state for pinch-to-zoom
  const touchStateRef = useRef({
    touches: [],
    initialDistance: 0,
    initialZoom: 1,
    initialMidpoint: { x: 0, y: 0 },
    initialViewX: 0,
    initialViewY: 0,
  });

  // Touch event handlers for mobile
  const handleTouchStart = useCallback(
    (e) => {
      if (!connected) return;
      e.preventDefault();

      const touches = Array.from(e.touches).map(t => ({
        id: t.identifier,
        x: t.clientX,
        y: t.clientY,
      }));

      touchStateRef.current.touches = touches;

      if (touches.length === 1) {
        // Single touch - prepare for panning
        userMovedRef.current = true;
        setDragStart({
          x: touches[0].x,
          y: touches[0].y,
          viewX: viewRef.current.x,
          viewY: viewRef.current.y,
        });

        // Start long press timer for flagging
        dragTimeoutRef.current = setTimeout(() => {
          // This is a long press - place a flag
          const { x: worldX, y: worldY } = screenToWorld(touches[0].x, touches[0].y);
          handleCellClick(worldX, worldY, true); // true = right click (flag)
          // Clear drag start to prevent any further actions
          setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });
        }, 200);
      } else if (touches.length === 2) {
        // Two fingers - prepare for pinch zoom
        if (dragTimeoutRef.current) {
          clearTimeout(dragTimeoutRef.current);
          dragTimeoutRef.current = null;
        }
        setIsDragging(false);

        const dx = touches[1].x - touches[0].x;
        const dy = touches[1].y - touches[0].y;
        const distance = Math.sqrt(dx * dx + dy * dy);
        const midX = (touches[0].x + touches[1].x) / 2;
        const midY = (touches[0].y + touches[1].y) / 2;

        touchStateRef.current.initialDistance = distance;
        touchStateRef.current.initialZoom = zoomRef.current;
        touchStateRef.current.initialMidpoint = { x: midX, y: midY };
        touchStateRef.current.initialViewX = viewRef.current.x;
        touchStateRef.current.initialViewY = viewRef.current.y;
      }
    },
    [screenToWorld, handleCellClick, connected],
  );

  const handleTouchMove = useCallback(
    (e) => {
      e.preventDefault(); // Prevent scrolling
      if (!connected) return;

      const touches = Array.from(e.touches).map(t => ({
        id: t.identifier,
        x: t.clientX,
        y: t.clientY,
      }));

      if (touches.length === 1 && dragStart.x) {
        // Single touch panning
        const touch = touches[0];
        const dx = Math.abs(touch.x - dragStart.x);
        const dy = Math.abs(touch.y - dragStart.y);

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
          userHasDraggedRef.current = true;
          const z = zoomRef.current;
          const deltaX = (touch.x - dragStart.x) / z;
          const deltaY = (touch.y - dragStart.y) / z;

          scheduleViewUpdate(
            dragStart.viewX - deltaX,
            dragStart.viewY - deltaY,
          );
          bumpMainViewMove();
        }
      } else if (touches.length === 2) {
        // Two finger pinch zoom
        const dx = touches[1].x - touches[0].x;
        const dy = touches[1].y - touches[0].y;
        const distance = Math.sqrt(dx * dx + dy * dy);
        const midX = (touches[0].x + touches[1].x) / 2;
        const midY = (touches[0].y + touches[1].y) / 2;

        const { initialDistance, initialZoom, initialMidpoint, initialViewX, initialViewY } = touchStateRef.current;

        if (initialDistance > 0) {
          const container = containerRef.current;
          if (!container) return;

          const zoomFactor = distance / initialDistance;
          const MIN_ZOOM = 0.1;
          const MAX_ZOOM = 5;
          const targetZoom = Math.min(Math.max(initialZoom * zoomFactor, MIN_ZOOM), MAX_ZOOM);

          // Calculate world coordinates at the midpoint during initial touch
          const worldX = initialViewX + initialMidpoint.x / initialZoom;
          const worldY = initialViewY + initialMidpoint.y / initialZoom;

          // Calculate new view position to keep the midpoint stable
          const newViewX = worldX - midX / targetZoom;
          const newViewY = worldY - midY / targetZoom;

          // Update zoom and view
          zoomRef.current = targetZoom;
          scheduleViewUpdate(newViewX, newViewY);
          setZoom(targetZoom);
          bumpMainViewMove();
        }
      }
    },
    [isDragging, dragStart, scheduleViewUpdate, bumpMainViewMove, connected],
  );

  const handleTouchEnd = useCallback(
    (e) => {
      if (!connected) return;
      
      // Clear the long press timeout
      if (dragTimeoutRef.current) {
        clearTimeout(dragTimeoutRef.current);
        dragTimeoutRef.current = null;
      }

      const remainingTouches = e.touches.length;

      if (remainingTouches === 0) {
        // All fingers lifted
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

        // Reset all touch state
        setIsDragging(false);
        setDragStart({ x: 0, y: 0, viewX: 0, viewY: 0 });
        touchStateRef.current = {
          touches: [],
          initialDistance: 0,
          initialZoom: 1,
          initialMidpoint: { x: 0, y: 0 },
          initialViewX: 0,
          initialViewY: 0,
        };

        // Cancel any pending RAF and commit final view
        if (rafRef.current) {
          cancelAnimationFrame(rafRef.current);
          rafRef.current = null;
          commitViewRef();
        }
      } else if (remainingTouches === 1 && touchStateRef.current.touches.length === 2) {
        // Went from two fingers to one finger - transition to single touch mode
        const remainingTouch = e.touches[0];
        setIsDragging(false);
        setDragStart({
          x: remainingTouch.clientX,
          y: remainingTouch.clientY,
          viewX: viewRef.current.x,
          viewY: viewRef.current.y,
        });
        
        // Reset pinch state but keep the remaining touch for potential panning
        touchStateRef.current.touches = [{
          id: remainingTouch.identifier,
          x: remainingTouch.clientX,
          y: remainingTouch.clientY,
        }];
        touchStateRef.current.initialDistance = 0;
      }
    },
    [
      isDragging,
      dragStart,
      screenToWorld,
      handleCellClick,
      commitViewRef,
      connected,
      worldToChunk,
    ],
  );

  // Attach non-passive touch listeners to prevent default behaviors
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener("touchstart", handleTouchStart, { passive: false });
    el.addEventListener("touchmove", handleTouchMove, { passive: false });
    el.addEventListener("touchend", handleTouchEnd, { passive: false });
    return () => {
      el.removeEventListener("touchstart", handleTouchStart);
      el.removeEventListener("touchmove", handleTouchMove);
      el.removeEventListener("touchend", handleTouchEnd);
    };
  }, [handleTouchStart, handleTouchMove, handleTouchEnd]);

  // Keyboard panning with Arrow keys (and optional WASD)
  useEffect(() => {
    if (!connected) return;
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
      userMovedRef.current = true; // keyboard pan shouldn't trigger minimap recenter
      const dxPx = dxCells * CELL_SIZE;
      const dyPx = dyCells * CELL_SIZE;
      // ArrowRight increases viewX to move camera right; ArrowDown increases viewY
      scheduleViewUpdate(viewRef.current.x + dxPx, viewRef.current.y + dyPx);
      bumpMainViewMove();
    };

    window.addEventListener("keydown", handleKeyDown, { passive: false });
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [CELL_SIZE, scheduleViewUpdate, connected, bumpMainViewMove]);

  // Single WebSocket connection that starts in spectator mode
  useEffect(() => {
    const cleanup = connectWebSocket();
    return cleanup;
  }, [connectWebSocket]);

  // Auto-join when username is set (triggered by joinGame)
  useEffect(() => {
    if (username && !connected) {
      // This happens when username is set but we haven't successfully joined yet
      // The join will happen via the joinGame function call
    }
  }, [username, connected]);

  // Initialize nameInput with current username when first connecting
  useEffect(() => {
    if (connected && username && nameInput === "") {
      setNameInput(username);
    }
  }, [connected, username]);

  // Sync flagID with server's authoritative flagID when it changes
  useEffect(() => {
    if (serverFlagID !== null && flagID !== serverFlagID) {
      setFlagID(serverFlagID);
    }
  }, [serverFlagID, flagID]);

  // Close home overlay when rename attempt succeeds
  useEffect(() => {
    if (updateSuccess && isRenameAttempt) {
      setShowHomeOverlay(false);
      setIsRenameAttempt(false); // Reset the flag
    }
  }, [updateSuccess, isRenameAttempt]);

  // Close home overlay when join succeeds (connected becomes true)
  useEffect(() => {
    if (connected) {
      setShowHomeOverlay(false);
    }
  }, [connected]);

  // Ensure overlay is shown when join fails
  useEffect(() => {
    if (joinError && !connected) {
      setShowHomeOverlay(true);
    }
  }, [joinError, connected]);

  // Reset rename attempt flag when there's an error
  useEffect(() => {
    if (updateError && isRenameAttempt) {
      setIsRenameAttempt(false); // Reset the flag so user can try again
    }
  }, [updateError, isRenameAttempt]);

  // Send view-based updates when view changes
  useEffect(() => {
    sendViewportUpdate();
    sendMinimapViewportUpdate();
  }, [sendViewportUpdate, sendMinimapViewportUpdate, viewX, viewY]);

  // Also send once on connect/zoom change and on resize
  useEffect(() => {
    if (connected) {
      sendViewportUpdate();
      sendMinimapViewportUpdate();
    }
  }, [connected, sendViewportUpdate, sendMinimapViewportUpdate, zoom]);

  useEffect(() => {
    const onResize = () => {
      sendViewportUpdate();
      sendMinimapViewportUpdate();
      setMINIMAP_SIZE(computeMinimapSize());
    };
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [sendViewportUpdate, sendMinimapViewportUpdate, computeMinimapSize]);

  // Sync viewRef to sessionStorage
  useEffect(() => {
    viewRef.current.x = viewX;
    viewRef.current.y = viewY;
    sessionStorage.setItem("viewX", String(viewX));
    sessionStorage.setItem("viewY", String(viewY));
  }, [viewX, viewY]);

  // When on the home overlay (spectating), proactively nudge view updates
  // until we receive at least one chunk so the background animates quickly.
  useEffect(() => {
    if (username) return; // only in spectate mode
    if (!showHomeOverlay) return;
    // If we already have any chunk seed, no need to spam
    if (seedCache.current && seedCache.current.size > 0) return;
    // Kick immediately and then retry a few times
    sendViewportUpdate();
    sendMinimapViewportUpdate();
    let tries = 0;
    const id = setInterval(() => {
      if (seedCache.current && seedCache.current.size > 0) {
        clearInterval(id);
        return;
      }
      if (tries++ > 8) {
        clearInterval(id);
        return;
      }
      sendViewportUpdate();
      sendMinimapViewportUpdate();
    }, 500);
    return () => clearInterval(id);
  }, [username, showHomeOverlay, sendViewportUpdate, sendMinimapViewportUpdate, seedCache]);

  // Clear viewport-based minimap subscriptions on unmount
  useEffect(() => {
    return () => {
      if (typeof clearMinimapSubscriptionsFor === 'function') {
        try { clearMinimapSubscriptionsFor('viewport'); } catch {}
      }
    };
  }, [clearMinimapSubscriptionsFor]);

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
    initSprites();
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
    const z = zoomRef.current;
    const newViewX = worldX * CELL_SIZE - centerX / z;
    const newViewY = worldY * CELL_SIZE - centerY / z;
    scheduleViewUpdate(newViewX, newViewY);
  }, [CELL_SIZE, scheduleViewUpdate]);

  // Fetch hotspot metadata (no auto-centering; user must drag to move the camera)
  useEffect(() => {
    let canceled = false;
    fetch("/hotspot")
      .then(r => (r.ok ? r.json() : null))
      .then(data => {
        if (canceled || !data) return;
        setHotspotInfo(data);
      })
      .catch(() => {});
    return () => {
      canceled = true;
    };
  }, []);

  // Auto-refresh home leaderboard while the tab is open
  useEffect(() => {
    if (showHomeOverlay && activeTab === "leaderboard") {
      // Fetch immediately and then on an interval
      fetchHomeLeaderboard();
      lbRefreshTimerRef.current = setInterval(fetchHomeLeaderboard, 5000);
      return () => {
        if (lbRefreshTimerRef.current) {
          clearInterval(lbRefreshTimerRef.current);
          lbRefreshTimerRef.current = null;
        }
      };
    }
    // Cleanup when leaving the tab/overlay
    if (lbRefreshTimerRef.current) {
      clearInterval(lbRefreshTimerRef.current);
      lbRefreshTimerRef.current = null;
    }
  }, [showHomeOverlay, activeTab, fetchHomeLeaderboard]);

  // Keep my row centered if "Follow me" is enabled
  useEffect(() => {
    if (!showHomeOverlay || activeTab !== "leaderboard") return;
    if (!lbFollowMe) return;
    const el = myLbRowRef.current;
    if (el && typeof el.scrollIntoView === "function") {
      el.scrollIntoView({ block: "center" });
    }
  }, [fullLeaderboard, showHomeOverlay, activeTab, lbFollowMe]);

  // No separate spectator connection needed - unified connection starts in spectator mode

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
  const debugZoom = zoomRef.current;
  const centerWorldX = Math.floor((viewX + containerWidth / 2 / debugZoom) / CELL_SIZE);
  const centerWorldY = Math.floor((viewY + containerHeight / 2 / debugZoom) / CELL_SIZE);
  const { chunkX: centerChunkX, chunkY: centerChunkY, cell: centerCell } = worldToChunk(
    centerWorldX,
    centerWorldY,
  );
  const centerDensity = densityCache.current.get(`${centerChunkX},${centerChunkY}`);

  const handleJoinGame = useCallback(() => {
    setValidationError("");
    const trimmedName = (nameInput || "").trim();
    const valid = /^[A-Za-z0-9_-]{1,20}$/.test(trimmedName);
    if (!valid) {
      setValidationError("Enter 1-20 characters: letters, numbers, _ or -");
      return;
    }
    
    if (connected) {
      // For connected users: this is a rename request - wait for server response
      setIsRenameAttempt(true);
      updateProfile(trimmedName, Number(flagID));
      // Don't close overlay yet - wait for updateAck response
    } else {
      // For new users: this is a join request - wait for server confirmation before hiding overlay
      joinGame(trimmedName, Number(flagID));
    }
  }, [nameInput, flagID, connected, joinGame, updateProfile]);

  return (
    <div className="game-container">
      {/* Home button (hidden on homepage) */}
      {!showHomeOverlay && (
        <button
          onClick={() => {
            // Keep connection alive; simply show overlay
            setShowHomeOverlay(true);
            setActiveTab("play");
            setNameInput(localStorage.getItem("username") || nameInput);
            // Suggest a contrasting flag based on visible flags near center
            try {
              const z = zoomRef.current;
              const centerX = Math.floor((viewRef.current.x + (containerRef.current?.clientWidth || 0) / 2 / z) / CELL_SIZE);
              const centerY = Math.floor((viewRef.current.y + (containerRef.current?.clientHeight || 0) / 2 / z) / CELL_SIZE);
              const R = 8; // radius in cells to scan for flags
              const nearby = new Set();
              for (let dy = -R; dy <= R; dy++) {
                for (let dx = -R; dx <= R; dx++) {
                  const id = flaggedCellsRef.current.get(`${centerX + dx},${centerY + dy}`);
                  if (Number.isFinite(id)) nearby.add(id);
                }
              }
              const pick = FLAG_IDS.find(id => !nearby.has(id)) ?? FLAG_IDS[0];
              if (Number.isFinite(pick)) setFlagID(pick);
            } catch {}
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
      )}
      {showMinimapOverlay && (
        <div
          style={{ position: "fixed", inset: 0, background: "#111", zIndex: 30 }}
          onDoubleClick={() => setShowMinimapOverlay(false)}
        >
          <div style={{ position: "absolute", top: 8, right: 8, zIndex: 31 }}>
            <button onClick={() => setShowMinimapOverlay(false)} style={{ padding: "6px 10px" }}>Close</button>
          </div>
          <Minimap
            mode="overlay"
            CELL_SIZE={CELL_SIZE}
            zoom={zoom}
            viewX={viewX}
            viewY={viewY}
            containerRef={containerRef}
            mainViewMoveToken={mainViewMoveToken}
            updateMinimapSubscriptions={updateMinimapSubscriptions}
            clearMinimapSubscriptionsFor={clearMinimapSubscriptionsFor}
            minimapTilesRef={minimapTilesRef}
          />
        </div>
      )}
      {showHomeOverlay && (
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
          onMouseDown={(e) => {
            if (e.currentTarget === e.target) {
              e.preventDefault();
              setShowHomeOverlay(false);
            }
          }}
          onTouchStart={(e) => {
            if (e.currentTarget === e.target) {
              e.preventDefault();
              setShowHomeOverlay(false);
            }
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
              WebkitOverflowScrolling: "touch",
              touchAction: "pan-y",
            }}
            onMouseDown={(e) => e.stopPropagation()}
            onTouchStart={(e) => e.stopPropagation()}
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
                { k: "minimap", label: "Minimap" },
                { k: "advancements", label: "Advancements" },
              ].map((t) => (
                <button
                  key={t.k}
                  onClick={() => {
                    setActiveTab(t.k);
                    if (t.k === "leaderboard" && fullLeaderboard == null && !lbLoading) {
                      fetchHomeLeaderboard();
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
                <h3>{connected ? "Change your name" : "Choose a name to join"}</h3>
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
                  onClick={handleJoinGame}
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
                  {connected ? "Update Name" : "Join Game"}
                </button>
                <div style={{ margin: "15px 0" }}>
                  <div style={{ marginBottom: "10px", fontWeight: "bold" }}>Choose your flag:</div>
                  <FlagSelector
                    value={flagID}
                    onChange={(id) => {
                      setFlagID(id);
                      localStorage.setItem("flagID", id);
                      // If connected as player, update profile on server
                      if (connected && username) {
                        updateProfile(username, id);
                      }
                    }}
                  />
                </div>
                {connected && (
                  <div style={{ fontSize: 12, color: "#777", marginBottom: 10 }}>Score: {playerScore}</div>
                )}
                {(validationError || joinError || updateError) && (
                  <div style={{ color: "#c00", marginTop: 8 }}>
                    {validationError || (connected ? updateError : joinError)}
                  </div>
                )}
              </>
            )}

            {activeTab === "leaderboard" && (
              <div style={{ textAlign: "left", maxWidth: 720, margin: "0 auto" }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
                  <div style={{ fontSize: 12, color: "#666" }}>
                    {Array.isArray(fullLeaderboard) && fullLeaderboard.length > 0 && (
                      <>Players: {fullLeaderboard.length}</>
                    )}
                  </div>
                  <label style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 6 }}>
                    <input
                      type="checkbox"
                      checked={lbFollowMe}
                      onChange={(e) => setLbFollowMe(e.target.checked)}
                    />
                    Follow me
                  </label>
                </div>
                {lbLoading && fullLeaderboard == null && <p>Loading leaderboard…</p>}
                {!lbLoading && fullLeaderboard && fullLeaderboard.length === 0 && <p>No players yet.</p>}
                {!lbLoading && Array.isArray(fullLeaderboard) && fullLeaderboard.length > 0 && (
                  <ol style={{ 
                    maxHeight: "60vh", 
                    overflow: "auto", 
                    paddingRight: 8,
                    WebkitOverflowScrolling: "touch",
                    scrollbarWidth: "thin"
                  }}>
                    {fullLeaderboard.map((row) => {
                      const myName = (localStorage.getItem("username") || username || "").trim();
                      const isMe = myName && row.name === myName;
                      return (
                        <li
                          key={row.name}
                          ref={isMe ? myLbRowRef : null}
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                            padding: "2px 4px",
                            borderRadius: 4,
                            background: isMe ? "#fff8d5" : "transparent",
                          }}
                        >
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
                              rendererRef.current.drawSprite(
                                ctx,
                                row.flagID ?? 0,
                                0,
                                0,
                                cssSize,
                                cssSize,
                              );
                            }}
                            style={{ imageRendering: "pixelated" }}
                          />
                          <span style={{ flex: 1, fontWeight: isMe ? "600" : undefined }}>{row.name}</span>
                          <b>{formatFullScore(row.score ?? 0)}</b>
                        </li>
                      );
                    })}
                  </ol>
                )}
              </div>
            )}

            {activeTab === "minimap" && (
              <div style={{ width: "100%", height: "72vh" }}>
                <Minimap
                  mode="overlay"
                  CELL_SIZE={CELL_SIZE}
                  zoom={zoom}
                  viewX={viewX}
                  viewY={viewY}
                  containerRef={containerRef}
                  mainViewMoveToken={mainViewMoveToken}
                  updateMinimapSubscriptions={updateMinimapSubscriptions}
                  clearMinimapSubscriptionsFor={clearMinimapSubscriptionsFor}
                  minimapTilesRef={minimapTilesRef}
                />
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
          style={{ touchAction: "none" }} // Prevent default touch behaviors
        >
          <canvas ref={canvasRef} id="game-canvas" />

          {scorePopups.map((p) => {
            const vx = viewRef.current.x;
            const vy = viewRef.current.y;
            const z = zoomRef.current;
            const x = (p.worldX * CELL_SIZE - vx) * z;
            const y = (p.worldY * CELL_SIZE - vy) * z;
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
            const z = zoomRef.current;
            const x = (h.worldX * CELL_SIZE - vx) * z;
            const y = (h.worldY * CELL_SIZE - vy) * z;
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
              Chunk: ({centerChunkX}, {centerChunkY})<br />
              Density: {centerDensity != null ? centerDensity.toFixed(3) : "n/a"}
            </div>
          )}
        </div>
      </div>

      <Minimap
        mode="hud"
        CHUNK={CHUNK}
        CELL_SIZE={CELL_SIZE}
        MINIMAP_SIZE={MINIMAP_SIZE}
        zoom={zoom}
        viewX={viewX}
        viewY={viewY}
        containerRef={containerRef}
        updateMinimapSubscriptions={updateMinimapSubscriptions}
        minimapTilesRef={minimapTilesRef}
        onOpenOverlay={() => setShowMinimapOverlay(true)}
        mainViewMoveToken={mainViewMoveToken}
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
                        );
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
