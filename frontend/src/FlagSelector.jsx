import React, { useState, useEffect, useMemo } from "react";
import { initSprites, getFlagIdsSortedByCost, drawSprite, frames } from "./sprites/index.js";

export default function FlagSelector({ value, onChange }) {
  const [ready, setReady] = useState(false);

  // Pre‑load sprite sheet once
  useEffect(() => {
    initSprites().then(() => setReady(true));
  }, []);

  // Get sorted flag IDs from centralized sprite system
  const sortedKeys = useMemo(() => {
    return getFlagIdsSortedByCost().map(id => String(id));
  }, []);

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
            ctx.clearRect(0, 0, 32, 32);
            drawSprite(ctx, id, 0, 0, 32, 32);
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
          WebkitOverflowScrolling: "touch",
          scrollbarWidth: "thin",
          maxHeight: "40vh",
        }}
      >
        {buttons}
      </div>
    </div>
  );
}
