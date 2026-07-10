#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

const args = Object.fromEntries(
  process.argv.slice(2).map((value) => {
    const [key, ...rest] = value.replace(/^--/, "").split("=");
    return [key, rest.length ? rest.join("=") : true];
  })
);
const baseURL = String(
  args.url || process.env.BASE_URL || "http://localhost:8080"
);
const metricsURL = args.metrics || process.env.METRICS_URL || "";
const label = String(args.label || "candidate");
const legacy = Boolean(args.legacy);
const outputDir = path.resolve(
  path.dirname(new URL(import.meta.url).pathname),
  "../../output/playwright/minimap"
);
await fs.mkdir(outputDir, { recursive: true });

const percentile = (values, p) => {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
};

const parseMetrics = (text) =>
  Object.fromEntries(
    text
      .split("\n")
      .filter((line) => line && !line.startsWith("#"))
      .map((line) => line.trim().split(/\s+/))
      .map(([key, value]) => [key, Number(value)])
  );

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1920, height: 1080 },
  deviceScaleFactor: 1,
});
await context.addInitScript(() => {
  window.__frameProbe = { deltas: [], longTasks: [] };
  let previous = performance.now();
  const frame = (now) => {
    window.__frameProbe.deltas.push(now - previous);
    if (window.__frameProbe.deltas.length > 2000) {
      window.__frameProbe.deltas.shift();
    }
    previous = now;
    requestAnimationFrame(frame);
  };
  requestAnimationFrame(frame);
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        window.__frameProbe.longTasks.push({
          startTime: entry.startTime,
          duration: entry.duration,
        });
      }
    }).observe({ type: "longtask", buffered: true });
  } catch {}
});

const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send("Network.enable");
let websocketBytes = 0;
let websocketFrames = 0;
cdp.on("Network.webSocketFrameReceived", ({ response }) => {
  websocketFrames++;
  websocketBytes +=
    response.opcode === 2
      ? Math.floor((response.payloadData.length * 3) / 4)
      : Buffer.byteLength(response.payloadData);
});

const metrics = async () => {
  if (!metricsURL) return {};
  const response = await context.request.get(metricsURL);
  return response.ok() ? parseMetrics(await response.text()) : {};
};

const resetFrameProbe = () =>
  page.evaluate(() => {
    window.__frameProbe.deltas = [];
    window.__frameProbe.longTasks = [];
  });

const readFrameProbe = async () => {
  const probe = await page.evaluate(() => ({
    deltas: [...window.__frameProbe.deltas],
    longTasks: [...window.__frameProbe.longTasks],
  }));
  const deltas = probe.deltas.filter((value) => value < 1000);
  return {
    frames: deltas.length,
    p50Ms: percentile(deltas, 0.5),
    p95Ms: percentile(deltas, 0.95),
    p99Ms: percentile(deltas, 0.99),
    maxMs: Math.max(0, ...deltas),
    framesOver20Ms: deltas.filter((value) => value > 20).length,
    framesOver33Ms: deltas.filter((value) => value > 33.34).length,
    longTasks: probe.longTasks.length,
    longTaskTotalMs: probe.longTasks.reduce(
      (sum, entry) => sum + entry.duration,
      0
    ),
    longTaskEntries: probe.longTasks,
  };
};

const overview = page.getByTestId("overview-minimap");
const diagnostics = () =>
  page.evaluate(() =>
    window.__minimapBenchmark
      ? JSON.parse(JSON.stringify(window.__minimapBenchmark))
      : null
  );

