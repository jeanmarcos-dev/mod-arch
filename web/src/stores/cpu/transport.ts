import { ref } from 'vue';
import { formatLog } from '../../i18n/formatLog';

export interface TransportOptions {
  onFrame: (msg: any) => void;
  onLog: (text: string, color: string, scope?: string) => void;
  onBeforeConnect?: () => Promise<void>;
}

export function useCpuTransport(options: TransportOptions) {
  let ws: WebSocket | null = null;
  const connected = ref(false);
  const hasEverConnected = ref(false);
  
  const BACKOFF_MS = [1000, 2000, 5000, 10000, 30000];
  const reconnectAttempts = ref(0);
  const nextRetryAt = ref<number | null>(null);
  let retryTimer: number | null = null;

  function clearRetryTimer() {
    if (retryTimer !== null) {
      window.clearTimeout(retryTimer);
      retryTimer = null;
    }
    nextRetryAt.value = null;
  }

  function scheduleReconnect() {
    if (retryTimer !== null) return;
    const idx = Math.min(reconnectAttempts.value, BACKOFF_MS.length - 1);
    const delay = BACKOFF_MS[idx];
    nextRetryAt.value = Date.now() + delay;
    retryTimer = window.setTimeout(() => {
      retryTimer = null;
      nextRetryAt.value = null;
      reconnectAttempts.value += 1;
      connect();
    }, delay);
  }

  function reconnectNow() {
    clearRetryTimer();
    reconnectAttempts.value += 1;
    connect();
  }

  let sessionBootstrapped = false;
  async function bootstrapSession(): Promise<void> {
    if (sessionBootstrapped) return;
    try {
      const r = await fetch('/api/session', { credentials: 'same-origin' });
      if (r.ok) sessionBootstrapped = true;
    } catch {
      // swallow: WS open will fail and trigger normal reconnect backoff
    }
  }

  const pendingFrames: unknown[] = [];
  let rafHandle: number | null = null;
  const PENDING_CAP = 20;

  const SYNC_TYPES = new Set<string>([
    'snapshot',
    'loaded',
    'programs',
    'reset',
    'session_info',
    'presence',
    'tutor_answer',
    'tutor_progress',
    'error',
    'state',
  ]);

  function flushPendingFrames() {
    if (pendingFrames.length === 0) return;
    const batch = pendingFrames.splice(0, pendingFrames.length);
    for (const msg of batch) options.onFrame(msg);
  }

  function scheduleFlush() {
    if (rafHandle !== null) return;
    rafHandle = requestAnimationFrame(() => {
      rafHandle = null;
      flushPendingFrames();
    });
  }

  function routeIncomingFrame(msg: any) {
    const type = typeof msg?.type === 'string' ? msg.type : undefined;
    if (type && SYNC_TYPES.has(type)) {
      flushPendingFrames();
      options.onFrame(msg);
      return;
    }
    pendingFrames.push(msg);
    if (pendingFrames.length > PENDING_CAP) {
      pendingFrames.splice(0, pendingFrames.length - PENDING_CAP);
    }
    scheduleFlush();
  }

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible' && pendingFrames.length > 0) {
        if (rafHandle !== null) {
          cancelAnimationFrame(rafHandle);
          rafHandle = null;
        }
        flushPendingFrames();
      }
    });
  }

  async function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    await bootstrapSession();
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    if (options.onBeforeConnect) {
      await options.onBeforeConnect();
    }
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws`;
    ws = new WebSocket(url);
    ws.onopen = () => {
      connected.value = true;
      hasEverConnected.value = true;
      reconnectAttempts.value = 0;
      clearRetryTimer();
    };
    ws.onclose = () => {
      connected.value = false;
      ws = null;
      if (rafHandle !== null) {
        cancelAnimationFrame(rafHandle);
        rafHandle = null;
      }
      pendingFrames.length = 0;
      options.onLog(formatLog.wsDisconnected(), 'red', 'global');
      scheduleReconnect();
    };
    ws.onerror = () => {
      options.onLog(formatLog.wsError(), 'red', 'global');
    };
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        routeIncomingFrame(msg);
      } catch (err) {
        options.onLog(formatLog.wsJsonErr(err), 'red', 'global');
      }
    };
  }

  function send(cmd: object) {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(cmd));
    }
  }

  return {
    connected,
    hasEverConnected,
    reconnectAttempts,
    nextRetryAt,
    connect,
    reconnectNow,
    send,
  };
}
