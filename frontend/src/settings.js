export const DEFAULT_LONG_PRESS_MS = 200;
export const MIN_LONG_PRESS_MS = 100;
export const MAX_LONG_PRESS_MS = 1000;

export const DEFAULT_SETTINGS = {
  swapClicks: false,
  longPressMs: DEFAULT_LONG_PRESS_MS,
};

export function clampLongPressMs(value) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return DEFAULT_LONG_PRESS_MS;
  return Math.min(Math.max(n, MIN_LONG_PRESS_MS), MAX_LONG_PRESS_MS);
}

export function loadSettings(storage = globalThis.localStorage) {
  try {
    const swapClicks = storage.getItem("swapClicks") === "1";
    const raw = storage.getItem("longPressMs");
    const longPressMs =
      raw == null ? DEFAULT_LONG_PRESS_MS : clampLongPressMs(raw);
    return { swapClicks, longPressMs };
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(settings, storage = globalThis.localStorage) {
  try {
    storage.setItem("swapClicks", settings.swapClicks ? "1" : "0");
    storage.setItem(
      "longPressMs",
      String(clampLongPressMs(settings.longPressMs))
    );
  } catch {
    // Storage unavailable (private mode): settings last for the session only.
  }
}

// `isTap` is a plain tap/left click; otherwise it is a long press/right click.
export function actionFor(isTap, swapClicks) {
  return isTap === swapClicks ? "flag" : "reveal";
}
