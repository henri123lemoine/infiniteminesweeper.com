import meta from "../assets/spritesheet.json";
import sheetUrl from "../assets/spritesheet.png?url";

// Shared sprite sheet state
let sheetImg,
  ready = false;

// Pre-computed lookups
const frames = meta.frames;
const frameKeys = Object.keys(frames);
const idToKey = {};
for (const k of frameKeys) {
  if (!Number.isNaN(Number(k))) idToKey[Number(k)] = k;
}

const flagIds = frameKeys
  .filter((k) => !Number.isNaN(Number(k)) && frames[k]?.category === "flag")
  .map((k) => Number(k))
  .sort((a, b) => a - b);

const flagIdsSortedByCost = flagIds.slice().sort((a, b) => {
  const ca = frames[String(a)]?.cost ?? 0;
  const cb = frames[String(b)]?.cost ?? 0;
  return ca !== cb ? ca - cb : a - b;
});

// Initialize sprite sheet
export const initSprites = () => {
  if (ready) return Promise.resolve();
  if (sheetImg)
    return new Promise((resolve) => {
      if (ready) resolve();
      else sheetImg.addEventListener("load", () => resolve(), { once: true });
    });

  return new Promise((resolve) => {
    sheetImg = new Image();
    sheetImg.onload = () => {
      ready = true;
      resolve();
    };
    sheetImg.onerror = resolve; // Don't block on error
    sheetImg.src = sheetUrl;
  });
};

// Draw sprite
export const drawSprite = (ctx, spriteID, dx, dy, dw, dh) => {
  if (!ready) return;
  const key =
    typeof spriteID === "string"
      ? spriteID
      : idToKey[spriteID] || frameKeys[spriteID % frameKeys.length];
  const frame = frames[key];
  if (frame) {
    const { x, y, w, h } = frame.frame;
    ctx.drawImage(sheetImg, x, y, w, h, dx, dy, dw, dh);
  }
};

// Utilities
export const getFlagIds = () => flagIds;
export const getFlagIdsSortedByCost = () => flagIdsSortedByCost;
export const getHexForFlag = (flagID) =>
  frames[String(flagID)]?.hex || "#202020";
export { frames };
