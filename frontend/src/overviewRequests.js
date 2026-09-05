import { overviewRegionKey } from "./overviewCache.js";

export class OverviewRequests {
  constructor(send, onTimeout, timeoutMs = 10000) {
    this.send = send;
    this.onTimeout = onTimeout;
    this.timeoutMs = timeoutMs;
    this.nextId = 0;
    this.pending = null;
    this.queued = null;
    this.timer = null;
  }

  request(request) {
    if (this.pending) {
      if (
        !request.subscribe &&
        (this.pending.subscribe || this.queued?.subscribe)
      )
        return true;
      const pending = this.pending;
      if (
        !request.subscribe &&
        pending.lod === request.lod &&
        (pending.global ||
          (!request.global &&
            pending.originX <= request.originX &&
            pending.originY <= request.originY &&
            pending.originX + pending.widthChunks >=
              request.originX + request.widthChunks &&
            pending.originY + pending.heightChunks >=
              request.originY + request.heightChunks))
      ) {
        this.queued = null;
        return true;
      }
      const same =
        overviewRegionKey(this.pending) === overviewRegionKey(request) &&
        Boolean(this.pending.subscribe) === Boolean(request.subscribe) &&
        Boolean(this.pending.replaceSubscription) ===
          Boolean(request.replaceSubscription);
      this.queued = same ? null : request;
      return true;
    }
    const next = { ...request, requestId: ++this.nextId };
    if (!this.send(next)) return false;
    this.pending = next;
    this.timer = setTimeout(() => {
      this.reset();
      this.onTimeout();
    }, this.timeoutMs);
    return true;
  }

  complete(requestId) {
    if (!this.pending || this.pending.requestId !== requestId) return;
    clearTimeout(this.timer);
    this.timer = null;
    this.pending = null;
    const next = this.queued;
    this.queued = null;
    if (next) this.request(next);
  }

  release() {
    this.queued = null;
    if (this.pending) this.pending.subscribe = false;
  }

  reset() {
    clearTimeout(this.timer);
    this.timer = null;
    this.pending = null;
    this.queued = null;
  }
}
