<script setup lang="ts">
import { computed } from 'vue';
import { useCpuStore, type CoreInfo } from '../../stores/cpu';
import Term from '../glossary/Term.vue';

// SimStatusRow renders the CPU status chips. Single-core sessions get the
// flat one-row layout with the canonical pills (PID, MODE, EIP/EAX/ESP,
// TICK, STATE, WS). Multicore sessions split into a header (cores filter +
// TICK, WS) plus one sub-row per visible core so the spectator sees real
// per-core registers AND each core's own STATE chip — a global STATE in
// multicore hides the case where one core runs while another is halted.
//
// `readonly` (set by the viewer shell) disables the only interactive bits:
// the core filter tabs and the click-to-focus behavior on sub-rows.
const props = withDefaults(defineProps<{ readonly?: boolean }>(), { readonly: false });

const cpu = useCpuStore();

const visibleCores = computed<CoreInfo[]>(() => {
  return cpu.cores
    .filter((c) => cpu.isCoreVisible(c.id))
    .slice()
    .sort((a, b) => a.id - b.id);
});

// Flat single-row layout when there is only one CPU OR only one visible
// core. The per-core grid only earns its place when ≥2 cores are showing —
// otherwise it would push everything onto an extra line for nothing.
const flat = computed(() => cpu.numCores === 1 || visibleCores.value.length === 1);

type CoreStateKind = 'RUN' | 'HALT' | 'IDLE';

function coreStateLabel(c: CoreInfo): CoreStateKind {
  if (c.halted) return 'HALT';
  if (c.current_pid > 0) return 'RUN';
  return 'IDLE';
}
function coreStateClass(c: CoreInfo): string {
  const l = coreStateLabel(c);
  if (l === 'HALT') return 'sim-pill--halt';
  if (l === 'RUN') return 'sim-pill--running';
  return 'sim-pill--idle';
}
function coreStateTerm(c: CoreInfo): string {
  const l = coreStateLabel(c);
  if (l === 'HALT') return 'HLT';
  if (l === 'RUN') return 'RUN';
  return 'Round-Robin';
}

// Flat-mode aggregate kept for the single-core / single-visible-core path:
// when only one row is rendered, the chip lives next to it inline.
const stateLabel = computed<CoreStateKind>(() => {
  if (cpu.halted) return 'HALT';
  if (cpu.running) return 'RUN';
  return 'IDLE';
});
const stateClass = computed(() => {
  const l = stateLabel.value;
  if (l === 'HALT') return 'sim-pill--halt';
  if (l === 'RUN') return 'sim-pill--running';
  return 'sim-pill--idle';
});
const stateTerm = computed(() => {
  const l = stateLabel.value;
  if (l === 'HALT') return 'HLT';
  if (l === 'RUN') return 'RUN';
  return 'Round-Robin';
});

function hex(v: number | undefined): string {
  if (typeof v !== 'number') return '—';
  return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0');
}

function coreModeLabel(c: CoreInfo): string {
  return c.mode === 3 ? 'USER' : 'KERNEL';
}

function coreModeClass(c: CoreInfo): string {
  return c.mode === 3 ? 'sim-pill--user' : 'sim-pill--kernel';
}

function onCoreTabClick(id: number) {
  if (props.readonly) return;
  cpu.toggleCoreSelection(id);
}
function onCoreTabDblClick(id: number) {
  if (props.readonly) return;
  cpu.selectOnlyCore(id);
}
function onCoreAllClick() {
  if (props.readonly) return;
  if (cpu.selectedCoreIds.size === cpu.numCores) {
    cpu.selectOnlyCore(cpu.selectedCoreId);
  } else {
    cpu.selectAllCores();
  }
}
</script>

