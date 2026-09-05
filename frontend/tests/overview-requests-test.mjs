import assert from "node:assert/strict";
import { setTimeout as delay } from "node:timers/promises";
import { OverviewRequests } from "../src/overviewRequests.js";
import {
  overviewRegionForView,
  overviewRunwayZoom,
  targetOverviewLOD,
  OVERVIEW_MAX_PIXELS,
  OVERVIEW_MAX_SIDE,
} from "../src/overviewGeometry.js";

const sent = [];
let timeouts = 0;
const requests = new OverviewRequests(
  (request) => {
    sent.push(request);
    return true;
  },
  () => timeouts++,
  30
);
requests.request({ lod: 4 });
for (let i = 0; i < 1000; i++)
  requests.request({ lod: 64, originX: i, subscribe: true });
assert.equal(
  sent.length,
  1,
  "rapid motion sends only one request before a response"
);
requests.complete(sent[0].requestId);
assert.equal(sent.length, 2);
assert.equal(sent[1].originX, 999, "only the latest view is fetched");
requests.complete(sent[0].requestId);
assert.equal(
  requests.pending.requestId,
  sent[1].requestId,
  "late responses cannot release another request"
);
requests.release();
assert.ok(
  requests.pending,
  "closing the overlay does not admit another concurrent snapshot"
);
await delay(45);
assert.equal(
  timeouts,
  1,
  "a missing response reconnects instead of leaving loading stuck"
);
assert.equal(requests.pending, null);
requests.request({ lod: 8 });
requests.reset();
await delay(45);
assert.equal(timeouts, 1, "disconnect cancels the response timer");

requests.request({ lod: 4, originX: 0, subscribe: false, global: false });
requests.request({
  global: false,
  subscribe: false,
  originX: 0,
  lod: 4,
  knownRevision: 99,
});
assert.equal(
  requests.queued,
  null,
  "equivalent requests do not depend on property order or stale revisions"
);
requests.request({ lod: 8, subscribe: true });
requests.request({ lod: 2, subscribe: false });
assert.equal(
  requests.queued.lod,
  8,
  "background previews cannot displace visible detail"
);
requests.complete(requests.pending.requestId);
requests.request({ lod: 4, subscribe: false });
assert.equal(
  requests.queued,
  null,
  "background previews wait for foreground loading to finish"
);
requests.request({ lod: 64, subscribe: true });
requests.request({ subscribe: true, lod: 8 });
assert.equal(
  requests.queued,
  null,
  "returning to the in-flight view cancels obsolete queued detail"
);
requests.reset();

requests.request({
  lod: 4,
  originX: -145,
  originY: -82,
  widthChunks: 291,
  heightChunks: 164,
});
requests.request({
  lod: 4,
  originX: -145,
  originY: -81,
  widthChunks: 291,
  heightChunks: 163,
});
assert.equal(
  requests.queued,
  null,
  "the opening preview reuses an in-flight image that covers it"
);
requests.request({ lod: 2, originX: 1000 });
requests.request({
  lod: 4,
  originX: -145,
  originY: -81,
  widthChunks: 291,
  heightChunks: 163,
});
assert.equal(
  requests.queued,
  null,
  "returning to a covered preview cancels obsolete background work"
);
requests.request({
  lod: 4,
  originX: -145,
  originY: -81,
  widthChunks: 291,
  heightChunks: 163,
  subscribe: true,
});
assert.ok(
  requests.queued?.subscribe,
  "a covered foreground request still establishes its subscription"
);
requests.reset();

for (const [width, height] of [
  [1920, 1080],
  [3840, 2160],
  [7680, 4320],
  [16384, 8192],
]) {
  for (const zoom of [0.125, 0.1875, 0.25, 0.5, 1, 2, 8]) {
    const lod = targetOverviewLOD(zoom, width, height);
    for (const x of [-1001.7, -0.1, 0, 1024.01]) {
      const view = { x, y: x / 3, zoom };
      const runway = overviewRunwayZoom(lod, width, height, zoom);
      const runwayRegion = overviewRegionForView(
        { ...view, zoom: runway },
        width,
        height,
        lod
      );
      assert.ok(
        runwayRegion.widthChunks * runwayRegion.heightChunks * lod * lod <=
          OVERVIEW_MAX_PIXELS
      );
      const r = overviewRegionForView(view, width, height, lod);
      assert.ok(
        r.widthChunks * r.heightChunks * lod * lod <= OVERVIEW_MAX_PIXELS
      );
      assert.ok(
        r.widthChunks * lod <= OVERVIEW_MAX_SIDE &&
          r.heightChunks * lod <= OVERVIEW_MAX_SIDE
      );
      assert.ok(r.originX * 64 <= view.x && r.originY * 64 <= view.y);
      assert.ok((r.originX + r.widthChunks) * 64 >= view.x + width / zoom);
      assert.ok((r.originY + r.heightChunks) * 64 >= view.y + height / zoom);
    }
  }
}
console.log(
  "OK  : bounded overview requests, timeout recovery, and viewport coverage through 16K"
);
