<script setup lang="ts">
import { computed } from 'vue';
import type { CoreInfo } from '../../stores/cpu';
import { fmtAddr, coreColorVar, coreGlowVar } from '../../lib/memoryLayout';
import Term from '../glossary/Term.vue';

const props = defineProps<{
  core: CoreInfo;
  selected: boolean;
  /** True when the core is excluded from the global SimBar filter. */
  hiddenByFilter?: boolean;
}>();

const emit = defineEmits<{
  (e: 'select', coreId: number): void;
  (e: 'jump', addr: number): void;
}>();

const styleVars = computed(() => ({
  '--core-color': coreColorVar(props.core.id),
  '--core-glow': coreGlowVar(props.core.id),
}));

function jumpEIP() { emit('jump', props.core.regs.EIP ?? 0); }
function jumpESP() {
  const esp = props.core.regs.ESP ?? 0;
  emit('jump', Math.max(0, (esp & ~0xff) - 0x80));
}
</script>

<template>
  <div
    :class="['core-chip', {
      'core-chip--selected': selected,
      'core-chip--idle': core.current_pid === 0,
      'core-chip--halted': core.halted,
      'core-chip--muted': hiddenByFilter,
    }]"
    :style="styleVars"
    :data-narrator-anchor="`cpu-${core.id}`"
    :title="hiddenByFilter ? `C${core.id} oculto en el filtro de la barra superior — click para enfocar` : undefined"
    @click="emit('select', core.id)"
  >
    <header class="core-chip__head">
      <span class="core-chip__id">C{{ core.id }}</span>
      <span class="core-chip__pid">
        <Term term="PID" mode="label" class="core-chip__pid-k">PID</Term>
        <span class="core-chip__pid-v">{{ core.current_pid || '—' }}</span>
      </span>
      <span class="core-chip__mode" :title="core.mode === 3 ? 'User mode (PL3)' : 'Kernel mode (PL0)'">
        {{ core.mode === 3 ? 'USR' : 'KRN' }}
      </span>
    </header>

    <dl class="core-chip__regs">
      <div class="core-chip__reg core-chip__reg--clickable" @click.stop="jumpEIP" title="Saltar la ventana de memoria a EIP">
        <dt><Term term="EIP" mode="label">EIP</Term></dt>
        <dd>{{ fmtAddr(core.regs.EIP ?? 0) }}</dd>
      </div>
      <div class="core-chip__reg core-chip__reg--clickable" @click.stop="jumpESP" title="Saltar la ventana al stack del proceso">
        <dt><Term term="ESP" mode="label">ESP</Term></dt>
        <dd>{{ fmtAddr(core.regs.ESP ?? 0) }}</dd>
      </div>
      <div class="core-chip__reg">
        <dt><Term term="EAX" mode="label">EAX</Term></dt>
        <dd>{{ fmtAddr(core.regs.EAX ?? 0) }}</dd>
      </div>
      <div class="core-chip__reg">
        <dt><Term term="EBX" mode="label">EBX</Term></dt>
        <dd>{{ fmtAddr(core.regs.EBX ?? 0) }}</dd>
      </div>
    </dl>

    <div v-if="core.halted" class="core-chip__halt">HALT</div>
  </div>
</template>

<style scoped>
.core-chip {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid color-mix(in oklab, var(--core-color) 40%, transparent);
  border-radius: var(--radius-md);
  background: var(--surface-2);
  cursor: pointer;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast), transform var(--transition-fast);
}

.core-chip::before,
.core-chip::after {
  content: '';
  position: absolute;
  width: 4px;
  height: 12px;
  background: var(--core-color);
  border-radius: 1px;
  opacity: 0.55;
  top: 16px;
}
.core-chip::before { left: -2px; }
.core-chip::after  { right: -2px; }

.core-chip:hover {
  border-color: var(--core-color);
}

.core-chip--selected {
  border-color: var(--core-color);
  box-shadow: var(--core-glow);
}

.core-chip--idle:not(.core-chip--selected) {
  opacity: 0.55;
}

.core-chip--halted {
  border-style: dashed;
}

.core-chip--muted {
  opacity: 0.30;
  filter: grayscale(0.65);
  border-style: dotted;
}
.core-chip--muted::before,
.core-chip--muted::after { opacity: 0.20; }
.core-chip--muted .core-chip__id { text-shadow: none; }

.core-chip__head {
  display: grid;
  grid-template-columns: 1fr auto auto;
  align-items: baseline;
  gap: var(--space-2);
}

.core-chip__id {
  font-family: 'Major Mono Display', monospace;
  font-size: 16px;
  letter-spacing: 3px;
  color: var(--core-color);
  text-shadow: var(--core-glow);
}

.core-chip__pid {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  font-size: 10px;
}
.core-chip__pid-k { color: var(--text-muted); letter-spacing: 1px; }
.core-chip__pid-v { color: var(--text); font-family: 'JetBrains Mono', monospace; }

.core-chip__mode {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-dim);
  border: 1px solid var(--border);
  padding: 1px 4px;
  border-radius: var(--radius-sm);
}

.core-chip__regs {
  display: grid;
  gap: 3px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
}

.core-chip__reg {
  display: grid;
  grid-template-columns: 32px 1fr;
  gap: var(--space-2);
  align-items: baseline;
  padding: 1px 2px;
  border-radius: var(--radius-sm);
}

.core-chip__reg dt {
  color: var(--text-muted);
  letter-spacing: 1px;
  font-size: 9.5px;
}

.core-chip__reg dd {
  color: var(--text);
  text-align: right;
}

.core-chip__reg--clickable {
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.core-chip__reg--clickable:hover {
  background: color-mix(in oklab, var(--core-color) 12%, transparent);
}

.core-chip__reg--clickable:hover dd {
  color: var(--core-color);
}

.core-chip__halt {
  text-align: center;
  font-size: 9px;
  letter-spacing: 3px;
  color: var(--red-soft);
  border-top: 1px dashed var(--red-soft);
  padding-top: 4px;
}
</style>
