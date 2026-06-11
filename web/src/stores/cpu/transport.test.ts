import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope } from 'vue';
import { useCpuTransport } from './transport';

// Fake WebSocket that records how many instances were constructed. The race we
// guard against opens a second socket that silently delivers every frame into
// the same store, so "how many sockets exist" is the exact thing to assert.
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  readyState = 0;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }
  send() {}
  close() {}
}

function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('useCpuTransport — connect() concurrency guard', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // The real bug: connect() awaits twice (bootstrapSession + onBeforeConnect)
  // before it assigns `ws`. Two synchronous callers both clear the post-await
  // `ws` guard while parked on the second await, then both build a socket. This
  // reproduces that interleaving with a controllable onBeforeConnect.
  it('opens a single socket when two callers race through onBeforeConnect', async () => {
    let releaseBeforeConnect = () => {};
    const beforeConnect = new Promise<void>((res) => {
      releaseBeforeConnect = res;
    });
    const scope = effectScope();
    const done = scope.run(async () => {
      const transport = useCpuTransport({
        onFrame: () => {},
        onLog: () => {},
        onBeforeConnect: () => beforeConnect,
      });
      const a = transport.connect();
      const b = transport.connect();
      // Let both callers advance past bootstrapSession and park on the
      // onBeforeConnect await, mirroring two onMounted hooks in one turn.
      await flush();
      releaseBeforeConnect();
      await Promise.all([a, b]);
    });
    await done;
    expect(FakeWebSocket.instances).toHaveLength(1);
    scope.stop();
  });

  it('does not open a second socket once one is already connecting', async () => {
    const scope = effectScope();
    await scope.run(async () => {
      const transport = useCpuTransport({ onFrame: () => {}, onLog: () => {} });
      await transport.connect();
      // The freshly built socket is still in CONNECTING; a later call must noop.
      await transport.connect();
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
    scope.stop();
  });
});
