import { CHUNK } from "./useGameState.js";
import { drawSprite } from "./sprites/index.js";

// Effective px/cell thresholds for LOD switching. The chunk-cache raster
// (used below FULL_QUALITY_THRESHOLD) is rendered at baseCell resolution
// equal to this threshold, so at the exact LOD0/LOD1 boundary the cached
// bitmap is blitted at native resolution with no up/downscaling artifact.
const MINIMAL_RENDERING_THRESHOLD = 5; // below this: minimal cached chunks
const FULL_QUALITY_THRESHOLD = 20; // above this: per-cell rendering

// Cached chunk rasters come in a few px-per-cell resolutions; the smallest
// bucket >= the current effective px-per-cell is used, so blits never
// upscale. Rasterizing at (roughly) screen resolution keeps a screenful of
// cache near-constant memory at any zoom, where a single fixed-resolution
// raster ballooned 16x when drawn far out.
const RASTER_BUCKETS = [20, 10, 5];
// Per-bucket LRU caps, each ~2 screenfuls on a large retina display.
const RASTER_CACHE_CAP = { 20: 12, 10: 40, 5: 160 };
// Max time per frame spent rasterizing chunk canvases. Chunks over budget
// draw a stale raster or the blank placeholder and finish on later frames.
const RASTER_BUDGET_MS = 6;

export class CanvasRenderer {
  constructor() {
    this.canvasSizeRef = { w: 0, h: 0, dpr: 1 };
    // Per-resolution chunk raster caches: bucket -> ("cx,cy" -> { canvas, version })
    this.chunkCaches = new Map(RASTER_BUCKETS.map((b) => [b, new Map()]));
    // Pre-rendered per-resolution stamps: base cell tiles, number glyphs,
    // and the shared all-unrevealed chunk placeholder.
    this.stampCache = new Map();
  }

  _makeCanvas(w, h) {
    return typeof OffscreenCanvas !== "undefined"
      ? new OffscreenCanvas(w, h)
      : Object.assign(document.createElement("canvas"), {
          width: w,
          height: h,
        });
  }

  _stamp(key, w, h, draw) {
    let c = this.stampCache.get(key);
    if (!c) {
      c = this._makeCanvas(w, h);
      const ctx = c.getContext("2d");
      ctx.imageSmoothingEnabled = false;
      draw(ctx);
      this.stampCache.set(key, c);
    }
    return c;
  }

  _baseTile(size, revealed) {
    return this._stamp(`tile,${size},${revealed ? 1 : 0}`, size, size, (ctx) =>
      this.draw3DCell(ctx, 0, 0, size, revealed)
    );
  }

  _numberGlyph(size, n, getNumberColor) {
    return this._stamp(`num,${size},${n}`, size, size, (ctx) =>
      this.drawNumber(ctx, 0, 0, size, n, getNumberColor)
    );
  }

  // Shared placeholder for chunks with no revealed cells or flags — the
  // vast majority of the world when zoomed out. Drawing it costs one
  // drawImage and caches nothing per-chunk.
  _blankChunk(size) {
    return this._stamp(`blank,${size}`, CHUNK * size, CHUNK * size, (ctx) => {
      const tile = this._baseTile(size, false);
      for (let ly = 0; ly < CHUNK; ly++) {
        for (let lx = 0; lx < CHUNK; lx++) {
          ctx.drawImage(tile, lx * size, ly * size);
        }
      }
    });
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
  _touch(cache, key) {
    const v = cache.get(key);
    if (!v) return;
    cache.delete(key);
    cache.set(key, v);
  }
  _evictIfNeeded(cache, cap) {
    while (cache.size > cap) {
      const oldestKey = cache.keys().next().value;
      cache.delete(oldestKey);
    }
  }

  // Rasterizes a chunk in the same visual style as close-up LOD0 rendering
  // (beveled cells, real flag/mine sprites) at `size` px per cell. The
  // beveled base and number glyphs are stamped from pre-rendered tiles —
  // an order of magnitude cheaper than the ~12 fillRects per cell that
  // drawing them directly costs, which is what makes warm-up over a fast
  // zoom-out feasible within the per-frame budget.
  _rasterizeChunk(cx, cy, refs, size) {
    const { revealedCellsRef, flaggedCellsRef, getNumberColor } = refs;
    const off = this._makeCanvas(CHUNK * size, CHUNK * size);
    const ctx = off.getContext("2d");
    ctx.imageSmoothingEnabled = false;

    ctx.drawImage(this._blankChunk(size), 0, 0);
    const revealedTile = this._baseTile(size, true);

    const prefix = `${cx},${cy},`;
    for (let ly = 0; ly < CHUNK; ly++) {
      for (let lx = 0; lx < CHUNK; lx++) {
        const cell = ly * CHUNK + lx;
        const cellDataRaw = revealedCellsRef.current.get(prefix + cell) || null;
        const wx = cx * CHUNK + lx;
        const wy = cy * CHUNK + ly;
        const flagForCell = flaggedCellsRef.current.get(`${wx},${wy}`);
        const isFlagged = flagForCell !== undefined;
        const dx = lx * size;
        const dy = ly * size;

        if (isFlagged) {
          this.drawSprite(ctx, flagForCell, dx, dy, size, size);
          continue;
        }
        if (cellDataRaw === null) continue; // unrevealed: placeholder already there

        ctx.drawImage(revealedTile, dx, dy);
        if (cellDataRaw.isMine) {
          this.drawSprite(ctx, "mine", dx, dy, size, size);
        } else if (cellDataRaw.adjacentMines > 0) {
          ctx.drawImage(
            this._numberGlyph(size, cellDataRaw.adjacentMines, getNumberColor),
            dx,
            dy
          );
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
    requestRerender,
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

    // Smallest raster resolution that never upscales at the current zoom.
    let bucket = RASTER_BUCKETS[RASTER_BUCKETS.length - 1];
    for (const b of RASTER_BUCKETS) {
      if (b >= effPx) bucket = b;
    }
    const cache = this.chunkCaches.get(bucket);
    const blank = this._blankChunk(bucket);
    const frameStart = performance.now();
    let rerenderNeeded = false;

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

        const chunkScreenX = snapToDevicePixel(
          cx * CHUNK * CELL_SIZE - viewRef.current.x
        );
        const chunkScreenY = snapToDevicePixel(
          cy * CHUNK * CELL_SIZE - viewRef.current.y
        );

        // Never-touched chunks (no reveals, no flags) all share one
        // placeholder canvas: no per-chunk raster work or cache memory.
        let source = version === 0 ? blank : null;

        if (!source) {
          let entry = cache.get(key);
          if (!entry || entry.version !== version) {
            if (performance.now() - frameStart <= RASTER_BUDGET_MS) {
              entry = {
                canvas: this._rasterizeChunk(cx, cy, refs, bucket),
                version,
              };
              cache.set(key, entry);
              this._evictIfNeeded(cache, RASTER_CACHE_CAP[bucket]);
            } else {
              // Over budget: show the stale raster (or the placeholder) now
              // and finish rasterizing on a following frame.
              rerenderNeeded = true;
            }
          } else {
            this._touch(cache, key);
          }
          source = entry ? entry.canvas : blank;
        }

        ctx.drawImage(
          source,
          0,
          0,
          source.width,
          source.height,
          chunkScreenX,
          chunkScreenY,
          CHUNK * CELL_SIZE,
          CHUNK * CELL_SIZE
        );
      }
    }

    if (rerenderNeeded && typeof requestRerender === "function") {
      requestRerender();
    }
  }
}
