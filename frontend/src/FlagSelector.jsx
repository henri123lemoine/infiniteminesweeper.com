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

  // Shape name -> the achievement(s) that unlock it. Shared shapes are
  // color-split between two achievements, so both names are listed.
  const unlockedBy = useMemo(() => {
    const m = new Map();
    for (const a of ACHIEVEMENTS) {
      if (!a.rewardFlagId) continue;
      const shapeName = frames[String(a.rewardFlagId)]?.name;
      if (!shapeName) continue;
      const prev = m.get(shapeName);
      m.set(shapeName, prev ? `${prev} / ${a.name}` : a.name);
    }
    return m;
  }, []);

  if (!ready) return <p>Loading flags...</p>;

  const currentShape =
    shapes.find((s) => s.ids.includes(value)) || shapes[0];
  const colorIdx = Math.max(0, currentShape.ids.indexOf(value));

  // Unlocks are per color variant: split-shape achievements grant only half
  // the palette, so a shape can be partially unlocked.
  const isVariantUnlocked = (shape, id) =>
    shape.cost <= STARTER_MAX_COST ||
    id === value ||
    unlockedFlagIds.includes(id);

  const isShapeUnlocked = (shape) =>
    shape.ids.some((id) => isVariantUnlocked(shape, id));

  const pickShape = (shape) => {
    if (!isShapeUnlocked(shape)) return;
    const preferred = shape.ids[Math.min(colorIdx, shape.ids.length - 1)];
    if (isVariantUnlocked(shape, preferred)) {
      onChange(preferred);
      return;
    }
    onChange(shape.ids.find((id) => isVariantUnlocked(shape, id)));
  };
  const pickColor = (idx) => {
    if (idx >= currentShape.ids.length) return;
    const id = currentShape.ids[idx];
    if (!isVariantUnlocked(currentShape, id)) return;
    onChange(id);
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
            const colorUnlocked =
              available && isVariantUnlocked(currentShape, currentShape.ids[idx]);
            const previewId = available
              ? currentShape.ids[idx]
              : shapes[0].ids[idx];
            let cls = "flag-cell color";
            if (colorUnlocked && idx === colorIdx) cls += " selected";
            if (!available || !colorUnlocked) cls += " locked";
            return (
              <button
                key={idx}
                className={cls}
                disabled={!available || !colorUnlocked}
                onClick={() => pickColor(idx)}
                title={
                  !available
                    ? "This flag only comes in one color"
                    : colorUnlocked
                      ? ""
                      : "This color belongs to a different advancement"
                }
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
