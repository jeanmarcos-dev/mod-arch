<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Hourglass, Eye, EyeOff } from '@lucide/vue';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import { useProcessTree } from '../../composables/useProcessTree';

const { t } = useI18n();
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

function hex(n: number, w = 6): string {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

// Same register set the Overview surfaces, ordered for at-a-glance reading.
const SHOWN_REGS = ['EAX', 'EBX', 'ECX', 'EDX', 'ESP', 'EBP', 'EIP'] as const;

// EFLAGS bit layout — mirror of pkg/cpu/cpu.go. Kept in sync with
// ProcessesPanel.vue so both views read the same flags.
type FlagName = 'CF' | 'ZF' | 'SF' | 'IF' | 'OF';
const EFLAGS_BITS: Array<{ name: FlagName; mask: number }> = [
  { name: 'CF', mask: 1 << 0  },
  { name: 'ZF', mask: 1 << 6  },
  { name: 'SF', mask: 1 << 7  },
  { name: 'IF', mask: 1 << 9  },
  { name: 'OF', mask: 1 << 11 },
];

// visibleCores honors the SimBar filter. Used both for the per-core CPU
// strips at the top and for the "current" highlight on the PCB cards —
// processes running on a hidden core no longer steal the highlight.
const visibleCores = computed(() => cpu.cores.filter(c => cpu.isCoreVisible(c.id)));

// Scheduling fact — do NOT filter by isCoreVisible (template adds it
// explicitly at the C{id} chip).
function coreOfPid(pid: number): number | null {
  for (const c of cpu.cores) {
    if (c.current_pid === pid) return c.id;
  }
  return null;
}

// Tree walk with no depth cap — this view has the horizontal room the
// Overview's narrow column lacks, so deeper fork chains render naturally.
const { treeNodes, initPID } = useProcessTree();

function flagOn(eflags: number, mask: number): boolean {
  return (eflags & mask) !== 0;
}

function flagHelp(name: FlagName): string {
  return t(`panels.processes.flagHelp.${name}`);
}

function flagChipTitle(name: FlagName, on: boolean): string {
  return on
    ? t('panels.processes.flagChipOn', { name, help: flagHelp(name) })
    : t('panels.processes.flagChipOff', { name, help: flagHelp(name) });
}

function regCellTitle(reg: string, p: PCBInfo): string {
  return t('panels.processes.regCellTitle', {
    cell: t('panels.processes.regCellSaved', { reg }),
    pid: p.pid,
  });
}

function stateTitle(p: PCBInfo): string {
  switch (p.state) {
    case 'RUN': {
      const core = coreOfPid(p.pid);
      return core === null
        ? t('panels.processes.states.runUnknownCore')
        : t('panels.processes.states.run', { core });
    }
    case 'READY':   return t('panels.processes.states.ready');
    case 'BLOCKED': return p.blocked_on
      ? t('panels.processes.states.blockedOn', { reason: p.blocked_on })
      : t('panels.processes.states.blocked');
    case 'NEW':     return t('panels.processes.states.new');
    case 'ZOMBIE':  return t('panels.processes.states.zombie');
    default:        return p.state;
  }
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
  if (diff < 1000) return t('sections.shared.timeAgoNow');
  if (diff < 60_000) return t('sections.shared.timeAgoSec', { n: Math.floor(diff / 1000) });
  return t('sections.shared.timeAgoMin', { n: Math.floor(diff / 60_000) });
}

function stateClass(state: string): string {
  return `state-${state.toLowerCase()}`;
}

const narratorHint = computed(() =>
  prefs.narratorsEnabled
    ? null
    : t('sections.processes.narratorHint'),
);

const cpuRegsForStrip = ['EIP', 'ESP', 'EBP', 'EAX', 'EBX', 'ECX', 'EDX'];
</script>

<template>
  <div class="procs-page" data-pizarra-anchor="view-procesos">
    <PCBCopyNarrator />

    <div class="procs-page__content">
      <header class="procs-page__hero">
        <h2 class="procs-page__title">
          <Term term="PCB" mode="title">{{ t('sections.processes.title') }}</Term>
        </h2>
        <p class="procs-page__tagline">{{ t('sections.processes.tagline') }}</p>
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
            <span class="cpu-strip__label">{{ cpu.numCores > 1 ? `CORE C${core.id}` : t('sections.processes.cpuInUse') }}</span>
            <span v-if="core.current_pid" class="cpu-strip__pid">P{{ core.current_pid }}</span>
            <span v-else class="cpu-strip__pid cpu-strip__pid--idle">{{ t('sections.processes.cpuIdle') }}</span>
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

      <section class="pcb-tree-wrap" v-if="treeNodes.length > 0">
        <header class="pcb-tree-wrap__head">
          <span class="pcb-tree-wrap__label">
            <Term term="PCB" mode="label">PCBs</Term>
            <span class="pcb-tree-wrap__hint">{{ t('sections.processes.pcbTreeHint') }}</span>
          </span>
          <button
            v-if="initPID > 0"
            type="button"
            class="pcb-tree-wrap__btn"
            :title="prefs.showInit ? t('panels.processes.hideInit') : t('panels.processes.showInit')"
            :aria-pressed="prefs.showInit"
            @click="prefs.setShowInit(!prefs.showInit)"
          >
            <Eye v-if="prefs.showInit" :size="13" :stroke-width="2" />
            <EyeOff v-else :size="13" :stroke-width="2" />
            <span>{{ prefs.showInit ? t('panels.processes.hideInit') : t('panels.processes.showInit') }}</span>
          </button>
        </header>
        <div class="pcb-tree">
          <article
            v-for="node in treeNodes"
            :key="node.proc.pid"
            :data-narrator-anchor="`pcb-${node.proc.pid}`"
            :class="[
              'pcb-card',
              stateClass(node.proc.state),
              {
                'pcb-card--current': coreOfPid(node.proc.pid) !== null,
                'pcb-card--flash': cpu.flashingPIDs.has(node.proc.pid),
                'pcb-card--child': node.depth > 0,
                'pcb-card--thread': node.isThread,
              },
            ]"
            :style="node.depth > 0 ? { marginLeft: (node.depth * 20) + 'px' } : undefined"
          >
            <header class="pcb-card__head" :title="stateTitle(node.proc)">
              <Term term="PID" mode="label" class="pcb-card__pid">{{ node.isThread ? 'T' : 'P' }}{{ node.proc.pid }}</Term>
              <span class="pcb-card__name">{{ node.proc.name }}</span>
              <span class="pcb-card__head-spacer" />
              <span
                v-if="coreOfPid(node.proc.pid) !== null
                      && cpu.isCoreVisible(coreOfPid(node.proc.pid) as number)"
                class="pcb-card__core"
                :style="{ color: `var(--core-${(coreOfPid(node.proc.pid) as number) % 4})` }"
                :title="t('panels.processes.coreRunning', { core: coreOfPid(node.proc.pid) ?? 0 })"
              >C{{ coreOfPid(node.proc.pid) }}</span>
              <Term :term="node.proc.state" mode="label" class="pcb-card__state">{{ node.proc.state }}</Term>
              <span
                v-if="node.isThread"
                class="pcb-card__chip pcb-card__chip--thread"
                :title="t('panels.processes.threadChipTitle', { tgid: node.proc.tgid })"
              >TGID {{ node.proc.tgid }}</span>
              <span
                v-else-if="(node.proc.parent_pid ?? 0) > 0"
                class="pcb-card__chip pcb-card__chip--parent"
                :title="t('panels.processes.parentChipTitle', { pid: node.proc.parent_pid })"
              >↑P{{ node.proc.parent_pid }}</span>
            </header>

            <div class="pcb-card__meta">
              <span class="pcb-card__addr" :title="t('sections.processes.codeBaseHint')">@ {{ hex(node.proc.code_base, 8) }}</span>
              <span class="pcb-card__regs-src" :title="t('panels.processes.regsValuesAre', { desc: t('panels.processes.regsHintSaved') })">
                {{ t('panels.processes.regsSrcSaved') }}
              </span>
              <span
                v-if="node.proc.state === 'ZOMBIE' && node.proc.exit_code !== undefined"
                class="pcb-card__exit"
                :title="t('panels.processes.exitCodeTitle')"
              >exit={{ node.proc.exit_code }}</span>
            </div>

            <div v-if="node.proc.state === 'BLOCKED' && node.proc.blocked_on" class="pcb-card__blocked-banner">
              <Hourglass :size="12" :stroke-width="2" />
              <span>{{ t('panels.processes.blockedWaiting', { reason: node.proc.blocked_on }) }}</span>
            </div>

            <ul class="pcb-card__regs">
              <li v-for="r in SHOWN_REGS" :key="r" :title="regCellTitle(r, node.proc)">
                <Term :term="r" mode="label" class="pcb-card__reg-name">{{ r }}</Term>
                <span class="pcb-card__reg-val">{{ hex(node.proc.regs[r] ?? 0, 8) }}</span>
              </li>
            </ul>

            <div class="pcb-card__flags">
              <span
                class="pcb-card__flags-label"
                :title="t('panels.processes.eflagsHint')"
                @click="cpu.openGlossary('EFLAGS')"
              >{{ t('panels.processes.eflagsTitle') }}</span>
              <span
                v-for="f in EFLAGS_BITS"
                :key="f.name"
                class="pcb-flag"
                :class="{ 'pcb-flag--on': flagOn(node.proc.regs.EFLAGS ?? 0, f.mask) }"
                :title="flagChipTitle(f.name, flagOn(node.proc.regs.EFLAGS ?? 0, f.mask))"
              >{{ f.name }}</span>
            </div>
          </article>
        </div>
      </section>
      <div v-else class="pcb-tree__empty">
        {{ t('sections.processes.pcbEmpty') }}
      </div>

      <div class="procs-page__grid">
        <section class="procs-page__panel">
          <header class="procs-page__panel-head">
            <h3><Term term="Round-Robin" mode="title">{{ t('sections.processes.schedulerTitle') }}</Term></h3>
            <span class="procs-page__panel-hint">
              <Term term="Round-Robin">Round-Robin</Term> ·
              <Term term="Quantum" as="code">quantum={{ cpu.timerQuantum }}</Term>
            </span>
          </header>
          <div class="sched-row">
            <Term term="Ready queue" mode="label" class="sched-row__label">READY</Term>
            <div class="sched-row__pills">
              <span v-if="cpu.readyQueue.length === 0" class="sched-row__empty">{{ t('sections.processes.queueEmpty') }}</span>
              <span v-for="pid in cpu.readyQueue" :key="`r-${pid}`" class="sched-pill sched-pill--ready">P{{ pid }}</span>
            </div>
          </div>
          <div class="sched-row">
            <Term term="Waiting queue" mode="label" class="sched-row__label">WAIT</Term>
            <div class="sched-row__pills">
              <span v-if="cpu.waitQueue.length === 0" class="sched-row__empty">{{ t('sections.processes.queueEmpty') }}</span>
              <span
                v-for="w in cpu.waitQueue"
                :key="`w-${w.pid}`"
                class="sched-pill sched-pill--wait"
                :title="t('sections.processes.waitingTooltip', { reason: w.reason })"
              >
                P{{ w.pid }} · {{ w.reason }}
              </span>
            </div>
          </div>
        </section>

        <section class="procs-page__panel">
          <header class="procs-page__panel-head">
            <h3><Term term="Context switch" mode="title">{{ t('sections.processes.ctxSwitchesTitle') }}</Term></h3>
            <span class="procs-page__panel-hint">
              <Term term="Preemption">sched_decision</Term>
            </span>
          </header>
          <div v-if="switches.length === 0" class="ctx-feed__empty">
            {{ t('sections.processes.ctxFeedEmpty') }}
          </div>
          <ol v-else class="ctx-feed">
            <li v-for="s in switches" :key="s.id" class="ctx-feed__row">
              <span class="ctx-feed__pid">P{{ s.fromPid }}</span>
              <span class="ctx-feed__arrow">→</span>
              <span class="ctx-feed__pid ctx-feed__pid--to">P{{ s.toPid }}</span>
              <span v-if="s.reason" class="ctx-feed__reason">{{ t('sections.processes.ctxFeedReason', { reason: s.reason }) }}</span>
              <span class="ctx-feed__ready">ready=[{{ s.ready.join(',') || '∅' }}]</span>
              <span class="ctx-feed__time">{{ timeAgo(s.ts) }}</span>
            </li>
          </ol>
        </section>
      </div>

      <section class="procs-page__panel">
        <header class="procs-page__panel-head">
          <h3><Term term="Round-Robin (RR)" mode="title">{{ t('sections.processes.metricsTitle') }}</Term></h3>
          <span class="procs-page__panel-hint">{{ t('sections.processes.metricsHint') }}</span>
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
          <h3><Term term="Quantum" mode="title">{{ t('sections.processes.quantumsTitle') }}</Term></h3>
          <span class="procs-page__panel-hint">{{ t('sections.processes.quantumsHint') }}</span>
        </header>
        <QuantumBarGraph :metrics="sched.metrics" />
      </section>

      <section class="procs-page__panel">
        <header class="procs-page__panel-head">
          <h3><Term term="Context switch" mode="title">{{ t('sections.processes.timelineTitle') }}</Term></h3>
          <span class="procs-page__panel-hint">{{ t('sections.processes.timelineHint') }}</span>
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
  font-family: var(--font-display);
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
  font-family: var(--font-display);
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

