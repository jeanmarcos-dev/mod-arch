import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';

// Tipos de eventos que llegan por WS (espejo de internal/cpu/events.go).
// Todos los eventos generados por la CPU llevan core_id: con multicore el
// store mantiene un CoreInfo por core y sólo "espejea" al estado top-level
// (regs, mode, halted, currentPID) cuando el evento viene del core que el
// usuario eligió ver en la SimBar.
export type CpuEvent =
  | { type: 'instruction'; core_id: number; pid: number; eip: number; mnemonic: string; bytes: number[] }
  | { type: 'reg_write'; core_id: number; reg: string; old: number; new: number }
  | { type: 'mem_read'; core_id: number; addr: number; value: number; size: number }
  | { type: 'mem_write'; core_id: number; addr: number; old: number; new: number; size: number }
  | { type: 'mode_switch'; core_id: number; from: number; to: number; cause: string }
  | { type: 'irq_fire'; core_id: number; vector: number; source: string }
  | { type: 'page_fault'; core_id: number; addr: number; mode: number; perm_needed: string }
  | { type: 'halt'; core_id: number; pid?: number }
  | { type: 'tick'; value: number }
  | { type: 'state'; halted?: boolean; running?: boolean; hz?: number }
  | { type: 'snapshot'; regs: Record<string, number>; segments: Record<string, number>; mode: number; halted: boolean; tick: number; timer_quantum?: number; current_pid?: number; ready_queue?: number[]; wait_queue?: WaitInfo[]; procs?: PCBInfo[]; sources?: Record<string, string>; cores?: CoreInfo[]; num_cores?: number }
  | { type: 'programs'; list: ProgramInfo[] }
  | { type: 'loaded'; name: string; pids: number[]; procs: PCBInfo[]; sources: Record<string, string> }
  | { type: 'reset' }
  | { type: 'procs'; procs: PCBInfo[] }
  | { type: 'pcb_save'; pid: number; regs: Record<string, number> }
  | { type: 'pcb_restore'; pid: number; regs: Record<string, number> }
  | { type: 'sched_decision'; from_pid: number; to_pid: number; ready_queue: number[] }
  | { type: 'process_state'; pid: number; state: string; reason?: string }
  | { type: 'syscall'; pid: number; num: number; name: string; args: number[]; result: number }
  | { type: 'console_out'; pid: number; bytes: number[]; text: string }
  | { type: 'core_assign'; core_id: number; pid: number }
  | { type: 'core_release'; core_id: number; pid: number; reason?: string }
  | { type: 'mutex_op'; core_id: number; pid: number; mutex_id: number; op: 'create' | 'acquired' | 'waiting' | 'released' | 'handoff'; next_pid?: number; waiters?: number[] }
  | { type: 'sem_op'; core_id: number; pid: number; sem_id: number; op: 'create' | 'wait_immediate' | 'wait_blocked' | 'signal' | 'handoff'; count: number; next_pid?: number; waiters?: number[] }
  | { type: 'session_info'; id: string; mode: string; role: string; label: string; view_token: string; user_id?: string; user_name?: string; user_email?: string }
  | { type: 'error'; message: string };

export interface CoreInfo {
  id: number;
  regs: Record<string, number>;
  segments: Record<string, number>;
  mode: number;
  halted: boolean;
  tick: number;
  current_pid: number;
}

export interface MutexState {
  id: number;
  holder: number;
  waiters: number[];
}

export interface SemaphoreState {
  id: number;
  count: number;
  waiters: number[];
}

export interface SessionInfo {
  id: string;
  mode: 'full' | 'privileged' | 'unknown';
  role: 'owner' | 'viewer' | 'none';
  label: string;
  view_token: string;
  user_id: string;
  user_name: string;
  user_email: string;
}

export interface ProgramInfo {
  name: string;
  description: string;
  category?: string;
  size: number;
  procs?: string[];
}

