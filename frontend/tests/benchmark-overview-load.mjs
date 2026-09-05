import { WebSocket } from "ws";
import zlib from "node:zlib";
import protobuf from "protobufjs";
import { setTimeout as delay } from "node:timers/promises";
const Msg = protobuf
  .loadSync(new URL("../../proto/messages.proto", import.meta.url).pathname)
  .lookupType("ms.Msg");
const url = process.env.WSURL || "ws://localhost:18093/ws";
if (!["localhost", "127.0.0.1"].includes(new URL(url).hostname))
  throw new Error("load test requires a local server");
const n = Number(process.env.CLIENTS || 100);
const rounds = Number(process.env.ROUNDS || 3);
const encode = (message) => zlib.gzipSync(Msg.encode(message).finish());
const decode = (data) => Msg.decode(zlib.gunzipSync(data));
const connect = () =>
  new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    ws.once("open", () => resolve(ws));
    ws.once("error", reject);
  });
const clients = await Promise.all(Array.from({ length: n }, connect));
const probe = await connect();
const pings = [];
let pingStart = 0,
  probing = true;
probe.on("message", (data) => {
  if (decode(data).seedResponse && pingStart) {
    pings.push(performance.now() - pingStart);
    pingStart = 0;
  }
});
const pingLoop = (async () => {
  while (probing) {
    if (!pingStart) {
      pingStart = performance.now();
      probe.send(encode({ seedRequest: { chunkIds: [{ X: 0, Y: 0 }] } }));
    }
    await delay(50);
  }
})();
const heap = [];
let sampling = true;
const sampler = (async () => {
  while (sampling) {
    const started = performance.now();
    const text = await (
      await fetch(process.env.METRICS_URL || "http://localhost:19094/metrics")
    ).text();
    heap.push({
      delay: performance.now() - started,
      bytes: Number(text.match(/^go_memstats_alloc_bytes (\d+)/m)?.[1]),
    });
    await delay(50);
  }
})();
const latencies = [];
let completed = 0;
const start = performance.now();
try {
  await Promise.all(
    clients.map(
      (ws, i) =>
        new Promise((resolve, reject) => {
          let round = 0,
            sentAt = 0;
          const send = () => {
            sentAt = performance.now();
            ws.send(
              encode({
                overviewRequest: {
                  lod: 8,
                  originX: -128 + (i % 4),
                  originY: -128,
                  widthChunks: 256,
                  heightChunks: 256,
                  subscribe: true,
                  replaceSubscription: true,
                  requestId: round + 1,
                },
              })
            );
          };
          const timer = setTimeout(
            () => reject(new Error("overview load timed out")),
            45000
          );
          ws.on("close", () => {
            if (round < rounds)
              reject(new Error("client closed before completing"));
          });
          ws.on("message", (data) => {
            if (!decode(data).overviewSnapshot) return;
            latencies.push(performance.now() - sentAt);
            completed++;
            if (++round < rounds) send();
            else {
              clearTimeout(timer);
              resolve();
            }
          });
          send();
        })
    )
  );
} finally {
  sampling = false;
  probing = false;
  await Promise.all([pingLoop, sampler]);
  for (const ws of clients) ws.close();
  probe.close();
}
const percentile = (values, p) =>
  [...values].sort((a, b) => a - b)[
    Math.min(values.length - 1, Math.floor(values.length * p))
  ];
console.log(
  JSON.stringify(
    {
      clients: n,
      completed,
      elapsedMs: performance.now() - start,
      peakSampledHeapMiB: Math.max(...heap.map((x) => x.bytes)) / 2 ** 20,
      overviewMs: {
        p50: percentile(latencies, 0.5),
        p95: percentile(latencies, 0.95),
      },
      probeMs: {
        count: pings.length,
        p50: percentile(pings, 0.5),
        p95: percentile(pings, 0.95),
        max: Math.max(...pings),
      },
      metricsMaxDelayMs: Math.max(...heap.map((x) => x.delay)),
    },
    null,
    2
  )
);
