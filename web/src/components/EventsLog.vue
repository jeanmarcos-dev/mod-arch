<script setup lang="ts">
import { useCpuStore, type LogEntry } from '../stores/cpu';
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { DynamicScroller, DynamicScrollerItem } from 'vue-virtual-scroller';

const props = defineProps<{ active: boolean }>();
const cpu = useCpuStore();
const { t } = useI18n();

// Ref to the DynamicScroller. Its root element is the scrollable container,
// so we hook the scroll listener there once the component is mounted. The
// component is generic over its items type, so we keep the ref untyped to
// avoid fighting TS over the generic instantiation.
type DynamicScrollerInstance = { $el: HTMLElement; scrollToBottom?: () => void };
const scroller = ref<DynamicScrollerInstance | null>(null);
let scrollEl: HTMLElement | null = null;

// Sticky autoscroll: follow new events only while the user is parked at the
// bottom. Once they scroll up to inspect history, we stop forcing the view
// down so they can read older ticks even as new ones keep arriving. A small
// threshold (in px) covers the gap left by fractional pixel rounding.
const STICK_THRESHOLD_PX = 24;
const stickToBottom = ref(true);

function handleScroll() {
  if (!scrollEl) return;
  const distance = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
  stickToBottom.value = distance <= STICK_THRESHOLD_PX;
}

watch(scroller, (s) => {
  if (scrollEl) {
    scrollEl.removeEventListener('scroll', handleScroll);
    scrollEl = null;
  }
  if (s) {
    scrollEl = s.$el ?? null;
    scrollEl?.addEventListener('scroll', handleScroll, { passive: true });
  }
});

onBeforeUnmount(() => {
  scrollEl?.removeEventListener('scroll', handleScroll);
  if (scrollHandle !== null) {
    cancelAnimationFrame(scrollHandle);
    scrollHandle = null;
  }
});

function colorOf(c: string) {
  switch (c) {
    case 'green':  return 'var(--green)';
    case 'red':    return 'var(--red-soft)';
    case 'cyan':   return 'var(--cyan)';
    case 'yellow': return 'var(--yellow)';
    default:       return 'var(--amber)';
  }
}

const visibleCores = computed(() =>
  cpu.cores
    .map(c => c.id)
    .filter(id => cpu.isCoreVisible(id))
    .sort((a, b) => a - b),
);

interface GridRow {
  key: string;
  tick: number | null;
  isGlobal: boolean;
  // Zebra parity is bound to `tick % 2` (not to the row's array index) so
  // shifts on cpu.log and global-row interleaving don't flip every row's
  // background when a new event arrives.
  alt: boolean;
  globalEntry?: LogEntry;
  cells: Map<number, LogEntry[]>;
}

// Group entries into rows. Each tick becomes one row; entries with no tick
// fall back to a synthetic row that follows their arrival order. Global
// entries (loaded/reset/ws_*) get their own full-width row.
const gridRows = computed<GridRow[]>(() => {
  // While the alumno is looking at another pizarra tab (Code / Memory) this
  // panel is hidden with v-show. Skip the O(n) rebuild — the simulator can
  // stream hundreds of events per second and recomputing for an invisible
  // view is pure waste. We reconstruct in one pass when active flips back.
  if (!props.active) return [];

  const rows: GridRow[] = [];
  const tickIndex = new Map<number, number>();
  let untickedSeq = 0;

  for (const e of cpu.log) {
    if (e.scope !== 'global' && e.coreId !== undefined && !cpu.isCoreVisible(e.coreId)) {
      continue;
    }
    if (e.scope === 'global') {
      rows.push({
        key: `g-${untickedSeq++}`,
        tick: null,
        isGlobal: true,
        alt: false,
        globalEntry: e,
        cells: new Map(),
      });
      continue;
    }

    const tick = typeof e.tick === 'number' ? e.tick : null;
    let idx: number | undefined;
    if (tick !== null) {
      idx = tickIndex.get(tick);
    }
    if (idx === undefined) {
      idx = rows.length;
      rows.push({
        key: tick !== null ? `t-${tick}` : `u-${untickedSeq++}`,
        tick,
        isGlobal: false,
        alt: tick !== null ? (tick % 2) === 1 : false,
        cells: new Map(),
      });
      if (tick !== null) tickIndex.set(tick, idx);
    }
    const row = rows[idx];
    const coreKey = e.coreId ?? -1;
    const bucket = row.cells.get(coreKey);
    if (bucket) bucket.push(e);
    else row.cells.set(coreKey, [e]);
  }

  return rows;
});

