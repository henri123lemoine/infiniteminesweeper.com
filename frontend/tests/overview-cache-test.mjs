import assert from "node:assert/strict";
import { OverviewCache } from "../src/overviewCache.js";

function record(lod, originX, pixels, global = false) {
  return {
    lod,
    global,
    originX,
    originY: 0,
    widthChunks: 2,
    heightChunks: 2,
    revision: 1,
    canvasByteLength: pixels * 4,
    canvas: {},
  };
}

const cache = new OverviewCache(100);
const global = cache.put(record(8, -10, 10, true));
cache.put(record(64, 0, 50));
cache.put(record(64, 2, 50));

assert.equal(
  cache.records.has(global.key),
  true,
  "global images remain pinned"
);

assert.equal(
  cache.stats().bytes,
  40,
  "least-recent regional images are evicted"
);
assert.equal(
  cache.findForView(8, -1000, -1000, 1000, 1000),
  global,
  "global images render known state even when the viewport extends beyond their bounds"
);

const regional = cache.put(record(32, 4, 8));
assert.equal(
  cache.findForView(32, 4 * 64, 0, 6 * 64, 2 * 64),
  regional,
  "regional lookup requires complete viewport coverage"
);
assert.equal(
  cache.findForView(32, 3 * 64, 0, 6 * 64, 2 * 64),
  null,
  "partial regional images are never selected"
);

const fallbackCache = new OverviewCache(1024);
const coarse = fallbackCache.put(record(16, 3, 6));
const fine = fallbackCache.put(record(64, 3, 6));
assert.equal(
  fallbackCache.findClosestForView(32, 3 * 64, 0, 5 * 64, 2 * 64),
  fine,
  "equidistant fallback prefers the higher-detail whole image"
);
assert.equal(
  fallbackCache.recordContainsView(coarse, 3 * 64, 0, 5 * 64, 2 * 64),
  true,
  "coverage checks include complete regional bounds"
);

const pinnedCache = new OverviewCache(12);
pinnedCache.put({ ...record(64, 0, 1), pinned: true });
pinnedCache.put(record(32, 1, 3));
assert.equal(
  pinnedCache.recordsAtLOD(64).length,
  1,
  "prewarmed regional images survive normal LRU eviction"
);

console.log("OK  : overview image cache retention and coherent lookup");
