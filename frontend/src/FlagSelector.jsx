import React, { useState, useEffect, useMemo } from "react";
import meta from "./assets/spritesheet.json";
import sheetUrl from "./assets/spritesheet.png?url";

let sheetImg;
const frames = meta.frames;

export default function FlagSelector({ value, onChange }) {
  const [ready, setReady] = useState(false);

  // Pre‑load sprite sheet once
  useEffect(() => {
    if (sheetImg) {
      setReady(true);
      return;
    }
    sheetImg = new Image();
    sheetImg.onload = () => setReady(true);
    sheetImg.src = sheetUrl;
  }, []);

  // Use only stable numeric keys for sprites in the 'flag' category
  const flagKeys = useMemo(
    () =>
      Object.keys(frames).filter(
        (k) => !Number.isNaN(Number(k)) && frames[k]?.category === "flag",
      ),
    [],
  );

  // Sort by cost ascending, then numeric ID
  const sortedKeys = useMemo(() => {
    return flagKeys
      .slice()
      .sort((a, b) => {
        const ca = frames[a].cost ?? 0;
        const cb = frames[b].cost ?? 0;
        if (ca !== cb) return ca - cb;
        return Number(a) - Number(b);
      });
  }, [flagKeys]);

  if (!ready) return <p>Loading flags...</p>;

  // Build buttons — insert a flex‑row break whenever cost increases
  const buttons = [];
  let lastCost = null;
  sortedKeys.forEach((id) => {
    const cost = frames[id].cost ?? 0;
    if (lastCost !== null && cost !== lastCost) {
      buttons.push(
        <div key={`sep-${cost}`} style={{ flexBasis: "100%", height: 0 }} />
      );
    }
    lastCost = cost;

    const idNum = Number(id); // stable numeric flag ID
    buttons.push(
      <button
        key={id}
        onClick={() => onChange(idNum)}
        style={{
          width: 60,
          height: 76,
          padding: 4,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          border:
            value === idNum
              ? "2px solid #4CAF50"
              : "1px solid rgba(0,0,0,.2)",
          borderRadius: 6,
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
        <span style={{ fontSize: 12, marginTop: 4 }}>🪙 {cost}</span>
      </button>
    );
  });

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        maxWidth: "100%",
        maxHeight: "90%",
      }}
    >
      <div
        style={{
          fontWeight: "bold",
          marginBottom: 8,
          textAlign: "center",
        }}
      >
        🪙 ∞ coins
      </div>
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 8,
          overflowY: "auto",
        }}
      >
        {buttons}
      </div>
    </div>
  );
}