const gridStyle = computed(() => {
  // 56px tick column + one fr per visible core. Single- and multi-core
  // sessions share the same layout so the alumno reads the panel the same
  // way regardless of how many cores are active. Exposed as a CSS variable
  // so the sticky header row and every body row stay column-aligned even
  // though they live in separate grids (the scroller sits between them).
  const cols = visibleCores.value.length;
  return {
    '--ev-cols': `56px repeat(${cols}, minmax(120px, 1fr))`,
  };
});

// Follow the tail when new events arrive. rAF coalesces bursts of pushes
// into one scroll per paint.
let scrollPending = false;
let scrollHandle: number | null = null;
watch(() => cpu.log.length, () => {
  if (!props.active || !stickToBottom.value) return;
  if (scrollPending) return;
  scrollPending = true;
  scrollHandle = requestAnimationFrame(() => {
    scrollPending = false;
    scrollHandle = null;
    scroller.value?.scrollToBottom?.();
  });
});

// Coming back from another tab: the body was empty while hidden, so it
// rebuilds in one pass and we need to re-stick to the tail.
watch(() => props.active, async (active) => {
  if (!active) return;
  await nextTick();
  if (stickToBottom.value) {
    scroller.value?.scrollToBottom?.();
  }
});
</script>

<template>
  <div class="panel log" data-pizarra-anchor="events-panel" :style="gridStyle">
    <div class="grid__head-row">
      <div class="grid__head grid__head--tick">{{ t('panels.events.tickColumn') }}</div>
      <div
        v-for="c in visibleCores"
        :key="`head-${c}`"
        class="grid__head"
        :style="{ color: `var(--core-${c % 4})` }"
      >C{{ c }}</div>
    </div>

    <DynamicScroller
      ref="scroller"
      class="log__scroller"
      :items="gridRows"
      :min-item-size="22"
      key-field="key"
    >
      <template #default="{ item: row, index, active: itemActive }">
        <DynamicScrollerItem
          :item="row"
          :active="itemActive"
          :index="index"
          :size-dependencies="[row.cells.size, row.isGlobal, visibleCores.length]"
        >
          <div
            v-if="row.isGlobal && row.globalEntry"
            class="grid__global"
            :style="{ color: colorOf(row.globalEntry.color) }"
            :aria-label="t('panels.events.globalRowAria')"
          >{{ row.globalEntry.text }}</div>

          <div v-else class="grid__row">
            <div class="grid__tick" :class="{ 'grid__cell--alt': row.alt }">{{ row.tick !== null ? row.tick : '·' }}</div>
            <div
              v-for="c in visibleCores"
              :key="`${row.key}-c${c}`"
              class="grid__cell"
              :class="{
                'grid__cell--alt': row.alt,
                'grid__cell--kernel': (row.cells.get(c) ?? []).some(e => e.inKernel),
                'grid__cell--scheduler': (row.cells.get(c) ?? []).some(e => e.kind === 'scheduler'),
              }"
              :style="{ borderLeftColor: `var(--core-${c % 4})` }"
            >
              <div
                v-for="(entry, i) in (row.cells.get(c) ?? [])"
                :key="`${row.key}-c${c}-${i}`"
                class="grid__entry"
                :class="{ 'grid__entry--kernel': entry.inKernel }"
                :style="{ color: colorOf(entry.color) }"
              ><span v-if="entry.kind === 'scheduler'" class="grid__sched-chip" :title="t('panels.events.schedulerChipTitle')">S</span><span v-if="entry.inKernel" class="grid__kernel-chip" :title="t('panels.events.kernelChipTitle')">K</span>{{ entry.text }}</div>
            </div>
          </div>
        </DynamicScrollerItem>
      </template>
    </DynamicScroller>

    <div v-if="props.active && gridRows.length === 0" class="empty">
      {{ cpu.log.length === 0 ? t('panels.events.emptyLoad') : t('panels.events.emptyNoCores') }}
    </div>
  </div>
</template>

<style scoped>
.log {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
  /* The grid renders its own sticky header flush against the panel edges;
     nuke the .panel inherited padding so the header sits on top without a
     strip of background showing through. */
  padding: 0 !important;
  font-size: 14px;
  font-family: 'JetBrains Mono', monospace;
}

