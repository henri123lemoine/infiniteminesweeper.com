import React, { useRef, useState, useEffect, useCallback } from "react";
import { CHUNK } from './useGameState.js';
import { usePinchPanZoom } from './hooks/usePinchPanZoom.js';

export default function Minimap({
  mode = "hud", // "hud" or "overlay"
  // HUD mode props
  CELL_SIZE,
  MINIMAP_SIZE,
  zoom,
  viewX,
  viewY,
  containerRef,
  onOpenOverlay,
  mainViewMoveToken,
  // Common props
  updateMinimapSubscriptions,
  clearMinimapSubscriptionsFor,
  minimapTilesRef,
}) {
  const canvasRef = useRef(null);
  const localContainerRef = useRef(null);
  const effectiveContainerRef = containerRef || localContainerRef;

  // State management based on mode
  const [hudView, setHudView] = useState({ cx: 0, cy: 0, cells: CHUNK * 3 });
  const [overlayView, setOverlayView] = useState({ x: 0, y: 0, zoom: 2 });

  const hudViewRef = useRef(hudView);
  const overlayViewRef = useRef(overlayView);
  const mainViewRef = useRef({ viewX: viewX || 0, viewY: viewY || 0, zoom: zoom || 1 });

  // Interaction state
  const userHasDraggedRef = useRef(false);
  const rafRef = useRef(null);

  // Calculate appropriate minimap resolution based on zoom level
  const calculateMinimapResolution = useCallback((mode, cells, overlayZoom, containerWidth = 0, containerHeight = 0) => {
    let chunksInView;

    if (mode === "hud") {
      // HUD mode: calculate how many chunks are in view
      chunksInView = (cells / CHUNK) * (cells / CHUNK);
    } else {
      // Overlay mode: calculate based on overlay zoom and container size
      const widthInCells = containerWidth / overlayZoom;
      const heightInCells = containerHeight / overlayZoom;
      chunksInView = (widthInCells / CHUNK) * (heightInCells / CHUNK);
    }

    // Keep full-res up to ~70x70 chunks, then 32 up to ~150x150 chunks
    if (chunksInView < 5000) { // < ~70x70 chunks
      return 64;
    } else if (chunksInView < 20000) { // < 150x150 chunks
      return 32;
    } else {
      return 16;
    }
  }, [MINIMAP_SIZE, CHUNK]);

  const schedulePaint = useCallback(() => {
    if (rafRef.current) return;
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = null;
      const canvas = canvasRef.current;
      const container = effectiveContainerRef.current;
      if (!canvas || !container) return;

      const ctx = canvas.getContext("2d");
      const dpr = window.devicePixelRatio || 1;

      if (mode === "hud") {
        // HUD mode rendering
        const cssSize = MINIMAP_SIZE;
        canvas.width = Math.floor(cssSize * dpr);
        canvas.height = Math.floor(cssSize * dpr);
        canvas.style.width = `${cssSize}px`;
        canvas.style.height = `${cssSize}px`;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.imageSmoothingEnabled = false;
        ctx.clearRect(0, 0, cssSize, cssSize);
        ctx.fillStyle = '#c0c0c0';
        ctx.fillRect(0, 0, cssSize, cssSize);

        const { cx: centerWorldX, cy: centerWorldY } = hudViewRef.current;
        const cellsPerSide = Math.max(CHUNK / 2, Math.min(CHUNK * 32, hudViewRef.current.cells));
        const halfCells = cellsPerSide / 2;
        const startWorldX = Math.floor(centerWorldX - halfCells);
        const startWorldY = Math.floor(centerWorldY - halfCells);
        const scale = cssSize / cellsPerSide;

        // Compute tile range
        const startChunkX = Math.floor(startWorldX / CHUNK);
        const startChunkY = Math.floor(startWorldY / CHUNK);
        const endChunkX = Math.floor((startWorldX + cellsPerSide - 1) / CHUNK);
        const endChunkY = Math.floor((startWorldY + cellsPerSide - 1) / CHUNK);

        for (let cy = startChunkY; cy <= endChunkY; cy++) {
          for (let cx = startChunkX; cx <= endChunkX; cx++) {
            const key = `${cx},${cy}`;
            const rec = minimapTilesRef.current.get(key);
            if (!rec || !rec.canvas) continue;

            // Tile world bounds
            const tileWorldX = cx * CHUNK;
            const tileWorldY = cy * CHUNK;

            // Overlap region in world cell space
            const srcX = Math.max(0, startWorldX - tileWorldX);
            const srcY = Math.max(0, startWorldY - tileWorldY);
            const srcW = Math.min(CHUNK - srcX, startWorldX + cellsPerSide - tileWorldX - srcX);
            const srcH = Math.min(CHUNK - srcY, startWorldY + cellsPerSide - tileWorldY - srcY);
            if (srcW <= 0 || srcH <= 0) continue;

            // Convert world cell coordinates to canvas pixel coordinates
            // Canvas may be lower resolution (16x16, 32x32) but represents CHUNK x CHUNK cells
            const resolution = rec.resolution || CHUNK;
            const canvasScale = resolution / CHUNK; // how many pixels per world cell in the canvas
            const canvasSrcX = srcX * canvasScale;
            const canvasSrcY = srcY * canvasScale;
            const canvasSrcW = srcW * canvasScale;
            const canvasSrcH = srcH * canvasScale;

            const dstX = Math.floor((tileWorldX + srcX - startWorldX) * scale);
            const dstY = Math.floor((tileWorldY + srcY - startWorldY) * scale);
            const dstW = Math.ceil(srcW * scale);
            const dstH = Math.ceil(srcH * scale);

            ctx.drawImage(rec.canvas, canvasSrcX, canvasSrcY, canvasSrcW, canvasSrcH, dstX, dstY, dstW, dstH);
          }
        }

        // Draw main viewport rectangle
        const width = containerRef?.current?.clientWidth || 0;
        const height = containerRef?.current?.clientHeight || 0;
        if (width && height) {
          const viewWidthCells = Math.ceil(width / zoom / CELL_SIZE);
          const viewHeightCells = Math.ceil(height / zoom / CELL_SIZE);
          const boxLeft = (Math.floor((viewX + width / 2 / zoom) / CELL_SIZE) - viewWidthCells / 2 - startWorldX) * scale;
          const boxTop = (Math.floor((viewY + height / 2 / zoom) / CELL_SIZE) - viewHeightCells / 2 - startWorldY) * scale;
          ctx.strokeStyle = "rgba(0,0,0,0.9)";
          ctx.lineWidth = 1;
          ctx.strokeRect(boxLeft, boxTop, viewWidthCells * scale, viewHeightCells * scale);
        }
      } else {
        // Overlay mode rendering
        const w = container.clientWidth;
        const h = container.clientHeight;
        canvas.width = Math.floor(w * dpr);
        canvas.height = Math.floor(h * dpr);
        canvas.style.width = `${w}px`;
        canvas.style.height = `${h}px`;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.imageSmoothingEnabled = false;
        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = '#c0c0c0';
        ctx.fillRect(0, 0, w, h);

        const { x, y, zoom: overlayZoom } = overlayViewRef.current;
        const tilePx = CHUNK * overlayZoom;
        const startChunkX = Math.floor(x / CHUNK);
        const startChunkY = Math.floor(y / CHUNK);
        const endChunkX = Math.floor((x + w / overlayZoom) / CHUNK);
        const endChunkY = Math.floor((y + h / overlayZoom) / CHUNK);

        for (let cy = startChunkY; cy <= endChunkY; cy++) {
          for (let cx = startChunkX; cx <= endChunkX; cx++) {
            const key = `${cx},${cy}`;
            const rec = minimapTilesRef.current.get(key);
            if (!rec || !rec.canvas) continue;
            const ox = Math.floor(cx * CHUNK - x);
            const oy = Math.floor(cy * CHUNK - y);
            const resolution = rec.resolution || CHUNK;
            ctx.drawImage(rec.canvas, 0, 0, resolution, resolution, Math.floor(ox * overlayZoom), Math.floor(oy * overlayZoom), Math.ceil(tilePx), Math.ceil(tilePx));
          }
        }

        // Draw main viewport rectangle for overlay mode
        if (mode === "overlay" && containerRef?.current && CELL_SIZE) {
          const gameWidth = containerRef.current.clientWidth || 0;
          const gameHeight = containerRef.current.clientHeight || 0;
          if (gameWidth && gameHeight) {
            const viewWidthCells = Math.ceil(gameWidth / zoom / CELL_SIZE);
            const viewHeightCells = Math.ceil(gameHeight / zoom / CELL_SIZE);

            // Calculate game viewport bounds in world coordinates
            const gameViewLeftX = viewX / CELL_SIZE;
            const gameViewTopY = viewY / CELL_SIZE;
            const gameViewRightX = (viewX + gameWidth / zoom) / CELL_SIZE;
            const gameViewBottomY = (viewY + gameHeight / zoom) / CELL_SIZE;

            // Convert to overlay canvas coordinates
            const boxLeft = (gameViewLeftX - x) * overlayZoom;
            const boxTop = (gameViewTopY - y) * overlayZoom;
            const boxWidth = (gameViewRightX - gameViewLeftX) * overlayZoom;
            const boxHeight = (gameViewBottomY - gameViewTopY) * overlayZoom;

            ctx.strokeStyle = "rgba(0,0,0,0.9)";
            ctx.lineWidth = 2;
            ctx.strokeRect(boxLeft, boxTop, boxWidth, boxHeight);
          }
        }
      }
    });
  }, [mode, MINIMAP_SIZE, CHUNK, CELL_SIZE, minimapTilesRef, effectiveContainerRef, containerRef, zoom, viewX, viewY]);

  // Pan/zoom/drag handler using the unified hook
  const { bind, lastInteractionRef } = usePinchPanZoom({
    elementRef: canvasRef,
    getPointToWorld: (screenX, screenY) => {
      // Return different coordinate systems based on mode
      if (mode === "hud") {
        return {
          x: hudViewRef.current.cx,
          y: hudViewRef.current.cy,
          cells: hudViewRef.current.cells,
          mode: "hud"
        };
      } else {
        return {
          x: overlayViewRef.current.x,
          y: overlayViewRef.current.y,
          zoom: overlayViewRef.current.zoom,
          mode: "overlay"
        };
      }
    },
    onPan: (deltaX, deltaY, context) => {
      if (mode === "hud") {
        const cells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, hudViewRef.current.cells));
        const scale = MINIMAP_SIZE / cells;
        hudViewRef.current = {
          cx: hudViewRef.current.cx - deltaX / scale,
          cy: hudViewRef.current.cy - deltaY / scale,
          cells
        };
        setHudView(hudViewRef.current);
      } else {
        const dzoom = overlayViewRef.current.zoom;
        overlayViewRef.current = {
          ...overlayViewRef.current,
          x: overlayViewRef.current.x - deltaX / dzoom,
          y: overlayViewRef.current.y - deltaY / dzoom
        };
        setOverlayView(overlayViewRef.current);
      }
      schedulePaint();
    },
    onZoom: (zoomFactor, anchor, context) => {
      if (mode === "hud") {
        // HUD zoom handling
        const rect = canvasRef.current?.getBoundingClientRect();
        if (!rect) return;
        const mx = context.x;
        const my = context.y;
        const cells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, hudViewRef.current.cells));
        const scale = MINIMAP_SIZE / cells;
        const worldX = hudViewRef.current.cx - (MINIMAP_SIZE / 2 - mx) / scale;
        const worldY = hudViewRef.current.cy - (MINIMAP_SIZE / 2 - my) / scale;
        const factor = 1 / zoomFactor; // Invert for minimap
        const newCells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, cells / factor));
        const newScale = MINIMAP_SIZE / newCells;
        const newCx = worldX + (MINIMAP_SIZE / 2 - mx) / newScale;
        const newCy = worldY + (MINIMAP_SIZE / 2 - my) / newScale;
        hudViewRef.current = { cx: newCx, cy: newCy, cells: newCells };
        setHudView(hudViewRef.current);
        schedulePaint();
        const resolution = calculateMinimapResolution('hud', newCells);
        requestAnimationFrame(() => updateMinimapSubscriptions(newCx, newCy, newCells, newCells, 2, 'hud', resolution));
      } else {
        // Overlay zoom handling
        const rect = canvasRef.current?.getBoundingClientRect();
        if (!rect) return;
        const mx = context.x;
        const my = context.y;
        const wz = overlayViewRef.current.zoom;
        const factor = 1 / zoomFactor; // Invert for minimap
        const nz = Math.min(Math.max(wz / factor, 0.125), 8);
        const worldX = overlayViewRef.current.x + mx / wz;
        const worldY = overlayViewRef.current.y + my / wz;
        const nx = worldX - mx / nz;
        const ny = worldY - my / nz;
        overlayViewRef.current = { x: nx, y: ny, zoom: nz };
        setOverlayView(overlayViewRef.current);
        schedulePaint();
      }
    },
    onInteraction: () => {
      // Custom interaction tracking for minimap
    },
    enableRightClick: false,
    dragDelayMs: 0, // Immediate drag for minimap
    useIncrementalPan: true // Use frame-to-frame deltas for minimap
  });

  const lastMmInteractionAtRef = lastInteractionRef;

  // Initialize overlay view to center on user or world origin
  useEffect(() => {
    if (mode !== "overlay") return;

    const el = effectiveContainerRef.current;
    if (!el) return;

    const w = el.clientWidth || 0;
    const h = el.clientHeight || 0;
    const currentZoom = overlayViewRef.current.zoom;

    // Smart centering: use main viewport center if available, otherwise world origin
    let centerX = 0;
    let centerY = 0;

    if (containerRef?.current && viewX !== undefined && viewY !== undefined && zoom && CELL_SIZE) {
      // Center on current main viewport
      const gameWidth = containerRef.current.clientWidth || 0;
      const gameHeight = containerRef.current.clientHeight || 0;
      centerX = Math.floor((viewX + gameWidth / 2 / zoom) / CELL_SIZE);
      centerY = Math.floor((viewY + gameHeight / 2 / zoom) / CELL_SIZE);
    }

    const nx = centerX - w / (2 * currentZoom);
    const ny = centerY - h / (2 * currentZoom);
    const next = { x: nx, y: ny, zoom: currentZoom };
    overlayViewRef.current = next;
    setOverlayView(next);

    schedulePaint();

    const widthCells = Math.ceil(w / currentZoom);
    const heightCells = Math.ceil(h / currentZoom);
    const resolution = calculateMinimapResolution('overlay', 0, currentZoom, w, h);
    updateMinimapSubscriptions(centerX, centerY, widthCells, heightCells, 2, 'overlay', resolution);

    return () => {
      clearMinimapSubscriptionsFor?.('overlay');
    };
  }, [mode, effectiveContainerRef, containerRef, viewX, viewY, zoom, CELL_SIZE, updateMinimapSubscriptions, clearMinimapSubscriptionsFor, schedulePaint, calculateMinimapResolution]);

  // Initialize HUD minimap center to main view center (spectator-friendly)
  useEffect(() => {
    if (mode !== 'hud') return;
    const container = containerRef?.current;
    if (!container || !CELL_SIZE || !zoom) return;
    const width = container.clientWidth || 0;
    const height = container.clientHeight || 0;
    const cwx = Math.floor((viewX + width / 2 / zoom) / CELL_SIZE);
    const cwy = Math.floor((viewY + height / 2 / zoom) / CELL_SIZE);
    // Only update if far from current to avoid fighting user interactions
    const dx = Math.abs((hudViewRef.current?.cx ?? 0) - cwx);
    const dy = Math.abs((hudViewRef.current?.cy ?? 0) - cwy);
    if (dx > 1 || dy > 1) {
      const next = { cx: cwx, cy: cwy, cells: hudViewRef.current?.cells || CHUNK * 3 };
      hudViewRef.current = next;
      setHudView(next);
      schedulePaint();
      updateMinimapSubscriptions(cwx, cwy, next.cells, next.cells, 2, 'hud');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, containerRef, viewX, viewY, zoom, CELL_SIZE]);

  // Keep streaming subscriptions in sync
  useEffect(() => {
    if (mode === "hud") {
      // For HUD mode, we can subscribe based on hudViewRef alone; no container needed
      const { cx, cy, cells } = hudViewRef.current;
      const resolution = calculateMinimapResolution('hud', cells);
      updateMinimapSubscriptions(cx, cy, cells, cells, 2, 'hud', resolution);
    } else {
      const el = effectiveContainerRef.current;
      if (!el) return;
      const w = el.clientWidth;
      const h = el.clientHeight;
      const { x, y, zoom: overlayZoom } = overlayViewRef.current;
      const widthCells = Math.ceil(w / overlayZoom);
      const heightCells = Math.ceil(h / overlayZoom);
      const centerWorldX = Math.floor(x + widthCells / 2);
      const centerWorldY = Math.floor(y + heightCells / 2);
      const resolution = calculateMinimapResolution('overlay', 0, overlayZoom, w, h);
      updateMinimapSubscriptions(centerWorldX, centerWorldY, widthCells, heightCells, 2, 'overlay', resolution);
    }
  }, [mode, hudView, overlayView, updateMinimapSubscriptions, containerRef, effectiveContainerRef, calculateMinimapResolution]);

  // Auto-follow main view for both modes
  useEffect(() => {
    if (!mainViewMoveToken) return;
    if (performance.now() - (lastMmInteractionAtRef.current || 0) < 300) return;

    const container = containerRef?.current;
    if (!container) return;

    const width = container.clientWidth || 0;
    const height = container.clientHeight || 0;
    const { viewX: vx, viewY: vy, zoom: z } = mainViewRef.current;
    const cwx = Math.floor((vx + width / 2 / z) / CELL_SIZE);
    const cwy = Math.floor((vy + height / 2 / z) / CELL_SIZE);

    if (mode === "hud") {
      hudViewRef.current = { cx: cwx, cy: cwy, cells: hudViewRef.current.cells };
      setHudView(hudViewRef.current);
      schedulePaint();
      const { cells } = hudViewRef.current;
      const resolution = calculateMinimapResolution('hud', cells);
      updateMinimapSubscriptions(cwx, cwy, cells, cells, 2, 'hud', resolution);
    } else if (mode === "overlay") {
      // Auto-follow for overlay mode too
      const el = effectiveContainerRef.current;
      if (!el) return;
      const w = el.clientWidth || 0;
      const h = el.clientHeight || 0;
      const currentZoom = overlayViewRef.current.zoom;
      const nx = cwx - w / (2 * currentZoom);
      const ny = cwy - h / (2 * currentZoom);
      overlayViewRef.current = { x: nx, y: ny, zoom: currentZoom };
      setOverlayView(overlayViewRef.current);
      schedulePaint();

      const widthCells = Math.ceil(w / currentZoom);
      const heightCells = Math.ceil(h / currentZoom);
      const resolution = calculateMinimapResolution('overlay', 0, currentZoom, w, h);
      updateMinimapSubscriptions(cwx, cwy, widthCells, heightCells, 2, 'overlay', resolution);
    }
  }, [mainViewMoveToken, mode, CELL_SIZE, containerRef, effectiveContainerRef, updateMinimapSubscriptions, schedulePaint, calculateMinimapResolution]);

  // Track latest main view values
  useEffect(() => {
    if (viewX !== undefined && viewY !== undefined && zoom !== undefined) {
      mainViewRef.current = { viewX, viewY, zoom };
    }
  }, [viewX, viewY, zoom]);

  // Paint on interval
  useEffect(() => {
    schedulePaint();
    const id = setInterval(schedulePaint, 100);
    return () => clearInterval(id);
  }, [schedulePaint]);

  if (mode === "hud") {
    return (
      <div
        className="minimap"
        style={{
          position: 'fixed',
          bottom: 10,
          right: 10,
          width: MINIMAP_SIZE,
          height: MINIMAP_SIZE,
          touchAction: 'none'
        }}
      >
        <canvas
          ref={canvasRef}
          style={{ width: '100%', height: '100%', cursor: "grab" }}
          {...bind}
        />
        <button
          onClick={onOpenOverlay}
          title="Expand minimap"
          style={{
            position: 'absolute',
            top: 4,
            right: 4,
            width: 20,
            height: 20,
            borderRadius: 4,
            border: '1px solid #888',
            background: 'rgba(255,255,255,0.9)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 0,
            cursor: 'pointer',
            lineHeight: 0,
            zIndex: 11,
          }}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#333" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 3 21 3 21 9" />
            <polyline points="9 21 3 21 3 15" />
            <line x1="21" y1="3" x2="14" y2="10" />
            <line x1="3" y1="21" x2="10" y2="14" />
          </svg>
        </button>
      </div>
    );
  }

  return (
    <div ref={localContainerRef} style={{ position: 'relative', width: '100%', height: '100%', background: '#202020' }}>
      <canvas ref={canvasRef} style={{ width: '100%', height: '100%' }} {...bind} />
    </div>
  );
}
