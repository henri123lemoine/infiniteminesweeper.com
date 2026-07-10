#!/usr/bin/env node

import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

const args = Object.fromEntries(
  process.argv.slice(2).map((value) => {
    const [key, ...rest] = value.replace(/^--/, "").split("=");
    return [key, rest.length ? rest.join("=") : true];
  })
);
const baseURL = String(args.url || "http://localhost:18090");
const label = String(args.label || "continuity");
const outputDir = path.resolve(
  path.dirname(new URL(import.meta.url).pathname),
  "../../output/playwright/minimap/continuity"
);
await fs.mkdir(outputDir, { recursive: true });

const percentile = (values, fraction) => {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[
    Math.min(sorted.length - 1, Math.floor(sorted.length * fraction))
  ];
};

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1920, height: 1080 },
  deviceScaleFactor: 1,
});
const page = await context.newPage();

try {
  const url = new URL(baseURL);
  url.searchParams.set("benchmark", "1");
  await page.goto(url.href, { waitUntil: "domcontentloaded", timeout: 15000 });
  await page
    .locator('input[placeholder="Your name"]')
    .fill(`continuity-${Date.now().toString(36)}`.slice(0, 20));
  await page.getByRole("button", { name: "Join Game" }).click();
  await page.waitForFunction(() =>
    Boolean(localStorage.getItem("session_token"))
  );
  await page.getByTitle("Expand minimap").click();

  const root = page.getByTestId("overview-minimap");
  const canvas = root.locator("canvas");
  await canvas.waitFor({ timeout: 10000 });
  const box = await canvas.boundingBox();
  if (!box) throw new Error("overview canvas has no bounds");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, -6000);
  await page.waitForTimeout(1200);
  await page.waitForFunction(
    () => {
      const element = document.querySelector(
        '[data-testid="overview-minimap"]'
      );
      return (
        element?.dataset.targetLod === "64" &&
        element?.dataset.activeLod === "64"
      );
    },
    null,
    { timeout: 10000 }
  );

  await page.evaluate(() => {
    window.__continuityPrevious = null;
  });

  const capture = () =>
    page.evaluate(() => {
      const root = document.querySelector('[data-testid="overview-minimap"]');
      const canvas = root?.querySelector("canvas");
      const benchmark = window.__minimapBenchmark;
      if (!canvas || !benchmark) return null;

      const width = 320;
      const height = 180;
      const makeCanvas = () => {
        const output = document.createElement("canvas");
        output.width = width;
        output.height = height;
        return output;
      };
      const full = makeCanvas();
      const fullContext = full.getContext("2d", { willReadFrequently: true });
      fullContext.imageSmoothingEnabled = true;
      fullContext.imageSmoothingQuality = "high";
      fullContext.drawImage(canvas, 0, 0, width, height);
      const fullImage = fullContext.getImageData(0, 0, width, height);

      const state = {
        zoom: benchmark.view.zoom,
        targetLOD: benchmark.targetLOD,
        activeLOD: benchmark.activeLOD,
        targetReady: benchmark.targetReady,
      };
      const previous = window.__continuityPrevious;
      window.__continuityPrevious = { image: fullImage, state };
      if (!previous) return { state, comparison: null };

      const ratio = Math.min(1, state.zoom / previous.state.zoom);
      const normalized = makeCanvas();
      const normalizedContext = normalized.getContext("2d", {
        willReadFrequently: true,
      });
      normalizedContext.imageSmoothingEnabled = true;
      normalizedContext.imageSmoothingQuality = "high";
      const sourceWidth = canvas.width * ratio;
      const sourceHeight = canvas.height * ratio;
      normalizedContext.drawImage(
        canvas,
        (canvas.width - sourceWidth) / 2,
        (canvas.height - sourceHeight) / 2,
        sourceWidth,
        sourceHeight,
        0,
        0,
        width,
        height
      );
      const current = normalizedContext.getImageData(0, 0, width, height).data;
      const prior = previous.image.data;
      const samples = width * height;
      const priorLuma = new Float32Array(samples);
      const currentLuma = new Float32Array(samples);
      let count = 0;
      let priorSum = 0;
      let currentSum = 0;
      let priorSquare = 0;
      let currentSquare = 0;
      let covariance = 0;
      let lumaError = 0;
      let chromaError = 0;
      for (let pixel = 0; pixel < samples; pixel++) {
        const offset = pixel * 4;
        const pr = prior[offset];
        const pg = prior[offset + 1];
        const pb = prior[offset + 2];
        const cr = current[offset];
        const cg = current[offset + 1];
        const cb = current[offset + 2];
        const foreground =
          Math.max(Math.abs(pr - 192), Math.abs(pg - 192), Math.abs(pb - 192)) >
            3 ||
          Math.max(Math.abs(cr - 192), Math.abs(cg - 192), Math.abs(cb - 192)) >
            3;
        if (!foreground) continue;
        const py = 0.2126 * pr + 0.7152 * pg + 0.0722 * pb;
        const cy = 0.2126 * cr + 0.7152 * cg + 0.0722 * cb;
        priorLuma[pixel] = py;
        currentLuma[pixel] = cy;
        priorSum += py;
        currentSum += cy;
        priorSquare += py * py;
        currentSquare += cy * cy;
        covariance += py * cy;
        lumaError += (py - cy) ** 2;
        const pcb = -0.168736 * pr - 0.331264 * pg + 0.5 * pb;
        const pcr = 0.5 * pr - 0.418688 * pg - 0.081312 * pb;
        const ccb = -0.168736 * cr - 0.331264 * cg + 0.5 * cb;
        const ccr = 0.5 * cr - 0.418688 * cg - 0.081312 * cb;
        chromaError += (pcb - ccb) ** 2 + (pcr - ccr) ** 2;
        count++;
      }
      const priorMean = priorSum / count;
      const currentMean = currentSum / count;
      const priorVariance = priorSquare / count - priorMean ** 2;
      const currentVariance = currentSquare / count - currentMean ** 2;
      const centeredCovariance = covariance / count - priorMean * currentMean;
      const c1 = (0.01 * 255) ** 2;
      const c2 = (0.03 * 255) ** 2;
      const ssim =
        ((2 * priorMean * currentMean + c1) * (2 * centeredCovariance + c2)) /
        ((priorMean ** 2 + currentMean ** 2 + c1) *
          (priorVariance + currentVariance + c2));

      const priorEdges = [];
      const currentEdges = [];
      for (let y = 1; y < height - 1; y++) {
        for (let x = 1; x < width - 1; x++) {
          const index = y * width + x;
          if (!priorLuma[index] && !currentLuma[index]) continue;
          const priorX = priorLuma[index + 1] - priorLuma[index - 1];
          const priorY = priorLuma[index + width] - priorLuma[index - width];
          const currentX = currentLuma[index + 1] - currentLuma[index - 1];
          const currentY =
            currentLuma[index + width] - currentLuma[index - width];
          priorEdges.push(Math.hypot(priorX, priorY));
          currentEdges.push(Math.hypot(currentX, currentY));
        }
      }
      const edgeMean = (values) =>
        values.reduce((sum, value) => sum + value, 0) / values.length;
      const priorEdgeMean = edgeMean(priorEdges);
      const currentEdgeMean = edgeMean(currentEdges);
      let edgeCovariance = 0;
      let priorEdgeVariance = 0;
      let currentEdgeVariance = 0;
      for (let i = 0; i < priorEdges.length; i++) {
        const p = priorEdges[i] - priorEdgeMean;
        const c = currentEdges[i] - currentEdgeMean;
        edgeCovariance += p * c;
        priorEdgeVariance += p * p;
        currentEdgeVariance += c * c;
      }
      const edgeCorrelation =
        edgeCovariance /
        Math.sqrt(Math.max(1e-9, priorEdgeVariance * currentEdgeVariance));

      return {
        state,
        comparison: {
          previous: previous.state,
          samplePixels: count,
          luminanceDrift: currentMean - priorMean,
          luminanceRMSE: Math.sqrt(lumaError / count),
          chromaRMSE: Math.sqrt(chromaError / (count * 2)),
          ssim,
          edgeCorrelation,
        },
      };
    });

  const frames = [];
  const screenshots = [];
  let previousSignature = "";
  const recordFrame = async () => {
    const frame = await capture();
    if (!frame) return;
    frame.atMs = performance.now();
    frames.push(frame);
    const signature = `${frame.state.targetLOD}-${frame.state.activeLOD}`;
    if (signature !== previousSignature) {
      const filename = `${label}-${String(screenshots.length).padStart(2, "0")}-target${frame.state.targetLOD}-active${frame.state.activeLOD}.png`;
      await page.screenshot({ path: path.join(outputDir, filename) });
      screenshots.push(filename);
      previousSignature = signature;
    }
  };

  await recordFrame();
  for (let step = 0; step < 32; step++) {
    await page.mouse.wheel(0, 200);
    for (let frame = 0; frame < 4; frame++) {
      await page.waitForTimeout(25);
      await recordFrame();
    }
    const state = frames.at(-1)?.state;
    if (state?.zoom <= 0.126 && state.targetLOD === 8 && state.activeLOD === 8)
      break;
  }

  const comparisons = frames.map((frame) => frame.comparison).filter(Boolean);
  const absoluteDrifts = comparisons.map((value) =>
    Math.abs(value.luminanceDrift)
  );
  const result = {
    label,
    baseURL,
    sampleSize: { width: 320, height: 180 },
    frames: frames.length,
    zoomRange: {
      from: frames[0]?.state.zoom,
      to: frames.at(-1)?.state.zoom,
    },
    summary: {
      maxAbsoluteLuminanceDrift: Math.max(0, ...absoluteDrifts),
      p95AbsoluteLuminanceDrift: percentile(absoluteDrifts, 0.95),
      p95LuminanceRMSE: percentile(
        comparisons.map((value) => value.luminanceRMSE),
        0.95
      ),
      p95ChromaRMSE: percentile(
        comparisons.map((value) => value.chromaRMSE),
        0.95
      ),
      minimumSSIM: Math.min(1, ...comparisons.map((value) => value.ssim)),
      minimumEdgeCorrelation: Math.min(
        1,
        ...comparisons.map((value) => value.edgeCorrelation)
      ),
      fallbackFrames: frames.filter(
        (frame) => frame.state.activeLOD !== frame.state.targetLOD
      ).length,
      globalFallbackFrames: frames.filter(
        (frame) => frame.state.targetLOD > 8 && frame.state.activeLOD === 8
      ).length,
      nonAdjacentFallbackFrames: frames.filter((frame) => {
        const order = [64, 32, 16, 12, 8];
        return (
          Math.abs(
            order.indexOf(frame.state.targetLOD) -
              order.indexOf(frame.state.activeLOD)
          ) > 1
        );
      }).length,
    },
    transitions: frames
      .filter(
        (frame, index) =>
          index === 0 ||
          frame.state.targetLOD !== frames[index - 1].state.targetLOD ||
          frame.state.activeLOD !== frames[index - 1].state.activeLOD
      )
      .map((frame, index) => ({
        index,
        ...frame.state,
        comparison: frame.comparison,
      })),
    screenshots,
    samples: frames,
  };
  const output = path.join(outputDir, `${label}.json`);
  await fs.writeFile(output, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify({ ...result, samples: undefined }, null, 2));
  console.log(`\nWrote ${output}`);
  if (args.assert) {
    const summary = result.summary;
    if (summary.nonAdjacentFallbackFrames !== 0)
      throw new Error("zoom used a non-adjacent fallback LOD");
    if (summary.maxAbsoluteLuminanceDrift > 5)
      throw new Error("zoom exceeded 5 levels of luminance drift");
    if (summary.p95AbsoluteLuminanceDrift > 1)
      throw new Error("zoom p95 luminance drift exceeded 1 level");
    if (summary.minimumSSIM < 0.65)
      throw new Error("zoom scale-normalized SSIM fell below 0.65");
  }
} finally {
  await browser.close();
}
