<script setup lang="ts">
import { computed } from 'vue';
import type { SchedulerMetricsState, TimelineSlice } from '../../composables/useSchedulerMetrics';
import { pidColor } from '../../lib/pidColors';

const props = defineProps<{
  metrics: SchedulerMetricsState;
  currentTickRel: number;
  visibleCoreIds: number[];
  timerQuantum: number;
}>();

const WINDOW_TICKS = 200;
const BAND_HEIGHT = 26;
const BAND_GAP = 6;

const windowStart = computed(() => Math.max(0, props.currentTickRel - WINDOW_TICKS));
const windowEnd = computed(() => windowStart.value + WINDOW_TICKS);

const visibleBands = computed(() => {
  const ids = [...props.visibleCoreIds].sort((a, b) => a - b);
  return ids;
});

const svgHeight = computed(() =>
  Math.max(BAND_HEIGHT, visibleBands.value.length * (BAND_HEIGHT + BAND_GAP)),
);

interface RenderedSlice {
  key: string;
  coreId: number;
  pid: number;
  x: number;        // in tick coordinates
  width: number;    // in tick coordinates
  bandY: number;    // y of the band top edge
  live: boolean;
  exitReason: string | null;
  durationTicks: number;
  startTick: number;
  endTick: number;
}

function intersect(s: TimelineSlice): RenderedSlice | null {
  const start = s.startTick;
  const end = s.endTick ?? props.currentTickRel;
  if (end < windowStart.value || start >= windowEnd.value) return null;
  const x = Math.max(start, windowStart.value) - windowStart.value;
  const xEnd = Math.min(end, windowEnd.value) - windowStart.value;
  const width = Math.max(0.5, xEnd - x);
  const bandIndex = visibleBands.value.indexOf(s.coreId);
  if (bandIndex < 0) return null;
  return {
    key: `s${s.seq}`,
    coreId: s.coreId,
    pid: s.pid,
    x,
    width,
    bandY: bandIndex * (BAND_HEIGHT + BAND_GAP),
    live: s.endTick === null,
    exitReason: s.exitReason,
    durationTicks: end - start,
    startTick: start,
    endTick: end,
  };
}

const renderedSlices = computed<RenderedSlice[]>(() => {
  const out: RenderedSlice[] = [];
  for (const s of props.metrics.slices) {
    const r = intersect(s);
    if (r) out.push(r);
  }
  // Live slices live in liveSlicesByCore and are not yet in the historic
  // array. Project them too so the user sees the current quantum extending
  // to the right edge.
  for (const coreId of Object.keys(props.metrics.liveSlicesByCore)) {
    const s = props.metrics.liveSlicesByCore[Number(coreId)];
    const r = intersect(s);
    if (r) out.push(r);
  }
  return out;
});

const tickMarks = computed<number[]>(() => {
  const q = Math.max(1, props.timerQuantum);
  const marks: number[] = [];
  // Start at the first multiple of q ≥ windowStart.
  const first = Math.ceil(windowStart.value / q) * q;
  for (let t = first; t <= windowEnd.value; t += q) {
    marks.push(t);
  }
  return marks;
});

function tooltipFor(s: RenderedSlice): string {
  const reason = s.live
    ? 'en ejecución'
    : (s.exitReason ?? 'sin razón registrada');
  return `C${s.coreId} · P${s.pid}\n${s.durationTicks} ticks  ·  ${reason}\nticks ${s.startTick}–${s.endTick}`;
}

const isEmpty = computed(() =>
  renderedSlices.value.length === 0 && visibleBands.value.length > 0,
);
</script>

<template>
  <div class="qt">
    <div v-if="visibleBands.length === 0" class="qt__empty">
      Sin cores visibles. Active al menos uno en el selector de la SimBar.
    </div>
    <div v-else class="qt__inner">
      <ul class="qt__labels" :style="{ height: svgHeight + 'px' }">
        <li
          v-for="(coreId, i) in visibleBands"
          :key="`label-${coreId}`"
          class="qt__label"
          :style="{ top: i * (BAND_HEIGHT + BAND_GAP) + 'px', height: BAND_HEIGHT + 'px' }"
        >C{{ coreId }}</li>
      </ul>
      <div class="qt__svg-wrap">
        <svg
          class="qt__svg"
          :viewBox="`0 0 ${WINDOW_TICKS} ${svgHeight}`"
          :style="{ height: svgHeight + 'px' }"
          preserveAspectRatio="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <!-- background bands -->
          <rect
            v-for="(coreId, i) in visibleBands"
            :key="`band-${coreId}`"
            class="qt__band-bg"
            :x="0"
            :y="i * (BAND_HEIGHT + BAND_GAP)"
            :width="WINDOW_TICKS"
            :height="BAND_HEIGHT"
          />
          <!-- quantum grid -->
          <line
            v-for="t in tickMarks"
            :key="`grid-${t}`"
            class="qt__grid"
            :x1="t - windowStart"
            :x2="t - windowStart"
            :y1="0"
            :y2="svgHeight"
          />
          <!-- slices -->
          <g v-for="s in renderedSlices" :key="s.key">
            <rect
              :class="['qt__slice', { 'qt__slice--live': s.live }]"
              :x="s.x"
              :y="s.bandY + 2"
              :width="s.width"
              :height="BAND_HEIGHT - 4"
              :fill="pidColor(s.pid)"
              :opacity="s.live ? 0.85 : 1"
            >
              <title>{{ tooltipFor(s) }}</title>
            </rect>
          </g>
        </svg>
        <div v-if="isEmpty" class="qt__overlay-empty">Esperando primer quantum…</div>
        <div class="qt__axis">
          <span
            v-for="t in tickMarks"
            :key="`ax-${t}`"
            class="qt__axis-tick"
            :style="{ left: ((t - windowStart) / WINDOW_TICKS) * 100 + '%' }"
          >{{ t }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.qt {
  width: 100%;
}

.qt__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
  text-align: center;
}

.qt__inner {
  display: grid;
  grid-template-columns: 36px 1fr;
  gap: var(--space-2);
  align-items: start;
}

.qt__labels {
  position: relative;
  list-style: none;
  margin: 0;
  padding: 0;
}

.qt__label {
  position: absolute;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  font-family: var(--font-display);
  font-size: 11px;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  padding-right: 6px;
}

.qt__svg-wrap {
  position: relative;
  width: 100%;
}

.qt__svg {
  width: 100%;
  display: block;
}

.qt__band-bg {
  fill: var(--surface-2);
  stroke: var(--border);
  stroke-width: 0.5;
  vector-effect: non-scaling-stroke;
}

.qt__grid {
  stroke: rgba(var(--accent-rgb), 0.08);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.qt__slice {
  /* fill is set inline via pidColor */
  stroke: rgba(0, 0, 0, 0.35);
  stroke-width: 0.5;
  vector-effect: non-scaling-stroke;
  transition: opacity var(--transition-fast);
}

.qt__slice:hover {
  opacity: 0.65;
}

.qt__slice--live {
  stroke: var(--amber);
  stroke-width: 1.5;
  stroke-dasharray: 2 2;
  vector-effect: non-scaling-stroke;
  animation: qt-live-pulse 1.2s ease-in-out infinite;
}

@keyframes qt-live-pulse {
  0%, 100% { opacity: 0.85; }
  50%      { opacity: 1; }
}

.qt__overlay-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  pointer-events: none;
}

.qt__axis {
  position: relative;
  height: 14px;
  margin-top: 2px;
}

.qt__axis-tick {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  color: var(--text-muted);
  letter-spacing: 0.5px;
}
</style>
