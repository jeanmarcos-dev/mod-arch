<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, onMounted, onBeforeUnmount, ref } from 'vue';
import { useRoute } from 'vue-router';
import { ChevronDown } from '@lucide/vue';

const cpu = useCpuStore();
const route = useRoute();
const open = ref(false);
const root = ref<HTMLElement | null>(null);

const selectedProgram = computed(() =>
  cpu.programs.find(p => p.name === cpu.selectedProgram)
);

// Routes can constrain the picker to a single demo category by setting
// `meta.category` (e.g. /concurrencia → 'concurrencia'). Without it, every
// loaded demo is offered.
const visiblePrograms = computed(() => {
  const cat = typeof route.meta.category === 'string' ? route.meta.category : '';
  if (!cat) return cpu.programs;
  return cpu.programs.filter(p => (p.category ?? '') === cat);
});

function toggle() {
  open.value = !open.value;
}

function pick(name: string) {
  cpu.selectedProgram = name;
  open.value = false;
}

// Cerramos el dropdown si se clickea afuera.
function onDocClick(e: MouseEvent) {
  if (!root.value) return;
  if (!root.value.contains(e.target as Node)) {
    open.value = false;
  }
}

function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false;
}

onMounted(() => {
  document.addEventListener('click', onDocClick);
  document.addEventListener('keydown', onEsc);
});
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', onEsc);
});
</script>

<template>
  <div class="picker" ref="root">
    <button class="trigger" :class="{ open }" @click="toggle">
      <span class="sel-name">{{ selectedProgram?.name ?? '— seleccione un demo —' }}</span>
      <span v-if="selectedProgram?.procs && selectedProgram.procs.length > 1" class="sel-procs">
        {{ selectedProgram.procs.length }} proc
      </span>
      <ChevronDown :size="14" :stroke-width="2" class="arrow" />
    </button>

    <div v-if="open" class="dropdown">
      <div
        v-for="p in visiblePrograms"
        :key="p.name"
        class="opt"
        :class="{ selected: p.name === cpu.selectedProgram }"
        role="button"
        tabindex="0"
        @click="pick(p.name)"
        @keydown.enter="pick(p.name)"
        @keydown.space.prevent="pick(p.name)"
      >
        <div class="opt-head">
          <span class="opt-name">{{ p.name }}</span>
          <span v-if="p.procs && p.procs.length > 1" class="opt-procs">{{ p.procs.length }} proc</span>
        </div>
        <div class="opt-desc">{{ p.description }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.picker {
  position: relative;
  flex: 0 1 280px;
  min-width: 200px;
}

.trigger {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  box-sizing: border-box;
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.trigger:hover {
  background: rgba(var(--accent-rgb), 0.12);
  border-color: var(--amber);
}

.trigger.open {
  border-color: var(--amber);
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.3);
}

.sel-name {
  flex: 1;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sel-procs {
  font-size: 11px;
  color: var(--cyan);
  font-weight: 400;
  letter-spacing: 0;
}

.arrow {
  font-size: 12px;
  color: var(--amber-dim);
}

.dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  /* Anchor to trigger's left edge but grow past its width so descriptions
     don't squish into 5-6 lines. Clamp to viewport so we never bleed off
     the right edge on narrow screens. */
  min-width: 360px;
  max-width: calc(100vw - var(--space-4));
  max-height: 60vh;
  overflow-y: auto;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-elevation-2, 0 8px 32px rgba(0, 0, 0, 0.6));
  z-index: 1000;
  display: flex;
  flex-direction: column;
}

.opt {
  /* It's a <div role="button"> rather than <button>: native <button> ignores
     content-driven height when the inner text wraps, causing items to
     overlap. A div with role/tabindex/handlers keeps the same a11y. */
  display: block;
  width: 100%;
  text-align: left;
  background: transparent;
  border-bottom: 1px solid var(--border);
  padding: 10px 14px;
  cursor: pointer;
  font-family: 'JetBrains Mono', monospace;
  transition: background var(--transition-fast);
  text-transform: none;
  letter-spacing: 0;
  color: var(--text);
}

.opt:last-child {
  border-bottom: none;
}

.opt:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.opt:last-child {
  border-bottom: none;
}

.opt:hover {
  background: rgba(var(--accent-rgb), 0.08);
  box-shadow: none;
}

.opt.selected {
  background: rgba(var(--accent-rgb), 0.15);
}

.opt.selected .opt-name {
  color: var(--green);
}

.opt-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.opt-name {
  color: var(--amber);
  font-weight: 700;
  font-size: 12px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.opt-procs {
  font-size: 10px;
  color: var(--cyan);
  background: rgba(106, 212, 255, 0.08);
  border: 1px solid var(--cyan);
  padding: 1px 6px;
}

.opt-desc {
  color: var(--text-dim);
  font-size: 11px;
  line-height: 1.45;
  font-style: italic;
  /* No truncation: each item grows to fit its full description. The dropdown
     has max-height + overflow-y:auto, so the scroll handles long lists. */
  white-space: normal;
  word-wrap: break-word;
}

@media (max-width: 768px) {
  .picker {
    flex: 1 1 100%;
    min-width: 100%;
  }

  .trigger {
    min-height: 44px;
    font-size: 13px;
    padding: 10px 14px;
  }

  .opt {
    padding: 12px 14px;
    /* min-height: 0 ya viene del scope desktop; en mobile dejamos que
       el content decida — touch target queda asegurado por el padding. */
  }

  .opt-desc {
    font-size: 13px;
    line-height: 1.55;
  }
}
</style>
