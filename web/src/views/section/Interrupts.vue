<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import ModeFlipNarrator from '../../components/narrators/ModeFlipNarrator.vue';
import InterruptNarrator from '../../components/narrators/InterruptNarrator.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();

// Mode label + colour hook for the big current-mode chip.
const modeLabel = computed(() => (cpu.mode === 3 ? 'USER' : 'KERNEL'));
const modeKey = computed<'user' | 'kernel'>(() => (cpu.mode === 3 ? 'user' : 'kernel'));

// IDT entries. We hard-code the well-known vectors the simulator implements
// so the table is meaningful even before a program runs. Each entry remembers
// the timestamp of its last activation so the row pulses briefly.
interface IDTRow {
  vec: number;
  hex: string;
  name: string;
  source: 'HW' | 'SW' | 'fault';
  lastFired: number;
}

const idt = reactive<IDTRow[]>([
  { vec: 0x00, hex: '0x00', name: 'Divide error',    source: 'fault', lastFired: 0 },
  { vec: 0x0e, hex: '0x0E', name: 'Page fault',      source: 'fault', lastFired: 0 },
  { vec: 0x20, hex: '0x20', name: 'IRQ timer',       source: 'HW',    lastFired: 0 },
  { vec: 0x21, hex: '0x21', name: 'IRQ keyboard',    source: 'HW',    lastFired: 0 },
  { vec: 0x80, hex: '0x80', name: 'System call',     source: 'SW',    lastFired: 0 },
]);

// Recent mode switches feed. We could parse the log but a structured ring
// buffer is cheaper and keeps the markup clean.
interface ModeSwitchRow {
  id: number;
  from: 'USER' | 'KERNEL';
  to: 'USER' | 'KERNEL';
  cause: string;
  ts: number;
}

const switches = ref<ModeSwitchRow[]>([]);
let nextId = 1;
const MAX_SWITCHES = 12;

const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
  if (ev.type === 'mode_switch') {
    switches.value.unshift({
      id: nextId++,
      from: ev.from === 3 ? 'USER' : 'KERNEL',
      to: ev.to === 3 ? 'USER' : 'KERNEL',
      cause: ev.cause,
      ts: Date.now(),
    });
    if (switches.value.length > MAX_SWITCHES) {
      switches.value = switches.value.slice(0, MAX_SWITCHES);
    }
  } else if (ev.type === 'irq_fire') {
    const row = idt.find(r => r.vec === ev.vector);
    if (row) row.lastFired = Date.now();
  } else if (ev.type === 'page_fault') {
    const row = idt.find(r => r.vec === 0x0e);
    if (row) row.lastFired = Date.now();
  } else if (ev.type === 'reset') {
    switches.value = [];
    for (const r of idt) r.lastFired = 0;
  }
});

onBeforeUnmount(unsubscribe);

// A row pulses for ~1.2s after firing. Force a redraw via a 100ms tick so the
// computed class flips off when the window expires.
const now = ref(Date.now());
const tickHandle = window.setInterval(() => { now.value = Date.now(); }, 200);
onBeforeUnmount(() => window.clearInterval(tickHandle));

function rowActive(r: IDTRow): boolean {
  return r.lastFired > 0 && now.value - r.lastFired < 1200;
}

function timeAgo(ts: number): string {
  const diff = Math.max(0, now.value - ts);
  if (diff < 1000) return 'ahora';
  if (diff < 60_000) return `${Math.floor(diff / 1000)}s`;
  return `${Math.floor(diff / 60_000)}m`;
}

const narratorHint = computed(() =>
  prefs.narratorsEnabled
    ? null
    : 'Active los narradores en Preferencias para ver el banner de transición.',
);
</script>