const visualHash = () =>
  page.evaluate(() => {
    const close = Array.from(document.querySelectorAll("button")).find(
      (button) => button.textContent.trim() === "Close"
    );
    let overlay = close;
    while (overlay && getComputedStyle(overlay).position !== "fixed") {
      overlay = overlay.parentElement;
    }
    const canvases = Array.from(overlay?.querySelectorAll("canvas") || []);
    const canvas = canvases.find(
      (candidate) =>
        candidate.width > innerWidth / 2 && candidate.height > innerHeight / 2
    );
    if (!canvas) return { hash: 0, canvases: canvases.length };
    const sample = document.createElement("canvas");
    sample.width = 48;
    sample.height = 27;
    const ctx = sample.getContext("2d", { willReadFrequently: true });
    ctx.drawImage(canvas, 0, 0, sample.width, sample.height);
    const pixels = ctx.getImageData(0, 0, sample.width, sample.height).data;
    let hash = 2166136261;
    for (let i = 0; i < pixels.length; i += 4) {
      hash ^= pixels[i] | (pixels[i + 1] << 8) | (pixels[i + 2] << 16);
      hash = Math.imul(hash, 16777619);
    }
    return { hash: hash >>> 0, canvases: canvases.length };
  });

try {
  const beforeMetrics = await metrics();
  const benchmarkURL = new URL(baseURL);
  benchmarkURL.searchParams.set("benchmark", "1");
  await page.goto(benchmarkURL.href, {
    waitUntil: "domcontentloaded",
    timeout: 15000,
  });
  await page
    .locator('input[placeholder="Your name"]')
    .waitFor({ timeout: 10000 });
  await page
    .locator('input[placeholder="Your name"]')
    .fill(`b-${Date.now().toString(36).slice(-6)}-${label}`.slice(0, 20));
  await page.getByRole("button", { name: "Join Game" }).click();
  await page.waitForFunction(
    () => Boolean(localStorage.getItem("session_token")),
    null,
    {
      timeout: 15000,
    }
  );
  const expand = page.getByTitle("Expand minimap");
  await expand.waitFor({ timeout: 10000 });
  await page.waitForFunction(
    () => window.__overviewPrefetchBenchmark?.network.prefetchReadyAt > 0,
    null,
    { timeout: 5000 }
  );

  await resetFrameProbe();
  const wsBeforeFast = { bytes: websocketBytes, frames: websocketFrames };
  const preopenPrefetch = await page.evaluate(() => ({
    now: performance.now(),
    ...(window.__overviewPrefetchBenchmark?.network || {}),
  }));
  await page.evaluate(() => {
    window.__overviewOpenStartedAt = performance.now();
  });
  await expand.click();
  const canvas = legacy
    ? page
        .locator('button:has-text("Close")')
        .locator("xpath=../..")
        .locator("canvas")
        .last()
    : overview.locator("canvas");
  await canvas.waitFor({ timeout: 10000 });
  const box = await canvas.boundingBox();
  assert.ok(box, "minimap canvas has no bounds");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  const zoomStarted = performance.now();
  await page.evaluate(() => {
    window.__overviewZoomStartedAt = performance.now();
    window.__overviewMaxReadyAt = 0;
    window.__overviewReadyObserver?.disconnect();
    const recordReady = () => {
      const root = document.querySelector('[data-testid="overview-minimap"]');
      if (
        !window.__overviewMaxReadyAt &&
        root?.dataset.targetLod === "8" &&
        root?.dataset.activeLod === "8" &&
        root?.dataset.targetReady === "true"
      ) {
        window.__overviewMaxReadyAt = performance.now();
      }
    };
    window.__overviewReadyObserver = new MutationObserver(recordReady);
    window.__overviewReadyObserver.observe(document.body, {
      subtree: true,
      attributes: true,
      attributeFilter: [
        "data-target-lod",
        "data-active-lod",
        "data-target-ready",
      ],
    });
    recordReady();
  });
  for (let i = 0; i < 7; i++) {
    await page.mouse.wheel(0, 650);
    await page.waitForTimeout(40);
  }

  let maxReadyMs = null;
  let openToMaxReadyMs = null;
  if (!legacy) {
    await page.waitForFunction(
      () => {
        const root = document.querySelector('[data-testid="overview-minimap"]');
        return (
          root?.dataset.targetLod === "8" &&
          root?.dataset.activeLod === "8" &&
          root?.dataset.targetReady === "true"
        );
      },
      null,
      { timeout: 5000 }
    );
    const timing = await page.evaluate(() => ({
      ready: window.__overviewMaxReadyAt,
      zoom: window.__overviewZoomStartedAt,
      open: window.__overviewOpenStartedAt,
    }));
    maxReadyMs = timing.ready - timing.zoom;
    openToMaxReadyMs = timing.ready - timing.open;
  }

  let previousHash = await visualHash();
  let visualChanges = 0;
  let lastVisualChangeMs = 0;
  const stabilityStarted = performance.now();
  while (performance.now() - stabilityStarted < 4000) {
    await page.waitForTimeout(50);
    const nextHash = await visualHash();
    if (nextHash.hash !== previousHash.hash) {
      visualChanges++;
      lastVisualChangeMs = performance.now() - zoomStarted;
      previousHash = nextHash;
    }
  }
  const fastFrames = await readFrameProbe();
  const fastDiagnostics = await diagnostics();
  const wsAfterFast = { bytes: websocketBytes, frames: websocketFrames };
  await page.screenshot({
    path: path.join(outputDir, `${label}-max-zoom.png`),
    fullPage: false,
  });

  const panNetworkBefore = fastDiagnostics?.network
    ? { ...fastDiagnostics.network }
    : { responseBytes: websocketBytes, requests: websocketFrames };
  await resetFrameProbe();
  await page.mouse.move(box.x + box.width * 0.7, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.2, box.y + box.height / 2, {
    steps: 10,
  });
  await page.mouse.up();
  await page.waitForTimeout(150);
  await page.mouse.move(box.x + box.width * 0.2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.7, box.y + box.height / 2, {
    steps: 10,
  });
  await page.mouse.up();
  await page.waitForTimeout(500);
  const panDiagnostics = await diagnostics();
  const panNetworkAfter = panDiagnostics?.network
    ? { ...panDiagnostics.network }
    : { responseBytes: websocketBytes, requests: websocketFrames };
  const panFrames = await readFrameProbe();

  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, -6000);
  await page.waitForTimeout(1200);
  await resetFrameProbe();
  const lodSamples = [];
  for (let i = 0; i < 30; i++) {
    await page.mouse.wheel(0, 250);
    await page.waitForTimeout(80);
    if (!legacy) {
      lodSamples.push(
        await overview.evaluate((element) => ({
          target: Number(element.dataset.targetLod),
          active: Number(element.dataset.activeLod),
          coherent: element.dataset.coherent,
        }))
      );
    }
  }
  if (!legacy) {
    await page.waitForFunction(
      () =>
        document.querySelector('[data-testid="overview-minimap"]')?.dataset
          .activeLod === "8",
      null,
      { timeout: 5000 }
    );
  }
  const slowFrames = await readFrameProbe();
  const finalDiagnostics = await diagnostics();
  const heap = await cdp.send("Runtime.getHeapUsage");

  const reopenNetworkBefore = finalDiagnostics?.network
    ? { ...finalDiagnostics.network }
    : { responseBytes: websocketBytes, requests: websocketFrames };
  await page.getByRole("button", { name: "Close" }).first().click();
  await expand.waitFor({ timeout: 5000 });
  await resetFrameProbe();
  const reopenStarted = performance.now();
  await expand.click();
  const reopenedCanvas = legacy
    ? page
        .locator('button:has-text("Close")')
        .locator("xpath=../..")
        .locator("canvas")
        .last()
    : overview.locator("canvas");
  await reopenedCanvas.waitFor({ timeout: 5000 });
  const reopenedBox = await reopenedCanvas.boundingBox();
  assert.ok(reopenedBox, "reopened minimap canvas has no bounds");
  await page.mouse.move(
    reopenedBox.x + reopenedBox.width / 2,
    reopenedBox.y + reopenedBox.height / 2
  );
  for (let i = 0; i < 7; i++) {
    await page.mouse.wheel(0, 650);
    await page.waitForTimeout(40);
  }
  if (!legacy) {
    await page.waitForFunction(
      () =>
        document.querySelector('[data-testid="overview-minimap"]')?.dataset
          .activeLod === "8",
      null,
      { timeout: 2000 }
    );
  }
  const reopenReadyMs = performance.now() - reopenStarted;
  await page.waitForTimeout(500);
  const reopenDiagnostics = await diagnostics();
  const reopenNetworkAfter = reopenDiagnostics?.network
    ? { ...reopenDiagnostics.network }
    : { responseBytes: websocketBytes, requests: websocketFrames };
  const reopenFrames = await readFrameProbe();
  const liveMetrics = await metrics();
  await page.close();
  await new Promise((resolve) => setTimeout(resolve, 500));
  const settledMetrics = await metrics();

  const result = {
    label,
    legacy,
    baseURL,
    viewport: { width: 1920, height: 1080, deviceScaleFactor: 1 },
    coldFastZoom: {
      prefetchMs:
        preopenPrefetch.prefetchReadyAt - preopenPrefetch.prefetchStartedAt,
      prefetchedBeforeOpen:
        preopenPrefetch.prefetchReadyAt > 0 &&
        preopenPrefetch.prefetchReadyAt <= preopenPrefetch.now,
      openToMaxReadyMs,
      maxReadyMs,
      visualChanges,
      lastVisualChangeMs,
      websocketBytes: wsAfterFast.bytes - wsBeforeFast.bytes,
      websocketFrames: wsAfterFast.frames - wsBeforeFast.frames,
      overviewNetwork: fastDiagnostics?.network || null,
      regionalRequestCount: (fastDiagnostics?.network?.requestLog || []).filter(
        (request) => !request.global
      ).length,
      events: fastDiagnostics?.events || [],
      framePacing: fastFrames,
    },
    panAwayAndBack: {
      requestDelta:
        (panNetworkAfter.requests || 0) - (panNetworkBefore.requests || 0),
      responseByteDelta:
        (panNetworkAfter.responseBytes || 0) -
        (panNetworkBefore.responseBytes || 0),
      framePacing: panFrames,
    },
    slowZoom: {
      samples: lodSamples,
      activeLODs: [...new Set(lodSamples.map((sample) => sample.active))],
      allFramesCoherent: lodSamples.every(
        (sample) => sample.coherent === "true"
      ),
      framePacing: slowFrames,
    },
    reopenAtMaxZoom: {
      readyMs: reopenReadyMs,
      requestDelta:
        (reopenNetworkAfter.requests || 0) -
        (reopenNetworkBefore.requests || 0),
      responseByteDelta:
        (reopenNetworkAfter.responseBytes || 0) -
        (reopenNetworkBefore.responseBytes || 0),
      framePacing: reopenFrames,
    },
    client: {
      diagnostics: finalDiagnostics,
      heapUsedBytes: heap.usedSize,
      backingStoreBytes: heap.backingStorageSize,
    },
    server: {
      before: beforeMetrics,
      live: liveMetrics,
      settled: settledMetrics,
    },
  };

  const output = path.join(outputDir, `${label}.json`);
  await fs.writeFile(output, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify(result, null, 2));
  console.log(`\nWrote ${output}`);

  if (!legacy) {
    assert.ok(
      result.coldFastZoom.maxReadyMs <= 500,
      "max zoom image missed 500ms target"
    );
    assert.equal(
      result.coldFastZoom.regionalRequestCount,
      0,
      "fast zoom fetched an intermediate regional overview"
    );
    assert.ok(
      result.coldFastZoom.websocketBytes <= 512 * 1024,
      "fast zoom downloaded more than 512 KiB"
    );
    assert.equal(
      result.panAwayAndBack.requestDelta,
      0,
      "pan-return made a new request"
    );
    assert.equal(
      result.panAwayAndBack.responseByteDelta,
      0,
      "pan-return downloaded overview data"
    );
    assert.equal(
      result.slowZoom.allFramesCoherent,
      true,
      "slow zoom mixed image LODs"
    );
    assert.ok(
      result.reopenAtMaxZoom.responseByteDelta < 1024,
      "reopening downloaded the full overview again"
    );
    assert.ok(
      (finalDiagnostics?.cache?.bytes || 0) <= 64 * 1024 * 1024,
      "overview cache exceeded 64 MiB"
    );
  }
} finally {
  await browser.close();
}
