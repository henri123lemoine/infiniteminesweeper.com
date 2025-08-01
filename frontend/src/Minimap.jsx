import meta from "./assets/spritesheet.json";
import React, { useRef, useState, useEffect, useCallback } from "react";

const FRAME_KEYS = Object.keys(meta.frames);

export default function Minimap({
  CHUNK,
  CELL_SIZE,
  MINIMAP_SIZE,
  zoom,
  viewX,
  viewY,
  tick,
  containerRef,
  seedCache,
  revealedCellsRef,
  flaggedCellsRef,
  worldToChunk,
  isMine,
}) {
  const miniRef = useRef(null);
  const [minimapChunks, setMinimapChunks] = useState(3); // 1→3→5 cycle

  const [updateCounter, setUpdateCounter] = useState(0);

  const toggleSize = useCallback(() => {
    setMinimapChunks((c) => (c === 1 ? 3 : c === 3 ? 5 : 1));
  }, []);

  // Force minimap updates on a timer as a fallback
  useEffect(() => {
    const interval = setInterval(() => {
      setUpdateCounter(c => c + 1);
    }, 100);

    return () => clearInterval(interval);
  }, []);

  /** full repaint */
  useEffect(() => {
    const canvas = miniRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const cellsPerSide = CHUNK * minimapChunks;
    canvas.width = cellsPerSide;
    canvas.height = cellsPerSide;
    canvas.style.width = `${MINIMAP_SIZE}px`;
    canvas.style.height = `${MINIMAP_SIZE}px`;

    const ctx = canvas.getContext("2d");

    // centre on viewX / viewY
    const width = container.clientWidth || 0;
    const height = container.clientHeight || 0;
    const centerWorldX = Math.floor((viewX + width / 2 / zoom) / CELL_SIZE);
    const centerWorldY = Math.floor((viewY + height / 2 / zoom) / CELL_SIZE);

    const minimapCenterX = cellsPerSide / 2;
    const minimapCenterY = cellsPerSide / 2;

    const worldStartX = centerWorldX - Math.floor(cellsPerSide / 2);
    const worldStartY = centerWorldY - Math.floor(cellsPerSide / 2);

    ctx.fillStyle = "#808080";
    ctx.fillRect(0, 0, cellsPerSide, cellsPerSide);

    for (let py = 0; py < cellsPerSide; py++) {
      for (let px = 0; px < cellsPerSide; px++) {
        const worldX = worldStartX + px;
        const worldY = worldStartY + py;

        const { chunkX, chunkY, localX, localY } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${localX},${localY}`;
        const flagKey = `${worldX},${worldY}`;
        const seed = seedCache.current.get(`${chunkX},${chunkY}`);

        let color = "#909090";
        const flagID = flaggedCellsRef.current.get(flagKey);
        if (flagID !== undefined) {
          const idx = flagID % FRAME_KEYS.length;
          const spriteKey = FRAME_KEYS[idx];
          color = meta.frames[spriteKey].hex;
        } else if (revealedCellsRef.current.has(cellKey)) {
          const cell = revealedCellsRef.current.get(cellKey);
          if (cell.isMine) color = "#333333";
          else {
            const n = cell.adjacentMines;
            color =
              n === 0
                ? "#e0e0e0"
                : n === 1
                  ? "#d0d0ff"
                  : n === 2
                    ? "#d0ffd0"
                    : n === 3
                      ? "#ffd0d0"
                      : n === 4
                        ? "#d0d0d0"
                        : n === 5
                          ? "#f0d0d0"
                          : n === 6
                            ? "#d0f0f0"
                            : "#c0c0c0";
          }
        } else if (seed && isMine(seed, localX, localY)) {
          color = "#909090";
        }

        ctx.fillStyle = color;
        ctx.fillRect(px, py, 1, 1);
      }
    }

    // viewport box
    if (width && height) {
      const viewWidthCells = Math.ceil(width / zoom / CELL_SIZE);
      const viewHeightCells = Math.ceil(height / zoom / CELL_SIZE);
      const boxLeft = minimapCenterX - viewWidthCells / 2;
      const boxTop = minimapCenterY - viewHeightCells / 2;
      ctx.strokeStyle = "rgba(200,200,200,0.8)";
      ctx.lineWidth = 1;
      ctx.strokeRect(boxLeft, boxTop, viewWidthCells, viewHeightCells);
    }
  }, [
    viewX,
    viewY,
    tick,
    updateCounter,
    minimapChunks,
    CHUNK,
    CELL_SIZE,
    MINIMAP_SIZE,
    zoom,
    containerRef,
    seedCache,
    revealedCellsRef,
    flaggedCellsRef,
    worldToChunk,
    isMine,
  ]);

  // repaint on window resize
  useEffect(() => {
    const fn = () =>
      miniRef.current &&
      miniRef.current.getContext &&
      window.requestAnimationFrame(() => {});
    window.addEventListener("resize", fn);
    return () => window.removeEventListener("resize", fn);
  }, []);

  return (
    <canvas
      ref={miniRef}
      className="minimap"
      onClick={toggleSize}
      style={{ cursor: "pointer" }}
    />
  );
}