<template>
  <div class="ints-page">
    <ModeFlipNarrator />
    <InterruptNarrator />

    <div class="ints-page__content">
      <header class="ints-page__hero">
        <div
          :class="['ints-page__mode-chip', `ints-page__mode-chip--${modeKey}`]"
          data-narrator-anchor="cpu"
        >
          <span class="ints-page__mode-caption">CPU en</span>
          <span class="ints-page__mode-value">{{ modeLabel }}</span>
        </div>
        <div class="ints-page__hero-text">
          <h2 class="ints-page__title">Interrupciones</h2>
          <p class="ints-page__tagline">
            La CPU vive en <strong>USER</strong> hasta que algo la fuerza a entrar al kernel:
            un <code>int 0x80</code> (syscall), el timer del PIT, una tecla o una falla.
            Cargue un programa con syscalls (hello, syscall_test, sleep) y avance paso a paso
            para ver el salto.
          </p>
          <p v-if="narratorHint" class="ints-page__hint">{{ narratorHint }}</p>
        </div>
      </header>

      <div class="ints-page__grid">
        <section class="ints-page__panel">
          <header class="ints-page__panel-head">
            <h3>IDT</h3>
            <span class="ints-page__panel-hint">Tabla de descriptores de interrupción</span>
          </header>
          <table class="ints-table">
            <thead>
              <tr>
                <th>Vec</th>
                <th>Nombre</th>
                <th>Origen</th>
                <th>Último disparo</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="r in idt"
                :key="r.vec"
                :class="['ints-table__row', { 'ints-table__row--active': rowActive(r) }]"
                :data-narrator-anchor="`idt-${r.vec}`"
              >
                <td class="ints-table__vec">{{ r.hex }}</td>
                <td>{{ r.name }}</td>
                <td>
                  <span :class="['ints-table__src', `ints-table__src--${r.source.toLowerCase()}`]">
                    {{ r.source }}
                  </span>
                </td>
                <td class="ints-table__time">
                  {{ r.lastFired ? timeAgo(r.lastFired) : '—' }}
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="ints-page__panel">
          <header class="ints-page__panel-head">
            <h3>Últimas transiciones</h3>
            <span class="ints-page__panel-hint">USER ↔ KERNEL en vivo</span>
          </header>
          <div v-if="switches.length === 0" class="ints-feed__empty">
            Sin transiciones aún. Avance la simulación con un programa que invoque syscalls.
          </div>
          <ol v-else class="ints-feed">
            <li v-for="s in switches" :key="s.id" class="ints-feed__row">
              <span :class="['ints-feed__chip', `ints-feed__chip--${s.from.toLowerCase()}`]">{{ s.from }}</span>
              <span class="ints-feed__arrow">→</span>
              <span :class="['ints-feed__chip', `ints-feed__chip--${s.to.toLowerCase()}`]">{{ s.to }}</span>
              <span class="ints-feed__cause">{{ s.cause }}</span>
              <span class="ints-feed__time">{{ timeAgo(s.ts) }}</span>
            </li>
          </ol>
        </section>
      </div>

      <section class="ints-page__upcoming">
        <h4>Próximamente en esta sección</h4>
        <ul>
          <li>Entradas IDT resaltadas con la dirección del handler al dispararse</li>
          <li>Stack de retorno mostrando <code>EIP / CS / EFLAGS</code> guardados antes del IRET</li>
          <li>Animación del IRET reconstruyendo el contexto y volviendo a USER</li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.ints-page {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ints-page__content {
  position: relative;
  z-index: 1;
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.ints-page__hero {
  display: flex;
  align-items: center;
  gap: var(--space-5);
}

.ints-page__mode-chip {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-width: 180px;
  padding: var(--space-4) var(--space-5);
  border: 2px solid currentColor;
  border-radius: var(--radius-md);
  background: var(--surface-2);
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 4px;
  transition: color var(--transition-base), box-shadow var(--transition-base);
}

.ints-page__mode-chip--user {
  color: var(--cyan);
  box-shadow: 0 0 12px rgba(106, 212, 255, 0.25);
}

.ints-page__mode-chip--kernel {
  color: var(--amber);
  box-shadow: var(--shadow-glow-amber-strong);
}

.ints-page__mode-caption {
  font-size: 10px;
  letter-spacing: 3px;
  color: var(--text-dim);
}

.ints-page__mode-value {
  font-size: 28px;
  margin-top: var(--space-1);
  text-shadow: 0 0 12px currentColor;
}

.ints-page__hero-text {
  flex: 1;
  min-width: 0;
}

.ints-page__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 5px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
  margin-bottom: var(--space-3);
}

.ints-page__tagline {
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
}

.ints-page__tagline code {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px;
  font-size: 11px;
  color: var(--amber);
}

.ints-page__hint {
  margin-top: var(--space-3);
  font-size: 11px;
  color: var(--text-dim);
  padding: var(--space-2) var(--space-3);
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
}

.ints-page__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
}

