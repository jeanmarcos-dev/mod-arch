import { onBeforeUnmount } from 'vue';
import { useCpuStore, type CpuEvent } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';

// Narrator events are the semantic, pedagogic projection of the raw WS
// stream. The store emits the underlying CpuEvent; narratorBus normalizes
// it and hands subscribers a tighter payload so the narrator components do
// not have to know about the wire format.

export interface MemWrite {
  addr: number;
  old: number;
  newValue: number;
  size: number;
  /** Process id active when the write happened. */
  pid: number;
  /** Monotonic id used as Vue key in animations. */
  seq: number;
}

export interface MemRead {
  addr: number;
  value: number;
  size: number;
  pid: number;
  seq: number;
}

export interface ModeSwitch {
  from: 'user' | 'kernel';
  to: 'user' | 'kernel';
  cause: string;
  seq: number;
}

export interface CtxSwitch {
  fromPid: number;
  toPid: number;
  seq: number;
}

export interface IrqFire {
  vector: number;
  source: string;
  seq: number;
}

type AnyHandler = (...args: unknown[]) => void;

export interface NarratorOptions {
  /**
   * Maximum events per second to forward. Lower values keep heavy workloads
   * (sieve, dining_philosophers) visually digestible. 0 disables throttling.
   */
  maxHz?: number;
  /**
   * When true, the bus listens regardless of the preferences flag. Use only
   * for diagnostics — production narrators must respect the user toggle.
   */
  force?: boolean;
}

const DEFAULT_MAX_HZ = 30;

/**
 * useNarrators subscribes to the cpu store for the lifetime of the calling
 * component and exposes per-event registration helpers. Subscriptions are
 * cleaned up automatically on unmount.
 */
export function useNarrators(opts: NarratorOptions = {}) {
  const cpu = useCpuStore();
  const prefs = usePreferencesStore();

  const handlers: Record<string, Set<AnyHandler>> = {
    memWrite: new Set(),
    memRead: new Set(),
    modeSwitch: new Set(),
    ctxSwitch: new Set(),
    irqFire: new Set(),
  };

  let seq = 0;
  const maxHz = opts.maxHz ?? DEFAULT_MAX_HZ;
  const minIntervalMs = maxHz > 0 ? 1000 / maxHz : 0;
  let lastTickMs = 0;

  function shouldEmit(): boolean {
    if (!opts.force && !prefs.narratorsEnabled) return false;
    if (minIntervalMs <= 0) return true;
    const now = performance.now();
    if (now - lastTickMs < minIntervalMs) return false;
    lastTickMs = now;
    return true;
  }

  function fire(channel: keyof typeof handlers, payload: unknown) {
    if (handlers[channel].size === 0) return;
    if (!shouldEmit()) return;
    for (const fn of handlers[channel]) {
      try { fn(payload); } catch { /* narrators must never crash the bus */ }
    }
  }

  const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
    switch (ev.type) {
      case 'mem_write':
        seq++;
        fire('memWrite', {
          addr: ev.addr,
          old: ev.old,
          newValue: ev.new,
          size: ev.size,
          pid: cpu.currentPID,
          seq,
        } satisfies MemWrite);
        break;
      case 'mem_read':
        seq++;
        fire('memRead', {
          addr: ev.addr,
          value: ev.value,
          size: ev.size,
          pid: cpu.currentPID,
          seq,
        } satisfies MemRead);
        break;
      case 'mode_switch':
        seq++;
        fire('modeSwitch', {
          from: ev.from === 3 ? 'user' : 'kernel',
          to: ev.to === 3 ? 'user' : 'kernel',
          cause: ev.cause,
          seq,
        } satisfies ModeSwitch);
        break;
      case 'sched_decision':
        seq++;
        fire('ctxSwitch', {
          fromPid: ev.from_pid,
          toPid: ev.to_pid,
          seq,
        } satisfies CtxSwitch);
        break;
      case 'irq_fire':
        seq++;
        fire('irqFire', {
          vector: ev.vector,
          source: ev.source,
          seq,
        } satisfies IrqFire);
        break;
    }
  });

  onBeforeUnmount(unsubscribe);

  return {
    onMemWrite(fn: (m: MemWrite) => void) { handlers.memWrite.add(fn as AnyHandler); },
    onMemRead(fn: (m: MemRead) => void) { handlers.memRead.add(fn as AnyHandler); },
    onModeSwitch(fn: (m: ModeSwitch) => void) { handlers.modeSwitch.add(fn as AnyHandler); },
    onCtxSwitch(fn: (m: CtxSwitch) => void) { handlers.ctxSwitch.add(fn as AnyHandler); },
    onIrqFire(fn: (m: IrqFire) => void) { handlers.irqFire.add(fn as AnyHandler); },
  };
}
