import assert from "node:assert/strict";
import {
  DEFAULT_SETTINGS,
  actionFor,
  clampLongPressMs,
  loadSettings,
  saveSettings,
} from "../src/settings.js";

const memory = () => {
  const m = new Map();
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
  };
};

assert.deepEqual(loadSettings(memory()), DEFAULT_SETTINGS);

const store = memory();
saveSettings({ swapClicks: true, longPressMs: 350 }, store);
assert.deepEqual(loadSettings(store), { swapClicks: true, longPressMs: 350 });

assert.equal(clampLongPressMs(5), 100);
assert.equal(clampLongPressMs(99999), 1000);
assert.equal(clampLongPressMs("abc"), 200);
assert.equal(clampLongPressMs("250.4"), 250);

store.setItem("longPressMs", "garbage");
assert.equal(loadSettings(store).longPressMs, 200);

assert.equal(actionFor(true, false), "reveal");
assert.equal(actionFor(false, false), "flag");
assert.equal(actionFor(true, true), "flag");
assert.equal(actionFor(false, true), "reveal");

console.log("OK  : settings");