.grid__head-row {
  display: grid;
  grid-template-columns: var(--ev-cols);
  background: var(--surface-2, #131a22);
  border-bottom: 1px solid var(--border-muted, rgba(255, 255, 255, 0.1));
  flex-shrink: 0;
  font-size: 13px;
  /* Reserve the same gutter the body scroller below uses (scrollbar-gutter:
     stable) so the column dividers line up vertically with the body grid. */
  padding-right: var(--ev-scrollbar-gutter, 10px);
}

.grid__head {
  padding: 4px 6px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.5px;
  text-align: center;
  border-right: 1px solid var(--border, rgba(255, 181, 71, 0.18));
}

.grid__head-row .grid__head:last-child {
  border-right: none;
}

.grid__head--tick {
  color: var(--text-muted, rgba(255, 255, 255, 0.5));
}

.log__scroller {
  flex: 1;
  min-height: 0;
  font-size: 13px;
  /* Keep the scrollbar gutter reserved even when content fits, so the body
     grid width matches the header row above regardless of row count. */
  scrollbar-gutter: stable;
}

.grid__row {
  display: grid;
  grid-template-columns: var(--ev-cols);
  contain: layout style;
}

.grid__tick {
  padding: 1px 4px;
  text-align: right;
  color: var(--text-muted, rgba(255, 255, 255, 0.45));
  font-size: 11px;
  border-right: 1px solid var(--border, rgba(255, 181, 71, 0.18));
  white-space: nowrap;
}

.grid__cell {
  border-left: 2px solid transparent;
  border-right: 1px solid var(--border, rgba(255, 181, 71, 0.12));
  padding: 1px 6px;
  min-height: 1.4em;
  white-space: pre-wrap;
  word-break: break-word;
}

.grid__row .grid__cell:last-child {
  border-right: none;
}

/* Zebra striping: parity is bound to row.tick (see GridRow.alt) so the
   pattern stays stable even when the log cap shifts entries off the front
   or a global row gets interleaved. */
.grid__cell--alt {
  background: rgba(var(--accent-rgb), 0.045);
}

/* Cells whose events ran in PL0 get an amber-tinted background. Combined
   with the K chip prefix, this makes "the kernel is running here" obvious
   without inventing a separate kernel PID per core. */
.grid__cell--kernel {
  background: linear-gradient(0deg, rgba(var(--accent-rgb), 0.08), rgba(var(--accent-rgb), 0.08));
}

.grid__cell--alt.grid__cell--kernel {
  background: linear-gradient(0deg, rgba(var(--accent-rgb), 0.11), rgba(var(--accent-rgb), 0.11));
}

.grid__kernel-chip {
  display: inline-block;
  width: 14px;
  margin-right: 4px;
  padding: 0 2px;
  background: var(--amber);
  color: var(--surface-2, #111);
  border-radius: 2px;
  font-size: 9px;
  font-weight: 700;
  text-align: center;
  vertical-align: baseline;
}

/* Scheduler chip: distinct from the kernel chip so the alumno can tell
   "kernel ran here" (K) apart from "the scheduler specifically ran here"
   (S). Uses the yellow accent — same hue formatLog already gives to
   sched_decision/wake events for consistency. */
.grid__sched-chip {
  display: inline-block;
  width: 14px;
  margin-right: 4px;
  padding: 0 2px;
  background: var(--yellow);
  color: var(--surface-2, #111);
  border-radius: 2px;
  font-size: 9px;
  font-weight: 700;
  text-align: center;
  vertical-align: baseline;
}

.grid__cell--scheduler {
  outline: 1px dashed rgba(255, 220, 80, 0.35);
  outline-offset: -2px;
}

.grid__entry {
  padding: 0;
  line-height: 1.4;
}

/* Full-width separator for events that have no per-core origin. */
.grid__global {
  padding: 3px 8px;
  margin: 2px 0;
  text-align: center;
  font-style: italic;
  font-size: 11px;
  background: var(--bg-soft, rgba(255, 255, 255, 0.03));
  border-top: 1px dashed var(--border-muted, rgba(255, 255, 255, 0.08));
  border-bottom: 1px dashed var(--border-muted, rgba(255, 255, 255, 0.08));
}

.empty {
  position: absolute;
  top: 32px;
  left: 0;
  right: 0;
  color: var(--text-muted);
  font-style: italic;
  padding: 20px 0;
  text-align: center;
  pointer-events: none;
}

@media (max-width: 768px) {
  .log {
    font-size: 12px;
  }
  .log__scroller {
    font-size: 11px;
  }
  .grid__cell {
    padding: 1px 4px;
  }
}
</style>
