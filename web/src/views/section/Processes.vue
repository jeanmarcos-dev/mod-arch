<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import { Hourglass } from '@lucide/vue';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import PCBCopyNarrator from '../../components/narrators/PCBCopyNarrator.vue';
import Term from '../../components/glossary/Term.vue';
// Scheduler-metrics widgets used in the template. vue-tsc's unused-locals
// pass does not detect template references, so we void-reference the
// component imports here to keep them registered without the warning.
import QuantumBarGraph from '../../components/processes/QuantumBarGraph.vue';
import SchedMetricsTable from '../../components/processes/SchedMetricsTable.vue';
import QuantumTimeline from '../../components/processes/QuantumTimeline.vue';
import { useSchedulerMetrics } from '../../composables/useSchedulerMetrics';
void QuantumBarGraph; void SchedMetricsTable; void QuantumTimeline;

const cpu = useCpuStore();
const prefs = usePreferencesStore();

const sched = useSchedulerMetrics();

// Sorted PCBs so the layout never jumps when state changes.
const sortedProcs = computed<PCBInfo[]>(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

function hex(n: number, w = 6): string {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

// Registers we show in each PCB card. EIP / ESP / EAX / EBX is the canonical
// "execution context" the kernel actually saves and restores.
const SHOWN_REGS = ['EIP', 'ESP', 'EAX', 'EBX'] as const;

// coreRunning returns the CoreInfo currently executing pid, or undefined if
// the process is READY/BLOCKED/ZOMBIE. With multicore several processes are
// RUN simultaneously (one per core), so checking cpu.currentPID alone would
// miss every PID running outside the core the user happens to be viewing.
function coreRunning(pid: number) {
  return cpu.cores.find(c => c.current_pid === pid);
}

// visibleCores honors the SimBar filter. Used both for the per-core CPU
// strips at the top and for the "current" highlight on the PCB cards —
// processes running on a hidden core no longer steal the highlight.
const visibleCores = computed(() => cpu.cores.filter(c => cpu.isCoreVisible(c.id)));
const visibleRunPIDs = computed(() => new Set(visibleCores.value.map(c => c.current_pid).filter(p => p > 0)));

function pcbValue(p: PCBInfo, reg: string): number {
  // For RUN processes the PCB snapshot is stale until the next pcb_save —
  // the live truth lives in the core executing it. We pull the live value
  // from that core's register file and flag it as "live" in the UI.
  const core = coreRunning(p.pid);
  if (core && p.state === 'RUN') {
    return core.regs[reg] ?? 0;
  }
  return p.regs[reg] ?? 0;
}

function isLive(p: PCBInfo): boolean {
  return p.state === 'RUN' && coreRunning(p.pid) !== undefined;
}

// Context switch feed: tap sched_decision via the store subscribe channel.
interface SwitchRow {
  id: number;
  fromPid: number;
  toPid: number;
  reason: string;
  ts: number;
  ready: number[];
}

const switches = ref<SwitchRow[]>([]);
let nextId = 1;
const MAX_SWITCHES = 10;

const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
  if (ev.type === 'sched_decision') {
    switches.value.unshift({
      id: nextId++,
      fromPid: ev.from_pid,
      toPid: ev.to_pid,
      reason: ev.reason ?? '',
      ts: Date.now(),
      // ready_queue serializes as null when the queue is empty; guard so the
      // spread does not crash with TypeError.
      ready: [...(ev.ready_queue ?? [])],
    });
    if (switches.value.length > MAX_SWITCHES) {
      switches.value = switches.value.slice(0, MAX_SWITCHES);
    }
  } else if (ev.type === 'reset') {
    switches.value = [];
  }
});

onBeforeUnmount(unsubscribe);

// Refresh "time ago" labels every 500ms.
const now = ref(Date.now());
const tickHandle = window.setInterval(() => { now.value = Date.now(); }, 500);
onBeforeUnmount(() => window.clearInterval(tickHandle));

function timeAgo(ts: number): string {
  const diff = Math.max(0, now.value - ts);
  if (diff < 1000) return 'ahora';
  if (diff < 60_000) return `${Math.floor(diff / 1000)}s`;
  return `${Math.floor(diff / 60_000)}m`;
}

function stateClass(state: string): string {
  return `state-${state.toLowerCase()}`;
}

const narratorHint = computed(() =>
  prefs.narratorsEnabled
    ? null
    : 'Active los narradores en Preferencias para ver los registros viajando entre CPU y PCB.',
);

const cpuRegsForStrip = ['EIP', 'ESP', 'EBP', 'EAX', 'EBX', 'ECX', 'EDX'];
</script>

