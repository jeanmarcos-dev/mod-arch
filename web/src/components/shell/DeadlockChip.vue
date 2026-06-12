<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { TriangleAlert } from '@lucide/vue';

// Presentational chip; the deadlock state is derived upstream (SimBar reads it
// from useConcurrencyState) and passed in as props so this stays trivial to
// test in isolation.
const props = defineProps<{
  hasDeadlock: boolean;
  tick: number | null;
}>();

const { t } = useI18n();

const label = computed(() =>
  props.tick !== null
    ? t('simbar.deadlock.warn', { tick: props.tick })
    : t('simbar.deadlock.warnNoTick'),
);
</script>

<template>
  <router-link
    v-if="hasDeadlock"
    to="/concurrencia"
    class="deadlock-chip"
    :title="t('simbar.deadlock.hint')"
  >
    <TriangleAlert :size="13" :stroke-width="2.5" class="deadlock-chip__icon" />
    <span class="deadlock-chip__text">{{ label }}</span>
  </router-link>
</template>

<style scoped>
.deadlock-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
  height: 26px;
  padding: 0 10px;
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--red-soft);
  background: color-mix(in oklab, var(--red) 12%, transparent);
  border: 1px solid color-mix(in oklab, var(--red) 45%, transparent);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-glow-red);
  text-decoration: none;
  white-space: nowrap;
  box-sizing: border-box;
  transition: background var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
}
.deadlock-chip:hover {
  background: color-mix(in oklab, var(--red) 22%, transparent);
  border-color: var(--red);
  box-shadow: var(--shadow-glow-red), var(--shadow-glow-red);
}
.deadlock-chip__icon {
  flex-shrink: 0;
  color: var(--red);
}
.deadlock-chip__text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
