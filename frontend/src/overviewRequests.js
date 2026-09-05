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
      const { requestId, ...pending } = this.pending;
      this.queued =
        JSON.stringify(pending) === JSON.stringify(request) ? null : request;
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