@media (max-width: 960px) {
  .ints-page__hero { flex-direction: column; align-items: flex-start; }
  .ints-page__grid { grid-template-columns: 1fr; }
}

.ints-page__panel {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.ints-page__panel-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
  padding-bottom: var(--space-2);
}

.ints-page__panel-head h3 {
  font-family: 'Major Mono Display', monospace;
  font-size: 12px;
  letter-spacing: 3px;
  color: var(--amber);
  font-weight: normal;
}

.ints-page__panel-hint {
  font-size: 10px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.ints-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.ints-table thead th {
  text-align: left;
  font-family: 'Major Mono Display', monospace;
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-dim);
  text-transform: uppercase;
  padding: var(--space-2) var(--space-2);
  border-bottom: 1px dashed var(--border);
  font-weight: normal;
}

.ints-table__row {
  transition: background var(--transition-base), box-shadow var(--transition-base);
}

.ints-table__row td {
  padding: var(--space-2);
  border-bottom: 1px dashed rgba(255, 179, 71, 0.06);
}

.ints-table__row--active {
  background: rgba(255, 179, 71, 0.12);
  box-shadow: inset 2px 0 0 var(--amber);
  animation: ints-row-pulse 1.2s ease-out;
}

@keyframes ints-row-pulse {
  0%   { background: rgba(255, 179, 71, 0.32); }
  100% { background: rgba(255, 179, 71, 0.05); }
}

.ints-table__vec {
  font-family: 'JetBrains Mono', monospace;
  color: var(--amber);
}

.ints-table__src {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
}

.ints-table__src--hw    { color: var(--cyan); }
.ints-table__src--sw    { color: var(--amber); }
.ints-table__src--fault { color: #ff6b6b; }

.ints-table__time {
  color: var(--text-dim);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  text-align: right;
}

.ints-feed {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0;
  margin: 0;
}

.ints-feed__empty {
  font-size: 12px;
  color: var(--text-dim);
  font-style: italic;
  padding: var(--space-3);
}

.ints-feed__row {
  display: grid;
  grid-template-columns: auto auto auto 1fr auto;
  align-items: center;
  gap: var(--space-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 4px var(--space-2);
  border-bottom: 1px dashed rgba(255, 179, 71, 0.06);
}

.ints-feed__chip {
  padding: 1px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
}

.ints-feed__chip--user   { color: var(--cyan); }
.ints-feed__chip--kernel { color: var(--amber); }

.ints-feed__arrow { color: var(--text-dim); }

.ints-feed__cause {
  color: var(--text);
  font-size: 11px;
}

.ints-feed__time {
  color: var(--text-dim);
  font-size: 10px;
  text-align: right;
}

.ints-page__upcoming {
  border-top: 1px dashed var(--border);
  padding-top: var(--space-4);
  font-size: 12px;
  color: var(--text-dim);
}

.ints-page__upcoming h4 {
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
  letter-spacing: 2px;
  color: var(--amber);
  text-transform: uppercase;
  font-weight: normal;
  margin-bottom: var(--space-2);
}

.ints-page__upcoming ul {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding-left: var(--space-4);
}

.ints-page__upcoming li {
  position: relative;
  line-height: 1.5;
}

.ints-page__upcoming li::before {
  content: '›';
  position: absolute;
  left: -14px;
  color: var(--amber);
}

.ints-page__upcoming code {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px;
  font-size: 11px;
  color: var(--amber);
}
</style>
