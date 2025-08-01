import React, { useState, useEffect } from "react";
import meta from "./assets/spritesheet.json";
import sheetUrl from "./assets/spritesheet.png?url";

let sheetImg; // cache across renders
const frames = meta.frames;

export default function FlagSelector({ value, onChange }) {
  const [ready, setReady] = useState(false);
  const [frameKeys, setFrameKeys] = useState([]);

  useEffect(() => {
    if (sheetImg) {
      setFrameKeys(Object.keys(frames));
      setReady(true);
      return;
    }
    setFrameKeys(Object.keys(frames));
    sheetImg = new Image();
    sheetImg.onload = () => setReady(true);
    sheetImg.src = sheetUrl;
  }, []);
  if (!ready) return <p>Loading flags…</p>;

  return (
    <div style={{ maxWidth: 320, maxHeight: 180, overflowY: "auto" }}>
      {frameKeys.map((id, idx) => (
        <button
          key={id}
          onClick={() => onChange(idx /* numeric id */)}
          style={{
            margin: 4,
            padding: 2,
            border:
              id === value ? "2px solid #4CAF50" : "1px solid rgba(0,0,0,.2)",
            borderRadius: 4,
            background: "white",
            cursor: "pointer",
          }}
        >
          <canvas
            width="32"
            height="32"
            ref={(c) => {
              if (!c) return;
              const ctx = c.getContext("2d");
              const { x, y, w, h } = frames[id].frame;
              ctx.clearRect(0, 0, 32, 32);
              ctx.drawImage(sheetImg, x, y, w, h, 0, 0, 32, 32);
            }}
          />
        </button>
      ))}
    </div>
  );
}
