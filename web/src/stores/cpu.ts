import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';

// Tipos de eventos que llegan por WS (espejo de internal/cpu/events.go).
export type CpuEvent =
  | { type: 'instruction'; pid: number; eip: number; mnemonic: string; bytes: number[] }
  | { type: 'reg_write'; reg: string; old: number; new: number }
  | { type: 'mem_read'; addr: number; value: number; size: number }
  | { type: 'mem_write'; addr: number; old: number; new: number; size: number }
  | { type: 'mode_switch'; from: number; to: number; cause: string }
  | { type: 'irq_fire'; vector: number; source: string }
  | { type: 'page_fault'; addr: number; mode: number; perm_needed: string }
  | { type: 'halt'; pid?: number }
  | { type: 'tick'; value: number }
  | { type: 'state'; halted?: boolean; running?: boolean; hz?: number }
  | { type: 'snapshot'; regs: Record<string, number>; segments: Record<string, number>; mode: number; halted: boolean; tick: number; timer_quantum?: number; current_pid?: number; ready_queue?: number[]; wait_queue?: WaitInfo[]; procs?: PCBInfo[]; sources?: Record<string, string> }
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
  | { type: 'session_info'; id: string; mode: string; role: string; label: string; view_token: string; user_id?: string; user_name?: string; user_email?: string }
  | { type: 'error'; message: string };

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
        for (const k of Object.keys(regs)) regs[k] = ev.regs[k] ?? 0;
        for (const k of Object.keys(segments)) segments[k] = ev.segments[k] ?? 0;
        mode.value = ev.mode;
        halted.value = ev.halted;
        tick.value = ev.tick;
        if (ev.timer_quantum !== undefined) timerQuantum.value = ev.timer_quantum;
        if (ev.current_pid !== undefined) currentPID.value = ev.current_pid;
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
      case 'reg_write':
        regs[ev.reg] = ev.new;
        flashReg(ev.reg);
        break;
      case 'instruction':
        pushLog(`[${ev.eip.toString(16).padStart(6, '0')}]  ${ev.mnemonic}`, 'amber');
        regs.EIP = ev.eip + ev.bytes.length;
        break;
      case 'tick':
        tick.value = ev.value;
        break;
      case 'mode_switch':
        mode.value = ev.to;
        pushLog(`▸ mode ${ev.from === 3 ? 'USER' : 'KERNEL'} → ${ev.to === 3 ? 'USER' : 'KERNEL'} (${ev.cause})`, 'red');
        break;
      case 'irq_fire':
        pushLog(`◉ IRQ 0x${ev.vector.toString(16).padStart(2, '0')} (${ev.source})`, 'yellow');
        break;
      case 'page_fault':
        pushLog(`✗ PAGE FAULT @ 0x${ev.addr.toString(16)} (need ${ev.perm_needed} in PL${ev.mode})`, 'red');
        faultShake.value = true;
        setTimeout(() => { faultShake.value = false; }, 500);
        break;
      case 'halt':
        halted.value = true;
        pushLog('▸ HALT', 'red');
        break;
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
        currentPID.value = ev.to_pid;
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
    subscribe,
    consoleChunks, faultShake,
    glossarySearch, glossaryOpenSignal,
    sessionInfo, isViewer, viewerLink,
    connect, load, step, run, pause, reset, clearLog, setQuantum, pushKey, openGlossary,
    share, unshare, setAlias,
  };
});
