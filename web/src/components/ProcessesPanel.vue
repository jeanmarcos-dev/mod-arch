<script setup lang="ts">
import { useCpuStore, type PCBInfo } from '../stores/cpu';
import { computed, ref } from 'vue';
import { ChevronRight } from '@lucide/vue';

const cpu = useCpuStore();

function hex(n: number, w = 6) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function stateClass(state: string) {
  return `state-${state.toLowerCase()}`;
}

// Mirror of pkg/cpu/cpu.go FlagCF/ZF/SF/IF/OF bit positions. Anything outside
// this set is irrelevant for the chip strip; if the backend adds new flags
// the strip degrades silently (no chip rendered) instead of breaking layout.
const EFLAGS_BITS: Array<{ name: string; mask: number; help: string }> = [
  { name: 'CF', mask: 1 << 0,  help: 'Carry: el resultado generó acarreo o pidió préstamo.' },
  { name: 'ZF', mask: 1 << 6,  help: 'Zero: el resultado fue cero.' },
  { name: 'SF', mask: 1 << 7,  help: 'Sign: el resultado fue negativo (bit alto en 1).' },
  { name: 'IF', mask: 1 << 9,  help: 'Interrupt Enable: el CPU acepta interrupciones de hardware.' },
  { name: 'OF', mask: 1 << 11, help: 'Overflow: el resultado se salió del rango con signo.' },
];

// Lista ordenada por PID para que las tarjetas no salten. El PID es secuencial
// al crear cada proceso, así que esto equivale al orden de carga del kernel.
const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

const EXPANDED_KEY = 'modos.procs.expandedPids';

function loadExpanded(): Set<number> {
  if (typeof window === 'undefined') return new Set();
  try {
    const raw = window.localStorage.getItem(EXPANDED_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return new Set(parsed.filter((x: unknown): x is number => typeof x === 'number'));
      }
    }
  } catch { /* corrupt entry, fall back to empty */ }
  return new Set();
}

function persistExpanded() {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(EXPANDED_KEY, JSON.stringify([...expandedPids.value]));
  } catch { /* quota / private mode → keep in memory only */ }
}

const expandedPids = ref<Set<number>>(loadExpanded());

function isExpanded(pid: number): boolean {
  return expandedPids.value.has(pid);
}

function toggleExpanded(p: PCBInfo) {
  const next = new Set(expandedPids.value);
  if (next.has(p.pid)) {
    next.delete(p.pid);
  } else {
    next.add(p.pid);
    // Expanding a PCB centers the memory panel on this process so the user
    // sees the bytes of the code segment update as the proc runs. ZOMBIEs are
    // skipped because their memory slot is gone.
    if (canJumpToMemory(p)) {
      cpu.requestMemoryJump(p.code_base);
    }
  }
  expandedPids.value = next;
  persistExpanded();
}

function expandAll() {
  // Bulk expand does not fire a memory jump: with N procs there is no good
  // "last one wins" answer, and the user expects this control to just open
  // the cards. They can still click an individual head to jump.
  expandedPids.value = new Set(sortedProcs.value.map(p => p.pid));
  persistExpanded();
}

function collapseAll() {
  expandedPids.value = new Set();
  persistExpanded();
}

const allExpanded = computed(() =>
  sortedProcs.value.length > 0 && sortedProcs.value.every(p => expandedPids.value.has(p.pid)),
);

function canJumpToMemory(p: PCBInfo): boolean {
  if (p.state === 'ZOMBIE') return false;
  return p.code_base > 0;
}

// PID → core that is currently executing it (if any). Used to surface the
// C<id> chip on RUN cards and to pick the live register source.
function coreOfPid(pid: number): number | null {
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    if (c.current_pid === pid) return c.id;
  }
  return null;
}

// Live register block: when the proc is RUN on a visible core we read the
// core's CPU registers (they tick every instruction). When the proc is
// READY/BLOCKED/NEW/ZOMBIE the CPU is busy with someone else, so we read its
// last saved PCB copy. This makes the "save → restore" mechanic visible: the
// numbers freeze the moment a context switch parks the proc.
function liveRegs(p: PCBInfo): Record<string, number> {
  const coreId = coreOfPid(p.pid);
  if (coreId === null) return p.regs;
  const c = cpu.cores.find(x => x.id === coreId);
  return c ? c.regs : p.regs;
}

function liveEIP(p: PCBInfo): number {
  return (liveRegs(p).EIP ?? 0) >>> 0;
}

