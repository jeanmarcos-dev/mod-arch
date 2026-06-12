<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';
import { useLockGraph } from '../../composables/useLockGraph';
import { useRaceDetector } from '../../composables/useRaceDetector';
import { useSchedulerMetrics } from '../../composables/useSchedulerMetrics';
import { pidColor } from '../../lib/pidColors';
import WaitGraph from '../../components/concurrency/WaitGraph.vue';
import LockBay from '../../components/concurrency/LockBay.vue';
import SyncTimeline from '../../components/concurrency/SyncTimeline.vue';
import RaceLens from '../../components/concurrency/RaceLens.vue';
import Term from '../../components/glossary/Term.vue';

const { t } = useI18n();
const cpu = useCpuStore();

onMounted(() => { cpu.connect(); });

// Single shared instances: every panel below consumes these, so the view
// holds exactly one subscription per composable.
const lockGraph = useLockGraph();
const detector = useRaceDetector();
const sched = useSchedulerMetrics();

// Latch the deadlock instant in the shared metrics' relative tick space so
// the timeline annotation lands on the same x axis its segments use. The
// latch pins the FIRST deadlock until loaded/reset (intentional: a deadlock
// never resolves on its own). Immediate covers a deadlock that already
// existed when the section mounted — the store persists across navigation.
const deadlockAtTick = ref<number | null>(null);
watch(lockGraph.hasDeadlock, (now, prev) => {
  if (now && !prev && deadlockAtTick.value === null) {
    deadlockAtTick.value = sched.currentTickRel.value;
  }
}, { immediate: true });

type Tab = 'race' | 'graph';
const manualTab = ref<Tab | null>(null);
const hasLocks = computed(() => cpu.mutexes.length > 0 || cpu.semaphores.length > 0);
// Demo-agnostic default: locks on stage mean the wait graph is the story;
// otherwise the race lens is (it covers both the tracked and the empty case).
const activeTab = computed<Tab>(() => manualTab.value ?? (hasLocks.value ? 'graph' : 'race'));
function selectTab(tab: Tab) { manualTab.value = tab; }

const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
  if (ev.type === 'loaded' || ev.type === 'reset') {
    deadlockAtTick.value = null;
    manualTab.value = null;
  }
});
onBeforeUnmount(unsubscribe);

const singleCoreHint = computed(() => cpu.numCores < 2);

const cores = computed(() => {
  if (cpu.cores.length > 0) return cpu.cores.filter(c => cpu.isCoreVisible(c.id));
  return [{ id: 0, mode: cpu.mode, halted: cpu.halted, current_pid: cpu.currentPID }];
});

// Chip status: visible label + the glossary term backing it.
function coreStatus(c: { halted: boolean; mode: number; current_pid: number }): { label: string; term: string } {
  if (c.halted) return { label: 'HALT', term: 'HLT' };
  if (c.current_pid === 0) return { label: 'IDLE', term: 'Ready queue' };
  return c.mode === 0
    ? { label: 'KERNEL', term: 'KERNEL MODE (PL0)' }
    : { label: 'USER', term: 'USER MODE (PL3)' };
}
</script>

<template>
  <section class="conc" data-pizarra-anchor="view-concurrencia">
    <header class="conc__head">
      <h1><Term term="Critical section" mode="title">{{ t('sections.concurrency.title') }}</Term></h1>
      <div class="conc__hint" v-if="singleCoreHint">
        {{ t('sections.concurrency.singleCoreHintPrefix') }}<strong>{{ cpu.numCores }}</strong>{{ t('sections.concurrency.singleCoreHintBody') }}<code>{{ t('sections.concurrency.singleCoreHintEnv') }}</code>{{ t('sections.concurrency.singleCoreHintSuffix') }}
      </div>
      <div class="conc__hint" v-else>
        {{ t('sections.concurrency.multiCoreHintPrefix') }}<strong>{{ cpu.numCores }}</strong>{{ t('sections.concurrency.multiCoreHintSuffix') }}
      </div>
    </header>

    <div class="conc__main">
      <section class="conc__panel conc__scenario">
        <div class="conc__tabs" role="tablist" :aria-label="t('sections.concurrency.view.scenarioTitle')">
          <button
            id="conc-tab-race"
            type="button"
            role="tab"
            class="conc__tab"
            :class="{ 'conc__tab--active': activeTab === 'race' }"
            :aria-selected="activeTab === 'race'"
            aria-controls="conc-stage"
            @click="selectTab('race')"
          >{{ t('sections.concurrency.view.tabRace') }}</button>
          <button
            id="conc-tab-graph"
            type="button"
            role="tab"
            class="conc__tab"
            :class="{ 'conc__tab--active': activeTab === 'graph' }"
            :aria-selected="activeTab === 'graph'"
            aria-controls="conc-stage"
            @click="selectTab('graph')"
          >{{ t('sections.concurrency.view.tabGraph') }}</button>
        </div>
        <div
          id="conc-stage"
          class="conc__stage"
          role="tabpanel"
          :aria-labelledby="activeTab === 'race' ? 'conc-tab-race' : 'conc-tab-graph'"
        >
          <RaceLens v-if="activeTab === 'race'" :detector="detector" />
          <WaitGraph
            v-else
            :nodes="lockGraph.nodes.value"
            :edges="lockGraph.edges.value"
            :cycle="lockGraph.deadlockCycle.value"
          />
        </div>
      </section>

      <aside class="conc__panel conc__locks">
        <h2 class="conc__panel-title"><Term term="Mutex" mode="title">{{ t('sections.concurrency.view.locksBoardTitle') }}</Term></h2>
        <LockBay :cycle-ids="lockGraph.deadlockCycle.value ?? undefined" />
      </aside>
    </div>

    <section class="conc__panel conc__timeline">
      <h2 class="conc__panel-title"><Term term="BLOCKED" mode="title">{{ t('sections.concurrency.view.timelineTitle') }}</Term></h2>
      <SyncTimeline :metrics="sched" :deadlock-at-tick="deadlockAtTick" />
    </section>

    <div class="conc__cores" role="group" :aria-label="t('sections.concurrency.view.coresAria')">
      <span
        v-for="c in cores"
        :key="c.id"
        class="core-chip"
        :class="{ 'core-chip--halted': c.halted, 'core-chip--kernel': !c.halted && c.current_pid !== 0 && c.mode === 0 }"
        :data-narrator-anchor="`cpu-${c.id}`"
      >
        <span class="core-chip__id">CORE {{ c.id }}</span>
        <span class="core-chip__sep" aria-hidden="true">·</span>
        <code
          class="core-chip__pid"
          :style="c.current_pid !== 0 ? { color: pidColor(c.current_pid) } : undefined"
        >{{ c.current_pid !== 0 ? `P${c.current_pid}` : '—' }}</code>
        <span class="core-chip__sep" aria-hidden="true">·</span>
        <Term :term="coreStatus(c).term" mode="label" class="core-chip__status">{{ coreStatus(c).label }}</Term>
      </span>
    </div>

    <details class="conc__procs">
      <summary><Term term="PCB" mode="label">{{ t('sections.concurrency.procStateTitle') }}</Term></summary>
      <table>
        <thead>
          <tr>
            <th><Term term="PID" mode="label">PID</Term></th>
            <th>{{ t('sections.concurrency.colName') }}</th>
            <th>{{ t('sections.concurrency.colState') }}</th>
            <th><Term term="BLOCKED" mode="label">{{ t('sections.concurrency.colBlockedOn') }}</Term></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in cpu.procs" :key="p.pid">
            <td>P{{ p.pid }}</td>
            <td>{{ p.name }}</td>
            <td>
              <Term
                :term="p.state"
                mode="label"
                :class="['state-tag', `state-${p.state.toLowerCase()}`]"
              >{{ p.state }}</Term>
            </td>
            <td><code v-if="p.blocked_on">{{ p.blocked_on }}</code><span v-else class="muted">—</span></td>
          </tr>
        </tbody>
      </table>
    </details>
  </section>
