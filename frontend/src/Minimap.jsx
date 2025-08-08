import meta from "./assets/spritesheet.json";
import React, { useRef, useState, useEffect, useCallback } from "react";

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

  // Resolve a minimap color from a stable numeric flag ID using spritesheet metadata
  const getFlagHex = useCallback((flagID) => {
    const frame = meta.frames[String(flagID)];
    if (!frame) return "#909090";

    // Normalize some colors for better minimap legibility
    const colorName = frame.colorName;
    const hex = frame.hex || "#909090";

    // Make white/light-gray variants fully white on the minimap
    if (colorName === "Light Gray") return "#FFFFFF";

    // Make very dark variants slightly lighter than bombs for contrast
    const isVeryDark = /^#([0-1][0-9a-f]{1}){3}$/i.test(hex) || /^#0{6}$/i.test(hex);
    if (colorName === "Dark Gray" || isVeryDark) return "#101010";

    return hex;
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
    ctx.imageSmoothingEnabled = false;

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

        const { chunkX, chunkY, cell: cellIndex } = worldToChunk(worldX, worldY);
        const cellKey = `${chunkX},${chunkY},${cellIndex}`;
        const flagKey = `${worldX},${worldY}`;
        const seed = seedCache.current.get(`${chunkX},${chunkY}`);

        let color = "#909090";
        const flagID = flaggedCellsRef.current.get(flagKey);
        if (flagID !== undefined) {
          color = getFlagHex(flagID);
        } else if (revealedCellsRef.current.has(cellKey)) {
          const cell = revealedCellsRef.current.get(cellKey);
          if (cell.isMine) color = "#000000"; // bombs slightly darker than black flags
          else {
            const n = cell.adjacentMines;
            // Very muted tints to avoid over-saturation (keep near-grey)
            color =
              n === 0
                ? "#e0e0e0"
                : n === 1
                  ? "#e9ecff" // soft blue
                  : n === 2
                    ? "#e9ffea" // soft green
                    : n === 3
                      ? "#ffe9ea" // soft red
                      : n === 4
                        ? "#ececff" // soft navy
                        : n === 5
                          ? "#fff0ea" // soft maroon
                          : n === 6
                            ? "#d4fff2" // slightly less muted cyan
                            : n === 7
                              ? "#f0e4ff" // visible soft purple
                              : n === 8
                                ? "#ffe8b3" // visible soft amber
                                : "#e4e4e4"; // others
          }
        } else if (seed && isMine(seed, cellIndex)) {
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
    getFlagHex,
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