export interface PCBInfo {
  pid: number;
  name: string;
  state: string;
  blocked_on?: string;
  regs: Record<string, number>;
  saved_user_esp: number;
  code_base: number;
  code_size: number;
  stack_top: number;
}

export interface WaitInfo {
  pid: number;
  name: string;
  reason: string;
}

export interface LogEntry {
  ts: number;
  text: string;
  color: string;
}

export const REGISTERS = [
  'EAX', 'EBX', 'ECX', 'EDX',
  'ESI', 'EDI', 'ESP', 'EBP',
  'EIP', 'EFLAGS',
] as const;

export const useCpuStore = defineStore('cpu', () => {
  // ─── Estado del CPU ─────────────────────────────────────────────────────
  const regs = reactive<Record<string, number>>(Object.fromEntries(REGISTERS.map(r => [r, 0])));
  const segments = reactive<Record<string, number>>({ CS: 0, DS: 0, SS: 0 });
  const mode = ref(3); // arranca en user
  const halted = ref(false);
  const running = ref(false);
  const tick = ref(0);
  const stepHz = ref(4);
  const timerQuantum = ref(8);

  // Registros que cambiaron en el último evento, para animar flash.
  const changedRegs = reactive<Set<string>>(new Set());

  // ─── Programas y log ────────────────────────────────────────────────────
  const programs = ref<ProgramInfo[]>([]);
  const selectedProgram = ref<string | null>(null);
  const log = ref<LogEntry[]>([]);
  const connected = ref(false);
  const lastError = ref<string | null>(null);

  // ─── Procesos ───────────────────────────────────────────────────────────
  const procs = ref<PCBInfo[]>([]);
  const currentPID = ref(0);
  const readyQueue = ref<number[]>([]);
  const waitQueue = ref<WaitInfo[]>([]);

  // ─── Multicore + sync primitives ─────────────────────────────────────────
  // cores tracks per-core state. numCores comes from the snapshot. mutexes
  // and semaphores are kept up-to-date via the mutex_op / sem_op events so
  // the /concurrencia view can render the lock tables without re-querying.
  //
  // selectedCoreId controls which core's state shows up in the top-level
  // regs/mode/halted/currentPID refs. Existing panels and the SimBar read
  // those refs unchanged; with multicore the user picks which core to
  // mirror through a selector in the SimBar. Without this routing, events
  // from every core would overwrite a single global register file and the
  // UI would flicker between processes — the bug that motivated the whole
  // refactor.
  const cores = ref<CoreInfo[]>([]);
  const numCores = ref(1);
  const selectedCoreId = ref(0);
  const mutexes = ref<MutexState[]>([]);
  const semaphores = ref<SemaphoreState[]>([]);

  function ensureCoreSlot(id: number): CoreInfo {
    let c = cores.value.find(x => x.id === id);
    if (!c) {
      c = {
        id,
        regs: Object.fromEntries(REGISTERS.map(r => [r, 0])),
        segments: { CS: 0, DS: 0, SS: 0 },
        mode: 3,
        halted: false,
        tick: 0,
        current_pid: 0,
      };
      cores.value.push(c);
      cores.value.sort((a, b) => a.id - b.id);
    }
    return c;
  }

  function isSelected(coreId: number): boolean {
    return coreId === selectedCoreId.value;
  }

  function setSelectedCore(id: number) {
    if (id === selectedCoreId.value) return;
    selectedCoreId.value = id;
    // Re-populate the top-level refs from the newly-selected core so
    // panels that read cpu.regs/mode/etc switch instantly without
    // waiting for the next event.
    const c = cores.value.find(x => x.id === id);
    if (!c) return;
    for (const k of Object.keys(regs)) regs[k] = c.regs[k] ?? 0;
    for (const k of Object.keys(segments)) segments[k] = c.segments[k] ?? 0;
    mode.value = c.mode;
    halted.value = c.halted;
    currentPID.value = c.current_pid;
  }
  // Fuente C (escrita a mano) por PID, para mostrar en el panel CÓDIGO.
  const procSources = ref<Record<number, string>>({});
  // PIDs que cambiaron en el último evento (para animación flash)
  const flashingPIDs = reactive<Set<number>>(new Set());

  // ─── Consola ────────────────────────────────────────────────────────────
  // Salida acumulada de sys_write. Cada bloque tiene el PID que escribió para
  // colorearlos por proceso.
  interface ConsoleChunk { pid: number; text: string }
  const consoleChunks = ref<ConsoleChunk[]>([]);

  // ─── Sesión multi-tenant ────────────────────────────────────────────────
  // Llega vía evento session_info al conectar + cada vez que cambia (share/unshare).
  const sessionInfo = ref<SessionInfo>({ id: '', mode: 'unknown', role: 'none', label: '', view_token: '', user_id: '', user_name: '', user_email: '' });
  // Computed cómodos para el resto de la app.
  const isViewer = () => sessionInfo.value.role === 'viewer';

  // ─── Animaciones globales ──────────────────────────────────────────────
  // shakeUntil es un timestamp; mientras Date.now() < shakeUntil, la UI shakea.
  const faultShake = ref(false);

  // ─── Glosario global ────────────────────────────────────────────────────
  // El componente App escucha glossaryOpenSignal para abrir el modal con la
  // búsqueda prefilteada. Cualquier elemento clickeable en la UI puede llamar
  // openGlossary("EAX") y mostrar info inmediatamente.
  const glossarySearch = ref('');
  const glossaryOpenSignal = ref(0);
  function openGlossary(term?: string) {
    glossarySearch.value = term ?? '';
    glossaryOpenSignal.value++;
  }

  // ─── WS ─────────────────────────────────────────────────────────────────
  let ws: WebSocket | null = null;

  // Event subscribers fan out raw CpuEvents to anything that wants to react —
  // narrators, panels, debugging overlays. The store's own applyEvent runs
  // first to keep its state consistent; subscribers fire after, so by the
  // time they read regs/procs/etc the store already reflects the event.
  type Subscriber = (ev: CpuEvent) => void;
  const subscribers = new Set<Subscriber>();
  function subscribe(fn: Subscriber): () => void {
    subscribers.add(fn);
    return () => subscribers.delete(fn);
  }

  // connect is idempotent. Re-mounting components that call connect() — for
  // example navigating away from the Simulator and back through the shell —
  // would otherwise leak a WebSocket per visit, each with its own onmessage
  // handler pushing events to the log a second, third, fourth time.
  function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws`;
    ws = new WebSocket(url);
    ws.onopen = () => {
      connected.value = true;
      pushLog('▸ conectado al backend', 'amber');
    };
    ws.onclose = () => {
      connected.value = false;
      ws = null;
      pushLog('⚠ desconectado', 'red');
    };
    ws.onerror = () => {
      pushLog('⚠ error de conexión WS', 'red');
    };
    ws.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data) as CpuEvent;
        applyEvent(ev);
      } catch (err) {
        pushLog(`⚠ json: ${err}`, 'red');
      }
    };
  }

  function send(cmd: object) {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(cmd));
    }
  }

  function applyEvent(ev: CpuEvent) {
    switch (ev.type) {
      case 'snapshot':
        if (ev.cores) cores.value = ev.cores;
        if (ev.num_cores !== undefined) numCores.value = ev.num_cores;
        if (selectedCoreId.value >= (ev.num_cores ?? cores.value.length)) {
          selectedCoreId.value = 0;
        }
        // Mirror the selected core's view into the top-level refs so the
        // panels that read cpu.regs/mode/etc keep working. Falls back to
        // the payload's boot-core copy if cores[] is missing for some
        // reason (legacy snapshot format).
        const sel = cores.value.find(x => x.id === selectedCoreId.value);
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
        if (ev.ready_queue) readyQueue.value = ev.ready_queue;
        if (ev.wait_queue) waitQueue.value = ev.wait_queue;
        if (ev.procs) procs.value = ev.procs;
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        changedRegs.clear();
        break;
      case 'programs':
        programs.value = ev.list;
        if (!selectedProgram.value && ev.list[0]) selectedProgram.value = ev.list[0].name;
        break;
      case 'reg_write': {
        const c = ensureCoreSlot(ev.core_id);
        c.regs[ev.reg] = ev.new;
        if (isSelected(ev.core_id)) {
          regs[ev.reg] = ev.new;
          flashReg(ev.reg);
        }
        break;
      }
      case 'instruction': {
        const c = ensureCoreSlot(ev.core_id);
        const nextEIP = ev.eip + ev.bytes.length;
        c.regs.EIP = nextEIP;
        const prefix = numCores.value > 1 ? `[C${ev.core_id}] ` : '';
        pushLog(`${prefix}[${ev.eip.toString(16).padStart(6, '0')}]  ${ev.mnemonic}`, 'amber');
        if (isSelected(ev.core_id)) regs.EIP = nextEIP;
        break;
      }
      case 'tick':
        tick.value = ev.value;
        break;
      case 'mode_switch': {
        const c = ensureCoreSlot(ev.core_id);
        c.mode = ev.to;
        const prefix = numCores.value > 1 ? `[C${ev.core_id}] ` : '';
        pushLog(`${prefix}▸ mode ${ev.from === 3 ? 'USER' : 'KERNEL'} → ${ev.to === 3 ? 'USER' : 'KERNEL'} (${ev.cause})`, 'red');
        if (isSelected(ev.core_id)) mode.value = ev.to;
        break;
      }
      case 'irq_fire': {
        const prefix = numCores.value > 1 ? `[C${ev.core_id}] ` : '';
        pushLog(`${prefix}◉ IRQ 0x${ev.vector.toString(16).padStart(2, '0')} (${ev.source})`, 'yellow');
        break;
      }
      case 'page_fault': {
        const prefix = numCores.value > 1 ? `[C${ev.core_id}] ` : '';
        pushLog(`${prefix}✗ PAGE FAULT @ 0x${ev.addr.toString(16)} (need ${ev.perm_needed} in PL${ev.mode})`, 'red');
        faultShake.value = true;
        setTimeout(() => { faultShake.value = false; }, 500);
        break;
      }
      case 'halt': {
        const c = ensureCoreSlot(ev.core_id);
        c.halted = true;
        const prefix = numCores.value > 1 ? `[C${ev.core_id}] ` : '';
        pushLog(`${prefix}▸ HALT`, 'red');
        if (isSelected(ev.core_id)) halted.value = true;
        break;
      }
      case 'state':
        if (ev.halted !== undefined) halted.value = ev.halted;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.hz !== undefined) stepHz.value = ev.hz;
        break;
      case 'loaded':
        pushLog(`▸ loaded ${ev.name} (${ev.pids.length} proc) PIDs=${ev.pids.join(',')}`, 'green');
        procs.value = ev.procs;
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        break;
      case 'reset':
        pushLog('▸ reset', 'cyan');
        changedRegs.clear();
        procs.value = [];
        readyQueue.value = [];
        waitQueue.value = [];
        currentPID.value = 0;
        consoleChunks.value = [];
        procSources.value = {};
        mutexes.value = [];
        semaphores.value = [];
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
        break;
      case 'procs':
        procs.value = ev.procs;
        break;
      case 'pcb_save': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(`◍ PCB save  PID=${ev.pid}  EIP=0x${(ev.regs.EIP || 0).toString(16)}`, 'red');
        break;
      }
      case 'pcb_restore': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(`◌ PCB restore PID=${ev.pid}  EIP=0x${(ev.regs.EIP || 0).toString(16)}`, 'green');
        break;
      }
      case 'sched_decision':
        // currentPID top-level is owned by core_assign/core_release so the
        // SimBar mirrors the *selected* core only. Updating it here would
        // pull the PID of whichever core just scheduled, regardless of
        // which one the user is watching.
        readyQueue.value = ev.ready_queue;
        pushLog(`⇄ schedule: P${ev.from_pid} → P${ev.to_pid}  ready=[${ev.ready_queue.join(',')}]`, 'yellow');
        break;
      case 'process_state': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) {
          p.state = ev.state;
          if (ev.reason) p.blocked_on = ev.reason;
        }
        // Mantener wait_queue sincronizada con BLOCKED ⇄ otros estados.
        if (ev.state === 'BLOCKED') {
          if (!waitQueue.value.find(w => w.pid === ev.pid)) {
            waitQueue.value.push({ pid: ev.pid, name: p?.name ?? `P${ev.pid}`, reason: ev.reason ?? '' });
          }
        } else {
          waitQueue.value = waitQueue.value.filter(w => w.pid !== ev.pid);
        }
        break;
      }
      case 'syscall':
        pushLog(`⚙ sys_${ev.name}(${ev.args[0]}, 0x${(ev.args[1]>>>0).toString(16)}, ${ev.args[2]}) → ${ev.result}  [P${ev.pid}]`, 'green');
        break;
      case 'console_out':
        consoleChunks.value.push({ pid: ev.pid, text: ev.text });
        if (consoleChunks.value.length > 200) consoleChunks.value.splice(0, consoleChunks.value.length - 200);
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
        if (ev.role === 'viewer') {
          pushLog(`▸ conectado como ESPECTADOR de "${ev.label || ev.id.slice(0, 6)}"`, 'cyan');
        }
        break;
      case 'error':
        lastError.value = ev.message;
        pushLog(`⚠ ${ev.message}`, 'red');
        break;
      case 'mem_read':
      case 'mem_write':
        // Memory traffic is consumed exclusively by subscribers (narrators,
        // memory panel filters, etc.) — the store carries no aggregated
        // counter for it because that would grow unbounded across runs.
        break;
      case 'core_assign': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = ev.pid;
        pushLog(`◈ core ${ev.core_id} ← P${ev.pid}`, 'amber');
        if (isSelected(ev.core_id)) currentPID.value = ev.pid;
        break;
      }
      case 'core_release': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = 0;
        pushLog(`◇ core ${ev.core_id} ↛ P${ev.pid}${ev.reason ? ' (' + ev.reason + ')' : ''}`, 'amber');
        if (isSelected(ev.core_id)) currentPID.value = 0;
        break;
      }
      case 'mutex_op': {
        let m = mutexes.value.find(x => x.id === ev.mutex_id);
        if (!m) {
          m = { id: ev.mutex_id, holder: -1, waiters: [] };
          mutexes.value.push(m);
        }
        switch (ev.op) {
          case 'create':
            // Object already added above; nothing else to track.
            pushLog(`🔒 mutex ${ev.mutex_id} creado por P${ev.pid}`, 'green');
            break;
          case 'acquired':
            m.holder = ev.pid;
            m.waiters = ev.waiters ?? [];
            pushLog(`🔒 mutex ${ev.mutex_id} adquirido por P${ev.pid}`, 'green');
            break;
          case 'waiting':
            m.waiters = ev.waiters ?? [];
            pushLog(`⏸ P${ev.pid} esperando mutex ${ev.mutex_id}`, 'yellow');
            break;
          case 'released':
            m.holder = -1;
            m.waiters = ev.waiters ?? [];
            pushLog(`🔓 mutex ${ev.mutex_id} liberado por P${ev.pid}`, 'cyan');
            break;
          case 'handoff':
            m.holder = ev.next_pid ?? -1;
            m.waiters = ev.waiters ?? [];
            pushLog(`↪ mutex ${ev.mutex_id}: P${ev.pid} → P${ev.next_pid}`, 'green');
            break;
        }
        break;
      }
      case 'sem_op': {
        let s = semaphores.value.find(x => x.id === ev.sem_id);
        if (!s) {
          s = { id: ev.sem_id, count: ev.count, waiters: [] };
          semaphores.value.push(s);
        }
        s.count = ev.count;
        s.waiters = ev.waiters ?? [];
        switch (ev.op) {
          case 'create':
            pushLog(`◯ semáforo ${ev.sem_id} creado (count=${ev.count})`, 'green');
            break;
          case 'wait_immediate':
            pushLog(`↓ P${ev.pid} wait sem ${ev.sem_id} (count=${ev.count})`, 'cyan');
            break;
          case 'wait_blocked':
            pushLog(`⏸ P${ev.pid} bloqueado en sem ${ev.sem_id}`, 'yellow');
            break;
          case 'signal':
            pushLog(`↑ P${ev.pid} signal sem ${ev.sem_id} (count=${ev.count})`, 'green');
            break;
          case 'handoff':
            pushLog(`↪ sem ${ev.sem_id}: P${ev.pid} → P${ev.next_pid}`, 'green');
            break;
        }
        break;
      }
    }
    // Fan-out to subscribers AFTER the store has reacted, so they can read
    // the up-to-date snapshot in their handler.
    if (subscribers.size > 0) {
      for (const fn of subscribers) {
        try { fn(ev); } catch (err) { pushLog(`⚠ subscriber: ${err}`, 'red'); }
      }
    }
  }

  function flashReg(name: string) {
    changedRegs.add(name);
    setTimeout(() => changedRegs.delete(name), 800);
  }

  function flashPID(pid: number) {
    flashingPIDs.add(pid);
    setTimeout(() => flashingPIDs.delete(pid), 800);
  }

  function pushLog(text: string, color: string) {
    log.value.push({ ts: Date.now(), text, color });
    if (log.value.length > 500) log.value.splice(0, log.value.length - 500);
  }

  // ─── Comandos públicos ──────────────────────────────────────────────────
  function load(name: string) { send({ cmd: 'load', program: name }); }
  function step() { send({ cmd: 'step' }); }
  function run(speed: number) { send({ cmd: 'run', speed }); }
  function pause() { send({ cmd: 'pause' }); }
  function reset() {
    // Reset es "borrar todo": el backend resetea kernel/CPU/procesos y nosotros
    // wipeamos el log y consola para empezar de cero visualmente.
    send({ cmd: 'reset' });
    log.value = [];
    consoleChunks.value = [];
  }
  function clearLog() {
    // Limpia solo la vista (log + consola) sin tocar el estado del simulador.
    // Útil durante una demo para "limpiar la pizarra" sin perder el programa cargado.
    log.value = [];
    consoleChunks.value = [];
  }
  function setQuantum(q: number) {
    timerQuantum.value = q;
    send({ cmd: 'timer_quantum', quantum: q });
  }
  function pushKey(key: string) { send({ cmd: 'push_key', key }); }
  function share() { send({ cmd: 'share' }); }
  function unshare() { send({ cmd: 'unshare' }); }
  function setAlias(alias: string) { send({ cmd: 'set_alias', alias }); }
  function setNumCores(n: number) { send({ cmd: 'set_num_cores', cores: n }); }

  // viewerLink arma la URL completa que el owner puede copiar para compartir.
  function viewerLink(): string {
    const t = sessionInfo.value.view_token;
    if (!t) return '';
    return `${location.origin}/v/${t}`;
  }

  return {
    regs, segments, mode, halted, running, tick, stepHz, timerQuantum,
    changedRegs,
    programs, selectedProgram, log, connected, lastError,
    procs, currentPID, readyQueue, waitQueue, flashingPIDs, procSources,
    cores, numCores, selectedCoreId, setSelectedCore, mutexes, semaphores,
    subscribe,
    consoleChunks, faultShake,
    glossarySearch, glossaryOpenSignal,
    sessionInfo, isViewer, viewerLink,
    connect, load, step, run, pause, reset, clearLog, setQuantum, pushKey, openGlossary,
    share, unshare, setAlias, setNumCores,
  };
});
