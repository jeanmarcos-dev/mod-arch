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
  /** Core that produced the event; lets narrators route per core. */
  coreId: number;
  /** Monotonic id used as Vue key in animations. */
  seq: number;
}

export interface MemRead {
  addr: number;
  value: number;
  size: number;
  pid: number;
  coreId: number;
  seq: number;
}

export interface ModeSwitch {
  from: 'user' | 'kernel';
  to: 'user' | 'kernel';
  cause: string;
  coreId: number;
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
  coreId: number;
  seq: number;
}

// PCB copy events. pcb_save flies registers CPU → PCB row; pcb_restore is the
// reverse. We carry the full register snapshot so the narrator can show the
// most relevant ones (EIP, ESP, EAX) on the flying card.
export interface PcbSave {
  pid: number;
  /** Core whose register file is being copied out. */
  coreId: number;
  regs: Record<string, number>;
  seq: number;
}

export interface PcbRestore {
  pid: number;
  /** Core whose register file is being loaded. */
  coreId: number;
  regs: Record<string, number>;
  seq: number;
}

type AnyHandler = (...args: unknown[]) => void;

export interface NarratorOptions {
  /**
   * When true, the bus listens regardless of the preferences flag. Use only
   * for diagnostics — production narrators must respect the user toggle.
   */
  force?: boolean;
}

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
    pcbSave: new Set(),
    pcbRestore: new Set(),
  };

  let seq = 0;

  function isEnabled(): boolean {
    return opts.force === true || prefs.narratorsEnabled;
  }

  function fire(channel: keyof typeof handlers, payload: unknown) {
    if (handlers[channel].size === 0) return;
    if (!isEnabled()) return;
    // Narrators receive every event; rate limiting belongs to each narrator
    // visual layer (e.g. ByteFlight caps in-flight animations rather than
    // dropping events). This avoids syscalls that emit a quick burst (8
    // mem_reads back-to-back in sys_write) being silently swallowed.
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
          pid: cpu.cores.find(c => c.id === ev.core_id)?.current_pid ?? cpu.currentPID,
          coreId: ev.core_id,
          seq,
        } satisfies MemWrite);
        break;
      case 'mem_read':
        seq++;
        fire('memRead', {
          addr: ev.addr,
          value: ev.value,
          size: ev.size,
          pid: cpu.cores.find(c => c.id === ev.core_id)?.current_pid ?? cpu.currentPID,
          coreId: ev.core_id,
          seq,
        } satisfies MemRead);
        break;
      case 'mode_switch':
        seq++;
        fire('modeSwitch', {
          from: ev.from === 3 ? 'user' : 'kernel',
          to: ev.to === 3 ? 'user' : 'kernel',
          cause: ev.cause,
          coreId: ev.core_id,
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
          coreId: ev.core_id,
          seq,
        } satisfies IrqFire);
        break;
      case 'pcb_save':
        seq++;
        fire('pcbSave', {
          pid: ev.pid,
          coreId: ev.core_id,
          regs: ev.regs,
          seq,
        } satisfies PcbSave);
        break;
      case 'pcb_restore':
        seq++;
        fire('pcbRestore', {
          pid: ev.pid,
          coreId: ev.core_id,
          regs: ev.regs,
          seq,
        } satisfies PcbRestore);
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
    onPcbSave(fn: (m: PcbSave) => void) { handlers.pcbSave.add(fn as AnyHandler); },
    onPcbRestore(fn: (m: PcbRestore) => void) { handlers.pcbRestore.add(fn as AnyHandler); },
  };
}