function regsList(regs: Record<string, number>): Array<{ name: string; val: number }> {
  const order = ['EAX', 'EBX', 'ECX', 'EDX', 'ESP', 'EBP', 'EIP'];
  return order.filter(k => k in regs).map(k => ({ name: k, val: regs[k] }));
}

function flagOn(eflags: number, mask: number): boolean {
  return (eflags & mask) !== 0;
}

function regsSource(p: PCBInfo): string {
  return coreOfPid(p.pid) !== null ? 'live · CPU' : 'guardado · PCB';
}

function stateTitle(p: PCBInfo): string {
  switch (p.state) {
    case 'RUN':     return `Corriendo en C${coreOfPid(p.pid) ?? '?'}: sus registros viven en el CPU.`;
    case 'READY':   return 'En cola listo para correr. Sus registros están guardados en el PCB.';
    case 'BLOCKED': return `Esperando un recurso${p.blocked_on ? ` (${p.blocked_on})` : ''}. Sus registros están guardados en el PCB.`;
    case 'NEW':     return 'Recién creado, todavía no entró a la cola de listos.';
    case 'ZOMBIE':  return 'Terminó; el kernel mantiene el PCB hasta que el padre lo recoja.';
    default:        return p.state;
  }
}
</script>

<template>
  <div class="panel procs" data-pizarra-anchor="procs-panel">
    <div class="proc-header">
      <div
        class="panel-title clickable-title"
        title="Process Control Block: tabla completa de procesos cargados por el kernel, en orden de carga (PID asc), sin importar el estado.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('PCB')"
      >PCB DE PROCESOS</div>
      <button
        v-if="sortedProcs.length > 0"
        type="button"
        class="proc-toolbar__btn"
        :class="{ 'proc-toolbar__btn--open': allExpanded }"
        :title="allExpanded ? 'Colapsar todos los PCBs.' : 'Expandir el PCB de todos los procesos.'"
        :aria-label="allExpanded ? 'Colapsar todos' : 'Expandir todos'"
        :aria-expanded="allExpanded"
        @click="allExpanded ? collapseAll() : expandAll()"
      >
        <ChevronRight :size="11" :stroke-width="2" />
      </button>
    </div>

    <div v-if="sortedProcs.length === 0" class="empty">
      Cargue un demo para ver los procesos.
    </div>

    <div v-if="sortedProcs.length > 0" class="proc-list">
      <section
        v-for="p in sortedProcs"
        :key="p.pid"
        class="proc-card"
        :class="[
          stateClass(p.state),
          {
            'proc-card--running': coreOfPid(p.pid) !== null,
            'proc-card--expanded': isExpanded(p.pid),
            'proc-card--flash': cpu.flashingPIDs.has(p.pid),
          },
        ]"
        :data-narrator-anchor="`pcb-${p.pid}`"
      >
        <header
          class="proc-card__head"
          role="button"
          tabindex="0"
          :aria-expanded="isExpanded(p.pid)"
          :title="stateTitle(p)"
          @click="toggleExpanded(p)"
          @keydown.enter.prevent="toggleExpanded(p)"
          @keydown.space.prevent="toggleExpanded(p)"
        >
          <span class="proc-card__chevron">{{ isExpanded(p.pid) ? '▾' : '▸' }}</span>
          <span class="proc-card__state" :class="stateClass(p.state)">{{ p.state }}</span>
          <span
            v-if="coreOfPid(p.pid) !== null"
            class="proc-card__core"
            :style="{ color: `var(--core-${(coreOfPid(p.pid) as number) % 4})` }"
            :title="`Núcleo C${coreOfPid(p.pid)} ejecutando este proceso.`"
          >C{{ coreOfPid(p.pid) }}</span>
          <span class="proc-card__pid">P{{ p.pid }}</span>
          <span class="proc-card__name" :title="`${p.name} · EIP ${hex(liveEIP(p), 8)}`">{{ p.name }}</span>
        </header>

        <div v-if="isExpanded(p.pid)" class="proc-card__body">
          <div class="proc-card__meta">
            <span class="proc-card__addr" title="Dirección donde el kernel cargó el binario .text de este proceso.">
              @ {{ hex(p.code_base, 8) }}
            </span>
            <span class="proc-card__regs-src" :title="`Los valores mostrados son ${coreOfPid(p.pid) !== null ? 'los registros vivos del CPU mientras este proceso corre. Cambian cada step.' : 'la última copia guardada en el PCB. Se congelaron en el último context switch hasta que el scheduler lo restaure.'}`">
              {{ regsSource(p) }}
            </span>
          </div>

          <div v-if="p.state === 'BLOCKED' && p.blocked_on" class="proc-card__blocked">
            ⏸ esperando: {{ p.blocked_on }}
          </div>

          <div class="proc-card__regs">
            <span
              v-for="r in regsList(liveRegs(p))"
              :key="r.name"
              :title="`${r.name} ${coreOfPid(p.pid) !== null ? 'live en CPU' : 'guardado en PCB'} de P${p.pid}.`"
            >
              <span class="r-name">{{ r.name }}</span>
              <span class="r-val">{{ hex(r.val, 8) }}</span>
            </span>
          </div>

          <div class="proc-card__flags">
            <span
              class="proc-card__flags-label clickable-label"
              title="EFLAGS: registro de banderas del CPU guardado en el PCB. Cada bit refleja el último resultado aritmético (CF/ZF/SF/OF) o configuración del núcleo (IF). Cada chip se enciende cuando su bandera está en 1.&#10;&#10;— click para más info —"
              @click="cpu.openGlossary('EFLAGS')"
            >EFLAGS</span>
            <span
              v-for="f in EFLAGS_BITS"
              :key="f.name"
              class="pcb-flag"
              :class="{ 'pcb-flag--on': flagOn(liveRegs(p).EFLAGS ?? 0, f.mask) }"
              :title="`${f.name} — ${f.help} Actualmente ${flagOn(liveRegs(p).EFLAGS ?? 0, f.mask) ? 'en 1 (encendida)' : 'en 0 (apagada)'}.`"
            >{{ f.name }}</span>
          </div>
        </div>
      </section>
    </div>

    <div class="ready-queue" v-if="sortedProcs.length > 0">
      <div
        class="rq-label clickable-label"
        title="Cola FIFO de procesos READY.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('Ready queue')"
      >READY QUEUE</div>
      <div class="rq-pills">
        <span v-if="cpu.readyQueue.length === 0" class="rq-empty">— vacía —</span>
        <span v-for="pid in cpu.readyQueue" :key="pid" class="rq-pill">P{{ pid }}</span>
      </div>
    </div>

    <div class="wait-queue" v-if="sortedProcs.length > 0">
      <div
        class="rq-label clickable-label"
        title="Cola de procesos BLOCKED esperando un recurso.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('Waiting queue')"
      >WAITING QUEUE
        <span v-if="cpu.waitQueue.length > 0" class="wq-hint">— bloqueados en I/O</span>
      </div>
      <div class="rq-pills">
        <span v-if="cpu.waitQueue.length === 0" class="rq-empty">— vacía —</span>
        <span v-for="w in cpu.waitQueue" :key="w.pid" class="wq-pill" :title="`P${w.pid} esperando: ${w.reason}. Va a despertarse cuando llegue el dato/evento que pidió.`">
          P{{ w.pid }}
          <span class="wq-reason">{{ w.reason }}</span>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.procs {
  width: 280px;
  flex-shrink: 0;
  height: 100%;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

@media (max-width: 768px) {
  .procs {
    width: 100%;
    height: auto;
    flex: 1;
  }
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 16px 0;
  text-align: center;
}

.clickable-title, .clickable-label {
  cursor: pointer;
  transition: color 0.15s ease;
}

.clickable-title:hover, .clickable-label:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

/* Pull the title's dashed underline up onto the whole header so it spans
   the full panel width and the toggle button sits on it. The original
   .panel-title underline is suppressed inside this flex container. */
.proc-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 8px;
  border-bottom: 1px dashed var(--border-strong);
  padding-bottom: 5px;
  margin-bottom: 8px;
}

