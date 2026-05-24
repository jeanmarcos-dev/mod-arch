<script setup lang="ts">
import { computed } from 'vue';
import { useCpuStore } from '../../stores/cpu';

// CoreLegend doubles as a color legend and a quick filter. It mirrors the
// state of SimStatusRow's C0/C1/... tabs so either control reads the same
// selectedCoreIds set. Visible only when ≥2 cores exist — single-core
// sessions have no legend to convey.
//
// Click toggles visibility; double click focuses one core exclusively. The
// readonly flag (set by the viewer shell) disables interaction so spectators
// follow whatever the host is showing.
withDefaults(defineProps<{ readonly?: boolean }>(), { readonly: false });

const cpu = useCpuStore();

const cores = computed(() => cpu.cores.slice().sort((a, b) => a.id - b.id));
const shouldRender = computed(() => cores.value.length >= 2);

function onClick(id: number, readonly: boolean) {
  if (readonly) return;
  cpu.toggleCoreSelection(id);
}
function onDblClick(id: number, readonly: boolean) {
  if (readonly) return;
  cpu.selectOnlyCore(id);
}
</script>

<template>
  <div
    v-if="shouldRender"
    class="core-legend"
    role="group"
    aria-label="Leyenda de colores por núcleo"
  >
    <span class="core-legend__intro">Núcleos:</span>
    <button
      v-for="c in cores"
      :key="c.id"
      type="button"
      :class="['core-legend__chip', { 'core-legend__chip--hidden': !cpu.isCoreVisible(c.id) }]"
      :style="{ '--core-tint': `var(--core-${c.id % 4})`, '--core-tint-fg': `var(--core-fg)` }"
      :aria-pressed="cpu.isCoreVisible(c.id)"
      :disabled="readonly"
      :title="readonly
        ? `Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'}`
        : `Muestre u oculte la actividad del núcleo C${c.id}. Doble click para enfocarlo en exclusiva.`"
      @click="onClick(c.id, readonly)"
      @dblclick="onDblClick(c.id, readonly)"
    >
      <span class="core-legend__swatch" aria-hidden="true">C{{ c.id }}</span>
      <span class="core-legend__label">Núcleo {{ c.id }}</span>
    </button>
  </div>
</template>

<style scoped>
.core-legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 4px var(--space-3);
  font-size: 10px;
}

.core-legend__intro {
  font-family: var(--font-display);
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-size: 9px;
}

.core-legend__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px 2px 2px;
  height: 22px;
  border: 1px solid color-mix(in oklab, var(--core-tint, var(--amber)) 50%, transparent);
  background: color-mix(in oklab, var(--core-tint, var(--amber)) 10%, transparent);
  color: var(--text);
  border-radius: var(--radius-pill, 999px);
  cursor: pointer;
  font: inherit;
  letter-spacing: 0.5px;
  transition: background var(--transition-fast), border-color var(--transition-fast), opacity var(--transition-fast);
}
.core-legend__chip:hover:not(:disabled) {
  background: color-mix(in oklab, var(--core-tint, var(--amber)) 18%, transparent);
  border-color: var(--core-tint, var(--amber));
}
.core-legend__chip:focus { outline: none; }
.core-legend__chip:focus-visible {
  outline: 2px solid var(--core-tint, var(--amber));
  outline-offset: 2px;
}
.core-legend__chip:disabled {
  cursor: default;
}

/* Hidden state: the chip stays visible (it's the only way to bring the core
   back) but its swatch dims so the user can read "this core is currently off". */
.core-legend__chip--hidden {
  opacity: 0.55;
}
.core-legend__chip--hidden .core-legend__swatch {
  background: var(--surface-2);
  color: var(--text-muted);
  border-color: var(--border);
}

.core-legend__swatch {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 18px;
  padding: 0 6px;
  background: var(--core-tint, var(--amber));
  color: var(--core-tint-fg, var(--core-fg, #0e0e10));
  border: 1px solid var(--core-tint, var(--amber));
  border-radius: var(--radius-sm);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
}

.core-legend__label {
  font-size: 10px;
  color: var(--text);
}

@media (max-width: 600px) {
  .core-legend__label { display: none; }
}
</style>
