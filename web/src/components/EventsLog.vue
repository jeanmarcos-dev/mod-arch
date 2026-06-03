<script setup lang="ts">
import { useCpuStore, type LogEntry } from '../stores/cpu';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { DynamicScroller, DynamicScrollerItem } from 'vue-virtual-scroller';
import { ArrowDownToLine, ArrowUpFromLine } from '@lucide/vue';

const props = defineProps<{ active: boolean }>();
const cpu = useCpuStore();
const { t } = useI18n();

type DynamicScrollerInstance = { $el: HTMLElement; scrollToBottom?: () => void; forceUpdate?: () => void };
const scroller = ref<DynamicScrollerInstance | null>(null);

// Auto-follow tracks whether the panel sticks to the tail when new events
// arrive. The button always jumps to the bottom on click and re-enables
// follow; the user disables follow implicitly by scrolling up to read
// history (handled in onScroll).
const followTail = ref(true);
let scrollListenerEl: HTMLElement | null = null;

function jumpToTail() {
  followTail.value = true;
  nextTick(() => {
    scroller.value?.scrollToBottom?.();
  });
}

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 12;
}

function onScroll(e: Event) {
  const el = e.target as HTMLElement;
  if (followTail.value && !isNearBottom(el)) {
    followTail.value = false;
  } else if (!followTail.value && isNearBottom(el)) {
    followTail.value = true;
  }
}

onMounted(async () => {
  await nextTick();
  const el = scroller.value?.$el;
  if (el) {
    scrollListenerEl = el;
    el.addEventListener('scroll', onScroll, { passive: true });
  }
});

onBeforeUnmount(() => {
  if (scrollHandle !== null) {
    cancelAnimationFrame(scrollHandle);
    scrollHandle = null;
  }
  scrollListenerEl?.removeEventListener('scroll', onScroll);
  scrollListenerEl = null;
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
  // Key embeds the entry count so that a row growing mid-stream (more events
  // landing on the same tick) yields a fresh key — DynamicScroller drops the
  // stale height cache and re-measures from scratch. Stable rows keep their
  // key across builds and are reused as-is.
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
  if (!props.active) return [];

  const rows: GridRow[] = [];
  const tickIndex = new Map<number, number>();
  const rowEntries: number[] = [];
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
      rowEntries.push(1);
      continue;
    }

    const tick = typeof e.tick === 'number' ? e.tick : null;
    let idx: number | undefined;
    if (tick !== null) {
      idx = tickIndex.get(tick);
    }
    if (idx === undefined) {
      idx = rows.length;
      const baseKey = tick !== null ? `t-${tick}` : `u-${untickedSeq++}`;
      rows.push({
        key: baseKey,
        tick,
        isGlobal: false,
        alt: tick !== null ? (tick % 2) === 1 : false,
        cells: new Map(),
      });
      rowEntries.push(0);
      if (tick !== null) tickIndex.set(tick, idx);
    }
    const row = rows[idx];
    const coreKey = e.coreId ?? -1;
    const bucket = row.cells.get(coreKey);
    if (bucket) bucket.push(e);
    else row.cells.set(coreKey, [e]);
    rowEntries[idx]++;
  }

  for (let i = 0; i < rows.length; i++) {
    const row = rows[i];
    if (!row.isGlobal) {
      row.key = `${row.key}-n${rowEntries[i]}`;
    }
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
// into one scroll per paint. forceUpdate flushes the DynamicScroller's
// pending size measurements before we scroll so rows that grew this batch
// (more events appended to the same tick) get laid out at their new
// height instead of overlapping the row below.
let scrollPending = false;
let scrollHandle: number | null = null;
watch(() => cpu.log.length, () => {
  if (!props.active) return;
  if (scrollPending) return;
  scrollPending = true;
  scrollHandle = requestAnimationFrame(() => {
    scrollPending = false;
    scrollHandle = null;
    scroller.value?.forceUpdate?.();
    if (followTail.value) {
      scroller.value?.scrollToBottom?.();
    }
  });
});

// Coming back from another tab: the body was empty while hidden, so it
// rebuilds in one pass and we need to re-stick to the tail.
watch(() => props.active, async (active) => {
  if (!active) return;
  await nextTick();
  if (followTail.value) {
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
              ><span v-if="entry.kind === 'scheduler'" class="grid__sched-chip" :title="t('panels.events.schedulerChipTitle')">S</span><span v-if="entry.inKernel" class="grid__kernel-chip" :title="t('panels.events.kernelChipTitle')">K</span><ArrowDownToLine v-if="entry.glyph === 'pcbSave'" class="grid__glyph" :size="12" :stroke-width="2" aria-hidden="true" /><ArrowUpFromLine v-else-if="entry.glyph === 'pcbRestore'" class="grid__glyph" :size="12" :stroke-width="2" aria-hidden="true" />{{ entry.text }}</div>
            </div>
          </div>
        </DynamicScrollerItem>
      </template>
    </DynamicScroller>

    <div v-if="props.active && gridRows.length === 0" class="empty">
      {{ cpu.log.length === 0 ? t('panels.events.emptyLoad') : t('panels.events.emptyNoCores') }}
    </div>

    <button
      type="button"
      class="follow-tail"
      :class="{ 'follow-tail--active': followTail }"
      :aria-pressed="followTail"
      :title="followTail ? t('panels.events.followTailOn') : t('panels.events.followTailOff')"
      @click="jumpToTail"
    ><ArrowDownToLine :size="14" :stroke-width="2.25" /></button>
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

/* Leading SVG glyph (pcb_save/restore). Kept inline with the row text and
   capped at the text cap-height so it can't push the row taller than the
   other entries that lead with a Unicode character. */
.grid__glyph {
  display: inline-block;
  width: 12px;
  height: 12px;
  margin-right: 4px;
  vertical-align: -2px;
  color: currentColor;
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

.follow-tail {
  position: absolute;
  right: 14px;
  bottom: 12px;
  width: 26px;
  height: 26px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--border);
  background: var(--surface-2, #131a22);
  color: var(--text-muted);
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
}
.follow-tail:hover {
  color: var(--text);
  border-color: var(--border-strong);
}
.follow-tail--active {
  color: var(--surface-2, #131a22);
  background: var(--accent);
  border-color: var(--accent);
}
.follow-tail--active:hover {
  background: color-mix(in oklab, var(--accent) 88%, white 12%);
  color: var(--surface-2, #131a22);
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