<template>
  <div
    class="sim-status-row"
    :class="{
      'sim-status-row--readonly': readonly,
      'sim-status-row--multi': !flat,
    }"
    aria-label="Estado del CPU"
    data-pizarra-anchor="sim-status-row"
  >
    <!-- Flat layout — true single core OR multicore with exactly one
         visible core. The per-core grid only kicks in when ≥2 cores are
         being shown. -->
    <template v-if="flat">
      <!-- Cores filter is still useful even when only one is visible,
           because that is how the user re-adds the others. -->
      <span
        v-if="cpu.numCores > 1"
        class="sim-bar__core-tabs"
        role="group"
        aria-label="Filtro de núcleos visibles"
        :title="readonly
          ? 'Filtro de núcleos controlado por el anfitrión'
          : 'Marque qué núcleos quiere ver. Doble click sobre un núcleo para enfocarlo en exclusiva.'"
      >
        <button
          v-for="c in cpu.cores"
          :key="c.id"
          type="button"
          :aria-pressed="cpu.isCoreVisible(c.id)"
          :class="['sim-bar__core-tab', {
            'sim-bar__core-tab--active': cpu.isCoreVisible(c.id),
            'sim-bar__core-tab--idle': c.current_pid === 0,
            'sim-bar__core-tab--halted': c.halted,
          }]"
          :style="{ '--core-tint': `var(--core-${c.id % 4})`, '--core-tint-fg': `var(--core-fg)` }"
          :disabled="readonly"
          :title="readonly
            ? `Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'} (vista del anfitrión)`
            : `Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'}${c.halted ? ' · HALT' : ''} — click para mostrar/ocultar, doble click para enfocar`"
          @click="onCoreTabClick(c.id)"
          @dblclick="onCoreTabDblClick(c.id)"
        >C{{ c.id }}</button>
        <button
          type="button"
          class="sim-bar__core-all"
          :disabled="readonly"
          :title="cpu.selectedCoreIds.size === cpu.numCores
            ? `Dejar visible sólo el núcleo enfocado (C${cpu.selectedCoreId})`
            : 'Mostrar todos los núcleos'"
          @click="onCoreAllClick"
        >{{ cpu.selectedCoreIds.size === cpu.numCores ? `ONLY C${cpu.selectedCoreId}` : 'ALL' }}</button>
      </span>
      <span class="sim-pill sim-pill--pid" :title="`Proceso actual en C${cpu.selectedCoreId}`">
        <Term term="PID" mode="label" class="sim-pill__k">PID</Term>
        <span class="sim-pill__v">{{ cpu.currentPID }}</span>
      </span>
      <span :class="['sim-pill', 'sim-pill--mode', cpu.mode === 3 ? 'sim-pill--user' : 'sim-pill--kernel']">
        <Term :term="cpu.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'" mode="label" class="sim-pill__k">MODE</Term>
        <span class="sim-pill__v">{{ cpu.mode === 3 ? 'USER' : 'KERNEL' }}</span>
      </span>
      <span class="sim-pill sim-pill--mono sim-pill--reg">
        <Term term="EIP" mode="label" class="sim-pill__k">EIP</Term>
        <span class="sim-pill__v">{{ hex(cpu.regs.EIP) }}</span>
      </span>
      <span class="sim-pill sim-pill--mono sim-pill--reg">
        <Term term="EAX" mode="label" class="sim-pill__k">EAX</Term>
        <span class="sim-pill__v">{{ hex(cpu.regs.EAX) }}</span>
      </span>
      <span class="sim-pill sim-pill--mono sim-pill--reg">
        <Term term="ESP" mode="label" class="sim-pill__k">ESP</Term>
        <span class="sim-pill__v">{{ hex(cpu.regs.ESP) }}</span>
      </span>
      <span class="sim-pill sim-pill--tick">
        <Term term="IRQ 0x20" mode="label" class="sim-pill__k">TICK</Term>
        <span class="sim-pill__v">{{ cpu.tick }}</span>
      </span>
      <span :class="['sim-pill', 'sim-pill--state', stateClass]">
        <Term :term="stateTerm" mode="label" class="sim-pill__k">STATE</Term>
        <span class="sim-pill__v">{{ stateLabel }}</span>
      </span>
      <span :class="['sim-pill', 'sim-pill--ws', cpu.connected ? 'sim-pill--ws-on' : 'sim-pill--ws-off']" :title="`WebSocket ${cpu.connected ? 'conectado' : 'desconectado'}`">
        <span class="sim-pill__k">WS</span>
        <span class="sim-pill__v">{{ cpu.connected ? 'ON' : 'OFF' }}</span>
      </span>
    </template>

    <!-- Multicore layout — header with aggregates + one sub-row per
         visible core. Each sub-row reads cpu.cores[i] directly so PID/MODE/
         registers are real, not a mirror of the focused core. -->
    <template v-else>
      <div class="sim-status-row__header">
        <span
          class="sim-bar__core-tabs"
          role="group"
          aria-label="Filtro de núcleos visibles"
          :title="readonly
            ? 'Filtro de núcleos controlado por el anfitrión'
            : 'Marque qué núcleos quiere ver. Doble click sobre un núcleo para enfocarlo en exclusiva.'"
        >
          <button
            v-for="c in cpu.cores"
            :key="c.id"
            type="button"
            :aria-pressed="cpu.isCoreVisible(c.id)"
            :class="['sim-bar__core-tab', {
              'sim-bar__core-tab--active': cpu.isCoreVisible(c.id),
              'sim-bar__core-tab--idle': c.current_pid === 0,
              'sim-bar__core-tab--halted': c.halted,
            }]"
            :disabled="readonly"
            :title="readonly
              ? `Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'} (vista del anfitrión)`
              : `Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'}${c.halted ? ' · HALT' : ''} — click para mostrar/ocultar, doble click para enfocar`"
            @click="onCoreTabClick(c.id)"
            @dblclick="onCoreTabDblClick(c.id)"
          >C{{ c.id }}</button>
          <button
            type="button"
            class="sim-bar__core-all"
            :disabled="readonly"
            :title="cpu.selectedCoreIds.size === cpu.numCores
              ? `Dejar visible sólo el núcleo enfocado (C${cpu.selectedCoreId})`
              : 'Mostrar todos los núcleos'"
            @click="onCoreAllClick"
          >{{ cpu.selectedCoreIds.size === cpu.numCores ? `ONLY C${cpu.selectedCoreId}` : 'ALL' }}</button>
        </span>
        <span class="sim-pill sim-pill--tick">
          <Term term="IRQ 0x20" mode="label" class="sim-pill__k">TICK</Term>
          <span class="sim-pill__v">{{ cpu.tick }}</span>
        </span>
        <span :class="['sim-pill', 'sim-pill--ws', cpu.connected ? 'sim-pill--ws-on' : 'sim-pill--ws-off']" :title="`WebSocket ${cpu.connected ? 'conectado' : 'desconectado'}`">
          <span class="sim-pill__k">WS</span>
          <span class="sim-pill__v">{{ cpu.connected ? 'ON' : 'OFF' }}</span>
        </span>
      </div>

      <div class="sim-status-row__cores">
        <div
          v-for="c in visibleCores"
          :key="c.id"
          :class="['sim-core-row', {
            'sim-core-row--halted': c.halted,
            'sim-core-row--idle': c.current_pid === 0 && !c.halted,
          }]"
          :style="{ '--core-tint': `var(--core-${c.id % 4})`, '--core-tint-fg': `var(--core-fg)` }"
          :title="`Estado del núcleo C${c.id}`"
        >
          <span class="sim-core-row__id">C{{ c.id }}</span>
          <span class="sim-pill sim-pill--pid">
            <Term term="PID" mode="label" class="sim-pill__k">PID</Term>
            <span class="sim-pill__v">{{ c.current_pid }}</span>
          </span>
          <span :class="['sim-pill', 'sim-pill--mode', coreModeClass(c)]">
            <Term :term="c.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'" mode="label" class="sim-pill__k">MODE</Term>
            <span class="sim-pill__v">{{ coreModeLabel(c) }}</span>
          </span>
          <span class="sim-pill sim-pill--mono sim-pill--reg">
            <Term term="EIP" mode="label" class="sim-pill__k">EIP</Term>
            <span class="sim-pill__v">{{ hex(c.regs.EIP) }}</span>
          </span>
          <span class="sim-pill sim-pill--mono sim-pill--reg">
            <Term term="EAX" mode="label" class="sim-pill__k">EAX</Term>
            <span class="sim-pill__v">{{ hex(c.regs.EAX) }}</span>
          </span>
          <span class="sim-pill sim-pill--mono sim-pill--reg">
            <Term term="ESP" mode="label" class="sim-pill__k">ESP</Term>
            <span class="sim-pill__v">{{ hex(c.regs.ESP) }}</span>
          </span>
          <span
            :class="['sim-pill', 'sim-pill--state', coreStateClass(c)]"
            :title="c.halted
              ? `C${c.id} ejecutó HLT y duerme hasta el próximo IRQ.`
              : (c.current_pid > 0
                ? `C${c.id} corre P${c.current_pid} en este momento.`
                : `C${c.id} está ocioso, esperando que el dispatcher le asigne un proceso.`)"
          >
            <Term :term="coreStateTerm(c)" mode="label" class="sim-pill__k">STATE</Term>
            <span class="sim-pill__v">{{ coreStateLabel(c) }}</span>
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.sim-status-row {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  min-height: 32px;
  padding: var(--space-1) var(--space-3);
  justify-content: flex-end;
}

