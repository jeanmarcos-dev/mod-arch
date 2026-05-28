<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDown } from '@lucide/vue';
import { useCpuStore, type CoreInfo } from '../../stores/cpu';
import { useSimBarStore } from '../../stores/simBar';
import Term from '../glossary/Term.vue';

// SimStatusRow renders the CPU status as a single multicore-shaped layout:
// a header row with the core-filter tabs + TICK + WS, and below it one
// sub-row per visible core with PID, MODE, EIP/EAX/ESP and STATE. The
// per-core sub-rows can be collapsed via the accordion toggle when the user
// only cares about the aggregate view.
//
// `inline` (set by the SimBar) flips the root to `display: contents` so the
// component's header and cores wrapper become direct children of the parent
// flex container — letting the controls and the status chips share one line
// when the viewport is wide enough, and wrapping into two only when needed.
//
// Single-core sessions (cpu.numCores === 1) reuse the same shape with the
// filter chips and the toggle hidden — they degrade gracefully into one
// header row + one detail row, always visible.
//
// `readonly` (set by the viewer shell) disables the only interactive bits:
// the core filter tabs and the click-to-focus behavior on sub-rows.
const props = withDefaults(
  defineProps<{ readonly?: boolean; inline?: boolean }>(),
  { readonly: false, inline: false },
);

const cpu = useCpuStore();
const bar = useSimBarStore();
const { t } = useI18n();

const visibleCores = computed<CoreInfo[]>(() => {
  return cpu.cores
    .filter((c) => cpu.isCoreVisible(c.id))
    .slice()
    .sort((a, b) => a.id - b.id);
});

const isMulti = computed(() => cpu.numCores > 1);
const showDetail = computed(() => !isMulti.value || bar.coresExpanded);

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
      'sim-status-row--inline': inline,
    }"
    aria-label="Estado del CPU"
    data-pizarra-anchor="sim-status-row"
  >
    <div class="sim-status-row__header">
      <span
        v-if="isMulti"
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

      <span class="sim-pill sim-pill--tick">
        <Term term="IRQ 0x20" mode="label" class="sim-pill__k">TICK</Term>
        <span class="sim-pill__v">{{ cpu.tick }}</span>
      </span>
      <span
        :class="['sim-pill', 'sim-pill--ws', cpu.connected ? 'sim-pill--ws-on' : 'sim-pill--ws-off']"
        :title="`WebSocket ${cpu.connected ? 'conectado' : 'desconectado'}`"
      >
        <span class="sim-pill__k">WS</span>
        <span class="sim-pill__v">{{ cpu.connected ? 'ON' : 'OFF' }}</span>
      </span>

      <!-- Cores accordion toggle — sits beside the WS pill, same row as
           the status chips. Plain chevron button, no flotation tricks. -->
      <button
        v-if="isMulti"
        type="button"
        :class="['sim-status-row__cores-btn', { 'sim-status-row__cores-btn--on': bar.coresExpanded }]"
        :aria-expanded="bar.coresExpanded"
        :aria-label="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
        :title="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
        @click="bar.toggleCores()"
      >
        <ChevronDown
          :size="13"
          :stroke-width="2.5"
          :class="['sim-status-row__cores-btn-icon', { 'sim-status-row__cores-btn-icon--collapsed': !bar.coresExpanded }]"
        />
      </button>
    </div>

    <Transition name="sim-cores">
      <div v-if="showDetail" class="sim-status-row__cores">
        <div
          v-for="c in visibleCores"
          :key="c.id"
          :class="['sim-core-row', {
            'sim-core-row--halted': c.halted,
            'sim-core-row--idle': c.current_pid === 0 && !c.halted,
            'sim-core-row--solo': !isMulti,
          }]"
          :style="{ '--core-tint': `var(--core-${c.id % 4})`, '--core-tint-fg': `var(--core-fg)` }"
          :title="`Estado del núcleo C${c.id}`"
        >
          <span v-if="isMulti" class="sim-core-row__id">C{{ c.id }}</span>
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
    </Transition>
  </div>
</template>

<style scoped>
.sim-status-row {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  min-height: 32px;
  padding: var(--space-1) var(--space-3);
}

/* Inline mode: the root disappears from the layout (display: contents) so
   the header and the cores wrapper participate directly in the parent flex
   row. The parent (SimBar's main row) wraps both clusters when the viewport
   is narrow, and keeps them on one line when there's space. */
.sim-status-row--inline {
  display: contents;
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
  overflow: hidden;
}

/* In inline mode the cores panel is rendered as a distinct "card" pulled
   out of the topbar: darker background, inset shadow at the top edge to
   imply depth, and a responsive grid of per-core rows. The panel claims a
   fresh row below the header via flex-basis:100%. */
.sim-status-row--inline .sim-status-row__cores {
  flex-basis: 100%;
  width: 100%;
  margin-top: 10px; /* clearance for the handle tab above */
  padding: var(--space-2) var(--space-3);
  background: var(--surface-0);
  border-top: 1px solid var(--border-strong);
  border-bottom: 1px solid var(--border);
  box-shadow:
    inset 0 6px 10px -8px rgba(0, 0, 0, 0.55),
    inset 0 -2px 4px -3px rgba(0, 0, 0, 0.25);
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(640px, 1fr));
  gap: 4px 16px;
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
/* Single-core sessions hide the C0 tag (no other core to disambiguate from)
   so the row reads as a plain status strip, like the legacy flat layout. */
.sim-core-row--solo .sim-core-row__id { display: none; }

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

/* Cores accordion toggle — small chevron button that sits inline with the
   status chips (right beside the WS pill). Same height as the other chips
   (24px) so it doesn't break the row visually. */
.sim-status-row__cores-btn {
  width: 24px;
  height: 24px;
  padding: 0;
  margin-left: 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-dim);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-sizing: border-box;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-status-row__cores-btn:hover {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.10);
  border-color: var(--border-strong);
}
.sim-status-row__cores-btn--on {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.14);
  border-color: var(--amber);
}
.sim-status-row__cores-btn:focus { outline: none; }
.sim-status-row__cores-btn:focus-visible { outline: 1px solid var(--amber); outline-offset: -1px; }

.sim-status-row__cores-btn-icon {
  transition: transform var(--transition-base);
}
.sim-status-row__cores-btn-icon--collapsed {
  transform: rotate(-90deg);
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
  min-height: 24px;
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
  min-height: 24px;
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

/* Accordion transition for the detail rows. max-height keyframes give a soft
   collapse without JS measurements — overflow:hidden on the wrapper clips
   the content while the height settles. 500px is high enough to host up to
   4 core rows of pills without clipping; if the layout ever grows past that,
   bump this number. */
.sim-cores-enter-active,
.sim-cores-leave-active {
  transition: max-height var(--transition-base) ease-out,
              opacity var(--transition-base) ease-out;
  overflow: hidden;
}
.sim-cores-enter-from,
.sim-cores-leave-to {
  max-height: 0;
  opacity: 0;
}
.sim-cores-enter-to,
.sim-cores-leave-from {
  max-height: 500px;
  opacity: 1;
}

@media (max-width: 480px) {
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
