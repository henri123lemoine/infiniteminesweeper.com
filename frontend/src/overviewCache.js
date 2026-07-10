const DEFAULT_BUDGET = 64 * 1024 * 1024;

const regionKey = ({
  lod,
  global,
  originX,
  originY,
  widthChunks,
  heightChunks,
}) =>
  global
    ? `${lod}:global`
    : `${lod}:${originX},${originY}:${widthChunks}x${heightChunks}`;

export class OverviewCache {
  constructor(budgetBytes = DEFAULT_BUDGET) {
    this.budgetBytes = budgetBytes;
    this.records = new Map();
    this.bytes = 0;
  }

  put(record) {
    record.key = regionKey(record);
    record.lastUsed = performance.now();
    record.byteCost = record.canvasByteLength;
    record.pinned = record.global;
    const previous = this.records.get(record.key);
    if (previous) this.bytes -= previous.byteCost;
    this.records.set(record.key, record);
    this.bytes += record.byteCost;
    this.evict();
    return record;
  }

  findExact(query) {
    const record = this.records.get(regionKey(query));
    if (record) record.lastUsed = performance.now();
    return record || null;
  }

  findForView(lod, left, top, right, bottom) {
    let best = null;
    for (const record of this.records.values()) {
      if (record.lod !== lod) continue;
      const recordLeft = record.originX * 64;
      const recordTop = record.originY * 64;
      const recordRight = recordLeft + record.widthChunks * 64;
      const recordBottom = recordTop + record.heightChunks * 64;
      if (
        !record.global &&
        (recordLeft > left ||
          recordTop > top ||
          recordRight < right ||
          recordBottom < bottom)
      ) {
        continue;
      }
      if (!best || record.global || record.lastUsed > best.lastUsed)
        best = record;
      if (record.global) break;
    }
    if (best) best.lastUsed = performance.now();
    return best;
  }

  recordsAtLOD(lod) {
    return Array.from(this.records.values()).filter(
      (record) => record.lod === lod
    );
  }

  evict() {
    if (this.bytes <= this.budgetBytes) return;
    const candidates = Array.from(this.records.values())
      .filter((record) => !record.pinned)
      .sort((a, b) => a.lastUsed - b.lastUsed);
    for (const record of candidates) {
      if (this.bytes <= this.budgetBytes) break;
      this.records.delete(record.key);
      this.bytes -= record.byteCost;
    }
  }

  stats() {
    const byLOD = {};
    for (const record of this.records.values()) {
      byLOD[record.lod] = (byLOD[record.lod] || 0) + 1;
    }
    return {
      records: this.records.size,
      bytes: this.bytes,
      budgetBytes: this.budgetBytes,
      byLOD,
    };
  }
}

export const overviewRegionKey = regionKey;