/* Multicore reshapes the row into a flex-column: aggregate header above,
   one sub-row per visible core below. align-items: stretch so each child
   spans the full width. */
.sim-status-row--multi {
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
}

.sim-status-row__header {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.sim-status-row__cores {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sim-core-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  flex-wrap: wrap;
  padding: 2px 0;
  background: transparent;
}
.sim-core-row--halted { opacity: 0.7; }
.sim-core-row--idle { opacity: 0.85; }

/* Per-core identity badge. Background + fg use the per-row --core-tint so
   the sub-row reads as "this is the C0 line" at a glance, matching the
   color used in the events log, narrators and console badges. */
.sim-core-row__id {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--core-tint-fg, var(--core-fg, #0e0e10));
  background: var(--core-tint, var(--text-muted));
  min-width: 28px;
  padding: 0 6px;
  text-align: center;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  font-weight: 700;
}

.sim-bar__core-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 24px;
  box-sizing: border-box;
}
.sim-bar__core-tab {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  line-height: 1;
  letter-spacing: 1px;
  text-transform: uppercase;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-muted);
  padding: 0 8px;
  height: 24px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-tab:hover:not(:disabled) {
  color: var(--core-tint, var(--amber));
  background: color-mix(in oklab, var(--core-tint, var(--amber)) 14%, transparent);
}
.sim-bar__core-tab:focus { outline: none; }
.sim-bar__core-tab:focus-visible {
  outline: 1px solid var(--core-tint, var(--amber));
  outline-offset: -1px;
}
/* Active state uses the core-specific tint as background + fg color so each
   active tab visibly carries its core identity. Falls back to --amber for
   the legacy single-core path that does not set --core-tint. */
.sim-bar__core-tab--active {
  background: var(--core-tint, var(--amber));
  color: var(--core-tint-fg, var(--core-fg, #0e0e10));
  border-color: var(--core-tint, var(--amber));
  font-weight: 600;
}
.sim-bar__core-tab--idle:not(.sim-bar__core-tab--active) {
  color: var(--text-dim);
  opacity: 0.7;
}
.sim-bar__core-tab--halted:not(.sim-bar__core-tab--active) {
  color: var(--red-soft, #ff7a7a);
  opacity: 0.75;
}
.sim-bar__core-tab:disabled {
  cursor: default;
}
.sim-bar__core-all {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  line-height: 1;
  letter-spacing: 1px;
  text-transform: uppercase;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  padding: 0 8px;
  height: 24px;
  /* Fixed width so the label oscillating between "ALL" and "ONLY Cx" does
     not reflow the surrounding chips. Sized for the longest variant. */
  min-width: 64px;
  box-sizing: border-box;
  margin-left: 2px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-all:hover:not(:disabled) {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
  border-color: var(--amber);
}
.sim-bar__core-all:focus { outline: none; }
.sim-bar__core-all:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}
.sim-bar__core-all:disabled {
  cursor: default;
}

.sim-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 8px;
  height: 24px;
  box-sizing: border-box;
  font-size: 10px;
  letter-spacing: 0.5px;
  background: rgba(255, 255, 255, 0.015);
  white-space: nowrap;
}
.sim-pill__k {
  color: var(--text-muted);
  letter-spacing: 1px;
  text-transform: uppercase;
  font-size: 9px;
  font-weight: 500;
}
.sim-pill__v {
  color: var(--text);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.sim-pill--mono .sim-pill__v { font-family: 'JetBrains Mono', monospace; font-size: 10px; }

.sim-pill--pid   { min-width: 66px; }
.sim-pill--mode  { min-width: 96px; }
.sim-pill--reg   { min-width: 132px; }
.sim-pill--tick  { min-width: 80px; }
.sim-pill--state { min-width: 78px; }
.sim-pill--ws    { min-width: 62px; }

.sim-pill--user { border-color: var(--green); }
.sim-pill--user .sim-pill__v { color: var(--green); }
.sim-pill--kernel { border-color: var(--red-soft, #ff7a7a); }
.sim-pill--kernel .sim-pill__v { color: var(--red-soft, #ff7a7a); }

.sim-pill--halt { border-color: var(--red); background: rgba(255, 58, 58, 0.06); }
.sim-pill--halt .sim-pill__v { color: var(--red); font-weight: 700; }
.sim-pill--running { border-color: var(--green); background: rgba(127, 255, 170, 0.05); }
.sim-pill--running .sim-pill__v { color: var(--green); font-weight: 700; }
.sim-pill--idle .sim-pill__v { color: var(--text-muted); }

.sim-pill--ws-on { border-color: var(--green); }
.sim-pill--ws-on .sim-pill__v { color: var(--green); }
.sim-pill--ws-off { border-color: var(--red); }
.sim-pill--ws-off .sim-pill__v { color: var(--red); }

@media (max-width: 480px) {
  /* Switch the row from wrap-friendly to a horizontally-scrollable strip so
     all pills remain in a single line in priority order. The mask hints
     that more content sits past the right edge. */
  .sim-status-row {
    flex-wrap: nowrap;
    overflow-x: auto;
    justify-content: flex-start;
    scrollbar-width: thin;
    -webkit-overflow-scrolling: touch;
    mask-image: linear-gradient(to right, black 88%, transparent);
  }
  .sim-status-row--multi {
    flex-direction: column;
    overflow-x: visible;
    mask-image: none;
  }
  .sim-status-row__header,
  .sim-core-row {
    flex-wrap: nowrap;
    overflow-x: auto;
    justify-content: flex-start;
    scrollbar-width: thin;
    -webkit-overflow-scrolling: touch;
    mask-image: linear-gradient(to right, black 88%, transparent);
  }
  .sim-pill {
    font-size: 9px;
    padding: 0 6px;
    flex-shrink: 0;
  }
  .sim-pill__k { font-size: 8px; }
  .sim-pill--mono .sim-pill__v { font-size: 9.5px; }
  .sim-pill--pid   { min-width: 50px; }
  .sim-pill--mode  { min-width: 72px; }
  .sim-pill--reg   { min-width: 100px; }
  .sim-pill--tick  { min-width: 64px; }
  .sim-pill--state { min-width: 64px; }
  .sim-pill--ws    { min-width: 52px; }
}
</style>
