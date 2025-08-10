import meta from "./assets/spritesheet.json";
import sheetUrl from "./assets/spritesheet.png?url";
import { CHUNK } from "./useGameState.js";

export class CanvasRenderer {
  constructor() {
    this.canvasSizeRef = { w: 0, h: 0, dpr: 1 };
    // Per-chunk raster cache: key "cx,cy" -> { canvas, version }
    this.chunkCache = new Map();
    this.maxCachedChunks = 200;
    this.baseCell = 16; // offscreen raster base size per cell
  }

  // Fast numeric-ID -> key table (built once at module load-time)
  static #idToKey = (() => {
    const map = {};
    for (const k of Object.keys(meta.frames)) {
      if (!Number.isNaN(Number(k))) map[Number(k)] = k; // numeric keys only
    }
    return map;
  })();

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
  }

  // Content Drawing Functions
  drawMine(ctx, x, y, size) {
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

  // Helper to draw a specific sprite from the sheet
  static #sheetImg;
  static #frames   = meta.frames;
  static #frameKeys = Object.keys(meta.frames);
  static #ready = false;

  static initSprites() {
    if (CanvasRenderer.#ready) return;
    CanvasRenderer.#sheetImg = new Image();
    CanvasRenderer.#sheetImg.onload = () => { CanvasRenderer.#ready = true; };
    CanvasRenderer.#sheetImg.src = sheetUrl;
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

  _rasterizeChunk(cx, cy, refs) {
    const { revealedCellsRef, flaggedCellsRef, getNumberColor } = refs;
    const size = this.baseCell;
    const off = typeof OffscreenCanvas !== "undefined"
      ? new OffscreenCanvas(CHUNK * size, CHUNK * size)
      : Object.assign(document.createElement("canvas"), { width: CHUNK * size, height: CHUNK * size });
    const ctx = off.getContext("2d");
    ctx.imageSmoothingEnabled = false;
    // Draw background
    ctx.fillStyle = "#c0c0c0";
    ctx.fillRect(0, 0, off.width, off.height);

    const prefix = `${cx},${cy},`;
    for (let ly = 0; ly < CHUNK; ly++) {
      for (let lx = 0; lx < CHUNK; lx++) {
        const cell = ly * CHUNK + lx;
        const cellData = revealedCellsRef.current.get(prefix + cell);
        const wx = cx * CHUNK + lx;
        const wy = cy * CHUNK + ly;
        const isFlagged = flaggedCellsRef.current.has(`${wx},${wy}`);
        const dx = lx * size;
        const dy = ly * size;

        if (!cellData && !isFlagged) {
          // unrevealed
          continue; // background already filled
        }
        if (!isFlagged) {
          // revealed flat tile
          ctx.fillStyle = "#e0e0e0";
          ctx.fillRect(dx, dy, size, size);
          if (cellData?.isMine) {
            ctx.fillStyle = "#101010";
            const r = Math.max(1, size * 0.25);
            ctx.beginPath();
            ctx.arc(dx + size / 2, dy + size / 2, r, 0, Math.PI * 2);
            ctx.fill();
          } else if (cellData && cellData.adjacentMines > 0) {
            ctx.font = `bold ${Math.max(8, size * 0.5)}px monospace`;
            ctx.textAlign = "center";
            ctx.textBaseline = "middle";
            ctx.fillStyle = getNumberColor(cellData.adjacentMines);
            ctx.fillText(String(cellData.adjacentMines), dx + size / 2, dy + size / 2 + 1);
          }
        }
        if (isFlagged) {
          // compact placeholder; main pass may overlay sprite when zoomed in
          ctx.fillStyle = "#202020";
          ctx.fillRect(dx + 1, dy + 1, size - 2, size - 2);
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
    if (!CanvasRenderer.#ready) return;
    let key;
    if (typeof spriteID === "string") {
      key = spriteID;                           // direct string lookup
    } else {
      key = CanvasRenderer.#idToKey[spriteID];  // numeric-ID fast path
      if (!key) {
        // Fallback for legacy / out-of-range IDs
        key =
          CanvasRenderer.#frameKeys[
            spriteID % CanvasRenderer.#frameKeys.length
          ];
      }
    }

    const frame = CanvasRenderer.#frames[key];
    if (!frame) return;
    const { x, y, w, h } = frame.frame;
    ctx.drawImage(CanvasRenderer.#sheetImg, x, y, w, h, dx, dy, dw, dh);
  }

  // Cell Rendering Logic
  renderCell(
    ctx,
    screenX,
    screenY,
    cellSize,
    cellData,
    isRevealed,
    getNumberColor,
    LOD,
  ) {
    // Base cell: beveled for LOD 0, flat for higher LODs
    if (LOD === 0) {
      this.draw3DCell(ctx, screenX, screenY, cellSize, isRevealed);
    } else {
      ctx.fillStyle = isRevealed ? "#e0e0e0" : "#c0c0c0";
      ctx.fillRect(screenX, screenY, cellSize, cellSize);
    }

    // If not revealed, we're done (content will be handled separately for flags)
    if (!isRevealed) return;

    // Draw revealed cell content. If the cell is flagged, suppress underlying content
    if (cellData.isFlagged) {
      return;
    }
    if (cellData.isMine) {
      if (LOD <= 1) {
        // Use the real mine sprite (stable key "mine", ID 162)
        this.drawSprite(ctx, "mine", screenX, screenY, cellSize, cellSize);
      } else {
        // LOD 2: draw a simple dot for a mine
        ctx.fillStyle = "#101010";
        const r = Math.max(1, cellSize * 0.25);
        ctx.beginPath();
        ctx.arc(screenX + cellSize / 2, screenY + cellSize / 2, r, 0, Math.PI * 2);
        ctx.fill();
      }
    } else if (cellData.adjacentMines > 0 && LOD === 0) {
      this.drawNumber(
        ctx,
        screenX,
        screenY,
        cellSize,
        cellData.adjacentMines,
        getNumberColor,
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
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      this.canvasSizeRef = { w: width, h: height, dpr };
    }

    // Apply zoom and DPR scaling
    ctx.setTransform(dpr * zoom, 0, 0, dpr * zoom, 0, 0);

    // Level of detail based on effective pixels per cell
    const effPx = CELL_SIZE * zoom * dpr;
    const LOD = effPx < 8 ? 2 : effPx < 16 ? 1 : 0; // 0=full, 1=simple, 2=ultra-simple

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
      (viewRef.current.y + height / zoom) / CELL_SIZE,
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

          const cellData = cellDataRaw
            ? { ...cellDataRaw, isFlagged: isFlagged || cellDataRaw.isFlagged }
            : { isMine: false, adjacentMines: 0, isFlagged };

          const isRevealedForRender = isRevealedState && !isFlagged;

          this.renderCell(
            ctx,
            screenX,
            screenY,
            CELL_SIZE,
            cellData,
            isRevealedForRender,
            getNumberColor,
            LOD,
          );

          if (isFlagged) {
            this.drawSprite(ctx, flagForCell, screenX, screenY, CELL_SIZE, CELL_SIZE);
          }
        }
      }
      return;
    }

    // Render by chunks using cache
    const startCX = Math.floor(startWorldX / CHUNK);
    const startCY = Math.floor(startWorldY / CHUNK);
    const endCX = Math.floor((endWorldX - 1) / CHUNK);
    const endCY = Math.floor((endWorldY - 1) / CHUNK);

    const refs = { revealedCellsRef, flaggedCellsRef, getNumberColor };

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

        const chunkScreenX = cx * CHUNK * CELL_SIZE - viewRef.current.x;
        const chunkScreenY = cy * CHUNK * CELL_SIZE - viewRef.current.y;
        ctx.drawImage(
          entry.canvas,
          0,
          0,
          entry.canvas.width,
          entry.canvas.height,
          chunkScreenX,
          chunkScreenY,
          CHUNK * CELL_SIZE,
          CHUNK * CELL_SIZE,
        );
      }
    }

    // Overlay high-quality flag sprites when in mid LOD (LOD 1)
    if (LOD === 1) {
      const minX = startWorldX;
      const minY = startWorldY;
      const maxX = endWorldX;
      const maxY = endWorldY;
      flaggedCellsRef.current.forEach((flagForCell, key) => {
        const comma = key.indexOf(",");
        if (comma <= 0) return;
        const wx = Number(key.slice(0, comma));
        const wy = Number(key.slice(comma + 1));
        if (wx < minX || wx >= maxX || wy < minY || wy >= maxY) return;
        const screenX = wx * CELL_SIZE - viewRef.current.x;
        const screenY = wy * CELL_SIZE - viewRef.current.y;
        this.drawSprite(ctx, flagForCell, screenX, screenY, CELL_SIZE, CELL_SIZE);
      });
    }
  }
}
