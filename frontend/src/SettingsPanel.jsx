import React, { useState } from "react";
import {
  DEFAULT_LONG_PRESS_MS,
  MIN_LONG_PRESS_MS,
  MAX_LONG_PRESS_MS,
  clampLongPressMs,
} from "./settings.js";

export default function SettingsPanel({ settings, onChange }) {
  const [draft, setDraft] = useState(String(settings.longPressMs));

  const commitDuration = () => {
    const value = clampLongPressMs(
      draft === "" ? DEFAULT_LONG_PRESS_MS : draft
    );
    setDraft(String(value));
    onChange({ ...settings, longPressMs: value });
  };

  return (
    <div className="settings-panel">
      <label className="settings-row">
        <span className="settings-label">Tap / left click</span>
        <select
          className="settings-control"
          value={settings.swapClicks ? "flag" : "reveal"}
          onChange={(e) =>
            onChange({ ...settings, swapClicks: e.target.value === "flag" })
          }
        >
          <option value="reveal">
            Reveals (long press / right click flags)
          </option>
          <option value="flag">Flags (long press / right click reveals)</option>
        </select>
      </label>
      <div className="settings-touch-only">
        <label className="settings-row">
          <span className="settings-label">Long press duration (ms)</span>
          <input
            className="settings-control"
            type="number"
            inputMode="numeric"
            min={MIN_LONG_PRESS_MS}
            max={MAX_LONG_PRESS_MS}
            step={50}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commitDuration}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
            }}
          />
        </label>
        <div className="settings-hint">
          Default {DEFAULT_LONG_PRESS_MS} ms, between {MIN_LONG_PRESS_MS} and{" "}
          {MAX_LONG_PRESS_MS}.
        </div>
      </div>
    </div>
  );
}
