export class CanvasRenderer {
  constructor() {
    this.canvasSizeRef = { w: 0, h: 0, dpr: 1 };
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

  drawFlag(ctx, x, y, size, flagColor) {
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
  ) {
    // Always draw the base cell first
    this.draw3DCell(ctx, screenX, screenY, cellSize, isRevealed);

    // If not revealed, we're done (content will be handled separately for flags)
    if (!isRevealed) return;

    // Draw revealed cell content
    if (cellData.isMine) {
      this.drawMine(ctx, screenX, screenY, cellSize);
    } else if (cellData.adjacentMines > 0) {
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
    worldToChunk,
    getNumberColor,
    playerColor,
  }) {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext("2d");
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
        this.renderCell(
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
      this.drawFlag(ctx, screenX, screenY, CELL_SIZE, flagColor);
    });
  }
}