<template>
  <div class="procs-page" data-pizarra-anchor="view-procesos">
    <PCBCopyNarrator />

    <div class="procs-page__content">
      <header class="procs-page__hero">
        <h2 class="procs-page__title">
          <Term term="PCB" mode="title">Procesos</Term>
        </h2>
        <p class="procs-page__tagline">
          La CPU sólo ejecuta uno a la vez. Cuando el
          <Term term="Round-Robin">scheduler</Term> decide cambiar
          (por timer o por bloqueo), el kernel <strong>copia</strong> los
          registros del CPU al <Term term="PCB" as="code">PCB</Term> del proceso saliente y carga
          los del entrante. Cargue <code>two_procs</code>, <code>three_procs</code>
          o <code>sleep</code> y avance para ver el viaje.
        </p>
        <p v-if="narratorHint" class="procs-page__hint">{{ narratorHint }}</p>
      </header>

      <section
        :class="['cpu-strip-grid', { 'cpu-strip-grid--multi': visibleCores.length > 1 }]"
      >
        <article
          v-for="core in visibleCores"
          :key="`core-${core.id}`"
          :class="['cpu-strip', { 'cpu-strip--idle': core.current_pid === 0, 'cpu-strip--halted': core.halted }]"
          :data-narrator-anchor="`cpu-${core.id}`"
        >
          <div class="cpu-strip__head">
            <span class="cpu-strip__label">{{ cpu.numCores > 1 ? `CORE C${core.id}` : 'CPU en uso' }}</span>
            <span v-if="core.current_pid" class="cpu-strip__pid">P{{ core.current_pid }}</span>
            <span v-else class="cpu-strip__pid cpu-strip__pid--idle">— ocioso —</span>
            <span v-if="core.halted" class="cpu-strip__tag cpu-strip__tag--halt">HALT</span>
          </div>
          <div class="cpu-strip__regs">
            <div v-for="r in cpuRegsForStrip" :key="r" class="cpu-strip__reg">
              <Term :term="r" mode="label" class="cpu-strip__reg-name">{{ r }}</Term>
              <span class="cpu-strip__reg-val">{{ hex(core.regs[r] ?? 0, 8) }}</span>
            </div>
          </div>
        </article>
      </section>

      <section class="pcb-grid" v-if="sortedProcs.length > 0">
        <article
          v-for="p in sortedProcs"
          :key="p.pid"
          :data-narrator-anchor="`pcb-${p.pid}`"
          :class="[
            'pcb-card',
            stateClass(p.state),
            { 'pcb-card--current': visibleRunPIDs.has(p.pid), 'pcb-card--flash': cpu.flashingPIDs.has(p.pid) },
          ]"
        >
          <header class="pcb-card__head">
            <Term term="PID" mode="label" class="pcb-card__pid">P{{ p.pid }}</Term>
            <Term :term="p.state" mode="label" class="pcb-card__state">{{ p.state }}</Term>
          </header>
          <div class="pcb-card__name">{{ p.name }}</div>
          <ul class="pcb-card__regs">
            <li v-for="r in SHOWN_REGS" :key="r">
              <Term :term="r" mode="label" class="pcb-card__reg-name">{{ r }}</Term>
              <span
                :class="['pcb-card__reg-val', { 'pcb-card__reg-val--live': isLive(p) }]"
                :title="isLive(p) ? 'Valor live del CPU — la copia del PCB se actualiza en el próximo pcb_save' : 'Valor congelado: snapshot guardado por el kernel'"
              >
                {{ hex(pcbValue(p, r), 8) }}
              </span>
            </li>
          </ul>
          <footer class="pcb-card__foot">
            <span title="Base del segmento de código de este proceso.">{{ hex(p.code_base) }}</span>
            <span v-if="p.blocked_on" class="pcb-card__blocked"><Hourglass :size="11" :stroke-width="2" /> {{ p.blocked_on }}</span>
          </footer>
        </article>
      </section>
      <div v-else class="pcb-grid__empty">
        Sin procesos cargados. Use el selector del topbar para cargar una demo.
      </div>

      <div class="procs-page__grid">
        <section class="procs-page__panel">
          <header class="procs-page__panel-head">
            <h3><Term term="Round-Robin" mode="title">Scheduler</Term></h3>
            <span class="procs-page__panel-hint">
              <Term term="Round-Robin">Round-Robin</Term> ·
              <Term term="Quantum" as="code">quantum={{ cpu.timerQuantum }}</Term>
            </span>
          </header>
          <div class="sched-row">
            <Term term="Ready queue" mode="label" class="sched-row__label">READY</Term>
            <div class="sched-row__pills">
              <span v-if="cpu.readyQueue.length === 0" class="sched-row__empty">— vacía —</span>
              <span v-for="pid in cpu.readyQueue" :key="`r-${pid}`" class="sched-pill sched-pill--ready">P{{ pid }}</span>
            </div>
          </div>
          <div class="sched-row">
            <Term term="Waiting queue" mode="label" class="sched-row__label">WAIT</Term>
            <div class="sched-row__pills">
              <span v-if="cpu.waitQueue.length === 0" class="sched-row__empty">— vacía —</span>
              <span
                v-for="w in cpu.waitQueue"
                :key="`w-${w.pid}`"
                class="sched-pill sched-pill--wait"
                :title="`Esperando: ${w.reason}`"
              >
                P{{ w.pid }} · {{ w.reason }}
              </span>
            </div>
          </div>
        </section>

        <section class="procs-page__panel">
          <header class="procs-page__panel-head">
            <h3><Term term="Context switch" mode="title">Últimos cambios de contexto</Term></h3>
            <span class="procs-page__panel-hint">
              <Term term="Preemption">sched_decision</Term> en vivo
            </span>
          </header>
          <div v-if="switches.length === 0" class="ctx-feed__empty">
            Sin cambios de contexto aún. Cargue un demo multi-proceso o avance hasta que el timer dispare.
          </div>
          <ol v-else class="ctx-feed">
            <li v-for="s in switches" :key="s.id" class="ctx-feed__row">
              <span class="ctx-feed__pid">P{{ s.fromPid }}</span>
              <span class="ctx-feed__arrow">→</span>
              <span class="ctx-feed__pid ctx-feed__pid--to">P{{ s.toPid }}</span>
              <span v-if="s.reason" class="ctx-feed__reason">· por {{ s.reason }}</span>
              <span class="ctx-feed__ready">ready=[{{ s.ready.join(',') || '∅' }}]</span>
              <span class="ctx-feed__time">{{ timeAgo(s.ts) }}</span>
            </li>
          </ol>
        </section>
      </div>

      <section class="procs-page__panel">
        <header class="procs-page__panel-head">
          <h3><Term term="Round-Robin (RR)" mode="title">Métricas del scheduler</Term></h3>
          <span class="procs-page__panel-hint">demo actual · ticks simulados</span>
        </header>
        <SchedMetricsTable
          :metrics="sched.metrics"
          :throughput-per-k-tick="sched.throughputPerKTick.value"
          :quantum-ms-hint="sched.quantumMsHint.value"
          :hz-sim="sched.hzSim"
        />
      </section>

      <section class="procs-page__panel">
        <header class="procs-page__panel-head">
          <h3><Term term="Quantum" mode="title">Quantums por proceso</Term></h3>
          <span class="procs-page__panel-hint">color: completos · rayado: parciales</span>
        </header>
        <QuantumBarGraph :metrics="sched.metrics" />
      </section>

      <section class="procs-page__panel">
        <header class="procs-page__panel-head">
          <h3><Term term="Context switch" mode="title">Timeline de quantums</Term></h3>
          <span class="procs-page__panel-hint">últimos 200 ticks · una banda por core visible</span>
        </header>
        <QuantumTimeline
          :metrics="sched.metrics"
          :current-tick-rel="sched.currentTickRel.value"
          :visible-core-ids="visibleCores.map(c => c.id)"
          :timer-quantum="cpu.timerQuantum"
        />
      </section>
    </div>
  </div>
