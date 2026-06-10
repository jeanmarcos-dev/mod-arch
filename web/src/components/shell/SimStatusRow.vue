<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Activity, Wifi, WifiOff } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';

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
      </span>

      <div class="sim-status-row__info-cluster">
        <button
          type="button"
          class="sim-pill sim-pill--tick"
          :title="t('shell.simStatusRow.tickLabel') + ' ' + cpu.tick"
          :aria-label="t('shell.simStatusRow.tickLabel') + ' ' + cpu.tick"
          @click="cpu.openGlossary('IRQ 0x20')"
        >
          <Activity :size="14" :stroke-width="2" class="sim-pill__icon" aria-hidden="true" />
          <span class="sim-pill__v">{{ cpu.tick }}</span>
        </button>
        <span
          :class="['sim-ws', cpu.connected ? 'sim-ws--on' : 'sim-ws--off']"
          role="img"
          :aria-label="cpu.connected ? t('shell.simStatusRow.wsTitleOn') : t('shell.simStatusRow.wsTitleOff')"
          :title="cpu.connected ? t('shell.simStatusRow.wsTitleOn') : t('shell.simStatusRow.wsTitleOff')"
        >
          <component
            :is="cpu.connected ? Wifi : WifiOff"
            :size="14"
            :stroke-width="2"
            aria-hidden="true"
          />
        </span>
      </div>
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
   the header chips participate directly in the parent flex row. The base
   `.sim-status-row` declares min-height + padding for standalone usage; in
   inline mode the parent controls spacing, so neutralise both — otherwise
   on mobile (where this rule flips to display: flex) the wrapper stretches
   the grid row beyond 28px and pulls TICK/WS off the hamburger's baseline. */
.sim-status-row--inline {
  display: contents;
  min-height: 0;
  padding: 0;
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
  height: 28px;
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
  width: 28px;
  height: 28px;
  min-height: 0;
  padding: 0;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-tab:hover:not(:disabled) {
  color: var(--core-tint, var(--amber));
  background: color-mix(in oklab, var(--core-tint, var(--amber)) 14%, transparent);
  box-shadow: none;
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

.sim-status-row__info-cluster {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.sim-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid color-mix(in oklab, var(--border) 50%, transparent);
  border-radius: var(--radius-sm);
  padding: 0 8px;
  height: 28px;
  min-height: 0;
  box-sizing: border-box;
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 0.5px;
  text-transform: none;
  background: transparent;
  color: var(--text-dim);
  white-space: nowrap;
  cursor: default;
  transition: color var(--transition-fast);
}
.sim-pill__icon {
  color: var(--text-muted);
  flex-shrink: 0;
  display: block;
}
.sim-pill__v {
  color: var(--text);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.sim-pill--tick {
  min-width: 60px;
  cursor: pointer;
}
.sim-pill--tick:hover { color: var(--amber); }
.sim-pill--tick:hover .sim-pill__icon { color: var(--amber); }

.sim-ws {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  box-sizing: border-box;
  border: 1px solid color-mix(in oklab, var(--border) 50%, transparent);
  border-radius: var(--radius-sm);
  background: transparent;
  transition: color var(--transition-fast), border-color var(--transition-fast);
}
.sim-ws > svg { display: block; }
.sim-ws--on {
  color: var(--green);
  border-color: color-mix(in oklab, var(--green) 40%, transparent);
}
.sim-ws--off {
  color: var(--red);
  border-color: color-mix(in oklab, var(--red) 40%, transparent);
}

@media (max-width: 768px) {
  /* Inline mode normally promotes children to the parent flex row via
     display:contents — that prevents the SimBar from giving this row its
     own grid cell. On mobile + small tablet we want a single horizontal
     strip with TICK/WS that lives in the centred status cell of the
     SimBar grid, so reclaim a real flex wrapper here. */
  .sim-status-row--inline {
    display: flex;
    flex-direction: row;
    align-items: center;
    min-width: 0;
  }
  .sim-status-row__header {
    flex-wrap: nowrap;
    overflow-x: auto;
    overflow-y: hidden;
    justify-content: center;
    -webkit-overflow-scrolling: touch;
    scroll-snap-type: x proximity;
    width: 100%;
    /* Hide the scrollbar entirely — TICK/WS almost always fit, and when
       they don't the chips themselves visually overflow toward the edge
       cleanly without a scrollbar undercutting them. Touch swipe still
       scrolls on devices that have it. */
    scrollbar-width: none;
  }
  .sim-status-row__header::-webkit-scrollbar { display: none; }
  .sim-pill {
    flex-shrink: 0;
    scroll-snap-align: start;
  }
}

@media (max-width: 480px) {
  /* Tighter typography only on phones — at 481–768 the chips can keep
     their regular size and breathe in the centred status cell. */
  .sim-pill { font-size: 10px; padding: 0 6px; }
  .sim-pill--tick  { min-width: 52px; }
  .sim-bar__core-tabs { display: none; }
}

@media (max-width: 300px) {
  /* The SimBar already reserves enough room for the hamburger and cluster
     via its grid columns and padding; the row wrapper's own lateral
     padding becomes redundant at ultra-narrow widths and pushes TICK/WS
     off-centre. */
  .sim-status-row { padding-left: 0; padding-right: 0; }
}

@media (max-width: 360px) {
  /* TICK and WS both stay visible on the smallest phones — they live in
     the centred status cell of the SimBar grid. If they don't fit, the
     header scrolls horizontally with the mask gradient already applied. */
  .sim-pill { padding: 0 4px; font-size: 9.5px; }
  .sim-pill--tick { min-width: 44px; }
}
</style>
