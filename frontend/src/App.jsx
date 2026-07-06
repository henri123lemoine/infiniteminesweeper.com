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
import { usePinchPanZoom } from "./hooks/usePinchPanZoom.js";
import { CELL_REVEALED } from "./cellStore.js";
import { initSprites, getFlagIds, drawSprite } from "./sprites/index.js";
import { CHAINS, achievementById } from "./achievements.js";

const isEmbed = new URLSearchParams(window.location.search).has("embed");

// Hand-drawn pixel house in the same style as the flag sprites.
const HOUSE_PIXELS = [
  ".....##.....",
  "....#rr#....",
  "...#rrrr#...",
  "..#rrrrrr#..",
  ".#rrrrrrrr#.",
  "#rrrrrrrrrr#",
  "############",
  ".#wwwwwwww#.",
  ".#ww#dd#ww#.",
  ".#ww#dd#ww#.",
  ".#ww#dd#ww#.",
  ".##########.",
];
const HOUSE_COLORS = {
  "#": "#101010",
  r: "#e31c1c",
  w: "#e0e0e0",
  d: "#7a4a12",
};
const HouseIcon = React.memo(function HouseIcon({ size = 22 }) {
  const grid = HOUSE_PIXELS.length;
  const draw = useCallback((c) => {
    if (!c) return;
    const ctx = c.getContext("2d");
    HOUSE_PIXELS.forEach((row, y) => {
      for (let x = 0; x < row.length; x++) {
        const color = HOUSE_COLORS[row[x]];
        if (!color) continue;
        ctx.fillStyle = color;
        ctx.fillRect(x, y, 1, 1);
      }
    });
  }, []);
  return (
    <canvas
      ref={draw}
      width={grid}
      height={grid}
      style={{
        width: size,
        height: size,
        imageRendering: "pixelated",
        display: "block",
      }}
    />
  );
});

// Pixel-art flag icon; memoized so broadcast ticks don't redraw every row.
// Sized up-front so it never flashes at the default 300x150 canvas size.
const FlagIcon = React.memo(function FlagIcon({ flagID, size = 20 }) {
  const dpr = window.devicePixelRatio || 1;
  const draw = useCallback(
    (c) => {
      if (!c) return;
      initSprites().then(() => {
        if (!c.isConnected) return;
        const ctx = c.getContext("2d");
        ctx.imageSmoothingEnabled = false;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, size, size);
        drawSprite(ctx, flagID ?? 0, 0, 0, size, size);
      });
    },
    [flagID, size, dpr]
  );
  return (
    <canvas
      key={`${flagID}-${size}`}
      ref={draw}
      width={Math.round(size * dpr)}
      height={Math.round(size * dpr)}
      style={{ width: size, height: size, imageRendering: "pixelated" }}
    />
  );
});