</template>

<style scoped>
.procs-page {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.procs-page__content {
  position: relative;
  z-index: 1;
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.procs-page__hero { display: flex; flex-direction: column; gap: var(--space-2); }

.procs-page__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 5px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.procs-page__tagline {
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
  max-width: 78ch;
}

.procs-page__tagline code {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px;
  font-size: 11px;
  color: var(--amber);
}

.procs-page__hint {
  font-size: 11px;
  color: var(--text-dim);
  padding: var(--space-2) var(--space-3);
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
  align-self: flex-start;
}

.cpu-strip-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-3);
}
.cpu-strip-grid--multi {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
}

.cpu-strip {
  background: var(--surface-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-4);
  box-shadow: var(--shadow-glow-amber);
}
.cpu-strip--idle {
  box-shadow: none;
  border-color: var(--border);
  opacity: 0.85;
}
.cpu-strip--halted {
  box-shadow: none;
  border-color: var(--red-soft, #ff7a7a);
  background: rgba(255, 122, 122, 0.04);
}

.cpu-strip__tag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1.5px;
  padding: 1px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  margin-left: auto;
}
.cpu-strip__tag--halt { color: var(--red-soft, #ff7a7a); }

.cpu-strip__head {
  display: flex;
  align-items: baseline;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  padding-bottom: var(--space-2);
  border-bottom: 1px dashed var(--border);
}

.cpu-strip__label {
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
  letter-spacing: 3px;
  color: var(--text-dim);
  text-transform: uppercase;
}

.cpu-strip__pid {
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  color: var(--amber);
  font-weight: 700;
  letter-spacing: 1px;
}

.cpu-strip__pid--idle {
  color: var(--text-dim);
  font-weight: normal;
  font-style: italic;
}

.cpu-strip__regs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 110px), 1fr));
  gap: var(--space-2) var(--space-4);
}

