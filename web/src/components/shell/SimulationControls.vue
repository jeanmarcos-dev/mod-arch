<script setup lang="ts">
// SimulationControls is the single source of truth for "load + step + run +
// reset" across every SIMULACIÓN route. It lives in the shell topbar so the
// pickers and buttons stay in the exact same place when the user jumps
// between Vista general, Memoria, Procesos, etc. The state (selected demo,
// run status, halted, viewer-mode) is read from the shared cpu store.

import ProgramPicker from '../ProgramPicker.vue';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();

function onLoad() {
  if (!cpu.selectedProgram) return;
  cpu.load(cpu.selectedProgram);
}
</script>

<template>
  <div class="sim-controls" :aria-disabled="cpu.isViewer()">
    <ProgramPicker class="sim-controls__picker" />

    <div class="sim-controls__btns">
      <button
        type="button"
        class="sim-btn"
        :disabled="!cpu.selectedProgram || cpu.isViewer()"
        title="Cargar el demo seleccionado"
        @click="onLoad"
      >▸ LOAD</button>

      <button
        type="button"
        class="sim-btn"
        :disabled="cpu.halted || cpu.running || cpu.isViewer()"
        title="Ejecutar una instrucción"
        @click="cpu.step()"
      >⏵ STEP</button>

      <button
        v-if="!cpu.running"
        type="button"
        class="sim-btn"
        :disabled="cpu.halted || cpu.isViewer()"
        title="Ejecutar continuamente"
        @click="cpu.run(cpu.stepHz)"
      >⏩ RUN</button>
      <button
        v-else
        type="button"
        class="sim-btn"
        :disabled="cpu.isViewer()"
        title="Pausar el modo continuo"
        @click="cpu.pause()"
      >⏸ PAUSE</button>

      <button
        type="button"
        class="sim-btn sim-btn--reset"
        :disabled="cpu.isViewer()"
        title="Resetear kernel, procesos y log"
        @click="cpu.reset()"
      >↻ RESET</button>
    </div>
  </div>
</template>

<style scoped>
.sim-controls {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  flex: 1;
}

.sim-controls__picker {
  flex: 0 1 240px;
  min-width: 160px;
}

.sim-controls__btns {
  display: flex;
  gap: var(--space-1);
}

.sim-btn {
  font-family: inherit;
  background: rgba(255, 179, 71, 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 5px 10px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
}

.sim-btn:hover:not(:disabled) {
  background: rgba(255, 179, 71, 0.18);
  box-shadow: var(--shadow-glow-amber);
}

.sim-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.sim-btn--reset {
  border-color: rgba(255, 58, 58, 0.35);
  color: var(--red-soft);
}

.sim-btn--reset:hover:not(:disabled) {
  background: rgba(255, 58, 58, 0.12);
  box-shadow: var(--shadow-glow-red);
}

@media (max-width: 900px) {
  .sim-controls__picker { display: none; }
}
</style>
