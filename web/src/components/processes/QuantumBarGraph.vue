<script setup lang="ts">
import { computed } from 'vue';
import type { SchedulerMetricsState } from '../../composables/useSchedulerMetrics';
import { pidColor } from '../../lib/pidColors';

const props = defineProps<{
  metrics: SchedulerMetricsState;
}>();

interface Row {
  pid: number;
  name: string;
  completed: number;
  partial: number;
  total: number;
}

const rows = computed<Row[]>(() => {
  const out: Row[] = [];
  for (const pid of Object.keys(props.metrics.byPid)) {
    const s = props.metrics.byPid[Number(pid)];
    out.push({
      pid: s.pid,
      name: s.name,
      completed: s.quantumsCompleted,
      partial: s.quantumsPartial,
      total: s.quantumsCompleted + s.quantumsPartial,
    });
  }
  out.sort((a, b) => a.pid - b.pid);
  return out;
});

const maxTotal = computed(() => {
  let m = 0;
  for (const r of rows.value) if (r.total > m) m = r.total;
  return m;
});

const hasData = computed(() => rows.value.some(r => r.total > 0));

function widthPct(n: number): string {
  if (maxTotal.value === 0) return '0%';
  return `${(n / maxTotal.value) * 100}%`;
}
</script>

<template>
  <div class="quantum-bars">
    <div v-if="rows.length === 0" class="quantum-bars__empty">
      Sin procesos cargados. Use el selector del topbar para cargar una demo.
    </div>
    <div v-else-if="!hasData" class="quantum-bars__empty">
      Sin quantums consumidos aún. Avance la simulación.
    </div>
    <ul v-else class="quantum-bars__list">
      <li v-for="r in rows" :key="r.pid" class="quantum-bars__row">
        <div class="quantum-bars__pid" :style="{ color: pidColor(r.pid) }">
          P{{ r.pid }}
        </div>
        <div class="quantum-bars__track">
          <div
            v-if="r.completed > 0"
            class="quantum-bars__seg quantum-bars__seg--ok"
            :style="{ width: widthPct(r.completed), background: pidColor(r.pid) }"
            :title="`${r.completed} quantums completos`"
          />
          <div
            v-if="r.partial > 0"
            class="quantum-bars__seg quantum-bars__seg--partial"
            :style="{ width: widthPct(r.partial) }"
            :title="`${r.partial} quantums parciales (bloqueo/exit)`"
          />
        </div>
        <div class="quantum-bars__total">
          <span class="quantum-bars__total-num">{{ r.total }}</span>
          <span v-if="r.partial > 0" class="quantum-bars__total-split">
            ({{ r.completed }}+{{ r.partial }})
          </span>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.quantum-bars {
  width: 100%;
}

.quantum-bars__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
  text-align: center;
}

.quantum-bars__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.quantum-bars__row {
  display: grid;
  grid-template-columns: 48px 1fr 90px;
  align-items: center;
  gap: var(--space-3);
}

.quantum-bars__pid {
  font-family: 'Major Mono Display', monospace;
  font-size: 12px;
  letter-spacing: 2px;
  text-align: right;
}

.quantum-bars__track {
  display: flex;
  height: 16px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.quantum-bars__seg {
  height: 100%;
  transition: width var(--transition-base);
}

.quantum-bars__seg--ok {
  /* colour is applied inline via the pid palette so each PID is distinct */
}

.quantum-bars__seg--partial {
  background: repeating-linear-gradient(
    45deg,
    rgba(255, 255, 255, 0.08),
    rgba(255, 255, 255, 0.08) 4px,
    rgba(255, 255, 255, 0.16) 4px,
    rgba(255, 255, 255, 0.16) 8px
  );
}

.quantum-bars__total {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text);
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.1;
}

.quantum-bars__total-num {
  font-size: 13px;
  color: var(--amber);
  font-weight: 700;
}

.quantum-bars__total-split {
  font-size: 9.5px;
  color: var(--text-dim);
  letter-spacing: 0.5px;
}
</style>