</template>

<style scoped>
.conc {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4);
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.conc__head { display: flex; flex-direction: column; gap: var(--space-3); }
.conc__head h1 {
  font-family: var(--font-display);
  letter-spacing: 4px;
  font-size: 18px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  margin: 0;
}

.conc__hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-dim);
  border-left: 3px solid var(--amber);
  padding: var(--space-2) var(--space-3);
  background: rgba(var(--accent-rgb), 0.05);
}
.conc__hint code {
  background: rgba(var(--accent-rgb), 0.1);
  padding: 0 4px;
  border-radius: var(--radius-sm);
  color: var(--amber);
}

.conc__main {
  display: grid;
  grid-template-columns: 3fr 2fr;
  gap: var(--space-4);
  align-items: start;
  min-height: 0;
}

.conc__panel {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}

.conc__panel-title {
  font-size: 11px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
  margin: 0 0 var(--space-2);
  font-family: var(--font-display);
}

.conc__tabs {
  display: flex;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  border-bottom: 1px dashed var(--border);
  padding-bottom: var(--space-2);
}

.conc__tab {
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
  background: transparent;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 4px 10px;
  cursor: pointer;
  transition: color var(--transition-base), border-color var(--transition-base), box-shadow var(--transition-base);
}
.conc__tab:hover { color: var(--text); }
.conc__tab--active {
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}

.conc__locks { min-width: 0; }

.conc__cores {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.core-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface-1);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  transition: border-color var(--transition-base), opacity var(--transition-base);
}
.core-chip--halted { opacity: 0.55; }
.core-chip--kernel { border-color: var(--amber); box-shadow: var(--shadow-glow-amber); }
.core-chip__id { color: var(--amber); font-weight: bold; }
.core-chip__sep { color: var(--text-muted); }
.core-chip__pid { color: var(--text-muted); }
.core-chip__status { color: var(--text-dim); font-size: 9px; }
.core-chip--kernel .core-chip__status { color: var(--amber); }

.conc__procs {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-2) var(--space-3);
}
.conc__procs summary {
  cursor: pointer;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-muted);
}
.conc__procs summary:hover { color: var(--cyan); }

.conc__procs table {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  margin-top: var(--space-2);
}
.conc__procs th {
  text-align: left;
  font-weight: normal;
  color: var(--text-muted);
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  padding: 4px 6px;
  border-bottom: 1px dashed var(--border);
}
.conc__procs td { padding: 4px 6px; border-bottom: 1px dashed rgba(255, 255, 255, 0.04); }
.conc__procs code { background: rgba(var(--accent-rgb), 0.1); padding: 0 4px; border-radius: var(--radius-sm); }
.conc__procs .muted { color: var(--text-muted); }

.state-tag {
  display: inline-block;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  font-size: 9px;
  letter-spacing: 1px;
  border: 1px solid var(--border);
}
.state-run { color: var(--amber); border-color: var(--amber); }
.state-ready { color: var(--cyan, #80d6ff); }
.state-blocked { color: var(--yellow, #ffd54f); border-color: var(--yellow, #ffd54f); }
.state-zombie { color: var(--text-muted); }

@media (max-width: 768px) {
  .conc__main { grid-template-columns: 1fr; }
}
</style>
