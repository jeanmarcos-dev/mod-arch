<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { usePreferencesStore, type PresentationMode } from '../../stores/preferences';

withDefaults(
  defineProps<{
    /** Section title shown next to the breadcrumb chevron. */
    crumb?: string;
  }>(),
  {},
);

const route = useRoute();
const prefs = usePreferencesStore();

const sectionTitle = computed(() => {
  if (typeof route.meta.title === 'string') return route.meta.title;
  return '';
});

const modes: { id: PresentationMode; label: string; help: string }[] = [
  { id: 'compact', label: 'Compacto', help: 'Densidad alta, sin animaciones' },
  { id: 'pedagogic', label: 'Pedagógico', help: 'Animaciones y tooltips' },
  { id: 'projector', label: 'Demo', help: 'Tamaño grande, ritmo lento' },
];
</script>

<template>
  <header class="topbar">
    <div class="topbar__crumb" aria-label="Ubicación">
      <span class="topbar__brand">MODOS-CPU</span>
      <span class="topbar__sep" aria-hidden="true">›</span>
      <span class="topbar__section">{{ crumb ?? sectionTitle }}</span>
    </div>

    <div class="topbar__actions">
      <slot name="actions" />

      <div class="topbar__modes" role="radiogroup" aria-label="Modo de presentación">
        <button
          v-for="m in modes"
          :key="m.id"
          type="button"
          role="radio"
          :aria-checked="prefs.mode === m.id"
          :class="['topbar__mode', { 'topbar__mode--on': prefs.mode === m.id }]"
          :title="m.help"
          @click="prefs.setMode(m.id)"
        >
          {{ m.label }}
        </button>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: 0 var(--space-4);
  height: 48px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.topbar__crumb {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
}

.topbar__brand {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 12px;
}

.topbar__sep {
  color: var(--text-muted);
}

.topbar__section {
  color: var(--text);
  letter-spacing: 1px;
  text-transform: uppercase;
  font-size: 11px;
}

.topbar__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.topbar__modes {
  display: flex;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.topbar__mode {
  background: transparent;
  border: none;
  border-right: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  padding: 6px 10px;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.topbar__mode:last-child {
  border-right: none;
}

.topbar__mode:hover {
  background: rgba(255, 179, 71, 0.08);
  color: var(--amber);
}

.topbar__mode--on {
  background: rgba(255, 179, 71, 0.18);
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}

@media (max-width: 768px) {
  .topbar__brand { display: none; }
  .topbar__mode { padding: 6px 7px; font-size: 9px; }
}
</style>