.proc-header .panel-title {
  border-bottom: none;
  padding-bottom: 0;
  margin-bottom: 0;
}

.proc-toolbar__btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  /* Match the .panel-title cap-height (font-size 11px). box-sizing: border-box
     keeps the outer footprint at exactly 14×14 including the 1px border. */
  box-sizing: border-box;
  width: 14px;
  height: 14px;
  /* Override the global `button { min-height: 32px }` mobile-touch baseline. */
  min-height: 0;
  padding: 0;
  /* Cancel the global `button { font-size: 14px }` so no line-box of the
     wrapped icon stretches the button taller than its declared height. */
  font-size: 0;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: color 0.18s ease, border-color 0.18s ease, background 0.18s ease;
}

.proc-toolbar__btn :deep(svg) {
  transition: transform 0.2s ease;
}

.proc-toolbar__btn--open :deep(svg) {
  transform: rotate(90deg);
}

.proc-toolbar__btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.proc-toolbar__btn:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}

.proc-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}

.proc-card {
  border: 1px solid var(--border);
  border-left-width: 3px;
  border-left-color: var(--border);
  background: var(--surface-overlay);
  transition: background 0.15s ease;
}

.proc-card.state-run     { border-left-color: var(--green); }
.proc-card.state-ready   { border-left-color: var(--yellow); }
.proc-card.state-blocked { border-left-color: var(--red-soft); }
.proc-card.state-new     { border-left-color: var(--cyan); }
.proc-card.state-zombie  { border-left-color: var(--text-muted); }

