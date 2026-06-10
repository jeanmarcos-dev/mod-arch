import { triggerRef } from 'vue';
import { usePreferencesStore } from '../preferences';
import type { CpuEvent, SessionInfo, Presence, VarMeta, VarSnapshot, LogEntry } from './types';
import { formatLog } from '../../i18n/formatLog';

// Using `any` for now to quickly find all missing variables using TS compiler
export function useCpuReducer(state: any) {
  const {
    activeFrameByCore,
    armGlossaryStaleTimer,
    changedRegs,
    changedRegsByCore,
    clearGlossaryStaleTimer,
    clockHz,
    consoleChunks,
    cores,
    currentPID,
    ensureCoreSlot,
    faultShake,
    flashPID,
    flashReg,
    halted,
    handRaiseSignal,
    isSelected,
    kbdBufCap,
    kbdBufUsed,
    kbdReaders,
    lastError,
    lastFiredHandlerByVector,
    lastHandRaiseLabel,
    lastIrq21At,
    lastMnemonic,
    lastPresence,
    log,
    markCoreActive,
    mode,
    mutexes,
    numCores,
    ownerGlossaryHover,
    paused,
    pendingMemoryJump,
    pipes,
    procs,
    procSourceMaps,
    procSources,
    procVarMeta,
    procVars,
    programs,
    readyQueue,
    reconcileFilterWithCores,
    regs,
    running,
    segments,
    selectedCoreId,
    selectedProgram,
    semaphores,
    sessionInfo,
    speedMul,
    tick,
    timerQuantum,
    waitQueue,
  } = state;

  type Subscriber = (ev: CpuEvent) => void;
  const subscribers = new Set<Subscriber>();
  function subscribe(fn: Subscriber): () => void {
    subscribers.add(fn);
    return () => subscribers.delete(fn);
  }

  function applyEvent(ev: CpuEvent) {
    // The tutor store listens via a window CustomEvent so cpu.ts does not
    // grow a hard dependency on it. Any payload typed as 'tutor_answer' is
    // forwarded verbatim; the tutor handler decides what to do with it.
    if ((ev as { type?: string }).type === 'tutor_answer') {
      window.dispatchEvent(new CustomEvent('modos:tutor-answer', { detail: ev }));
      return;
    }
    if ((ev as { type?: string }).type === 'tutor_progress') {
      window.dispatchEvent(new CustomEvent('modos:tutor-progress', { detail: ev }));
      return;
    }
    switch (ev.type) {
      case 'snapshot':
        if (ev.cores) cores.value = ev.cores;
        if (ev.num_cores !== undefined) numCores.value = ev.num_cores;
        // Drop filter ids that no longer exist (after a SetNumCores shrink);
        // repopulate with all available ones if the filter would otherwise
        // be empty.
        reconcileFilterWithCores(cores.value.map((c: any) => c.id));
        // Mirror the selected core's view into the top-level refs so the
        // panels that read cpu.regs/mode/etc keep working. Falls back to
        // the payload's boot-core copy if cores[] is missing for some
        // reason (legacy snapshot format).
        const sel = cores.value.find((x: any) => x.id === selectedCoreId.value);
        if (sel) {
          for (const k of Object.keys(regs)) regs[k] = sel.regs[k] ?? 0;
          for (const k of Object.keys(segments)) segments[k] = sel.segments[k] ?? 0;
          mode.value = sel.mode;
          halted.value = sel.halted;
          currentPID.value = sel.current_pid;
        } else {
          for (const k of Object.keys(regs)) regs[k] = ev.regs[k] ?? 0;
          for (const k of Object.keys(segments)) segments[k] = ev.segments[k] ?? 0;
          mode.value = ev.mode;
          halted.value = ev.halted;
          if (ev.current_pid !== undefined) currentPID.value = ev.current_pid;
        }
        tick.value = ev.tick;
        if (ev.timer_quantum !== undefined) timerQuantum.value = ev.timer_quantum;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.paused !== undefined) paused.value = ev.paused;
        if (ev.clockHz !== undefined) clockHz.value = ev.clockHz;
        if (ev.speedMul !== undefined) speedMul.value = ev.speedMul;
        if (ev.ready_queue) readyQueue.value = ev.ready_queue;
        if (ev.wait_queue) waitQueue.value = ev.wait_queue;
        if (ev.procs) procs.value = ev.procs;
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        if (ev.source_maps) {
          procSourceMaps.value = {};
          for (const [pid, lines] of Object.entries(ev.source_maps)) {
            procSourceMaps.value[Number(pid)] = lines;
          }
        }
        if (ev.vars) {
          const next: Record<number, VarSnapshot> = {};
          for (const [pid, snap] of Object.entries(ev.vars)) {
            next[Number(pid)] = snap;
          }
          procVars.value = next;
        } else if (ev.vars === null) {
          procVars.value = {};
        }
        changedRegs.clear();
        for (const k of Object.keys(changedRegsByCore)) delete changedRegsByCore[Number(k)];
        break;
      case 'programs':
        programs.value = ev.list;
        if (!selectedProgram.value && ev.list[0]) selectedProgram.value = ev.list[0].name;
        break;
      case 'reg_write': {
        const c = ensureCoreSlot(ev.core_id);
        c.regs[ev.reg] = ev.new;
        markCoreActive(c, ev.core_id);
        // Stamp the per-core flash unconditionally — every visible core lights
        // up its own register strip on a write, not only the focused one.
        flashReg(ev.reg, ev.core_id);
        if (isSelected(ev.core_id)) {
          regs[ev.reg] = ev.new;
        }
        break;
      }
      case 'instruction': {
        const c = ensureCoreSlot(ev.core_id);
        const nextEIP = ev.eip + ev.bytes.length;
        c.regs.EIP = nextEIP;
        markCoreActive(c, ev.core_id);
        lastMnemonic[ev.core_id] = ev.mnemonic;
        // EventsLog renders the per-core chip from coreId so the text itself
        // stays clean and identical across single- and multi-core sessions.
        pushLog(formatLog.instruction(ev.eip, ev.mnemonic), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) regs.EIP = nextEIP;
        break;
      }
      case 'tick':
        tick.value = ev.value;
        // Optimistically bump idle_ticks for every core currently bound to
        // the swapper so the UI sees a live heartbeat without waiting for
        // the next full snapshot. Snapshots from the server overwrite the
        // value when they arrive, keeping the local counter in sync.
        for (const c of cores.value) {
          if (c.running_idle) c.idle_ticks = (c.idle_ticks ?? 0) + 1;
        }
        break;
      case 'mode_switch': {
        const c = ensureCoreSlot(ev.core_id);
        c.mode = ev.to;
        pushLog(formatLog.modeSwitch(ev.from, ev.to, ev.cause, ev.kind), 'red', ev.core_id, ev.tick);
        const lastMS = log.value[log.value.length - 1];
        if (lastMS) lastMS.inKernel = false;
        if (isSelected(ev.core_id)) mode.value = ev.to;
        if (ev.new_cs  !== undefined) c.segments.CS = ev.new_cs;
        if (ev.new_ds  !== undefined) c.segments.DS = ev.new_ds;
        if (ev.new_ss  !== undefined) c.segments.SS = ev.new_ss;
        if (ev.new_esp !== undefined) c.regs.ESP    = ev.new_esp;
        if (isSelected(ev.core_id)) {
          if (ev.new_cs  !== undefined) segments.CS = ev.new_cs;
          if (ev.new_ds  !== undefined) segments.DS = ev.new_ds;
          if (ev.new_ss  !== undefined) segments.SS = ev.new_ss;
          if (ev.new_esp !== undefined) regs.ESP    = ev.new_esp;
        }
        delete lastMnemonic[ev.core_id];
        // Defer the frame-slot unmount so the InterruptNarrator can still
        // read DOM coordinates for the IRET mini-arrows before Vue flushes.
        if (ev.cause === 'IRET') {
          const coreId = ev.core_id;
          queueMicrotask(() => {
            activeFrameByCore.value = { ...activeFrameByCore.value, [coreId]: null };
          });
        }
        break;
      }
      case 'irq_fire': {
        pushLog(formatLog.irq(ev.vector, ev.source), 'yellow', ev.core_id, ev.tick);
        // K = kernel code; HW IRQ delivery is not, strip it.
        const lastIRQ = log.value[log.value.length - 1];
        if (lastIRQ) lastIRQ.inKernel = false;
        // Stamp the wall-clock for IRQ 0x21 (keyboard) so the Console drawer
        // can flash its badge when fires happen while collapsed.
        if (ev.vector === 0x21) lastIrq21At.value = Date.now();
        lastFiredHandlerByVector.value = {
          ...lastFiredHandlerByVector.value,
          [ev.vector]: {
            coreId: ev.core_id,
            handler: ev.handler ?? 0,
            source: ev.source,
            ts: Date.now(),
          },
        };
        // Only user→kernel traps push a frame. Kernel→kernel IRQs (timer
        // arriving while already in kernel mode) leave saved_* undefined.
        if (ev.from_mode === 3 && ev.saved_eip !== undefined) {
          activeFrameByCore.value = {
            ...activeFrameByCore.value,
            [ev.core_id]: {
              vector: ev.vector,
              handler: ev.handler ?? 0,
              savedEIP: ev.saved_eip,
              savedCS: ev.saved_cs ?? 0,
              savedEFLAGS: ev.saved_eflags ?? 0,
              fromMode: ev.from_mode,
              capturedAt: Date.now(),
            },
          };
        }
        break;
      }
      case 'page_fault': {
        pushLog(formatLog.pageFault(ev.addr, ev.perm_needed, ev.mode), 'red', ev.core_id, ev.tick);
        faultShake.value = true;
        setTimeout(() => { faultShake.value = false; }, 500);
        break;
      }
      case 'halt': {
        const c = ensureCoreSlot(ev.core_id);
        c.halted = true;
        const pid = (ev as { pid?: number }).pid ?? -1;
        // PID 0 is the swapper parking on HLT between IRQs — non-fatal, cyan.
        // Any other PID is the terminal cascade (process death) — red.
        const isIdle = pid === 0;
        pushLog(formatLog.halt(pid), isIdle ? 'cyan' : 'red', ev.core_id, ev.tick);
        // Only the cascade halt freezes the UI as "machine halted". Swapper
        // HLT keeps the run loop alive — the next IRQ wakes it back up.
        if (!isIdle && isSelected(ev.core_id)) halted.value = true;
        break;
      }
      case 'state':
        if (ev.halted !== undefined) halted.value = ev.halted;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.paused !== undefined) paused.value = ev.paused;
        if (ev.clockHz !== undefined) clockHz.value = ev.clockHz;
        if (ev.speedMul !== undefined) speedMul.value = ev.speedMul;
        break;
      case 'loaded': {
        pushLog(formatLog.loaded(ev.name, ev.pids.length, ev.pids), 'green', undefined, undefined, 'global');
        procs.value = ev.procs;
        // Pick a sensible default for the "Mostrar init" toggle based on how
        // many user-space procs the demo carries (init excluded). Single-proc
        // demos like ADD start with init hidden; fork/threads demos reveal it.
        const prefs = usePreferencesStore();
        const userProcs = ev.procs.filter(p => p.name !== 'init').length;
        prefs.applyShowInitDefault(userProcs);
        // A new demo starts with no open IPC channels — the kernel
        // resets its Pipes registry alongside its process table.
        pipes.value = [];
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        if (ev.source_maps) {
          procSourceMaps.value = {};
          for (const [pid, lines] of Object.entries(ev.source_maps)) {
            procSourceMaps.value[Number(pid)] = lines;
          }
        } else {
          procSourceMaps.value = {};
        }
        if (ev.var_meta) {
          const meta: Record<number, VarMeta> = {};
          for (const [pid, m] of Object.entries(ev.var_meta)) {
            meta[Number(pid)] = m;
          }
          procVarMeta.value = meta;
        } else {
          procVarMeta.value = {};
        }
        // A new demo always starts with no resolved values; the next snapshot
        // event will fill procVars in once the kernel ticks.
        procVars.value = {};
        break;
      }
      case 'vars_update':
        // Legacy path: kept so older sim builds that still send a standalone
        // vars_update frame keep working. The atomic delivery path is via
        // tick_events.vars (see the WS onmessage handler).
        applyVarsPayload(ev.vars);
        break;
      case 'reset':
        changedRegs.clear();
        for (const k of Object.keys(changedRegsByCore)) delete changedRegsByCore[Number(k)];
        procs.value = [];
        readyQueue.value = [];
        waitQueue.value = [];
        currentPID.value = 0;
        consoleChunks.value = [];
        kbdBufUsed.value = 0;
        kbdReaders.value = [];
        procSources.value = {};
        procSourceMaps.value = {};
        procVarMeta.value = {};
        procVars.value = {};
        mutexes.value = [];
        semaphores.value = [];
        pipes.value = [];
        activeFrameByCore.value = {};
        lastFiredHandlerByVector.value = {};
        pendingMemoryJump.value = null;
        for (const k of Object.keys(lastMnemonic)) delete lastMnemonic[Number(k)];
        // Reset per-core register/state caches so the next snapshot starts
        // from a clean slate. Without this, stale values from the previous
        // demo would survive a reset on cores that the new demo does not
        // touch (smaller process count than core count).
        for (const c of cores.value) {
          for (const k of Object.keys(c.regs)) c.regs[k] = 0;
          for (const k of Object.keys(c.segments)) c.segments[k] = 0;
          c.mode = 3;
          c.halted = false;
          c.tick = 0;
          c.current_pid = 0;
        }
        for (const k of Object.keys(regs)) regs[k] = 0;
        for (const k of Object.keys(segments)) segments[k] = 0;
        mode.value = 3;
        halted.value = false;
        paused.value = false;
        break;
      case 'procs':
        procs.value = ev.procs;
        break;
      case 'pcb_save': {
        const p = procs.value.find((x: any) => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(formatLog.pcbSave(ev.pid, ev.regs.EIP || 0), 'red', ev.core_id, ev.tick, undefined, 'scheduler', 'pcbSave');
        break;
      }
      case 'pcb_restore': {
        const p = procs.value.find((x: any) => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        // Inside a handler (c.mode === 0) the ring-dependent regs live in
        // the patched trap frame and become live only at the IRET
        // mode_switch — never overwrite them here. Ring-independent GP
        // regs are safe either way.
        const c = ensureCoreSlot(ev.core_id);
        const ringIndependent = ['EAX', 'EBX', 'ECX', 'EDX', 'ESI', 'EDI', 'EBP', 'EFLAGS'] as const;
        for (const k of ringIndependent) {
          if (k in ev.regs) c.regs[k] = ev.regs[k];
        }
        if (c.mode !== 0) {
          if ('EIP' in ev.regs) c.regs.EIP = ev.regs.EIP;
          if ('ESP' in ev.regs) c.regs.ESP = ev.regs.ESP;
          for (const k of ['CS', 'DS', 'SS'] as const) {
            if (k in ev.regs) c.segments[k] = ev.regs[k];
          }
        }
        if (isSelected(ev.core_id)) {
          for (const k of ringIndependent) {
            if (k in ev.regs) regs[k] = ev.regs[k];
          }
          if (c.mode !== 0) {
            if ('EIP' in ev.regs) regs.EIP = ev.regs.EIP;
            if ('ESP' in ev.regs) regs.ESP = ev.regs.ESP;
            for (const k of ['CS', 'DS', 'SS'] as const) {
              if (k in ev.regs) segments[k] = ev.regs[k];
            }
          }
        }
        flashPID(ev.pid);
        pushLog(formatLog.pcbRestore(ev.pid, ev.regs.EIP || 0), 'green', ev.core_id, ev.tick, undefined, 'scheduler', 'pcbRestore');
        break;
      }
      case 'sched_decision': {
        // currentPID top-level is owned by core_assign/core_release so the
        // SimBar mirrors the *selected* core only. Updating it here would
        // pull the PID of whichever core just scheduled, regardless of
        // which one the user is watching.
        // ready_queue arrives as null when the kernel's queue is empty;
        // normalize so consumers see an empty array.
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        pushLog(formatLog.schedule(ev.from_pid, ev.to_pid, ev.reason, rq), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'wake': {
        // BLOCKED → READY transition. Sync the ready queue so the haltedDead
        // heuristic in SimBar (halted && readyQueue.empty) does not stay
        // stuck after a child sys_exit wakes a parent. The state change
        // itself travels in the accompanying EventProcessState; this event
        // exists so wakes are not conflated with scheduling decisions.
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        pushLog(formatLog.wake(ev.pid, ev.from_reason, rq), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'init_reap': {
        // Kernel-emulated reap cycle of PID 1: init does not occupy a core,
        // it drains every PendingReap and goes back to wait(any) without
        // executing user code. The event carries one ReapDetail per zombie
        // collected (with Reparented telling apart genuine orphans from
        // birth-adopted children of init).
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        for (const r of ev.reaps) {
          // Remove the reaped child from the local procs mirror; the
          // backend already dropped it from k.Processes.
          const idx = procs.value.findIndex((x: any) => x.pid === r.child_pid);
          if (idx >= 0) procs.value.splice(idx, 1);
        }
        pushLog(formatLog.initReap(ev.init_pid, ev.reaps), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'process_state': {
        const p = procs.value.find((x: any) => x.pid === ev.pid);
        if (p) {
          p.state = ev.state;
          if (ev.reason) p.blocked_on = ev.reason;
          // The kernel attaches exit_code on the ZOMBIE transition so the
          // panel does not need to wait for the next full procs snapshot.
          if (ev.exit_code !== undefined) p.exit_code = ev.exit_code;
          // Stamp the local tick when entering BLOCKED so the Concurrency view
          // can render "esperando hace N ticks" without waiting for the next
          // snapshot. Clear on any other transition to avoid stale residue.
          if (ev.state === 'BLOCKED') {
            p.blocked_since = tick.value;
          } else {
            p.blocked_since = undefined;
          }
        }
        // Mantener wait_queue sincronizada con BLOCKED ⇄ otros estados.
        if (ev.state === 'BLOCKED') {
          if (!waitQueue.value.find((w: any) => w.pid === ev.pid)) {
            waitQueue.value.push({
              pid: ev.pid,
              name: p?.name ?? `P${ev.pid}`,
              reason: ev.reason ?? '',
              blocked_since: tick.value,
            });
          }
        } else {
          waitQueue.value = waitQueue.value.filter((w: any) => w.pid !== ev.pid);
        }
        break;
      }
      case 'syscall':
        if (ev.error) {
          pushLog(formatLog.syscallError(ev.name, ev.args, ev.error, ev.pid), 'red', ev.core_id, ev.tick);
        } else {
          pushLog(formatLog.syscall(ev.name, ev.args, ev.result, ev.pid), 'green', ev.core_id, ev.tick);
        }
        break;
      case 'console_out':
        consoleChunks.value.push({
          pid: ev.pid,
          coreId: ev.core_id,
          kind: (ev.kind as 'in' | 'out') ?? (ev.pid === 0 ? 'in' : 'out'),
          text: ev.text,
          bytes: ev.bytes,
          ts: Date.now(),
        });
        if (consoleChunks.value.length > 200) consoleChunks.value.splice(0, consoleChunks.value.length - 200);
        break;
      case 'kbd_state':
        kbdBufUsed.value = ev.buf_used ?? 0;
        kbdBufCap.value = ev.buf_cap ?? 16;
        kbdReaders.value = Array.isArray(ev.readers) ? ev.readers : [];
        break;
      case 'session_info':
        sessionInfo.value = {
          id: ev.id,
          mode: ev.mode as SessionInfo['mode'],
          role: ev.role as SessionInfo['role'],
          label: ev.label,
          view_token: ev.view_token,
          user_id: ev.user_id ?? '',
          user_name: ev.user_name ?? '',
          user_email: ev.user_email ?? '',
        };
        if (ev.presence && ev.presence.section) {
          lastPresence.value = {
            section: ev.presence.section,
            cores: ev.presence.cores ?? [],
            mode: (ev.presence.mode as Presence['mode']) ?? '',
            narrators: Boolean(ev.presence.narrators),
            focus: ev.presence.focus ?? {},
          };
        }
        if (ev.role === 'viewer') {
          pushLog(formatLog.spectatorMode(ev.label || ev.id.slice(0, 6)), 'cyan', undefined, undefined, 'global');
        }
        break;
      case 'presence':
        lastPresence.value = {
          section: ev.section,
          cores: ev.cores ?? [],
          mode: (ev.mode as Presence['mode']) ?? '',
          narrators: Boolean(ev.narrators),
          focus: ev.focus ?? {},
        };
        break;
      case 'hand_raise':
        lastHandRaiseLabel.value = ev.label;
        handRaiseSignal.value++;
        break;
      case 'owner_glossary_hover':
        if (ev.off) {
          ownerGlossaryHover.value = null;
          clearGlossaryStaleTimer();
        } else if (typeof ev.term === 'string' && ev.term.length > 0) {
          ownerGlossaryHover.value = ev.term;
          armGlossaryStaleTimer();
        }
        break;
      case 'error':
        lastError.value = ev.message;
        if (ev.code) {
          pushLog(formatLog.backendError(ev.code, ev.params, ev.message), 'red', undefined, undefined, 'global');
        } else {
          pushLog(formatLog.genericErr(ev.message), 'red', undefined, undefined, 'global');
        }
        break;
      case 'mem_read':
      case 'mem_write': {
        // Memory traffic is consumed exclusively by subscribers (narrators,
        // memory panel filters, etc.) — the store carries no aggregated
        // counter for it. We only use it as a heartbeat to clear a stale
        // HALT flag when an IRQ has actually woken the core back up.
        const c = cores.value.find((x: any) => x.id === ev.core_id);
        if (c) markCoreActive(c, ev.core_id);
        break;
      }
      case 'core_assign': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = ev.pid;
        // Any non-zero PID assignment unbinds the swapper from this core.
        // PID 0 also lands here when the kernel uses EventCoreAssign for
        // the idle case; the dedicated core_idle event sets the flag back.
        if (ev.pid !== 0) c.running_idle = false;
        markCoreActive(c, ev.core_id);
        pushLog(formatLog.coreAssign(ev.core_id, ev.pid), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) currentPID.value = ev.pid;
        break;
      }
      case 'core_release': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = 0;
        c.running_idle = false;
        pushLog(formatLog.coreRelease(ev.core_id, ev.pid, ev.reason), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) currentPID.value = 0;
        break;
      }
      case 'core_idle': {
        const c = ensureCoreSlot(ev.core_id);
        c.running_idle = true;
        break;
      }
      case 'mutex_op': {
        let m = mutexes.value.find((x: any) => x.id === ev.mutex_id);
        if (!m) {
          m = { id: ev.mutex_id, holder: -1, waiters: [] };
          mutexes.value.push(m);
        }
        switch (ev.op) {
          case 'create':
            // Object already added above; nothing else to track.
            pushLog(formatLog.mutexCreated(ev.mutex_id, ev.pid), 'green', ev.core_id, ev.tick);
            break;
          case 'acquired':
            m.holder = ev.pid;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexAcquired(ev.mutex_id, ev.pid), 'green', ev.core_id, ev.tick);
            break;
          case 'waiting':
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexWaiting(ev.mutex_id, ev.pid), 'yellow', ev.core_id, ev.tick);
            break;
          case 'released':
            m.holder = -1;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexReleased(ev.mutex_id, ev.pid), 'cyan', ev.core_id, ev.tick);
            break;
          case 'handoff':
            m.holder = ev.next_pid ?? -1;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexHandoff(ev.mutex_id, ev.pid, ev.next_pid ?? -1), 'green', ev.core_id, ev.tick);
            break;
          case 'rejected':
            pushLog(formatLog.mutexRejected(ev.mutex_id, ev.pid), 'red', ev.core_id, ev.tick);
            break;
        }
        break;
      }
      case 'pipe_create': {
        // Avoid duplicates if the server replays the snapshot after a
        // reconnect — the kernel guarantees pipe_id is monotonic per
        // session, so an existing entry just refreshes its metadata.
        const existing = pipes.value.find((p: any) => p.pipe_id === ev.pipe_id);
        if (existing) {
          existing.owner_pid = ev.owner_pid;
          existing.fd_r = ev.fd_r;
          existing.fd_w = ev.fd_w;
          existing.is_fifo = ev.is_fifo;
          existing.name = ev.name;
        } else {
          pipes.value.push({
            pipe_id: ev.pipe_id, owner_pid: ev.owner_pid,
            fd_r: ev.fd_r, fd_w: ev.fd_w,
            is_fifo: ev.is_fifo, name: ev.name,
            buf_level: 0, buf_cap: 16,
          });
        }
        break;
      }
      case 'pipe_op': {
        const p = pipes.value.find((x: any) => x.pipe_id === ev.pipe_id);
        if (p) {
          p.buf_level = ev.buf_level;
          p.buf_cap = ev.buf_cap;
          p.last_bytes = ev.bytes;
          p.last_op = ev.op as 'read' | 'write';
          p.last_pid = ev.pid;
        }
        break;
      }
      case 'fork': {
        // Tree shape is rebuilt from the next 'procs' snapshot; here we
        // just flash both PCBs so the user notices the relationship.
        flashPID(ev.parent_pid);
        flashPID(ev.child_pid);
        pushLog(formatLog.forkSpawn(ev.parent_pid, ev.child_pid, ev.is_vfork), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'reparent': {
        // Same as fork: the periodic procs snapshot carries the new
        // parent_pid. We flash the affected PCB so the move is visible.
        flashPID(ev.pid);
        // Update parent_pid locally so the tree re-renders without
        // waiting for the next procs snapshot.
        const moved = procs.value.find((x: any) => x.pid === ev.pid);
        if (moved) moved.parent_pid = ev.new_parent;
        pushLog(formatLog.reparent(ev.pid, ev.old_parent, ev.new_parent), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'thread_create': {
        // A sibling thread was spawned via clone(CLONE_THREAD). Flash both
        // creator and new task so the user sees them tied to the same
        // address space; the next 'procs' snapshot fills in tgid/tid for
        // the panel to fold the new TID under its leader.
        flashPID(ev.creator_pid);
        flashPID(ev.tid);
        pushLog(formatLog.threadCreate(ev.tid, ev.tgid, ev.creator_pid), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'thread_exit': {
        flashPID(ev.tid);
        pushLog(formatLog.threadExit(ev.tid, ev.tgid, ev.retval), 'yellow', ev.core_id, ev.tick);
        break;
      }
      case 'wait_op': {
        if (ev.op === 'reap') {
          // Drop the reaped zombie from the local snapshot so the panel
          // empties at the same tick the kernel emits the event. Without
          // this the zombie sticks around until the next procs broadcast
          // (which never comes when AllHalted fires immediately after).
          procs.value = procs.value.filter((x: any) => x.pid !== ev.child_pid);
          // Drop the orphan from the parent's children list too.
          const parent = procs.value.find((x: any) => x.pid === ev.pid);
          if (parent && Array.isArray(parent.children)) {
            parent.children = parent.children.filter((c: any) => c !== ev.child_pid);
          }
          flashPID(ev.pid);
          // Init reaps no longer travel as EventWaitOp — they fire as the
          // consolidated EventInitReap, formatted by formatLog.initReap.
          // So a wait_op event reaching here is always a regular parent
          // collecting through sys_waitpid.
          pushLog(formatLog.waitReap(ev.pid, ev.child_pid, ev.exit_code ?? 0), 'yellow', ev.core_id, ev.tick);
        } else if (ev.op === 'call') {
          pushLog(formatLog.waitCall(ev.pid, ev.child_pid), 'cyan', ev.core_id, ev.tick);
        }
        break;
      }
      case 'sem_op': {
        let s = semaphores.value.find((x: any) => x.id === ev.sem_id);
        if (!s) {
          s = { id: ev.sem_id, count: ev.count, waiters: [] };
          semaphores.value.push(s);
        }
        s.count = ev.count;
        s.waiters = ev.waiters ?? [];
        switch (ev.op) {
          case 'create':
            pushLog(formatLog.semCreated(ev.sem_id, ev.count), 'green', ev.core_id, ev.tick);
            break;
          case 'wait_immediate':
            pushLog(formatLog.semWait(ev.sem_id, ev.pid, ev.count), 'cyan', ev.core_id, ev.tick);
            break;
          case 'wait_blocked':
            pushLog(formatLog.semBlocked(ev.sem_id, ev.pid), 'yellow', ev.core_id, ev.tick);
            break;
          case 'signal':
            pushLog(formatLog.semSignal(ev.sem_id, ev.pid, ev.count), 'green', ev.core_id, ev.tick);
            break;
          case 'handoff':
            pushLog(formatLog.semHandoff(ev.sem_id, ev.pid, ev.next_pid ?? -1), 'green', ev.core_id, ev.tick);
            break;
        }
        break;
      }
    }
    // Fan-out to subscribers AFTER the store has reacted, so they can read
    // the up-to-date snapshot in their handler.
    if (subscribers.size > 0) {
      for (const fn of subscribers) {
        try { fn(ev); } catch (err) { pushLog(formatLog.subscriberErr(err), 'red'); }
      }
    }
  }

  function pushLog(
    text: string,
    color: string,
    coreId?: number,
    tick?: number,
    scope?: 'global',
    kind?: 'scheduler',
    glyph?: LogEntry['glyph'],
  ) {
    // Derive inKernel from the live per-core mode tracked in cores.value.
    // mode_switch events update that value before subsequent pushLog calls
    // (the WS handler dispatches events in arrival order), so a kernel-mode
    // instruction that arrives right after mode_switch{to:0} is correctly
    // tagged. Mode 0 = PL0 (kernel), 3 = PL3 (user).
    let inKernel: boolean | undefined;
    if (coreId !== undefined) {
      const c = cores.value.find((x: any) => x.id === coreId);
      if (c) inKernel = c.mode === 0;
    }
    log.value.push({ ts: Date.now(), text, color, coreId, tick, scope, inKernel, kind, glyph });
    if (log.value.length > 500) log.value.splice(0, log.value.length - 500);
    triggerRef(log);
  }

  function applyVarsPayload(vars: Record<string, VarSnapshot> | null | undefined) {
    if (!vars) {
      procVars.value = {};
      return;
    }
    const next: Record<number, VarSnapshot> = {};
    for (const [pid, snap] of Object.entries(vars)) {
      next[Number(pid)] = snap;
    }
    procVars.value = next;
  }

  return { applyEvent, applyVarsPayload, subscribe, pushLog };
}
