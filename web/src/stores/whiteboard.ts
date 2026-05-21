import { defineStore } from 'pinia';
import { ref } from 'vue';

// PointerState mirrors the broadcast payload the server forwards from the
// owner to every viewer. ts is the millisecond timestamp the server stamped
// at emit; we use it as the staleness watchdog.
export interface PointerState {
  x: number;
  y: number;
  route: string;
  ts: number;
}

const STORAGE_KEY = 'modos-cpu:whiteboard:v1';
// If we stop receiving owner_pointer events for this long, fade out the
// cursor — the owner most likely disconnected without sending pointer_off.
const STALE_MS = 2000;

interface Persisted {
  enabled?: boolean;
}

function loadEnabled(): boolean {
  if (typeof window === 'undefined') return false;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return false;
    const parsed = JSON.parse(raw) as Persisted;
    return Boolean(parsed.enabled);
  } catch {
    return false;
  }
}

function persistEnabled(value: boolean) {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ enabled: value }));
  } catch { /* quota / private mode */ }
}

// useWhiteboardStore owns the per-tab state for the owner's "whiteboard" mode:
// whether the local owner is broadcasting their cursor and, on the viewer
// side, the latest received cursor position. The actual WS plumbing (sending
// pointer commands, dispatching owner_pointer events) lives in the AppShell
// and the cpu store respectively — this store is the small reactive seam
// they share.
export const useWhiteboardStore = defineStore('whiteboard', () => {
  // Owner-side: did the user toggle "Pizarra" on? Persisted across reloads so
  // a teacher who keeps the same browser session does not have to re-arm it
  // every class.
  const broadcasting = ref<boolean>(loadEnabled());

  // Viewer-side: latest pointer from the owner (null when off / never seen).
  const ownerPointer = ref<PointerState | null>(null);

  let staleTimer: number | undefined;

  function armStaleTimer() {
    if (staleTimer !== undefined) window.clearTimeout(staleTimer);
    staleTimer = window.setTimeout(() => {
      ownerPointer.value = null;
      staleTimer = undefined;
    }, STALE_MS);
  }

  function setBroadcasting(v: boolean) {
    broadcasting.value = v;
    persistEnabled(v);
  }

  function setOwnerPointer(ev: { x?: number; y?: number; route?: string; ts?: number; off?: boolean }) {
    if (ev.off) {
      ownerPointer.value = null;
      if (staleTimer !== undefined) {
        window.clearTimeout(staleTimer);
        staleTimer = undefined;
      }
      return;
    }
    if (typeof ev.x !== 'number' || typeof ev.y !== 'number' || typeof ev.route !== 'string') {
      return;
    }
    ownerPointer.value = {
      x: ev.x,
      y: ev.y,
      route: ev.route,
      ts: typeof ev.ts === 'number' ? ev.ts : Date.now(),
    };
    armStaleTimer();
  }

  function reset() {
    ownerPointer.value = null;
    if (staleTimer !== undefined) {
      window.clearTimeout(staleTimer);
      staleTimer = undefined;
    }
  }

  return {
    broadcasting,
    ownerPointer,
    setBroadcasting,
    setOwnerPointer,
    reset,
  };
});
