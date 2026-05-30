<script setup lang="ts">
import { useCpuStore, type LogEntry } from '../stores/cpu';
import { computed, nextTick, onUpdated, ref } from 'vue';
import { useI18n } from 'vue-i18n';

const cpu = useCpuStore();
const scroller = ref<HTMLElement | null>(null);
const { t } = useI18n();

// Sticky autoscroll: follow new events only while the user is parked at the
// bottom. Once they scroll up to inspect history, we stop forcing the view
// down so they can read older ticks even as new ones keep arriving. A small
// threshold (in px) covers the gap left by fractional pixel rounding.
const STICK_THRESHOLD_PX = 24;
const stickToBottom = ref(true);

function onScroll() {
  const el = scroller.value;
  if (!el) return;
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
  stickToBottom.value = distance <= STICK_THRESHOLD_PX;
}

onUpdated(() => {
  if (!stickToBottom.value) return;
  // Defer one frame so layout has settled before measuring scrollHeight.
  nextTick(() => {
    const el = scroller.value;
    if (el) el.scrollTop = el.scrollHeight;
  });
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

// Visible cores from the SimBar filter. Entries without a coreId stay visible
// regardless of the filter (global rows + legacy events with no per-core tag).
const visibleLog = computed(() => cpu.log.filter(
  e => e.scope === 'global' || e.coreId === undefined || cpu.isCoreVisible(e.coreId),
));

const visibleCores = computed(() =>
  cpu.cores
    .map(c => c.id)
    .filter(id => cpu.isCoreVisible(id))
    .sort((a, b) => a - b),
);

const multicore = computed(() => visibleCores.value.length > 1);

interface GridRow {
  key: string;
  tick: number | null;
  isGlobal: boolean;
  globalEntry?: LogEntry;
  cells: Map<number, LogEntry[]>;
}

// Group entries into rows. Each tick becomes one row; entries with no tick
// fall back to a synthetic row that follows their arrival order (so legacy
// events without a tick stamp still show up). Global entries (loaded/reset/
// ws_*) get their own full-width row.
const gridRows = computed<GridRow[]>(() => {
  const rows: GridRow[] = [];
  const tickIndex = new Map<number, number>();
  let untickedSeq = 0;

  for (const e of visibleLog.value) {
    if (e.scope === 'global') {
      rows.push({
        key: `g-${rows.length}`,
        tick: null,
        isGlobal: true,
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
        key: tick !== null ? `t-${tick}-${idx}` : `u-${untickedSeq++}`,
        tick,
        isGlobal: false,
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
  // 56px for the tick column + one fr per visible core. Single-core has no
  // tick column at all (single-core sessions stay visually identical to the
  // legacy stream view).
  const cols = visibleCores.value.length;
  return {
    gridTemplateColumns: `56px repeat(${cols}, minmax(120px, 1fr))`,
  };
});
</script>

<template>
  <div class="panel log" ref="scroller" data-pizarra-anchor="events-panel" @scroll.passive="onScroll">
    <!-- Multi-core: grid with one column per core and one row per tick. -->
    <div v-if="multicore" class="grid" :style="gridStyle">
      <div class="grid__head grid__head--tick">{{ t('panels.events.tickColumn') }}</div>
      <div
        v-for="c in visibleCores"
        :key="`head-${c}`"
        class="grid__head"
        :style="{ color: `var(--core-${c % 4})` }"
      >C{{ c }}</div>

      <template v-for="(row, rowIdx) in gridRows" :key="row.key">
        <!-- Global rows: span the whole grid as a horizontal separator. -->
        <div
          v-if="row.isGlobal && row.globalEntry"
          class="grid__global"
          :style="{ gridColumn: `1 / span ${visibleCores.length + 1}`, color: colorOf(row.globalEntry.color) }"
          :aria-label="t('panels.events.globalRowAria')"
        >{{ row.globalEntry.text }}</div>

        <template v-else>
          <div class="grid__tick" :class="{ 'grid__cell--alt': rowIdx % 2 === 1 }">{{ row.tick !== null ? row.tick : '·' }}</div>
          <div
            v-for="c in visibleCores"
            :key="`${row.key}-c${c}`"
            class="grid__cell"
            :class="{
              'grid__cell--alt': rowIdx % 2 === 1,
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
        </template>
      </template>

      <div v-if="gridRows.length === 0" class="empty"
        :style="{ gridColumn: `1 / span ${visibleCores.length + 1}` }">
        {{ cpu.log.length === 0 ? t('panels.events.emptyLoad') : t('panels.events.emptyNoCores') }}
      </div>
    </div>

    <!-- Single-core: legacy stream layout to keep the introductory demo simple. -->
    <template v-else>
      <div class="panel-title">{{ t('panels.events.title') }}</div>
      <div
        v-for="(e, i) in visibleLog"
        :key="i"
        class="line"
        :class="{ 'line--global': e.scope === 'global', 'line--alt': i % 2 === 1 }"
        :style="{ color: colorOf(e.color) }"
      >
        <span class="line__text">{{ e.text }}</span>
      </div>
      <div v-if="visibleLog.length === 0" class="empty">
        {{ cpu.log.length === 0 ? t('panels.events.emptyLoad') : t('panels.events.emptyNoCores') }}
      </div>
    </template>
  </div>
</template>

<style scoped>
.log {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  /* Multicore grid renders its own sticky header flush against the panel
     edges; nuke the .panel inherited padding so the header sits on top
     without a strip of background showing through. */
  padding: 0 !important;
  font-size: 14px;
}

.line {
  padding: 1px 8px;
  font-family: 'JetBrains Mono', monospace;
  white-space: pre;
}

.line--alt {
  background: rgba(255, 255, 255, 0.025);
}

.line--global {
  font-style: italic;
  opacity: 0.85;
}

.line__text { /* baseline alignment handled by parent */ }

.grid {
  display: grid;
  gap: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
}

.grid__head {
  position: sticky;
  top: 0;
  background: var(--bg, #111);
  border-bottom: 1px solid var(--border-muted, rgba(255, 255, 255, 0.1));
  padding: 4px 6px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.5px;
  text-align: center;
  z-index: 1;
}

.grid__head--tick {
  color: var(--text-muted, rgba(255, 255, 255, 0.5));
}

.grid__tick {
  padding: 1px 4px;
  text-align: right;
  color: var(--text-muted, rgba(255, 255, 255, 0.45));
  font-size: 11px;
  border-right: 1px solid var(--border-muted, rgba(255, 255, 255, 0.06));
  white-space: nowrap;
}

.grid__cell {
  border-left: 2px solid transparent;
  padding: 1px 6px;
  min-height: 1.4em;
  white-space: pre-wrap;
  word-break: break-word;
}

/* Zebra striping: every other row gets a faint background so the eye can
   trace which (tick, core) cells belong together as one horizontal slice. */
.grid__cell--alt {
  background: rgba(255, 255, 255, 0.025);
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
  color: var(--text-muted);
  font-style: italic;
  padding: 20px 0;
  text-align: center;
}

@media (max-width: 768px) {
  .log {
    font-size: 12px;
  }
  .line {
    white-space: pre-wrap;
    word-break: break-word;
  }
  .grid {
    font-size: 11px;
  }
  .grid__cell {
    padding: 1px 4px;
  }
}
</style>