.cpu-strip__reg {
  display: flex;
  justify-content: space-between;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
}

.cpu-strip__reg-name { color: var(--cyan); }
.cpu-strip__reg-val  { color: var(--amber); }

.pcb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr));
  gap: var(--space-3);
}

.pcb-grid__empty {
  color: var(--text-dim);
  font-style: italic;
  padding: var(--space-5);
  text-align: center;
  border: 1px dashed var(--border);
  border-radius: var(--radius-md);
}

.pcb-card {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  transition: border-color var(--transition-base), box-shadow var(--transition-base), background var(--transition-base);
}

.pcb-card--current {
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
  background: rgba(var(--accent-rgb), 0.04);
}

.pcb-card--flash {
  animation: pcb-card-flash 800ms ease-out;
}

@keyframes pcb-card-flash {
  0%   { background: rgba(var(--accent-rgb), 0.22); }
  100% { background: rgba(var(--accent-rgb), 0.04); }
}

.pcb-card.state-run    { color: var(--green); }
.pcb-card.state-ready  { color: #ffd24f; }
.pcb-card.state-blocked { color: #ff7a7a; }
.pcb-card.state-zombie { color: var(--text-dim); }

.pcb-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px dashed var(--border);
  padding-bottom: 4px;
}

.pcb-card__pid {
  font-family: 'Major Mono Display', monospace;
  font-size: 14px;
  letter-spacing: 2px;
  color: var(--amber);
  font-weight: normal;
}

.pcb-card__state {
  font-size: 10px;
  letter-spacing: 2px;
  font-family: 'JetBrains Mono', monospace;
}

.pcb-card__name {
  color: var(--text);
  font-size: 12px;
  font-family: 'JetBrains Mono', monospace;
}

.pcb-card__regs {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pcb-card__regs li {
  display: flex;
  justify-content: space-between;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.pcb-card__reg-name { color: var(--cyan); }

.pcb-card__reg-val {
  color: var(--text);
  position: relative;
}

.pcb-card__reg-val--live {
  color: var(--text-dim);
  font-style: italic;
}

.pcb-card__reg-val--live::after {
  content: '·live';
  margin-left: 4px;
  font-size: 9px;
  color: var(--green);
  font-style: normal;
}

.pcb-card__foot {
  display: flex;
  justify-content: space-between;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
  margin-top: auto;
  border-top: 1px dashed var(--border);
  padding-top: 4px;
}

.pcb-card__blocked {
  color: #ff7a7a;
}

.procs-page__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}

@media (max-width: 768px) {
  .procs-page__grid { grid-template-columns: 1fr; }
}

.procs-page__panel {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.procs-page__panel-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
  padding-bottom: var(--space-2);
}

.procs-page__panel-head h3 {
  font-family: 'Major Mono Display', monospace;
  font-size: 12px;
  letter-spacing: 3px;
  color: var(--amber);
  font-weight: normal;
}

.procs-page__panel-hint {
  font-size: 10px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.sched-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.sched-row__label {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 1px;
  min-width: 48px;
}

.sched-row__pills {
  display: flex;
  gap: var(--space-1);
  flex-wrap: wrap;
}

.sched-row__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 11px;
}

.sched-pill {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 1px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
}

.sched-pill--ready { color: #ffd24f; }
.sched-pill--wait  { color: #ff7a7a; }

.ctx-feed {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.ctx-feed__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
}

.ctx-feed__row {
  display: grid;
  grid-template-columns: auto auto auto auto 1fr auto;
  align-items: center;
  gap: var(--space-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 3px var(--space-2);
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.06);
}

.ctx-feed__pid {
  color: var(--cyan);
  font-weight: 700;
}

.ctx-feed__pid--to { color: var(--amber); }

.ctx-feed__arrow { color: var(--text-dim); }

.ctx-feed__reason {
  color: var(--text-dim);
  font-style: italic;
  font-size: 10px;
}

.ctx-feed__ready {
  color: var(--text-dim);
  font-size: 10px;
}

.ctx-feed__time {
  color: var(--text-dim);
  font-size: 10px;
  text-align: right;
}
</style>
