import React, { useState, useEffect, useMemo } from "react";
import {
  initSprites,
  getFlagIdsSortedByCost,
  drawSprite,
  frames,
} from "./sprites/index.js";
import { ACHIEVEMENTS } from "./achievements.js";

const STARTER_MAX_COST = 5;

function Sprite({ id, size }) {
  return (
    <canvas
      width={size}
      height={size}
      ref={(c) => {
        if (!c) return;
        const ctx = c.getContext("2d");
        ctx.clearRect(0, 0, size, size);
        drawSprite(ctx, String(id), 0, 0, size, size);
      }}
    />
  );
}

// Flags are shapes x a shared color palette, so the picker is combinatoric:
// pick a shape (drawn in your current color), pick a color (drawn as your
// current shape). 27 controls instead of 161 buttons.
export default function FlagSelector({ value, onChange, unlockedFlagIds = [] }) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    initSprites().then(() => setReady(true));
  }, []);

  // Shapes in cost order, each with color-variant IDs in shared palette order.
  const shapes = useMemo(() => {
    const byName = new Map();
    for (const id of getFlagIdsSortedByCost()) {
      const f = frames[String(id)];
      if (!f) continue;
      let s = byName.get(f.name);
      if (!s) {
        s = { name: f.name, cost: f.cost ?? 0, ids: [] };
        byName.set(f.name, s);
      }
      s.ids.push(id);
    }
    for (const s of byName.values()) s.ids.sort((a, b) => a - b);
    return Array.from(byName.values());
  }, []);

  const paletteSize = useMemo(
    () => Math.max(...shapes.map((s) => s.ids.length)),
    [shapes]
  );

  // Shape name -> the achievement that unlocks it
  const unlockedBy = useMemo(() => {
    const m = new Map();
    for (const a of ACHIEVEMENTS) {
      if (!a.rewardFlagId) continue;
      const shapeName = frames[String(a.rewardFlagId)]?.name;
      if (shapeName && !m.has(shapeName)) m.set(shapeName, a.name);
    }
    return m;
  }, []);

  if (!ready) return <p>Loading flags...</p>;

  const currentShape =
    shapes.find((s) => s.ids.includes(value)) || shapes[0];
  const colorIdx = Math.max(0, currentShape.ids.indexOf(value));

  const isShapeUnlocked = (shape) =>
    shape.cost <= STARTER_MAX_COST ||
    shape.ids.includes(value) ||
    shape.ids.some((id) => unlockedFlagIds.includes(id));

  const pickShape = (shape) => {
    if (!isShapeUnlocked(shape)) return;
    onChange(shape.ids[Math.min(colorIdx, shape.ids.length - 1)]);
  };
  const pickColor = (idx) => {
    if (idx >= currentShape.ids.length) return;
    onChange(currentShape.ids[idx]);
  };

  return (
    <div className="flag-selector">
      <div className="flag-picker">
        <div className="flag-shapes">
          {shapes.map((shape) => {
            const unlocked = isShapeUnlocked(shape);
            const previewId =
              shape.ids[Math.min(colorIdx, shape.ids.length - 1)];
            const selected = shape === currentShape;
            let cls = "flag-cell";
            if (selected) cls += " selected";
            if (!unlocked) cls += " locked";
            return (
              <button
                key={shape.name}
                className={cls}
                onClick={() => pickShape(shape)}
                title={
                  unlocked
                    ? shape.name
                    : `${shape.name} — unlock: ${unlockedBy.get(shape.name) || "advancements"}`
                }
              >
                <Sprite id={previewId} size={30} />
                {!unlocked && <span className="flag-lock">🔒</span>}
              </button>
            );
          })}
        </div>
        <div className="flag-colors">
          {Array.from({ length: paletteSize }, (_, idx) => {
            const available = idx < currentShape.ids.length;
            const previewId = available
              ? currentShape.ids[idx]
              : shapes[0].ids[idx];
            let cls = "flag-cell color";
            if (available && idx === colorIdx) cls += " selected";
            if (!available) cls += " locked";
            return (
              <button
                key={idx}
                className={cls}
                disabled={!available}
                onClick={() => pickColor(idx)}
                title={available ? "" : "This flag only comes in one color"}
              >
                <Sprite id={previewId} size={22} />
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
