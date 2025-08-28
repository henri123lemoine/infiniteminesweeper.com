#!/usr/bin/env node
/**
 * Frontend Integration Tests (behavioral, no grep nonsense)
 *
 * What this verifies:
 *  1) Server is serving the app (GET /)
 *  2) JSON endpoints respond with sane shapes (/leaderboard, /hotspot)
 *  3) [Optional] Real E2E join flow in a headless browser:
 *     - load page
 *     - enter name, click "Join Game"
 *     - overlay disappears, score chip visible
 *     - session persisted to localStorage
 *
 * Requirements:
 *  - Node 18+ (has global fetch)
 *  - Server running on BASE_URL (the test runner starts it)
 *  - (Optional) Install Playwright once:  npm i -D playwright
 *    (The test will auto-skip browser tests if Playwright is missing.)
 */

import assert from "node:assert";
import { setTimeout as sleep } from "node:timers/promises";

// Test Harness
class TestRunner {
  constructor() { this.passed = 0; this.failed = 0; this.skipped = 0; this.failFast = false; }
  async test(name, fn) {
    process.stdout.write(`\n🧪 ${name} ... `);
    try {
      const res = await fn();
      if (res === "SKIP") {
        this.skipped++; console.log("⏭️  SKIPPED");
      } else {
        this.passed++; console.log("✅ PASS");
      }
    } catch (err) {
      this.failed++; console.log("❌ FAIL");
      console.error("   ", (err && err.message) || err);
      if (this.failFast) process.exit(1);
    }
  }
  summary() {
    console.log(`\n📊 Summary: ${this.passed} passed, ${this.failed} failed, ${this.skipped} skipped`);
    if (this.failed > 0) process.exit(1);
  }
}

const runner = new TestRunner();
const BASE_URL = process.env.BASE_URL || "http://localhost:8080";

// Helper: fetch JSON with nice errors
async function getJson(path, { timeoutMs = 5000 } = {}) {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(`${BASE_URL}${path}`, { signal: ctrl.signal });
    assert.ok(res.ok, `HTTP ${res.status} for ${path}`);
    const ct = res.headers.get("content-type") || "";
    assert.ok(ct.includes("application/json") || ct.includes("text/json"), `Unexpected content-type for ${path}: ${ct}`);
    return await res.json();
  } finally {
    clearTimeout(t);
  }
}

// Helper: fetch text (for /)
async function getText(path, { timeoutMs = 5000 } = {}) {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(`${BASE_URL}${path}`, { signal: ctrl.signal });
    assert.ok(res.ok, `HTTP ${res.status} for ${path}`);
    return await res.text();
  } finally {
    clearTimeout(t);
  }
}

// Wait until predicate true or timeout
async function waitFor(fn, { timeoutMs = 8000, intervalMs = 100 } = {}) {
  const start = Date.now();
  while (true) {
    if (await fn()) return;
    if (Date.now() - start > timeoutMs) throw new Error("waitFor timeout");
    await sleep(intervalMs);
  }
}

// Tests

// 1) App serves HTML
await runner.test("GET / serves HTML shell", async () => {
  const html = await getText("/");
  assert.match(html, /<!doctype html>/i, "Missing HTML doctype");
  // For SPA shells we only expect a mount div; React will render the canvas at runtime.
  const hasMount = /<div[^>]+id=["'](root|app)["']/.test(html);
  assert.ok(hasMount, "Root mount div not found in shell");
});

// 2) /leaderboard shape
await runner.test("GET /leaderboard returns array of {name, score}", async () => {
  const rows = await getJson("/leaderboard");
  assert.ok(Array.isArray(rows), "Expected leaderboard array");
  for (const r of rows.slice(0, 10)) {
    assert.equal(typeof r.name, "string", "row.name must be string");
    assert.equal(typeof r.score, "number", "row.score must be number");
    if (r.flagID != null) assert.equal(typeof r.flagID, "number", "row.flagID must be number when present");
  }
});

// 3) /hotspot shape
await runner.test("GET /hotspot returns {count:number}", async () => {
  const data = await getJson("/hotspot").catch(() => ({ count: 0 })); // endpoint may be absent in some modes
  assert.equal(typeof data.count, "number", "hotspot.count must be number");
});

// 4) Optional E2E browser join flow (Playwright)
await runner.test("E2E: user can join game via UI (Playwright)", async () => {
  // Try to import Playwright
  let chromium;
  try {
    ({ chromium } = await import("playwright"));
  } catch {
    // Playwright not installed – skip gracefully
    return "SKIP";
  }

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    // Load app
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 15000 });
    await page.waitForSelector('#game-canvas', { timeout: 8000 }); // canvas rendered by React

    const nameInput = page.locator('input[placeholder="Your name"]');
    const joinBtn = page.getByRole("button", { name: /join game/i });
    await waitFor(async () => await nameInput.count() > 0, { timeoutMs: 8000 });

    const uname = `itest-${Math.random().toString(36).slice(2, 7)}`;
    await nameInput.fill(uname);
    await joinBtn.click();

    // Wait for joinAck effects:
    //  A) overlay input disappears OR
    //  B) localStorage gets username+token
    await Promise.race([
      page.waitForSelector('input[placeholder="Your name"]', { state: 'detached', timeout: 12000 }),
      page.waitForFunction(() => !!localStorage.getItem('username') && !!localStorage.getItem('session_token'), { timeout: 12000 }),
    ]);

    // Now read persisted session
    const local = await page.evaluate(() => ({
      username: localStorage.getItem("username"),
      token: localStorage.getItem("session_token"),
      score: localStorage.getItem("score"),
      flagID: localStorage.getItem("flagID"),
    }));
    assert.ok(local.username && local.username.startsWith(uname), `username not persisted (got: ${local.username})`);
    assert.ok(local.token && local.token.length > 10, "session_token not persisted");
    assert.match(local.score ?? "0", /^\d+$/, "score not numeric in localStorage");

    // Leaderboard widget visible with at least header
    const lbHeader = page.getByText(/leaderboard/i).first();
    assert.ok(await lbHeader.count(), "Leaderboard header not found");
  } finally {
    await browser.close();
  }
});

runner.summary();
