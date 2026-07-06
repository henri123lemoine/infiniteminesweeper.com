import { CHUNK } from "./useGameState.js";
import { drawSprite } from "./sprites/index.js";

// Effective px/cell thresholds for LOD switching. The chunk-cache raster
// (used below FULL_QUALITY_THRESHOLD) is rendered at baseCell resolution
// equal to this threshold, so at the exact LOD0/LOD1 boundary the cached
// bitmap is blitted at native resolution with no up/downscaling artifact.
const MINIMAL_RENDERING_THRESHOLD = 5; // below this: minimal cached chunks
const FULL_QUALITY_THRESHOLD = 20; // above this: per-cell rendering

export class CanvasRenderer {
  constructor() {
    this.canvasSizeRef = { w: 0, h: 0, dpr: 1 };
    // Per-chunk raster cache: key "cx,cy" -> { canvas, version }
    this.chunkCache = new Map();
    this.baseCell = FULL_QUALITY_THRESHOLD; // offscreen raster base size per cell
    // maxCachedChunks is tuned so total cache memory stays roughly constant
    // regardless of baseCell (chunk pixel area grows with baseCell^2).
    this.maxCachedChunks = Math.round(200 * (16 / this.baseCell) ** 2);
  }

  // 3D Cell Drawing Functions
  draw3DCell(ctx, x, y, size, isRevealed) {
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
        size - borderWidth
      );
      ctx.fillRect(
        x + borderWidth,
        y + size - borderWidth,
        size - borderWidth,
        borderWidth
      );
    } else {
      // Unrevealed cell - raised 3D appearance
      ctx.fillStyle = "#c0c0c0";
      ctx.fillRect(
        x + borderWidth,
        y + borderWidth,
        size - 2 * borderWidth,
        size - 2 * borderWidth
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
        innerBorderWidth
      );
      ctx.fillRect(
        x + borderWidth,
        y + borderWidth,
        innerBorderWidth,
        size - 2 * borderWidth
      );

      // Bottom and right shadows
      ctx.fillStyle = "#808080";
      ctx.fillRect(
        x + borderWidth,
        y + size - borderWidth,
        size - borderWidth,
        borderWidth
      );
      ctx.fillRect(
        x + size - borderWidth,
        y + borderWidth,
        borderWidth,
        size - borderWidth
      );

      ctx.fillStyle = "rgba(128, 128, 128, 0.6)";
      const innerShadowOffset = borderWidth + innerBorderWidth;
      ctx.fillRect(
        x + innerShadowOffset,
        y + size - innerShadowOffset,
        size - 2 * innerShadowOffset,
        innerBorderWidth
      );
      ctx.fillRect(
        x + size - innerShadowOffset,
        y + innerShadowOffset,
        innerBorderWidth,
        size - 2 * innerShadowOffset
      );

      // Corner highlights/shadows
      ctx.fillStyle = "#d4d4d4";
      ctx.fillRect(x, y, borderWidth, borderWidth);

      ctx.fillStyle = "#606060";
      ctx.fillRect(
        x + size - borderWidth,
        y + size - borderWidth,
        borderWidth,
        borderWidth
      );
    }
  }

  drawNumber(ctx, x, y, size, number, getNumberColor) {
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
  }

  // Simple LRU touch and evict helpers
  _touch(key) {
    const v = this.chunkCache.get(key);
    if (!v) return;
    this.chunkCache.delete(key);
    this.chunkCache.set(key, v);
  }
  _evictIfNeeded() {
    while (this.chunkCache.size > this.maxCachedChunks) {
      const oldestKey = this.chunkCache.keys().next().value;
      this.chunkCache.delete(oldestKey);
    }
  }

  // Rasterizes a chunk using the exact same per-cell renderer as LOD0
  // (beveled cells, real flag/mine sprites), so the cached bitmap used at
  // LOD1/LOD2 is visually identical in style to close-up rendering. Only
  // the resolution (baseCell) differs, and only near the LOD0 boundary is
  // that resolution difference perceptible at all.
  _rasterizeChunk(cx, cy, refs) {
    const { revealedCellsRef, flaggedCellsRef, getNumberColor } = refs;
    const size = this.baseCell;
    const off =
      typeof OffscreenCanvas !== "undefined"
        ? new OffscreenCanvas(CHUNK * size, CHUNK * size)
        : Object.assign(document.createElement("canvas"), {
            width: CHUNK * size,
            height: CHUNK * size,
          });
    const ctx = off.getContext("2d");
    ctx.imageSmoothingEnabled = false;

    const prefix = `${cx},${cy},`;
    for (let ly = 0; ly < CHUNK; ly++) {
      for (let lx = 0; lx < CHUNK; lx++) {
        const cell = ly * CHUNK + lx;
        const cellDataRaw = revealedCellsRef.current.get(prefix + cell) || null;
        const wx = cx * CHUNK + lx;
        const wy = cy * CHUNK + ly;
        const flagForCell = flaggedCellsRef.current.get(`${wx},${wy}`);
        const isFlagged = flagForCell !== undefined;
        const isRevealedState = cellDataRaw !== null;
        const dx = lx * size;
        const dy = ly * size;

        this.renderCell(
          ctx,
          dx,
          dy,
          size,
          cellDataRaw,
          isFlagged,
          isRevealedState,
          getNumberColor,
          0
        );

        if (isFlagged) {
          this.drawSprite(ctx, flagForCell, dx, dy, size, size);
        }
      }
    }
    return off;
  }

  /**
   * @param {CanvasRenderingContext2D} ctx
   * @param {number|string} spriteID uint32 from server OR direct key/string
   */
  drawSprite(ctx, spriteID, dx, dy, dw, dh) {
    drawSprite(ctx, spriteID, dx, dy, dw, dh);
  }

  // Cell Rendering Logic. Takes the raw revealed-cell record and flag state
  // directly (rather than a merged object) to avoid an allocation per cell
  // per frame at LOD0, where this runs for every visible cell.
  renderCell(
    ctx,
    screenX,
    screenY,
    cellSize,
    cellDataRaw,
    isFlagged,
    isRevealedState,
    getNumberColor,
    LOD
  ) {
    const isRevealed = isRevealedState && !isFlagged;

    // Base cell: beveled for LOD 0, flat for higher LODs
    if (LOD === 0) {
      this.draw3DCell(ctx, screenX, screenY, cellSize, isRevealed);
    } else {
      ctx.fillStyle = isRevealed ? "#e0e0e0" : "#c0c0c0";
      ctx.fillRect(screenX, screenY, cellSize, cellSize);
    }

    // Flagged/unrevealed cells are done here; flag sprites are drawn by the caller.
    if (!isRevealed) return;

    if (cellDataRaw.isMine) {
      if (LOD <= 1) {
        // Use the real mine sprite (stable key "mine", ID 162)
        this.drawSprite(ctx, "mine", screenX, screenY, cellSize, cellSize);
      } else {
        // LOD 2: draw a simple dot for a mine
        ctx.fillStyle = "#101010";
        const r = Math.max(1, cellSize * 0.25);
        ctx.beginPath();
        ctx.arc(
          screenX + cellSize / 2,
          screenY + cellSize / 2,
          r,
          0,
          Math.PI * 2
        );
        ctx.fill();
      }
    } else if (cellDataRaw.adjacentMines > 0 && LOD === 0) {
      this.drawNumber(
        ctx,
        screenX,
        screenY,
        cellSize,
        cellDataRaw.adjacentMines,
        getNumberColor
      );
    }
  }

  // Main Canvas Rendering
  render({
    canvasRef,
    containerRef,
    viewRef,
    zoom,
    CELL_SIZE,
    revealedCellsRef,
    flaggedCellsRef,
    chunkVersionRef,
    worldToChunk,
    getNumberColor,
    flagID,
    activePlayersRef,
  }) {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext("2d");
    ctx.imageSmoothingEnabled = false;
    const width = container.clientWidth;
    const height = container.clientHeight;
    const dpr = window.devicePixelRatio || 1;

    // Only resize canvas when necessary
    if (
      width !== this.canvasSizeRef.w ||
      height !== this.canvasSizeRef.h ||
      dpr !== this.canvasSizeRef.dpr
    ) {
      // Round rather than truncate: canvas.width/height coerce to an
      // unsigned integer, and a fractional dpr (e.g. 1.5, 2.625) truncating
      // down would leave the backing store a device pixel short of the
      // container, offsetting the ctx transform's scale from reality.
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(height * dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      this.canvasSizeRef = { w: width, h: height, dpr };
    }

    // Apply zoom and DPR scaling
    ctx.setTransform(dpr * zoom, 0, 0, dpr * zoom, 0, 0);

    const effPx = CELL_SIZE * zoom * dpr;

    // Thresholds are in effective pixels per cell (module-level constants)
    const LOD =
      effPx < MINIMAL_RENDERING_THRESHOLD
        ? 2
        : effPx < FULL_QUALITY_THRESHOLD
          ? 1
          : 0;

    // Clear background
    ctx.fillStyle = "#c0c0c0";
    // Always clear the visible viewport in logical (pre-zoom) units
    // The current transform is only a scale; the visible logical viewport is [0, width/zoom] x [0, height/zoom]
    ctx.fillRect(0, 0, width / zoom, height / zoom);

    // Calculate visible world coordinates
    const startWorldX = Math.floor(viewRef.current.x / CELL_SIZE);
    const startWorldY = Math.floor(viewRef.current.y / CELL_SIZE);
    const endWorldX = Math.ceil((viewRef.current.x + width / zoom) / CELL_SIZE);
    const endWorldY = Math.ceil(
      (viewRef.current.y + height / zoom) / CELL_SIZE
    );

    // At close zoom (LOD 0), draw per-cell with beveled tiles and high detail
    if (LOD === 0) {
      for (let worldY = startWorldY; worldY <= endWorldY; worldY++) {
        for (let worldX = startWorldX; worldX <= endWorldX; worldX++) {
          const screenX = worldX * CELL_SIZE - viewRef.current.x;
          const screenY = worldY * CELL_SIZE - viewRef.current.y;

          const { chunkX, chunkY, cell } = worldToChunk(worldX, worldY);
          const cellKey = `${chunkX},${chunkY},${cell}`;
          const cellDataRaw = revealedCellsRef.current.get(cellKey) || null;
          const isRevealedState = cellDataRaw !== null;

          const flagKey = `${worldX},${worldY}`;
          const flagForCell = flaggedCellsRef.current.get(flagKey);
          const isFlagged = flagForCell !== undefined;

          this.renderCell(
            ctx,
            screenX,
            screenY,
            CELL_SIZE,
            cellDataRaw,
            isFlagged,
            isRevealedState,
            getNumberColor,
            LOD
          );

          if (isFlagged) {
            this.drawSprite(
              ctx,
              flagForCell,
              screenX,
              screenY,
              CELL_SIZE,
              CELL_SIZE
            );
          }
        }
      }

      // Draw player icons at LOD 0
      if (activePlayersRef?.current) {
        ctx.save();
        ctx.globalAlpha = 0.5;
        const iconSize = CELL_SIZE;
        const offset = 0;
        const smoothing = 0.15;

        for (const [, player] of activePlayersRef.current) {
          player.x += (player.targetX - player.x) * smoothing;
          player.y += (player.targetY - player.y) * smoothing;

          // Check if player is within viewport
          if (
            player.x < startWorldX - 1 ||
            player.x > endWorldX + 1 ||
            player.y < startWorldY - 1 ||
            player.y > endWorldY + 1
          ) {
            continue;
          }
          const px = player.x * CELL_SIZE - viewRef.current.x + offset;
          const py = player.y * CELL_SIZE - viewRef.current.y + offset;

          this.drawSprite(ctx, player.flagId || 0, px, py, iconSize, iconSize);
        }
        ctx.restore();
      }
      return;
    }

    // Render by chunks using cache
    const startCX = Math.floor(startWorldX / CHUNK);
    const startCY = Math.floor(startWorldY / CHUNK);
    const endCX = Math.floor((endWorldX - 1) / CHUNK);
    const endCY = Math.floor((endWorldY - 1) / CHUNK);

    const refs = { revealedCellsRef, flaggedCellsRef, getNumberColor };

    // Snap each tile's destination rect to whole device pixels. Adjacent
    // chunk tiles share a boundary computed from the same viewRef/zoom
    // values, but the browser rasterizes each drawImage call independently;
    // a fractional-device-pixel edge lets anti-aliased coverage on that
    // shared seam drift a hair between frames as zoom changes continuously,
    // which reads as the world "jiggling". Rounding to the current
    // dpr*zoom scale keeps every tile edge pixel-exact and stable.
    const scale = dpr * zoom;
    const snapToDevicePixel = (v) => Math.round(v * scale) / scale;

    for (let cy = startCY; cy <= endCY; cy++) {
      for (let cx = startCX; cx <= endCX; cx++) {
        const key = `${cx},${cy}`;
        const version = chunkVersionRef?.current.get(key) || 0;
        let entry = this.chunkCache.get(key);
        if (!entry || entry.version !== version) {
          const canvas = this._rasterizeChunk(cx, cy, refs);
          entry = { canvas, version };
          this.chunkCache.set(key, entry);
          this._evictIfNeeded();
        } else {
          this._touch(key);
        }

        const chunkScreenX = snapToDevicePixel(
          cx * CHUNK * CELL_SIZE - viewRef.current.x
        );
        const chunkScreenY = snapToDevicePixel(
          cy * CHUNK * CELL_SIZE - viewRef.current.y
        );
        ctx.drawImage(
          entry.canvas,
          0,
          0,
          entry.canvas.width,
          entry.canvas.height,
          chunkScreenX,
          chunkScreenY,
          CHUNK * CELL_SIZE,
          CHUNK * CELL_SIZE
        );
      }
    }
  }
}
