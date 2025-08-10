import React, { useRef, useState, useEffect, useCallback } from "react";

// Streaming, tile-composited minimap for the game HUD
export default function MinimapHUD({
  CHUNK,
  CELL_SIZE,
  MINIMAP_SIZE,
  zoom,
  viewX,
  viewY,
  containerRef,
  // streaming props
  updateMinimapSubscriptions,
  minimapTilesRef,
  onOpenOverlay,
  mainViewMoveToken,
}) {
  const canvasRef = useRef(null);
  const containerWrapRef = useRef(null);
  const mainViewRef = useRef({ viewX, viewY, zoom });
  const userHasDraggedRef = useRef(false);
  const lastMmInteractionAtRef = useRef(0);
  // Internal view: center and cells-per-side; initialized from game view center
  const [mmView, setMmView] = useState({ cx: 0, cy: 0, cells: CHUNK * 3 });
  const mmRef = useRef(mmView);
  const draggingRef = useRef(false);
  const lastPosRef = useRef({ x: 0, y: 0 });

  const paint = useCallback(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    const cssSize = MINIMAP_SIZE;
    canvas.width = Math.floor(cssSize * dpr);
    canvas.height = Math.floor(cssSize * dpr);
    canvas.style.width = `${cssSize}px`;
    canvas.style.height = `${cssSize}px`;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, cssSize, cssSize);
    // Fill unseen background so unloaded tiles appear as unrevealed
    ctx.fillStyle = '#808080';
    ctx.fillRect(0, 0, cssSize, cssSize);

    const { cx: centerWorldX, cy: centerWorldY } = mmRef.current;

    // Compose a square of current cells-per-side around mm center
    const cellsPerSide = Math.max(CHUNK / 2, Math.min(CHUNK * 32, mmRef.current.cells));
    const halfCells = cellsPerSide / 2;
    const startWorldX = Math.floor(centerWorldX - halfCells);
    const startWorldY = Math.floor(centerWorldY - halfCells);
    const scale = cssSize / cellsPerSide; // pixels per world cell on minimap

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

        const dstX = Math.floor((tileWorldX + srcX - startWorldX) * scale);
        const dstY = Math.floor((tileWorldY + srcY - startWorldY) * scale);
        const dstW = Math.ceil(srcW * scale);
        const dstH = Math.ceil(srcH * scale);

        const c = rec.canvas;
        const ctx2d = ctx;
        // drawImage(Image, sx, sy, sw, sh, dx, dy, dw, dh)
        ctx2d.drawImage(c, srcX, srcY, srcW, srcH, dstX, dstY, dstW, dstH);
      }
    }

    // Draw main viewport rectangle
    const width = container.clientWidth || 0;
    const height = container.clientHeight || 0;
    if (width && height) {
      const viewWidthCells = Math.ceil(width / zoom / CELL_SIZE);
      const viewHeightCells = Math.ceil(height / zoom / CELL_SIZE);
      const boxLeft = (Math.floor((viewX + width / 2 / zoom) / CELL_SIZE) - viewWidthCells / 2 - startWorldX) * scale;
      const boxTop = (Math.floor((viewY + height / 2 / zoom) / CELL_SIZE) - viewHeightCells / 2 - startWorldY) * scale;
      ctx.strokeStyle = "rgba(0,0,0,0.9)";
      ctx.lineWidth = 1;
      ctx.strokeRect(boxLeft, boxTop, viewWidthCells * scale, viewHeightCells * scale);
    }
  }, [CHUNK, CELL_SIZE, MINIMAP_SIZE, zoom, viewX, viewY, containerRef, minimapTilesRef]);

  // Keep streaming subscriptions in sync with minimap’s view
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const { cx, cy, cells } = mmRef.current;
    updateMinimapSubscriptions(cx, cy, cells, cells, 1, 'hud');
    if (__DEV__) console.log('minimap subs sync effect fired');
  }, [updateMinimapSubscriptions, containerRef]);

  // Track latest main view values without retriggering auto-follow
  useEffect(() => {
    mainViewRef.current = { viewX, viewY, zoom };
  }, [viewX, viewY, zoom]);

  // Paint on changes
  useEffect(() => {
    paint();
    const id = setInterval(paint, 100);
    return () => clearInterval(id);
  }, [paint]);

  // Attach non-passive wheel and pan handlers directly to canvas to allow preventDefault
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const onWheel = (e) => {
      e.preventDefault();
      lastMmInteractionAtRef.current = performance.now();
      const rect = el.getBoundingClientRect();
      const mx = e.clientX - rect.left;
      const my = e.clientY - rect.top;
      const cells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, mmRef.current.cells));
      const scale = MINIMAP_SIZE / cells;
      const worldX = mmRef.current.cx - (MINIMAP_SIZE / 2 - mx) / scale;
      const worldY = mmRef.current.cy - (MINIMAP_SIZE / 2 - my) / scale;
      const factor = Math.exp(-e.deltaY * 0.0006); // slightly faster than before
      const newCells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, cells / factor));
      // Recompute center to keep mouse point stable
      const newScale = MINIMAP_SIZE / newCells;
      const newCx = worldX + (MINIMAP_SIZE / 2 - mx) / newScale;
      const newCy = worldY + (MINIMAP_SIZE / 2 - my) / newScale;
      mmRef.current = { cx: newCx, cy: newCy, cells: newCells };
      setMmView(mmRef.current);
      paint();
      // update subs next tick
      requestAnimationFrame(() => updateMinimapSubscriptions(newCx, newCy, newCells, newCells, 1, 'hud'));
    };
    const onDown = (e) => {
      e.preventDefault();
      lastMmInteractionAtRef.current = performance.now();
      draggingRef.current = true;
      lastPosRef.current = { x: e.clientX, y: e.clientY };
    };
    const onMove = (e) => {
      if (!draggingRef.current) return;
      e.preventDefault();
      lastMmInteractionAtRef.current = performance.now();
      const dx = e.clientX - lastPosRef.current.x;
      const dy = e.clientY - lastPosRef.current.y;
      lastPosRef.current = { x: e.clientX, y: e.clientY };
      const cells = Math.max(CHUNK / 2, Math.min(CHUNK * 32, mmRef.current.cells));
      const scale = MINIMAP_SIZE / cells;
      mmRef.current = { cx: mmRef.current.cx - dx / scale, cy: mmRef.current.cy - dy / scale, cells };
      setMmView(mmRef.current);
      paint();
    };
    const onUp = () => {
      if (!draggingRef.current) return;
      draggingRef.current = false;
      lastMmInteractionAtRef.current = performance.now();
      const { cx, cy, cells } = mmRef.current;
      requestAnimationFrame(() => updateMinimapSubscriptions(cx, cy, cells, cells, 1, 'hud'));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    el.addEventListener('mousedown', onDown);
    window.addEventListener('mousemove', onMove, { passive: false });
    window.addEventListener('mouseup', onUp);
    return () => {
      el.removeEventListener('wheel', onWheel);
      el.removeEventListener('mousedown', onDown);
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  }, [CHUNK, MINIMAP_SIZE, updateMinimapSubscriptions, paint]);

  // Auto-follow: only when mainViewMoveToken changes (actual main map interactions)
  // Auto-follow only when the main map actually moved via drag. Also, if the user
  // interacted with the minimap very recently, skip one auto-follow to avoid fighting the user.
  useEffect(() => {
    if (__DEV__) console.log('minimap auto-follow effect', { t: Date.now(), mainViewMoveToken });
    const container = containerRef.current;
    if (!container) return;
    if (performance.now() - (lastMmInteractionAtRef.current || 0) < 300) return;
    const width = container.clientWidth || 0;
    const height = container.clientHeight || 0;
    const { viewX: vx, viewY: vy, zoom: z } = mainViewRef.current;
    const cwx = Math.floor((vx + width / 2 / z) / CELL_SIZE);
    const cwy = Math.floor((vy + height / 2 / z) / CELL_SIZE);
    mmRef.current = { cx: cwx, cy: cwy, cells: mmRef.current.cells };
    setMmView(mmRef.current);
    paint();
    // Refresh subscriptions to follow
    const { cells } = mmRef.current;
    updateMinimapSubscriptions(cwx, cwy, cells, cells, 1, 'hud');
  }, [mainViewMoveToken, CELL_SIZE, containerRef, updateMinimapSubscriptions]);

  // Dev-only: detect identity changes of updateMinimapSubscriptions
  useEffect(() => {
    if (!__DEV__) return;
    console.log('updateMinimapSubscriptions identity changed');
  }, [updateMinimapSubscriptions]);

  return (
    <div ref={containerWrapRef} className="minimap" style={{ position: 'fixed', bottom: 10, right: 10, width: MINIMAP_SIZE, height: MINIMAP_SIZE }}>
      <canvas
        ref={canvasRef}
        style={{ width: '100%', height: '100%', cursor: "grab" }}
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
        {/* simple “expand” glyph */}
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


