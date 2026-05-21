<script setup lang="ts">
import { computed } from 'vue';
import type { SchedulerMetricsState } from '../../composables/useSchedulerMetrics';
import { useCpuStore } from '../../stores/cpu';
import { pidColor } from '../../lib/pidColors';
import Term from '../glossary/Term.vue';

const props = defineProps<{
  metrics: SchedulerMetricsState;
  throughputPerKTick: number;
  quantumMsHint: number;
  hzSim: number;
}>();

const cpu = useCpuStore();

interface Row {
  pid: number;
  name: string;
  state: string;
  tat: number | null;
  wt: number;
  rt: number | null;
  qOk: number;
  qPartial: number;
}

const rows = computed<Row[]>(() => {
  const out: Row[] = [];
  for (const pid of Object.keys(props.metrics.byPid)) {
    const s = props.metrics.byPid[Number(pid)];
    out.push({
      pid: s.pid,
      name: s.name,
      state: s.state,
      tat: s.endedAtTick !== null ? s.endedAtTick - s.createdAtTick : null,
      wt: s.totalReadyTicks,
      rt: s.firstRunAtTick !== null ? s.firstRunAtTick - s.createdAtTick : null,
      qOk: s.quantumsCompleted,
      qPartial: s.quantumsPartial,
    });
  }
  out.sort((a, b) => a.pid - b.pid);
  return out;
});

function fmt(n: number | null): string {
  return n === null ? '—' : String(n);
}

function stateClass(state: string): string {
  return `is-${state.toLowerCase()}`;
}
</script>

<template>
  <div class="sched-metrics">
    <div v-if="rows.length === 0" class="sched-metrics__empty">
      Sin procesos cargados. Use el selector del topbar para cargar una demo.
    </div>
    <table v-else class="sched-metrics__table">
      <thead>
        <tr>
          <th class="sched-metrics__col-pid">PID</th>
          <th class="sched-metrics__col-state">Estado</th>
          <th class="sched-metrics__col-num">
            <Term term="Turnaround" mode="label">TAT</Term>
          </th>
          <th class="sched-metrics__col-num">
            <Term term="Waiting time" mode="label">WT</Term>
          </th>
          <th class="sched-metrics__col-num">
            <Term term="Response time" mode="label">RT</Term>
          </th>
          <th class="sched-metrics__col-num" title="Quantums completos (preempt por timer)">Q✓</th>
          <th class="sched-metrics__col-num" title="Quantums parciales (bloqueo/exit)">Q◐</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.pid" :class="stateClass(r.state)">
          <td class="sched-metrics__pid" :style="{ color: pidColor(r.pid) }">P{{ r.pid }}</td>
          <td class="sched-metrics__state">{{ r.state }}</td>
          <td class="sched-metrics__num">{{ fmt(r.tat) }}</td>
          <td class="sched-metrics__num">{{ r.wt }}</td>
          <td class="sched-metrics__num">{{ fmt(r.rt) }}</td>
          <td class="sched-metrics__num sched-metrics__num--ok">{{ r.qOk }}</td>
          <td class="sched-metrics__num sched-metrics__num--partial">{{ r.qPartial }}</td>
        </tr>
      </tbody>
    </table>
    <footer class="sched-metrics__foot">
      <div class="sched-metrics__foot-row">
        <span>
          <Term term="Throughput" mode="label">Throughput</Term>:
          <strong>{{ throughputPerKTick.toFixed(2) }}</strong>
          procs / 1000 ticks
        </span>
        <span>
          <Term term="Quantum" mode="label">TimerQuantum</Term>:
          <strong>{{ cpu.timerQuantum }}</strong> ticks
        </span>
      </div>
      <div class="sched-metrics__foot-row sched-metrics__foot-row--hint">
        <span>Procesos completados: <strong>{{ metrics.completedProcs }}</strong></span>
        <span class="sched-metrics__hint">
          1 quantum ≈ {{ quantumMsHint.toFixed(1) }} ms simulados (HZ={{ hzSim }})
        </span>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.sched-metrics {
  width: 100%;
}

.sched-metrics__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
  text-align: center;
}

.sched-metrics__table {
  width: auto;
  min-width: 480px;
  border-collapse: collapse;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.sched-metrics__table th {
  text-align: left;
  padding: 4px 12px 4px 8px;
  border-bottom: 1px dashed var(--border);
  color: var(--text-dim);
  font-weight: normal;
  text-transform: uppercase;
  letter-spacing: 1px;
  font-size: 10px;
}

.sched-metrics__col-pid,
.sched-metrics__col-state {
  text-align: left;
}

.sched-metrics__col-num {
  text-align: right;
  min-width: 56px;
}

.sched-metrics__table tbody tr {
  transition: background var(--transition-fast);
}

.sched-metrics__table tbody tr:hover {
  background: rgba(var(--accent-rgb), 0.04);
}

.sched-metrics__table td {
  padding: 4px 8px;
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.04);
}

.sched-metrics__pid {
  font-family: 'Major Mono Display', monospace;
  font-weight: 700;
  letter-spacing: 1px;
}

.sched-metrics__state {
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-dim);
}

.sched-metrics__table tr.is-run .sched-metrics__state    { color: var(--green); }
.sched-metrics__table tr.is-ready .sched-metrics__state  { color: #ffd24f; }
.sched-metrics__table tr.is-blocked .sched-metrics__state { color: #ff7a7a; }
.sched-metrics__table tr.is-zombie .sched-metrics__state { color: var(--text-dim); }

.sched-metrics__num {
  text-align: right;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}

.sched-metrics__num--ok      { color: var(--green); }
.sched-metrics__num--partial { color: var(--text-dim); }

.sched-metrics__foot {
  margin-top: var(--space-3);
  border-top: 1px dashed var(--border);
  padding-top: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
  color: var(--text);
}

.sched-metrics__foot-row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
}

.sched-metrics__foot-row--hint {
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 0.5px;
}

.sched-metrics__foot strong {
  color: var(--amber);
}

.sched-metrics__hint {
  font-style: italic;
}
</style>
