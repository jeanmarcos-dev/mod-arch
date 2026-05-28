<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import Term from '../glossary/Term.vue';

const { t } = useI18n();

// SimStatusRow renders the inline status header inside the SimBar: the
// per-core filter chips (C0/C1/…/ALL), the tick counter and the WebSocket
// indicator. The per-core detail panel and its accordion handle now live
// directly in SimBar.vue so the controls row keeps a fixed height — see the
// SimBar template for the cores panel and its bottom-edge handle.
//
// `inline` (set by the SimBar) flips the root to `display: contents` so this
// component's chips become direct children of the parent flex row.
//
// `readonly` (set by the viewer shell) disables the only interactive bits:
// the core filter chips.
const props = withDefaults(
  defineProps<{ readonly?: boolean; inline?: boolean }>(),
  { readonly: false, inline: false },
);

const cpu = useCpuStore();

const isMulti = computed(() => cpu.numCores > 1);

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
    :aria-label="t('shell.simStatusRow.ariaCpuStatus')"
    data-pizarra-anchor="sim-status-row"
  >
    <div class="sim-status-row__header">
      <span
        v-if="isMulti"
        class="sim-bar__core-tabs"
        role="group"
        :aria-label="t('shell.simStatusRow.coreFilterAria')"
        :title="readonly
          ? t('shell.simStatusRow.coreFilterReadOnly')
          : t('shell.simStatusRow.coreFilterHint')"
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
            ? t('shell.simStatusRow.coreReadOnly', { id: c.id, state: c.current_pid ? t('shell.simStatusRow.coreRunning', { pid: c.current_pid }) : t('shell.simStatusRow.coreIdle') })
            : t('shell.simStatusRow.coreInteractive', { id: c.id, state: c.current_pid ? t('shell.simStatusRow.coreRunning', { pid: c.current_pid }) : t('shell.simStatusRow.coreIdle'), halt: c.halted ? t('shell.simStatusRow.coreHalted') : '' })"
          @click="onCoreTabClick(c.id)"
          @dblclick="onCoreTabDblClick(c.id)"
        >C{{ c.id }}</button>
        <button
          type="button"
          class="sim-bar__core-all"
          :disabled="readonly"
          :title="cpu.selectedCoreIds.size === cpu.numCores
            ? t('shell.simStatusRow.onlyFocused', { id: cpu.selectedCoreId })
            : t('shell.simStatusRow.showAll')"
          @click="onCoreAllClick"
        >{{ cpu.selectedCoreIds.size === cpu.numCores ? `ONLY C${cpu.selectedCoreId}` : 'ALL' }}</button>
      </span>

      <span class="sim-pill sim-pill--tick">
        <Term term="IRQ 0x20" mode="label" class="sim-pill__k">{{ t('shell.simStatusRow.tickLabel') }}</Term>
        <span class="sim-pill__v">{{ cpu.tick }}</span>
      </span>
      <span
        :class="['sim-pill', 'sim-pill--ws', cpu.connected ? 'sim-pill--ws-on' : 'sim-pill--ws-off']"
        :title="cpu.connected ? t('shell.simStatusRow.wsTitleOn') : t('shell.simStatusRow.wsTitleOff')"
      >
        <span class="sim-pill__k">WS</span>
        <span class="sim-pill__v">{{ cpu.connected ? t('shell.simStatusRow.onLabel') : t('shell.simStatusRow.offLabel') }}</span>
      </span>
    </div>
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
   the header chips participate directly in the parent flex row. */
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

.sim-pill--tick  { min-width: 80px; }
.sim-pill--ws    { min-width: 62px; }

.sim-pill--ws-on { border-color: var(--green); }
.sim-pill--ws-on .sim-pill__v { color: var(--green); }
.sim-pill--ws-off { border-color: var(--red); }
.sim-pill--ws-off .sim-pill__v { color: var(--red); }

@media (max-width: 480px) {
  .sim-status-row__header {
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
  .sim-pill--tick  { min-width: 64px; }
  .sim-pill--ws    { min-width: 52px; }
}
</style>
