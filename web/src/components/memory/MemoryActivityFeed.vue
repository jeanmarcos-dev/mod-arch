<script setup lang="ts">
import { computed } from 'vue';
import Card from '../ui/Card.vue';
import { fmtAddr, fmtByte, coreColorVar } from '../../lib/memoryLayout';

export interface FeedEntry {
  id: number;
  addr: number;
  value: number;
  kind: 'write' | 'read';
  coreId: number;
  pid: number;
  ts: number;
}

const props = defineProps<{
  entries: FeedEntry[];
  numCores: number;
  /** Visibility filter shared across the whole app (cpu.selectedCoreIds). */
  visibleCoreIds: Set<number>;
}>();

const emit = defineEmits<{
  (e: 'jump', addr: number): void;
}>();

// Feed shows only events from cores currently visible per the SimBar
// filter. Hiding a core in the SimBar makes its events disappear here too,
// matching the rest of the UI.
const visible = computed(() =>
  props.entries.filter(e => props.visibleCoreIds.has(e.coreId)),
);

const isFiltered = computed(() =>
  props.numCores > 1 && props.visibleCoreIds.size < props.numCores,
);

const visibleLabels = computed(() =>
  [...props.visibleCoreIds].sort((a, b) => a - b).map(id => `C${id}`).join(' · '),
);

function ageOf(ts: number): number {
  return performance.now() - ts;
}

function styleFor(coreId: number) {
  return { '--core-color': coreColorVar(coreId) };
}
</script>

<template>
  <Card title="Actividad" :elevation="1" class="feed-card">
    <p v-if="isFiltered" class="feed-card__filter-note" :title="`Cambie el filtro en la barra superior (botones de núcleos)`">
      mostrando sólo <strong>{{ visibleLabels }}</strong>
    </p>

    <p v-if="!visible.length" class="feed-card__empty">
      Cargue un demo y ejecute para ver el tráfico.
    </p>
    <ol v-else class="feed-card__list">
      <li
        v-for="e in visible"
        :key="e.id"
        :class="['feed-row', `feed-row--${e.kind}`]"
        :style="{ ...styleFor(e.coreId), opacity: Math.max(0.45, 1 - ageOf(e.ts) / 5000) }"
        role="button"
        :title="`Saltar a ${fmtAddr(e.addr)}`"
        @click="emit('jump', e.addr)"
      >
        <span class="feed-row__tag">[C{{ e.coreId }}/P{{ e.pid || '—' }}]</span>
        <span class="feed-row__kind">{{ e.kind === 'write' ? '◣ W' : '◤ R' }}</span>
        <code class="feed-row__addr">{{ fmtAddr(e.addr) }}</code>
        <code class="feed-row__val">{{ fmtByte(e.value) }}</code>
      </li>
    </ol>
  </Card>
</template>

<style scoped>
.feed-card { display: flex; flex-direction: column; min-height: 0; }
.feed-card :deep(.card__body) { display: flex; flex-direction: column; gap: var(--space-2); min-height: 0; }

.feed-card__filter-note {
  font-size: 10px;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  padding: 2px 0 var(--space-2) 0;
  border-bottom: 1px dashed var(--border);
}

.feed-card__filter-note strong {
  color: var(--amber);
  font-weight: 600;
  letter-spacing: 1px;
}

.feed-card__empty {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.feed-card__list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  overflow-y: auto;
  min-height: 0;
}

.feed-row {
  display: grid;
  grid-template-columns: auto 36px 1fr 36px;
  gap: var(--space-2);
  align-items: center;
  padding: 4px var(--space-2);
  border-left: 2px solid var(--core-color, var(--amber));
  background: var(--surface-2);
  color: var(--core-color, var(--amber));
  cursor: pointer;
  transition: opacity var(--transition-base), background var(--transition-fast);
}

.feed-row:hover {
  background: color-mix(in oklab, var(--core-color, var(--amber)) 14%, var(--surface-2));
}

.feed-row__tag {
  font-size: 9.5px;
  letter-spacing: 0.5px;
  color: var(--text-muted);
}

.feed-row--write .feed-row__kind,
.feed-row--read .feed-row__kind {
  font-weight: bold;
  letter-spacing: 1px;
}

.feed-row__addr,
.feed-row__val {
  color: inherit;
}
</style>