.pcb-tree-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.pcb-tree-wrap__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-3);
}

.pcb-tree-wrap__label {
  display: inline-flex;
  align-items: baseline;
  gap: var(--space-3);
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 3px;
  color: var(--amber);
  text-transform: uppercase;
}

.pcb-tree-wrap__hint {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: none;
  color: var(--text-dim);
}

.pcb-tree-wrap__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: transparent;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-dim);
  padding: 4px 10px;
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}

.pcb-tree-wrap__btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.pcb-tree {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.pcb-tree__empty {
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
  border-left: 3px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  transition: border-color var(--transition-base), box-shadow var(--transition-base), background var(--transition-base);
}

.pcb-card.state-run     { border-left-color: var(--green); }
.pcb-card.state-ready   { border-left-color: #ffd24f; }
.pcb-card.state-blocked { border-left-color: #ff7a7a; }
.pcb-card.state-new     { border-left-color: var(--cyan); }
.pcb-card.state-zombie  { border-left-color: var(--text-dim); }

.pcb-card--child {
  border-left-style: dotted;
  border-left-color: var(--accent);
}

.pcb-card--current {
  background: color-mix(in oklab, var(--green) 6%, var(--surface-1));
  box-shadow: var(--shadow-glow-amber);
}

.pcb-card--flash {
  animation: pcb-card-flash 800ms ease-out;
}

@keyframes pcb-card-flash {
  0%   { background: rgba(var(--accent-rgb), 0.22); }
  100% { background: rgba(var(--accent-rgb), 0.04); }
}

.pcb-card.state-zombie {
  opacity: 0.7;
}
.pcb-card.state-zombie .pcb-card__name {
  text-decoration: line-through;
  text-decoration-color: var(--text-dim);
}

.pcb-card__head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px var(--space-3);
  padding-bottom: 4px;
  border-bottom: 1px dashed var(--border);
}

.pcb-card__pid {
  font-family: var(--font-display);
  font-size: 14px;
  letter-spacing: 2px;
  color: var(--amber);
  font-weight: normal;
  flex-shrink: 0;
}

.pcb-card__name {
  color: var(--text);
  font-size: 12px;
  font-family: 'JetBrains Mono', monospace;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pcb-card__head-spacer {
  flex: 1 1 auto;
}

.pcb-card__core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}

.pcb-card__state {
  font-size: 10px;
  letter-spacing: 2px;
  font-family: 'JetBrains Mono', monospace;
  flex-shrink: 0;
}

.pcb-card__state.state-run     { color: var(--green); }
.pcb-card__state.state-ready   { color: #ffd24f; }
.pcb-card__state.state-blocked { color: #ff7a7a; }
.pcb-card__state.state-new     { color: var(--cyan); }
.pcb-card__state.state-zombie  { color: var(--text-dim); }

.pcb-card__chip {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid var(--border);
  border-radius: 2px;
  color: var(--text-dim);
  flex-shrink: 0;
}

.pcb-card__chip--thread {
  color: var(--cyan);
  border-color: var(--cyan);
}

.pcb-card__chip--parent {
  color: var(--text-dim);
}

.pcb-card__meta {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: var(--space-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
}

.pcb-card__addr {
  color: var(--cyan);
}

.pcb-card__regs-src {
  margin-left: auto;
  font-style: italic;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.pcb-card__exit {
  color: var(--text);
  letter-spacing: 1px;
}

.pcb-card__blocked-banner {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: #ff7a7a;
  font-style: italic;
  padding: 4px 8px;
  border: 1px dashed rgba(255, 122, 122, 0.4);
  border-radius: var(--radius-sm);
  background: rgba(255, 122, 122, 0.04);
  align-self: flex-start;
}

.pcb-card__regs {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 150px), 1fr));
  gap: 3px var(--space-4);
}

.pcb-card__regs li {
  display: flex;
  justify-content: space-between;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  cursor: help;
}

.pcb-card__reg-name { color: var(--cyan); }
.pcb-card__reg-val  { color: var(--yellow, #ffd24f); }

.pcb-card__flags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding-top: 4px;
  border-top: 1px dashed var(--border);
}

.pcb-card__flags-label {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
  transition: color 0.15s ease, text-shadow 0.15s ease;
}

.pcb-card__flags-label:hover {
  color: var(--amber);
  text-shadow: 0 0 6px rgba(var(--accent-rgb), 0.4);
}

.pcb-flag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid var(--border);
  border-radius: 2px;
  color: var(--text-dim);
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
  font-family: var(--font-display);
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