// Windowed leaderboard with fully custom scrolling. The offset lives in a
// ref that only user input (wheel, drag, scrollbar) and "find me" can move,
// so data refreshes can never shift the list.
const LB_ROW_H = 26;
const LB_THUMB_MIN = 24;
function VirtualLeaderboard({ rows, myName, formatFullScore, findMeToken }) {
  const containerRef = useRef(null);
  const scrollRef = useRef(0);
  const [viewH, setViewH] = useState(420);
  const [, forceRender] = useState(0);
  const animRef = useRef(null); // find-me / momentum animation frame
  const dragRef = useRef(null); // active pointer drag state

  const contentH = rows.length * LB_ROW_H;
  const maxScroll = Math.max(0, contentH - viewH);
  if (scrollRef.current > maxScroll) scrollRef.current = maxScroll;

  const repaint = useCallback(() => forceRender((n) => n + 1), []);
  const stopAnim = useCallback(() => {
    if (animRef.current) {
      cancelAnimationFrame(animRef.current);
      animRef.current = null;
    }
  }, []);
  const setScroll = useCallback(
    (v) => {
      const next = Math.max(
        0,
        Math.min(rows.length * LB_ROW_H - viewH, v)
      );
      if (next !== scrollRef.current) {
        scrollRef.current = next;
        repaint();
      }
    },
    [rows.length, viewH, repaint]
  );

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => setViewH(el.clientHeight || 420);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Wheel: native listener because React wheel handlers are passive and
  // preventDefault (needed to keep the page still) would be ignored.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (e) => {
      e.preventDefault();
      stopAnim();
      setScroll(scrollRef.current + e.deltaY);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [setScroll, stopAnim]);

  // Content drag (mouse or touch) with light touch momentum
  const onPointerDown = useCallback(
    (e) => {
      if (e.target.closest?.("[data-lb-thumb]")) return; // thumb has its own
      stopAnim();
      e.currentTarget.setPointerCapture(e.pointerId);
      dragRef.current = {
        mode: "content",
        y0: e.clientY,
        scroll0: scrollRef.current,
        lastY: e.clientY,
        lastT: performance.now(),
        velocity: 0,
        touch: e.pointerType === "touch",
      };
    },
    [stopAnim]
  );
  const onPointerMove = useCallback(
    (e) => {
      const d = dragRef.current;
      if (!d) return;
      if (d.mode === "content") {
        setScroll(d.scroll0 - (e.clientY - d.y0));
        const now = performance.now();
        const dt = now - d.lastT;
        if (dt > 0) {
          d.velocity = (d.lastY - e.clientY) / dt; // px per ms, scroll direction
          d.lastY = e.clientY;
          d.lastT = now;
        }
      } else {
        // scrollbar thumb drag: map thumb pixels to content pixels
        const track = viewH - d.thumbH;
        if (track > 0) {
          const frac = (d.thumb0 + (e.clientY - d.y0)) / track;
          setScroll(frac * maxScroll);
        }
      }
    },
    [setScroll, viewH, maxScroll]
  );
  const onPointerUp = useCallback(() => {
    const d = dragRef.current;
    dragRef.current = null;
    // Momentum only for touch flicks; mouse drags stop dead.
    if (d?.mode === "content" && d.touch && Math.abs(d.velocity) > 0.05) {
      let v = d.velocity * 16; // px per frame
      const decay = () => {
        v *= 0.94;
        if (Math.abs(v) < 0.5) {
          animRef.current = null;
          return;
        }
        setScroll(scrollRef.current + v);
        animRef.current = requestAnimationFrame(decay);
      };
      animRef.current = requestAnimationFrame(decay);
    }
  }, [setScroll]);

  const myIndex = useMemo(
    () => (myName ? rows.findIndex((r) => r.name === myName) : -1),
    [rows, myName]
  );

  // One-shot eased scroll to my row; user input cancels it.
  useEffect(() => {
    if (!findMeToken || myIndex < 0) return;
    stopAnim();
    const from = scrollRef.current;
    const to = Math.max(
      0,
      Math.min(maxScroll, myIndex * LB_ROW_H - viewH / 2)
    );
    const t0 = performance.now();
    const DUR = 350;
    const ease = (t) => 1 - (1 - t) ** 3;
    const step = () => {
      const t = Math.min(1, (performance.now() - t0) / DUR);
      setScroll(from + (to - from) * ease(t));
      animRef.current = t < 1 ? requestAnimationFrame(step) : null;
    };
    animRef.current = requestAnimationFrame(step);
    return stopAnim;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [findMeToken]);

  // Scrollbar thumb geometry
  const thumbH =
    maxScroll > 0
      ? Math.max(LB_THUMB_MIN, (viewH / contentH) * viewH)
      : 0;
  const thumbTop =
    maxScroll > 0 ? (scrollRef.current / maxScroll) * (viewH - thumbH) : 0;

  const onThumbDown = useCallback(
    (e) => {
      e.stopPropagation();
      stopAnim();
      e.currentTarget.setPointerCapture(e.pointerId);
      dragRef.current = {
        mode: "thumb",
        y0: e.clientY,
        thumb0: thumbTop,
        thumbH,
      };
    },
    [stopAnim, thumbTop, thumbH]
  );
  const onTrackDown = useCallback(
    (e) => {
      if (e.target.closest?.("[data-lb-thumb]")) return;
      stopAnim();
      const rect = e.currentTarget.getBoundingClientRect();
      const frac =
        (e.clientY - rect.top - thumbH / 2) / Math.max(1, viewH - thumbH);
      setScroll(frac * maxScroll);
    },
    [stopAnim, setScroll, thumbH, viewH, maxScroll]
  );

  const scrollTop = scrollRef.current;
  const start = Math.max(0, Math.floor(scrollTop / LB_ROW_H) - 10);
  const end = Math.min(
    rows.length,
    Math.ceil((scrollTop + viewH) / LB_ROW_H) + 10
  );
  const rankWidth = `${String(rows.length).length}ch`;

  return (
    <div style={{ position: "relative", height: "60vh" }}>
      <div
        ref={containerRef}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        style={{
          position: "absolute",
          inset: 0,
          overflow: "hidden",
          touchAction: "none",
          cursor: "grab",
          userSelect: "none",
        }}
      >
        <div
          style={{
            height: contentH,
            position: "relative",
            transform: `translateY(${-scrollTop}px)`,
            willChange: "transform",
          }}
        >
          {rows.slice(start, end).map((row, i) => {
            const idx = start + i;
            const isMe = myName && row.name === myName;
            return (
              <div
                key={row.name}
                className={isMe ? "lb-row lb-me" : "lb-row"}
                style={{
                  position: "absolute",
                  top: idx * LB_ROW_H,
                  left: 0,
                  right: 14,
                  height: LB_ROW_H - 3,
                }}
              >
                <span className="lb-rank" style={{ width: rankWidth }}>
                  {idx + 1}
                </span>
                <FlagIcon flagID={row.flagID ?? 0} size={20} />
                <span
                  className="lb-name"
                  style={{ fontWeight: isMe ? 600 : undefined }}
                >
                  {row.name}
                </span>
                <span className="lb-score">
                  {formatFullScore(row.score ?? 0)}
                </span>
              </div>
            );
          })}
        </div>
      </div>
      {maxScroll > 0 && (
        <div
          onPointerDown={onTrackDown}
          style={{
            position: "absolute",
            top: 0,
            right: 0,
            bottom: 0,
            width: 10,
            borderRadius: 5,
            background: "rgba(0,0,0,0.08)",
          }}
        >
          <div
            data-lb-thumb
            onPointerDown={onThumbDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerCancel={onPointerUp}
            style={{
              position: "absolute",
              top: thumbTop,
              left: 1,
              right: 1,
              height: thumbH,
              borderRadius: 4,
              background: "rgba(0,0,0,0.35)",
              touchAction: "none",
              cursor: "default",
            }}
          />
        </div>
      )}
    </div>
  );
}

const LeaderboardRow = React.memo(function LeaderboardRow({
  rank,
  rankWidth,
  name,
  score,
  flagID,
  isMe,
  formatted,
}) {
  return (
    <li className={isMe ? "lb-row lb-me" : "lb-row"}>
      <span className="lb-rank" style={{ width: rankWidth }}>
        {rank}
      </span>
      <FlagIcon flagID={flagID} size={20} />
      <span className="lb-name" style={{ fontWeight: isMe ? 600 : undefined }}>
        {name}
      </span>
      <span className="lb-score">{formatted}</span>
    </li>
  );
});

function App() {
  const storedName = localStorage.getItem("username") || "";
  const [nameInput, setNameInput] = useState(storedName);
  const [hotspotInfo, setHotspotInfo] = useState(null);
  const [activeTab, setActiveTab] = useState("play"); // play | leaderboard | advancements
  const [showMinimapOverlay, setShowMinimapOverlay] = useState(false);
  const [fullLeaderboard, setFullLeaderboard] = useState(null);
  const [lbLoading, setLbLoading] = useState(false);
  const lbLoadingRef = useRef(false);
  const [lbFindMeToken, setLbFindMeToken] = useState(0);
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
    handleVisibilityChange,
    serverSpawnRef,
    activePlayersRef,
    advStats,
    unlockedAdvIds,
    unlockedFlagIds,
    advToasts,
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
    [commitViewRef]
  );
  const [zoom, setZoom] = useState(initialZoom);
  const [mainViewMoveToken, setMainViewMoveToken] = useState(0);
  const bumpMainViewMove = useCallback(
    () => setMainViewMoveToken((t) => t + 1),
    []
  );

  // Rendering optimization refs
  const lastRenderTime = useRef(0);
  const renderRequestId = useRef(null);

  // Leaderboard visibility and number formatting
  const [leaderboardVisible, setLeaderboardVisible] = useState(true);
  const [helpOpen, setHelpOpen] = useState(false);
  const [showHomeOverlay, setShowHomeOverlay] = useState(!isEmbed);
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
    [CELL_SIZE]
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
      activePlayersRef,
      // Lets the renderer finish budget-deferred chunk rasters next frame
      requestRerender: () => {
        if (!renderRequestId.current) {
          renderRequestId.current = requestAnimationFrame(() => {
            renderRequestId.current = null;
            render();
          });
        }
      },
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
    const centerWorldX = Math.floor(
      (viewRef.current.x + width / z / 2) / CELL_SIZE
    );
    const centerWorldY = Math.floor(
      (viewRef.current.y + height / z / 2) / CELL_SIZE
    );
    return { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY };
  }, [CELL_SIZE]);

  // Send authoritative viewport update to the server (chunks for gameplay)
  const sendViewportUpdate = useCallback(() => {
    if (!connected && typeof sendViewUpdateRef?.current !== "function") return;
    requestAnimationFrame(() => {
      const { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY } =
        getViewportInfo();
      const { chunkX, chunkY, cell } = worldToChunk(centerWorldX, centerWorldY);
      if (typeof sendViewUpdateRef?.current === "function") {
        sendViewUpdateRef.current(
          chunkX,
          chunkY,
          cell,
          worldWidthCells,
          worldHeightCells
        );
      }
    });
  }, [connected, worldToChunk, getViewportInfo]);

  // Send minimap streaming subscriptions matching the current viewport
  const sendMinimapViewportUpdate = useCallback(() => {
    if (typeof updateMinimapSubscriptions !== "function") return;
    requestAnimationFrame(() => {
      const { worldWidthCells, worldHeightCells, centerWorldX, centerWorldY } =
        getViewportInfo();
      updateMinimapSubscriptions(
        centerWorldX,
        centerWorldY,
        worldWidthCells,
        worldHeightCells,
        2,
        "viewport"
      );
    });
  }, [getViewportInfo, updateMinimapSubscriptions]);

  // Keyboard panning with Arrow keys (and optional WASD)
  useEffect(() => {
    if (!connected) return;
    const handleKeyDown = (e) => {
      // Ignore if typing into inputs or contentEditable elements
      const tag =
        e.target && e.target.tagName ? e.target.tagName.toLowerCase() : "";
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

  // First ever join: surface the Help & Scoring panel once so new players
  // learn the controls and scoring rules without hunting for them.
  useEffect(() => {
    if (connected && !localStorage.getItem("helpSeen")) {
      localStorage.setItem("helpSeen", "1");
      setHelpOpen(true);
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
  }, [
    username,
    showHomeOverlay,
    sendViewportUpdate,
    sendMinimapViewportUpdate,
    seedCache,
  ]);

  // Clear viewport-based minimap subscriptions on unmount
  useEffect(() => {
    return () => {
      if (typeof clearMinimapSubscriptionsFor === "function") {
        try {
          clearMinimapSubscriptionsFor("viewport");
        } catch {}
      }
    };
  }, [clearMinimapSubscriptionsFor]);

  // Handle page visibility changes for mobile reconnection
  useEffect(() => {
    if (typeof handleVisibilityChange === "function") {
      document.addEventListener("visibilitychange", handleVisibilityChange);
      window.addEventListener("focus", handleVisibilityChange);
      return () => {
        document.removeEventListener(
          "visibilitychange",
          handleVisibilityChange
        );
        window.removeEventListener("focus", handleVisibilityChange);
      };
    }
  }, [handleVisibilityChange]);

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
  const centerCameraOnWorld = useCallback(
    (worldX, worldY) => {
      const container = containerRef.current;
      if (!container) return;
      const centerX = container.clientWidth / 2;
      const centerY = container.clientHeight / 2;
      const z = zoomRef.current;
      const newViewX = worldX * CELL_SIZE - centerX / z;
      const newViewY = worldY * CELL_SIZE - centerY / z;
      scheduleViewUpdate(newViewX, newViewY);
    },
    [CELL_SIZE, scheduleViewUpdate]
  );

  // Double-click navigation from any minimap
  const navigateToWorld = useCallback(
    (worldX, worldY) => {
      userMovedRef.current = true;
      centerCameraOnWorld(worldX, worldY);
      bumpMainViewMove();
    },
    [centerCameraOnWorld, bumpMainViewMove]
  );

  // On server-suggested spawn, center the camera once
  const lastAppliedSpawnRef = useRef(0);
  useEffect(() => {
    const maybeApply = () => {
      const rec = serverSpawnRef?.current;
      if (!rec) return;
      if (rec.at && rec.at === lastAppliedSpawnRef.current) return;
      lastAppliedSpawnRef.current = rec.at || Date.now();
      centerCameraOnWorld(rec.x | 0, rec.y | 0);
      sendViewportUpdate();
    };
    maybeApply();
    const id = requestAnimationFrame(maybeApply);
    return () => cancelAnimationFrame(id);
  }, [serverSpawnRef, centerCameraOnWorld, sendViewportUpdate]);

  // Pan/zoom/drag handler using the unified hook
  const { bind } = usePinchPanZoom({
    elementRef: containerRef,
    getPointToWorld: (screenX, screenY) => {
      const world = screenToWorld(screenX, screenY);
      return {
        x: world.x,
        y: world.y,
        viewX: viewRef.current.x,
        viewY: viewRef.current.y,
        zoom: zoomRef.current,
      };
    },
    onPan: (deltaX, deltaY, context) => {
      if (!connected) return;
      userMovedRef.current = true;
      userHasDraggedRef.current = true;
      const z = zoomRef.current;
      scheduleViewUpdate(
        context.viewStartX - deltaX / z,
        context.viewStartY - deltaY / z
      );
      bumpMainViewMove();
    },
    onZoom: (zoomFactor, anchor, context) => {
      if (!connected) return;
      userMovedRef.current = true;

      const MIN_ZOOM = 0.1;
      const MAX_ZOOM = 5;

      if (context.targetZoom) {
        // Pinch zoom with pre-calculated values
        zoomRef.current = context.targetZoom;
        scheduleViewUpdate(context.newViewX, context.newViewY);
        setZoom(context.targetZoom);
      } else {
        // Wheel zoom
        const prevZoom = zoomRef.current;
        const targetZoom = Math.min(
          Math.max(prevZoom * zoomFactor, MIN_ZOOM),
          MAX_ZOOM
        );

        const container = containerRef.current;
        if (!container) return;
        const rect = container.getBoundingClientRect();
        const mouseX = context.x;
        const mouseY = context.y;

        const worldX = viewRef.current.x + mouseX / prevZoom;
        const worldY = viewRef.current.y + mouseY / prevZoom;
        const newViewX = worldX - mouseX / targetZoom;
        const newViewY = worldY - mouseY / targetZoom;

        zoomRef.current = targetZoom;
        scheduleViewUpdate(newViewX, newViewY);
        setZoom(targetZoom);
      }

      bumpMainViewMove();
    },
    onLongPress: (worldX, worldY, isLongPress) => {
      if (!connected) return;

      if (isLongPress === false) {
        // This was a tap/click - reveal cell or chord
        const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
        const isRevealed =
          (revealedCellsRef.current.get(chunkX, chunkY, cell) &
            CELL_REVEALED) !==
          0;
        handleCellClick(worldX, worldY, false, isRevealed);
      } else {
        // This was a right-click or long press - place flag
        handleCellClick(worldX, worldY, true);
      }

      // Commit final view
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
        commitViewRef();
      }
    },
    minZoom: 0.1,
    maxZoom: 5,
    dragDelayMs: 150,
  });

  // Fetch hotspot metadata (no auto-centering; user must drag to move the camera)
  useEffect(() => {
    let canceled = false;
    fetch("/hotspot")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
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
  const centerWorldX = Math.floor(
    (viewX + containerWidth / 2 / debugZoom) / CELL_SIZE
  );
  const centerWorldY = Math.floor(
    (viewY + containerHeight / 2 / debugZoom) / CELL_SIZE
  );
  const {
    chunkX: centerChunkX,
    chunkY: centerChunkY,
    cell: centerCell,
  } = worldToChunk(centerWorldX, centerWorldY);
  const centerDensity = densityCache.current.get(
    `${centerChunkX},${centerChunkY}`
  );

  const handleJoinGame = useCallback(() => {
    setValidationError("");
    const trimmedName = (nameInput || "").trim();
    const valid = /^[A-Za-z0-9 ._'-]{1,30}$/.test(trimmedName);
    if (!valid) {
      setValidationError(
        "Enter 1-30 characters: letters, numbers, spaces, _ . ' or -"
      );
      return;
    }

    if (connected) {
      if (trimmedName === username) {
        // Nothing to change — just get back in the game.
        setShowHomeOverlay(false);
        return;
      }
      // Rename request - wait for server response before closing
      setIsRenameAttempt(true);
      updateProfile(trimmedName, Number(flagID));
    } else {
      // For new users: this is a join request - wait for server confirmation before hiding overlay
      joinGame(trimmedName, Number(flagID));
    }
  }, [nameInput, flagID, connected, username, joinGame, updateProfile]);

  const openHome = useCallback(() => {
    // Keep connection alive; simply show overlay
    setShowHomeOverlay(true);
    setActiveTab("play");
    setNameInput(localStorage.getItem("username") || nameInput);
    // Suggest a contrasting flag based on visible flags near center
    try {
      const z = zoomRef.current;
      const centerX = Math.floor(
        (viewRef.current.x + (containerRef.current?.clientWidth || 0) / 2 / z) /
          CELL_SIZE
      );
      const centerY = Math.floor(
        (viewRef.current.y +
          (containerRef.current?.clientHeight || 0) / 2 / z) /
          CELL_SIZE
      );
      const R = 8; // radius in cells to scan for flags
      const nearby = new Set();
      for (let dy = -R; dy <= R; dy++) {
        for (let dx = -R; dx <= R; dx++) {
          const id = flaggedCellsRef.current.get(
            `${centerX + dx},${centerY + dy}`
          );
          if (Number.isFinite(id)) nearby.add(id);
        }
      }
      const pick = FLAG_IDS.find((id) => !nearby.has(id)) ?? FLAG_IDS[0];
      if (Number.isFinite(pick)) setFlagID(pick);
    } catch {}
  }, [nameInput, FLAG_IDS, CELL_SIZE, flaggedCellsRef]);

  return (
    <div className={isEmbed ? "game-container embed" : "game-container"}>
      {showMinimapOverlay && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "#111",
            zIndex: 30,
          }}
          onDoubleClick={() => setShowMinimapOverlay(false)}
        >
          <div style={{ position: "absolute", top: 8, right: 8, zIndex: 31 }}>
            <button
              onClick={() => setShowMinimapOverlay(false)}
              style={{ padding: "6px 10px" }}
            >
              Close
            </button>
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
            activePlayersRef={activePlayersRef}
            onNavigate={(wx, wy) => {
              navigateToWorld(wx, wy);
              setShowMinimapOverlay(false);
            }}
          />
        </div>
      )}
      {showHomeOverlay && (
        <div
          className="home-backdrop"
          onMouseDown={(e) => {
            if (e.currentTarget === e.target) {
              e.preventDefault();
              // Only allow closing overlay by clicking background if user has successfully joined
              // This prevents users from bypassing the join flow and ending up in a broken spectator state
              if (connected) {
                setShowHomeOverlay(false);
              }
            }
          }}
          onTouchStart={(e) => {
            if (e.currentTarget === e.target) {
              e.preventDefault();
              // Only allow closing overlay by clicking background if user has successfully joined
              // This prevents users from bypassing the join flow and ending up in a broken spectator state
              if (connected) {
                setShowHomeOverlay(false);
              }
            }
          }}
        >
          <div
            className="home-modal"
            onMouseDown={(e) => e.stopPropagation()}
            onTouchStart={(e) => e.stopPropagation()}
          >
            <h1 className="home-title">Infinite Minesweeper</h1>
            <p className="home-subtitle">
              Discover an endless world together. You are currently spectating
              live explorers.
              {hotspotInfo && hotspotInfo.count > 0 && (
                <>
                  {" "}
                  There are <b>{hotspotInfo.count}</b> players near the hotspot.
                </>
              )}
            </p>

            {/* Tabs */}
            <div className="home-tabs">
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
                    if (
                      t.k === "leaderboard" &&
                      fullLeaderboard == null &&
                      !lbLoading
                    ) {
                      fetchHomeLeaderboard();
                    }
                  }}
                  className={
                    activeTab === t.k ? "home-tab active" : "home-tab"
                  }
                >
                  {t.label}
                </button>
              ))}
            </div>

            {activeTab === "play" && (
              <>
                <h3>
                  {connected ? "Change your name" : "Choose a name to join"}
                </h3>
                <input
                  className="join-input"
                  value={nameInput}
                  onChange={(e) => setNameInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleJoinGame();
                  }}
                  placeholder="Your name"
                  maxLength={20}
                  pattern="[A-Za-z0-9_-]{1,30}"
                  title="Use 1-30 characters: letters, numbers, underscores, or hyphens"
                />
                <div>
                  <button className="join-button" onClick={handleJoinGame}>
                    {connected
                      ? (nameInput || "").trim() !== username
                        ? "Rename & Play"
                        : "Play"
                      : "Join Game"}
                  </button>
                </div>
                <div style={{ margin: "15px 0" }}>
                  <div style={{ marginBottom: "10px", fontWeight: "bold" }}>
                    Choose your flag:
                  </div>
                  <FlagSelector
                    value={flagID}
                    unlockedFlagIds={unlockedFlagIds}
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
                  <div
                    style={{ fontSize: 12, color: "#777", marginBottom: 10 }}
                  >
                    Score: {playerScore}
                  </div>
                )}
                {(validationError || joinError || updateError) && (
                  <div style={{ color: "#c00", marginTop: 8 }}>
                    {validationError || (connected ? updateError : joinError)}
                  </div>
                )}
              </>
            )}

            {activeTab === "leaderboard" && (
              <div
                style={{ textAlign: "left", maxWidth: 720, margin: "0 auto" }}
              >
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    marginBottom: 8,
                  }}
                >
                  <div style={{ fontSize: 12, color: "#666" }}>
                    {Array.isArray(fullLeaderboard) &&
                      fullLeaderboard.length > 0 && (
                        <>Players: {fullLeaderboard.length}</>
                      )}
                  </div>
                  <button
                    className="home-tab"
                    style={{ fontSize: 12, padding: "3px 10px" }}
                    onClick={() => setLbFindMeToken((t) => t + 1)}
                  >
                    Find me
                  </button>
                </div>
                {fullLeaderboard == null && lbLoading && (
                  <p>Loading leaderboard…</p>
                )}
                {fullLeaderboard && fullLeaderboard.length === 0 && (
                  <p>No players yet.</p>
                )}
                {/* Must stay mounted through the 5s refreshes or scroll resets */}
                {Array.isArray(fullLeaderboard) &&
                  fullLeaderboard.length > 0 && (
                    <VirtualLeaderboard
                      rows={fullLeaderboard}
                      myName={(
                        localStorage.getItem("username") ||
                        username ||
                        ""
                      ).trim()}
                      formatFullScore={formatFullScore}
                      findMeToken={lbFindMeToken}
                    />
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
                  activePlayersRef={activePlayersRef}
                  onNavigate={(wx, wy) => {
                    navigateToWorld(wx, wy);
                    if (connected) setShowHomeOverlay(false);
                  }}
                />
              </div>
            )}

            {activeTab === "advancements" && (
              <div>
                {!connected && (
                  <p style={{ color: "#666", marginTop: 0 }}>
                    Join the game to start earning advancements.
                  </p>
                )}
                <div className="adv-grid">
                  {CHAINS.map((chain) => {
                    const rewardUnlockCount = unlockedAdvIds.filter(
                      (id) => achievementById.get(id)?.hasReward
                    ).length;
                    const statVal =
                      chain.key === "collector"
                        ? rewardUnlockCount
                        : advStats
                          ? Number(advStats[chain.stat]) || 0
                          : 0;
                    const unlockedCount = chain.levels.filter((l) =>
                      unlockedAdvIds.includes(l.id)
                    ).length;
                    const next = chain.levels[unlockedCount] || null;
                    const pct = next
                      ? Math.min(100, (statVal / next.threshold) * 100)
                      : 100;
                    const roman = ["I", "II", "III", "IV", "V"];
                    return (
                      <div key={chain.key} className="adv-card">
                        <div className="adv-card-head">
                          <span className="adv-card-title">{chain.title}</span>
                          <span className="adv-card-stat">
                            {statVal.toLocaleString()}
                          </span>
                        </div>
                        <div className="adv-card-desc">{chain.desc}</div>
                        <div className="adv-levels">
                          {chain.levels.map((l, i) => {
                            const unlocked = unlockedAdvIds.includes(l.id);
                            return (
                              <div
                                key={l.id}
                                className={
                                  unlocked ? "adv-pip unlocked" : "adv-pip"
                                }
                                title={`${l.name} — ${l.threshold.toLocaleString()}${
                                  l.rewardFlagId ? " · unlocks a flag" : ""
                                }`}
                              >
                                {l.rewardFlagId ? (
                                  <FlagIcon flagID={l.rewardFlagId} size={16} />
                                ) : (
                                  <span style={{ fontSize: 13 }}>★</span>
                                )}
                                <span>{roman[i] || i + 1}</span>
                              </div>
                            );
                          })}
                        </div>
                        {next ? (
                          <>
                            <div className="adv-progress">
                              <div
                                className="adv-progress-fill"
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <div className="adv-next">
                              <span>
                                next: <b>{next.name}</b>
                              </span>
                              <span>
                                {statVal.toLocaleString()} /{" "}
                                {next.threshold.toLocaleString()}
                              </span>
                            </div>
                          </>
                        ) : (
                          <div className="adv-done">Complete ✓</div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Advancement unlock toasts */}
      {advToasts.length > 0 && (
        <div className="adv-toast-stack">
          {advToasts.map((t) => {
            const def = achievementById.get(t.id);
            return (
              <div key={t.toastId} className="adv-toast">
                <span style={{ fontSize: 22 }}>🏆</span>
                <div>
                  <div className="adv-toast-title">Advancement unlocked</div>
                  <div className="adv-toast-name">{def?.name || t.id}</div>
                  {t.rewardFlagId > 0 && (
                    <div style={{ fontSize: 11, color: "#555" }}>
                      New flag unlocked!
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Help & Scoring dropdown */}
      <div className="help-dropdown">
        <div style={{ display: "flex", gap: 6 }}>
          {!showHomeOverlay && (
            <button
              className="help-button"
              onClick={openHome}
              title="Home"
              aria-label="Home"
              style={{ padding: "3px 7px" }}
            >
              <HouseIcon size={22} />
            </button>
          )}
          <button
            className="help-button"
            onClick={() => setHelpOpen((v) => !v)}
          >
            {helpOpen ? "Close" : "Help & Scoring"}
          </button>
        </div>
        {helpOpen && (
          <div className="help-content">
            <h3 style={{ marginTop: 0 }}>Scoring</h3>
            <ul style={{ paddingLeft: 18, marginTop: 8 }}>
              <li>Reveal a hidden cell: +1 × sector multiplier</li>
              <li>Place a flag on a mine: +10 × sector multiplier</li>
              <li>Wrong flag: -20 × sector multiplier</li>
              <li>Hit a mine: -100 × sector multiplier (ouch!)</li>
              <li>
                Flawless streak: +5% per correct action (up to +200%). Any
                mistake resets it.
              </li>
              <li>
                Denser sectors and nearby players raise the multiplier — up to
                15×
              </li>
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
            <div
              style={{
                marginTop: 16,
                paddingTop: 12,
                borderTop: "1px solid #ddd",
                fontSize: 12,
                color: "#888",
              }}
            >
              Website by Henri Lemoine
              <br />
              Art by Johara Goni
            </div>
          </div>
        )}
      </div>

      <div className="board-container">
        <div ref={containerRef} className="canvas-container" {...bind}>
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
              Density:{" "}
              {centerDensity != null ? centerDensity.toFixed(3) : "n/a"}
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
        activePlayersRef={activePlayersRef}
        onNavigate={navigateToWorld}
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
          (() => {
            const showMe =
              connected &&
              username &&
              !topPlayers.some((p) => p.name === username);
            // Rank gutter sized to the widest rank actually shown
            const rankWidth = `${
              String(
                Math.max(topPlayers.length, showMe ? userRank : 0)
              ).length
            }ch`;
            return (
              <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
                {topPlayers.map((p, index) => (
                  <LeaderboardRow
                    key={p.name}
                    rank={index + 1}
                    rankWidth={rankWidth}
                    name={p.name}
                    score={p.score ?? 0}
                    formatted={formatScore(p.score ?? 0)}
                    flagID={p.flagID ?? playerFlagsRef.current.get(p.name) ?? 0}
                    isMe={connected && p.name === username}
                  />
                ))}
                {showMe && (
                  <>
                    <li className="lb-divider">⋯</li>
                    <LeaderboardRow
                      rank={userRank > 0 ? userRank : "—"}
                      rankWidth={rankWidth}
                      name={username}
                      score={playerScore}
                      formatted={formatFullScore(playerScore ?? 0)}
                      flagID={flagID}
                      isMe
                    />
                  </>
                )}
              </ul>
            );
          })()
        ) : (
          <p>No players yet</p>
        )}
      </div>
    </div>
  );
}

export default App;