.proc-card--running {
  background: rgba(127, 255, 170, 0.04);
}

.proc-card--flash {
  background: rgba(var(--accent-rgb), 0.18);
  animation: flash-amber 800ms ease-out;
}

@keyframes flash-amber {
  0%   { background: rgba(var(--accent-rgb), 0.30); }
  100% { background: rgba(var(--accent-rgb), 0.04); }
}

.proc-card__head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 5px 8px;
  cursor: pointer;
  user-select: none;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
}

.proc-card__head:hover {
  background: rgba(var(--accent-rgb), 0.06);
}

.proc-card__head:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.proc-card__chevron {
  width: 8px;
  color: var(--text-dim);
  flex-shrink: 0;
  font-size: 10px;
}

/* All in-header tags are plain coloured text — no boxes, no backgrounds. The
   visual hierarchy comes from colour and weight, not from chip frames that
   compete with the panel border. */
.proc-card__state {
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}

.proc-card__state.state-run     { color: var(--green); }
.proc-card__state.state-ready   { color: var(--yellow); }
.proc-card__state.state-blocked { color: var(--red-soft); }
.proc-card__state.state-new     { color: var(--cyan); }
.proc-card__state.state-zombie  { color: var(--text-muted); }

.proc-card__core {
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}

.proc-card__pid {
  color: var(--amber);
  font-weight: 700;
  font-size: 11px;
  flex-shrink: 0;
}

.proc-card__name {
  color: var(--text);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.proc-card__body {
  padding: 6px 10px 8px;
  border-top: 1px dashed var(--border);
  background: rgba(var(--accent-rgb), 0.03);
}

.proc-card__meta {
  display: flex;
  gap: 10px;
  font-size: 10px;
  color: var(--text-muted);
  margin-bottom: 6px;
  flex-wrap: wrap;
  align-items: baseline;
}

.proc-card__addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}

.proc-card__regs-src {
  margin-left: auto;
  font-style: italic;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--text-muted);
}

.proc-card__blocked {
  font-size: 11px;
  color: var(--red-soft);
  margin-bottom: 6px;
  font-style: italic;
}

.proc-card__regs {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 3px 12px;
  font-size: 12px;
}

.proc-card__regs > span {
  display: flex;
  justify-content: space-between;
  cursor: help;
}

.r-name {
  color: var(--cyan);
}

.r-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}

.proc-card__flags-label {
  color: var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.proc-card__flags {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.pcb-flag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid var(--border);
  color: var(--text-muted);
  background: transparent;
  cursor: help;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}

.pcb-flag--on {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.12);
  text-shadow: 0 0 4px rgba(var(--accent-rgb), 0.45);
}

.ready-queue,
.wait-queue {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.rq-label {
  color: var(--cyan);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.rq-pills {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.rq-pill {
  padding: 2px 8px;
  background: rgba(255, 213, 79, 0.08);
  border: 1px solid var(--yellow);
  color: var(--yellow);
  font-size: 13px;
  font-weight: 700;
}

.rq-empty {
  color: var(--text-muted);
  font-style: italic;
  font-size: 13px;
}

.wq-hint {
  color: var(--text-muted);
  font-style: italic;
  font-size: 10px;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 4px;
}

.wq-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  background: rgba(255, 122, 122, 0.06);
  border: 1px solid var(--red-soft);
  color: var(--red-soft);
  font-size: 13px;
  font-weight: 700;
  animation: wait-pulse 1.6s ease-in-out infinite;
}

.wq-reason {
  font-size: 11px;
  font-weight: 400;
  color: var(--text-dim);
  text-transform: lowercase;
  letter-spacing: 0;
}

@keyframes wait-pulse {
  0%, 100% { opacity: 1; box-shadow: none; }
  50%      { opacity: 0.7; box-shadow: 0 0 6px rgba(255, 122, 122, 0.3); }
}
</style>
