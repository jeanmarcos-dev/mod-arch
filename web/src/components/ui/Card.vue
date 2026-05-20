<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    surface?: 1 | 2 | 3;
    elevation?: 0 | 1 | 2;
    padded?: boolean;
  }>(),
  { surface: 1, elevation: 0, padded: true },
);
</script>

<template>
  <section
    :class="['card', `card--surface-${surface}`, `card--elev-${elevation}`, { 'card--padded': padded }]"
  >
    <header v-if="title || $slots.header" class="card__header">
      <h2 v-if="title" class="card__title">{{ title }}</h2>
      <div v-if="$slots.header" class="card__header-actions"><slot name="header" /></div>
    </header>
    <div class="card__body"><slot /></div>
    <footer v-if="$slots.footer" class="card__footer"><slot name="footer" /></footer>
  </section>
</template>

<style scoped>
.card {
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
}

.card--surface-1 { background: rgba(13, 18, 24, 0.65); }
.card--surface-2 { background: rgba(19, 26, 34, 0.85); }
.card--surface-3 { background: var(--surface-3); }

.card--elev-1 { box-shadow: var(--shadow-elevation-1); }
.card--elev-2 { box-shadow: var(--shadow-elevation-2); }

.card--padded .card__body { padding: var(--space-4); }
.card--padded .card__header { padding: var(--space-3) var(--space-4); }
.card--padded .card__footer { padding: var(--space-3) var(--space-4); }

.card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  border-bottom: 1px dashed var(--border);
}

.card__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 12px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
  font-weight: normal;
}

.card__header-actions {
  display: flex;
  gap: var(--space-2);
}

.card__footer {
  border-top: 1px dashed var(--border);
}
</style>
