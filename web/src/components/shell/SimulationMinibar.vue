<script setup lang="ts">
import { computed } from 'vue';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();

const modeLabel = computed(() => (cpu.mode === 0 ? 'kernel' : 'user'));
const halted = computed(() => cpu.halted);

function fmt(v: number | undefined): string {
  if (typeof v !== 'number') return '—';
  return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0');
}
</script>

<template>
  <div class="minibar" role="status" aria-label="Estado resumido de la simulación">
    <div class="minibar__cell">
      <span class="minibar__label">PID</span>
      <span class="minibar__value">{{ cpu.currentPID }}</span>
    </div>
    <div class="minibar__cell">
      <span class="minibar__label">Modo</span>
      <span :class="['minibar__value', `minibar__value--mode-${modeLabel}`]">
        {{ modeLabel }}
      </span>
    </div>
    <div class="minibar__cell">
      <span class="minibar__label">EIP</span>
      <span class="minibar__value minibar__value--mono">{{ fmt(cpu.regs.EIP) }}</span>
    </div>
    <div class="minibar__cell">
      <span class="minibar__label">EAX</span>
      <span class="minibar__value minibar__value--mono">{{ fmt(cpu.regs.EAX) }}</span>
    </div>
    <div class="minibar__cell">
      <span class="minibar__label">ESP</span>
      <span class="minibar__value minibar__value--mono">{{ fmt(cpu.regs.ESP) }}</span>
    </div>
    <div v-if="halted" class="minibar__cell minibar__cell--halt">
      <span class="minibar__value minibar__value--halt">HALT</span>
    </div>
  </div>
</template>

<style scoped>
.minibar {
  display: flex;
  align-items: stretch;
  gap: 0;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
  font-size: 11px;
  flex-shrink: 0;
  overflow-x: auto;
}

.minibar__cell {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: var(--space-2) var(--space-3);
  border-right: 1px dashed var(--border);
  min-width: 92px;
}

.minibar__cell:last-child { border-right: none; }

.minibar__label {
  font-size: 9px;
  letter-spacing: 2px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.minibar__value {
  color: var(--text);
  letter-spacing: 0.5px;
}

.minibar__value--mono {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
}

.minibar__value--mode-user {
  color: var(--green);
  text-shadow: 0 0 6px var(--green-glow);
}

.minibar__value--mode-kernel {
  color: var(--red-soft);
  text-shadow: 0 0 6px rgba(255, 122, 122, 0.5);
}

.minibar__cell--halt {
  margin-left: auto;
  border-left: 1px solid var(--red);
}

.minibar__value--halt {
  color: var(--red);
  letter-spacing: 2px;
  font-weight: bold;
}
</style>
