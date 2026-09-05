import assert from "node:assert/strict";
import fs from "node:fs/promises";
import { chromium } from "playwright";
import protobuf from "protobufjs";
import pako from "pako";
const Msg = protobuf
  .loadSync(new URL("../../proto/messages.proto", import.meta.url).pathname)
  .lookupType("ms.Msg");

const url = process.env.BASE_URL || "http://localhost:18088";
if (!["localhost", "127.0.0.1"].includes(new URL(url).hostname))
  throw new Error("benchmark requires a local server");
const label = process.env.LABEL || "cold";
const browser = await chromium.launch({ headless: true });
const results = [];
await fs.mkdir("output/playwright/minimap", { recursive: true });
const cases = Array.from({ length: Number(process.env.RUNS || 1) }, (_, run) =>
  [
    [1920, 1080],
    [3840, 2160],
  ].map(([width, height]) => ({ run, width, height }))
).flat();
try {
  for (const { run, width, height } of cases) {
    const context = await browser.newContext({ viewport: { width, height } });
    await context.addInitScript(() => {
      window.coldProbe = { frames: [], longTasks: [], errors: [] };
      let last = performance.now();
      const frame = (now) => {
        window.coldProbe.frames.push(now - last);
        last = now;
        const probe = window.coldProbe;
        const paint = window.__minimapPaint;
        if (
          probe.gestureAt &&
          paint?.at >= probe.gestureAt &&
          paint.zoom === 0.125
        ) {
          for (const key of ["covered", "ready", "settled"]) {
            if (paint[key] && probe.timings[key] == null)
              probe.timings[key] = paint.at - probe.gestureAt;
          }
        }
        requestAnimationFrame(frame);
      };
      requestAnimationFrame(frame);
      new PerformanceObserver((list) => {
        window.coldProbe.longTasks.push(
          ...list.getEntries().map((e) => e.duration)
        );
      }).observe({ type: "longtask", buffered: true });
    });
    const page = await context.newPage();
    const snapshots = [];
    let outstanding = 0,
      maxOutstanding = 0;
    const responseDelay = Number(process.env.RESPONSE_DELAY || 0);
    const timers = new Set();
    let dropped = false;
    await page.routeWebSocket("**/ws", (route) => {
      const server = route.connectToServer();
      route.onClose(() => {
        outstanding = 0;
        server.close();
      });
      route.onMessage((data) => {
        const message = Msg.decode(pako.ungzip(data));
        if (message.overviewRequest) {
          outstanding++;
          maxOutstanding = Math.max(maxOutstanding, outstanding);
        }
        server.send(data);
      });
      server.onMessage((data) => {
        const message = Msg.decode(pako.ungzip(data));
        if (message.overviewSnapshot) {
          if (process.env.DROP_FIRST && !dropped) {
            dropped = true;
            return;
          }
          const snapshot = message.overviewSnapshot;
          const deliver = () => {
            outstanding--;
            snapshots.push({
              lod: snapshot.lod,
              unchanged: Boolean(snapshot.unchanged),
              pixelBytes: snapshot.pixels?.length || 0,
              global: snapshot.global,
              originX: snapshot.originX,
              originY: snapshot.originY,
              widthChunks: snapshot.widthChunks,
              heightChunks: snapshot.heightChunks,
            });
            route.send(data);
          };
          const timer = setTimeout(() => {
            timers.delete(timer);
            deliver();
          }, responseDelay);
          timers.add(timer);
        } else route.send(data);
      });
      if (process.env.PATCH_TRAFFIC) {
        const interval = setInterval(
          () =>
            route.send(
              pako.gzip(
                Msg.encode({
                  overviewPatch: { lod: 8, revision: 0, tiles: [] },
                }).finish()
              )
            ),
          40
        );
        timers.add(interval);
      }
    });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`${url}/?benchmark=1`);
    await page
      .locator('input[placeholder="Your name"]')
      .fill(`cold-${Date.now().toString(36)}`);
    await page.getByRole("button", { name: "Join Game" }).click();
    await page.getByTitle("Expand minimap").click();
    const root = page.getByTestId("overview-minimap");
    await root.waitFor();
    await page.waitForTimeout(Number(process.env.OPEN_DELAY ?? 750));
    const started = Date.now();
    await root.locator("canvas").evaluate((canvas) => {
      const rect = canvas.getBoundingClientRect();
      window.coldProbe.gestureAt = performance.now();
      window.coldProbe.timings = {};
      canvas.dispatchEvent(
        new WheelEvent("wheel", {
          deltaY: 8000,
          clientX: rect.x + rect.width / 2,
          clientY: rect.y + rect.height / 2,
          bubbles: true,
          cancelable: true,
        })
      );
    });
    if (process.env.CHURN) {
      for (let i = 0; i < 12; i++) {
        await root.locator("canvas").evaluate((canvas, i) => {
          const rect = canvas.getBoundingClientRect();
          window.coldProbe.gestureAt = performance.now();
          window.coldProbe.timings = {};
          canvas.dispatchEvent(
            new WheelEvent("wheel", {
              deltaY: i % 2 ? 8000 : -8000,
              clientX: rect.x + rect.width / 2,
              clientY: rect.y + rect.height / 2,
              bubbles: true,
              cancelable: true,
            })
          );
        }, i);
        await page.waitForTimeout(70);
      }
    }
    let ready = false;
    try {
      await page.waitForFunction(
        () =>
          window.__minimapBenchmark?.view.zoom === 0.125 &&
          window.__minimapBenchmark.targetReady,
        null,
        { timeout: process.env.DROP_FIRST ? 20000 : 8000 }
      );
      ready = true;
    } catch {}
    const elapsed = Date.now() - started;
    await page.waitForTimeout(500);
    const result = await page.evaluate(() => ({
      ...window.__minimapBenchmark,
      paintTimings: window.coldProbe.timings,
      probe: {
        maxFrameMs: Math.max(...window.coldProbe.frames),
        longTasks: window.coldProbe.longTasks,
      },
    }));
    const view = result.view;
    const box = await root.boundingBox();
    const covered = snapshots.some(
      (r) =>
        r.lod === result.targetLOD &&
        (r.global ||
          (r.originX * 64 <= view.x &&
            r.originY * 64 <= view.y &&
            (r.originX + r.widthChunks) * 64 >=
              view.x + box.width / view.zoom &&
            (r.originY + r.heightChunks) * 64 >=
              view.y + box.height / view.zoom))
    );
    results.push({
      run,
      width,
      height,
      ready,
      covered,
      maxOutstanding,
      snapshots,
      elapsed,
      errors,
      ...result,
    });
    await page.screenshot({
      path: `output/playwright/minimap/${label}-${run}-${width}.png`,
    });
    for (const timer of timers) {
      clearTimeout(timer);
      clearInterval(timer);
    }
    await context.close();
  }
} finally {
  await browser.close();
}
await fs.writeFile(
  `output/playwright/minimap/${label}.json`,
  JSON.stringify(results, null, 2)
);
console.log(
  JSON.stringify(
    results.map(
      ({
        width,
        ready,
        covered,
        maxOutstanding,
        elapsed,
        paintTimings,
        errors,
        targetLOD,
        activeLOD,
        cache,
        network,
        probe,
      }) => ({
        width,
        ready,
        covered,
        maxOutstanding,
        elapsed,
        paintTimings,
        errors,
        targetLOD,
        activeLOD,
        cache,
        requests: network?.requests,
        rawPixelBytes: network?.rawPixelBytes,
        probe,
      })
    ),
    null,
    2
  )
);
if (process.env.ASSERT_READY) {
  for (const result of results) {
    assert.ok(result.ready, `${result.width}px cold zoom never finished`);
    assert.ok(result.covered, "ready image must cover the full viewport");
    assert.ok(
      Number.isFinite(result.paintTimings.settled),
      "target detail must finish painting"
    );
    assert.equal(result.maxOutstanding, 1, "one overview request in flight");
    assert.deepEqual(result.errors, []);
    assert.ok(result.cache.bytes <= result.cache.budgetBytes);
  }
}
