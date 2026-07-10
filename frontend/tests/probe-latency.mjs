// Gameplay latency + payload probe: joins as a real player, subscribes to the
// spawn region and minimap, reveals a cell, and reports per-message latencies
// and payload volumes. Usage:
//   REPO=$(git rev-parse --show-toplevel) [WSURL=ws://localhost:8080/ws] node tests/probe-latency.mjs
import { WebSocket } from "ws";
import zlib from "zlib";
import protobuf from "protobufjs";
import path from "path";
const root = protobuf.loadSync(path.join(process.env.REPO, "proto/messages.proto"));
const Msg = root.lookupType("ms.Msg");
const ChunkRegion = root.lookupType("ms.ChunkRegion");
const enc = (p) => zlib.gzipSync(Msg.encode(Msg.create(p)).finish());
const dec = (b) => Msg.toObject(Msg.decode(zlib.gunzipSync(b)), { longs: String, defaults: false });
const toBuf = (x) => (Buffer.isBuffer(x) ? x : Buffer.from(x, "base64"));
const maybeGunzip = (b) => { try { return zlib.gunzipSync(b); } catch { return b; } };

const t0 = performance.now();
const mark = (label) => console.log(`${label}: ${(performance.now() - t0).toFixed(0)}ms`);
const ws = new WebSocket(process.env.WSURL || "wss://infiniteminesweeper.com/ws");
let spawn = null, revealSent = 0, bytesIn = 0, msgs = 0, cells = 0, flags = 0, tiles = 0, chunkSyncs = 0;
setTimeout(() => { console.log("== summary (10s) =="); console.log(`msgs=${msgs} bytesIn=${(bytesIn/1024).toFixed(0)}KB chunkSyncs=${chunkSyncs} revealedCells=${cells} flags=${flags} minimapTiles=${tiles}`); process.exit(0); }, 10000);

function countSync(sync) {
  chunkSyncs++;
  if (sync.reveals) {
    const bm = maybeGunzip(toBuf(sync.reveals));
    let c = 0;
    for (const b of bm) c += popcount(b);
    cells += c;
  }
  for (const g of sync.flagGroups || []) flags += (g.cells?.cells || []).length;
}
const popcount = (b) => { let c = 0; while (b) { c += b & 1; b >>= 1; } return c; };

ws.on("open", () => { mark("ws open"); ws.send(enc({ join: { name: "LatProbe" + Math.floor(Math.random()*1e6), flagID: 0 } })); });
ws.on("message", (data) => {
  bytesIn += data.byteLength || data.length; msgs++;
  const obj = dec(Buffer.from(data));
  if (obj.joinAck) mark("joinAck");
  if (obj.spawnHint && !spawn) {
    spawn = obj.spawnHint; mark("spawnHint");
    const X = Number(spawn.chunkId?.X ?? 0), Y = Number(spawn.chunkId?.Y ?? 0);
    for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) ws.send(enc({ subscribe: { chunkId: { X: X + dx, Y: Y + dy } } }));
    ws.send(enc({ minimapSubscribe: { tiles: Array.from({length: 25}, (_, i) => ({ x: X - 2 + (i % 5), y: Y - 2 + Math.floor(i / 5) })), resolution: 64 } }));
    ws.send(enc({ viewUpdate: { chunkId: { X, Y }, cell: 2080, widthCells: 120, heightCells: 70 } }));
  }
  if (obj.chunkSync) { if (chunkSyncs === 0) mark("first chunkSync"); countSync(obj.chunkSync); }
  if (obj.chunkRegionSync) {
    if (chunkSyncs === 0) mark("first chunkRegionSync");
    const region = ChunkRegion.toObject(ChunkRegion.decode(toBuf(obj.chunkRegionSync.chunks)), { longs: String });
    for (const c of region.chunks || []) countSync(c);
  }
  if (obj.minimapFullTile) { if (tiles === 0) mark("first minimapTile"); tiles++; }
  if (obj.minimapFullTileBatch) {
    if (tiles === 0 && obj.minimapFullTileBatch.tiles?.length) mark("first minimapTile");
    tiles += obj.minimapFullTileBatch.tiles?.length || 0;
  }
  if (obj.revealAck) mark("revealAck RTT");
  if (spawn && !revealSent && chunkSyncs > 0) {
    revealSent = 1;
    const X = Number(spawn.chunkId?.X ?? 0), Y = Number(spawn.chunkId?.Y ?? 0);
    ws.send(enc({ reveal: { requestId: 424242, chunkId: { X, Y }, cell: Number(spawn.cell ?? 2080) } }));
    mark("reveal sent");
  }
});
ws.on("error", (e) => { console.log("WS error:", e.message); process.exit(1); });
