<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    items: Array<{ name: string; count: number }>;
    emptyLabel: string;
    limit?: number;
    labelFor?: (name: string) => string;
  }>(),
  { limit: 6, labelFor: undefined },
);

const rows = computed(() => {
  const top = [...props.items].sort((a, b) => b.count - a.count).slice(0, props.limit);
  const max = Math.max(1, ...top.map((r) => r.count));
  return top.map((r) => ({
    key: r.name,
    label: props.labelFor ? props.labelFor(r.name) : r.name,
    count: r.count,
    pct: Math.round((r.count / max) * 100),
  }));
});
</script>

<template>
  <ul v-if="rows.length" class="bars">
    <li v-for="row in rows" :key="row.key" class="bars__row" data-testid="live-bar-row">
      <span class="bars__label" :title="row.label">{{ row.label }}</span>
      <span class="bars__track" aria-hidden="true">
        <span class="bars__fill" :style="{ width: row.pct + '%' }" />
      </span>
      <span class="bars__count">{{ row.count }}</span>
    </li>
  </ul>
  <p v-else class="bars__empty">{{ emptyLabel }}</p>
</template>

<style scoped>
.bars {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.bars__row {
  display: grid;
  grid-template-columns: minmax(7rem, 11rem) 1fr auto;
  align-items: center;
  gap: var(--space-3);
  font-size: clamp(15px, 1.3vw, 22px);
}

.bars__label {
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bars__track {
  height: clamp(14px, 1.5vw, 24px);
  background: var(--surface-3);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.bars__fill {
  display: block;
  height: 100%;
  background: var(--accent);
  border-radius: var(--radius-sm);
  transition: width var(--transition-slow);
}

.bars__count {
  min-width: 2.5ch;
  text-align: right;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--accent);
}

.bars__empty {
  margin: 0;
  color: var(--text-muted);
  font-size: clamp(14px, 1.2vw, 20px);
}

@media (prefers-reduced-motion: reduce) {
  .bars__fill { transition: none; }
}
</style>
