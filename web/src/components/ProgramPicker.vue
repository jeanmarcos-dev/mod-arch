<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, onMounted, onBeforeUnmount, ref } from 'vue';

const cpu = useCpuStore();
const open = ref(false);
const root = ref<HTMLElement | null>(null);

const selectedProgram = computed(() =>
  cpu.programs.find(p => p.name === cpu.selectedProgram)
);

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
      <span class="arrow">▾</span>
    </button>

    <div v-if="open" class="dropdown">
      <div
        v-for="p in cpu.programs"
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
  background: rgba(255, 179, 71, 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 6px 12px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.trigger:hover {
  background: rgba(255, 179, 71, 0.12);
  border-color: var(--amber);
}

.trigger.open {
  border-color: var(--amber);
  box-shadow: 0 0 8px rgba(255, 179, 71, 0.3);
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
  right: 0;
  /* 60vh es más conservador que 70vh y evita choques contra el borde
     inferior cuando hay 11 items con descripción larga. */
  max-height: 60vh;
  overflow-y: auto;
  background: var(--bg-1);
  border: 1px solid var(--border-strong);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
  z-index: 1000;
  display: flex;
  flex-direction: column;
}

.opt {
  /* Es un <div role="button">, no <button>. Lo cambiamos porque <button> no
     respeta la altura natural cuando el contenido es multilinea — los items
     se superponian. Un div con padding + role/tabindex/handler tiene la
     misma semantica accesible. */
  display: block;
  width: 100%;
  text-align: left;
  background: transparent;
  border-bottom: 1px solid var(--border);
  padding: 12px 14px;
  cursor: pointer;
  font-family: 'JetBrains Mono', monospace;
  transition: background 0.1s ease;
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
  background: rgba(255, 179, 71, 0.08);
  box-shadow: none;
}

.opt.selected {
  background: rgba(255, 179, 71, 0.15);
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
  font-size: 14px;
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
  color: var(--text);
  font-size: 12px;
  line-height: 1.5;
  font-style: italic;
  /* Sin truncate: cada item crece a la altura natural de su descripción.
     El dropdown tiene max-height + overflow-y:auto, así el scroll vertical
     maneja los 11 items sin recortar contenido. */
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
