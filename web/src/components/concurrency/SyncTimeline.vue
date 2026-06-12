<script setup lang="ts">
import { computed, useId } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import {
  useSchedulerMetrics,
  type UseSchedulerMetricsReturn,
  type StateSegment,
  type LockMark,
} from '../../composables/useSchedulerMetrics';
import { pidColor } from '../../lib/pidColors';
import { isUserProc } from '../../lib/userProcs';

// An external metrics instance lets the parent view share one accumulator
// across panels (and guarantees deadlockAtTick lives in the same relative
// tick space). Standalone usage falls back to a private one. Read once at
// setup: swapping the prop later is not supported.
const props = defineProps<{
  deadlockAtTick?: number | null;
  metrics?: UseSchedulerMetricsReturn;
}>();

const { t } = useI18n();
const cpu = useCpuStore();
const internal = props.metrics ? null : useSchedulerMetrics();
const { metrics, currentTickRel } = props.metrics ?? internal!;

const WINDOW_TICKS = 200;
const LANE_HEIGHT = 22;
const LANE_GAP = 6;
// Inline labels (lock id, core id) only render on segments wide enough to
// host them without spilling over the neighbours.
const MIN_LABEL_TICKS = 8;

// Scope the SVG pattern id per instance so two mounted timelines never
// reference each other's defs.
const patternId = `st-blocked-${useId()}`;

const windowStart = computed(() => Math.max(0, currentTickRel.value - WINDOW_TICKS));
const windowEnd = computed(() => windowStart.value + WINDOW_TICKS);

const procByPid = computed(() => new Map(cpu.procs.map(p => [p.pid, p])));

// User processes only: the kernel-side machinery (swapper, kthreads, init)
// follows the same exclusion the process tree applies. PIDs whose PCB is
// already gone (reaped) stay visible — only user procs get reaped anyway.
function isUserPid(pid: number): boolean {
  if (pid <= 0) return false;
  const p = procByPid.value.get(pid);
  return !p || isUserProc(p);
}

const lanePids = computed<number[]>(() => {
  const set = new Set<number>();
  for (const s of metrics.stateSegments) if (isUserPid(s.pid)) set.add(s.pid);
  for (const key of Object.keys(metrics.liveSegmentByPid)) {
    const pid = Number(key);
    if (isUserPid(pid)) set.add(pid);
  }
  return [...set].sort((a, b) => a - b);
});

const laneIndex = computed(() => new Map(lanePids.value.map((pid, i) => [pid, i])));

const svgHeight = computed(() =>
  Math.max(LANE_HEIGHT, lanePids.value.length * (LANE_HEIGHT + LANE_GAP)),
);

function laneName(pid: number): string {
  return procByPid.value.get(pid)?.name ?? metrics.byPid[pid]?.name ?? `P${pid}`;
}

function laneLabel(pid: number): string {
  const name = laneName(pid);
  return name === `P${pid}` ? name : `P${pid} ${name}`;
}

interface RenderedSeg {
  key: string;
  pid: number;
  state: StateSegment['state'];
  x: number;       // tick coordinates inside the window
  width: number;   // tick coordinates
  laneY: number;   // px, lane top edge
  live: boolean;
  coreId?: number;
  reason?: string;
  lockTag: string;
  startTick: number;
  endTick: number;
  durationTicks: number;
}

function lockTagFor(reason?: string): string {
  const m = /^mutex:(\d+)$/.exec(reason ?? '');
  if (m) return `M${m[1]}`;
  const s = /^sem:(\d+)$/.exec(reason ?? '');
  if (s) return `S${s[1]}`;
  return '';
}

function lockNameFor(reason: string): string {
  const m = /^mutex:(\d+)$/.exec(reason);
  if (m) return t('sections.concurrency.syncTimeline.lockMutex', { id: m[1] });
  const s = /^sem:(\d+)$/.exec(reason);
  if (s) return t('sections.concurrency.syncTimeline.lockSem', { id: s[1] });
  return reason;
}

function intersect(s: StateSegment): RenderedSeg | null {
  const start = s.startTick;
  const end = s.endTick ?? currentTickRel.value;
  if (end < windowStart.value || start >= windowEnd.value) return null;
  const lane = laneIndex.value.get(s.pid);
  if (lane === undefined) return null;
  const x = Math.max(start, windowStart.value) - windowStart.value;
  const xEnd = Math.min(end, windowEnd.value) - windowStart.value;
  return {
    key: `g${s.seq}`,
    pid: s.pid,
    state: s.state,
    x,
    width: Math.max(0.5, xEnd - x),
    laneY: lane * (LANE_HEIGHT + LANE_GAP),
    live: s.endTick === null,
    coreId: s.coreId,
    reason: s.reason,
    lockTag: s.state === 'BLOCKED' ? lockTagFor(s.reason) : '',
    startTick: start,
    endTick: end,
    durationTicks: end - start,
  };
}

const renderedSegments = computed<RenderedSeg[]>(() => {
  const out: RenderedSeg[] = [];
  for (const s of metrics.stateSegments) {
    const r = intersect(s);
    if (r) out.push(r);
  }
  for (const key of Object.keys(metrics.liveSegmentByPid)) {
    const r = intersect(metrics.liveSegmentByPid[Number(key)]);
    if (r) out.push(r);
  }
  return out;
});

interface RenderedMark {
  key: string;
  pid: number;
  kind: LockMark['kind'];
  leftPct: number;
  laneY: number;
  glyph: string;
  tick: number;
}

const MARK_GLYPHS: Record<LockMark['kind'], string> = {
  acquired: '▼',
  waiting: '✖',
  released: '▲',
};

const renderedMarks = computed<RenderedMark[]>(() => {
  const out: RenderedMark[] = [];
  for (const m of metrics.lockMarks) {
    if (m.tick < windowStart.value || m.tick > windowEnd.value) continue;
    const lane = laneIndex.value.get(m.pid);
    if (lane === undefined) continue;
    out.push({
      key: `m${m.seq}`,
      pid: m.pid,
      kind: m.kind,
      leftPct: ((m.tick - windowStart.value) / WINDOW_TICKS) * 100,
      laneY: lane * (LANE_HEIGHT + LANE_GAP),
      glyph: MARK_GLYPHS[m.kind],
      tick: m.tick,
    });
  }
  return out;
});

function markTooltip(m: RenderedMark): string {
  const key = m.kind === 'acquired' ? 'markAcquired' : m.kind === 'waiting' ? 'markWaiting' : 'markReleased';
  return t(`sections.concurrency.syncTimeline.${key}`, { pid: m.pid, tick: m.tick });
}

function tooltipFor(s: RenderedSeg): string {
  const params = { pid: s.pid, n: s.durationTicks, start: s.startTick, end: s.endTick };
  let msg: string;
  if (s.state === 'RUN') {
    msg = s.coreId !== undefined && cpu.numCores > 1
      ? t('sections.concurrency.syncTimeline.tipRunCore', { ...params, core: s.coreId })
      : t('sections.concurrency.syncTimeline.tipRun', params);
  } else if (s.state === 'BLOCKED') {
    msg = s.reason
      ? t('sections.concurrency.syncTimeline.tipBlockedOn', { ...params, lock: lockNameFor(s.reason) })
      : t('sections.concurrency.syncTimeline.tipBlocked', params);
  } else {
    msg = t('sections.concurrency.syncTimeline.tipReady', params);
  }
  return s.live ? msg + t('sections.concurrency.syncTimeline.tipLiveSuffix') : msg;
}

const tickMarks = computed<number[]>(() => {
  const q = Math.max(1, cpu.timerQuantum);
  const marks: number[] = [];
  const first = Math.ceil(windowStart.value / q) * q;
  for (let tk = first; tk <= windowEnd.value; tk += q) marks.push(tk);
  return marks;
});

// With a tiny quantum the grid yields up to 201 marks; the labels would
// overlap into an unreadable smear, so only every k-th gets a number.
const MAX_AXIS_LABELS = 25;
const axisMarks = computed<number[]>(() => {
  const marks = tickMarks.value;
  const stride = Math.max(1, Math.ceil(marks.length / MAX_AXIS_LABELS));
  return marks.filter((_, i) => i % stride === 0);
});

// The deadlock instant clamps to the left edge once it scrolls out of the
// window: a deadlock never resolves on its own, so the line staying pinned
// reads as "blocked since before what you can see".
const deadlockX = computed<number | null>(() => {
  const at = props.deadlockAtTick;
  if (at === null || at === undefined) return null;
  if (at > windowEnd.value) return null;
  return Math.max(0, at - windowStart.value);
});

const hasLanes = computed(() => lanePids.value.length > 0);
</script>

<template>
  <div class="st">
    <div v-if="!hasLanes" class="st__empty">
      {{ t('sections.concurrency.syncTimeline.empty') }}
    </div>

    <template v-else>
      <div class="st__inner">
        <ul class="st__labels" :style="{ height: svgHeight + 'px' }">
          <li
            v-for="(pid, i) in lanePids"
            :key="`label-${pid}`"
            class="st__lane-label"
            :style="{ top: i * (LANE_HEIGHT + LANE_GAP) + 'px', height: LANE_HEIGHT + 'px' }"
          >
            <span class="st__chip" :style="{ background: pidColor(pid) }" aria-hidden="true" />
            <span class="st__lane-text" :title="laneLabel(pid)">{{ laneLabel(pid) }}</span>
          </li>
        </ul>

        <div class="st__svg-wrap">
          <svg
            class="st__svg"
            :viewBox="`0 0 ${WINDOW_TICKS} ${svgHeight}`"
            :style="{ height: svgHeight + 'px' }"
            preserveAspectRatio="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <defs>
              <pattern :id="patternId" patternUnits="userSpaceOnUse" width="2" height="8">
                <rect class="st__blocked-bg" x="0" y="0" width="2" height="8" />
                <path class="st__blocked-stripe" d="M0 8 L2 0" />
              </pattern>
            </defs>

            <!-- lane backgrounds -->
            <rect
              v-for="(pid, i) in lanePids"
              :key="`band-${pid}`"
              class="st__lane-bg"
              :x="0"
              :y="i * (LANE_HEIGHT + LANE_GAP)"
              :width="WINDOW_TICKS"
              :height="LANE_HEIGHT"
            />

            <!-- quantum grid -->
            <line
              v-for="tk in tickMarks"
              :key="`grid-${tk}`"
              class="st__grid"
              :x1="tk - windowStart"
              :x2="tk - windowStart"
              :y1="0"
              :y2="svgHeight"
            />

            <!-- state segments -->
            <g v-for="s in renderedSegments" :key="s.key">
              <rect
                class="st__seg"
                :class="[`st__seg--${s.state.toLowerCase()}`, { 'st__seg--live': s.live }]"
                :x="s.x"
                :y="s.laneY + 2"
                :width="s.width"
                :height="LANE_HEIGHT - 4"
                :fill="s.state === 'RUN' ? pidColor(s.pid) : s.state === 'BLOCKED' ? `url(#${patternId})` : undefined"
              >
                <title>{{ tooltipFor(s) }}</title>
              </rect>
            </g>

            <!-- deadlock instant -->
            <line
              v-if="deadlockX !== null"
              class="st__deadlock-line"
              :x1="deadlockX"
              :x2="deadlockX"
              :y1="0"
              :y2="svgHeight"
            />
          </svg>

          <!-- HTML overlay: text would distort inside the stretched SVG -->
          <div class="st__overlay" aria-hidden="true">
            <template v-for="s in renderedSegments" :key="`ov-${s.key}`">
              <span
                v-if="s.lockTag && s.width >= MIN_LABEL_TICKS"
                class="st__lock-tag"
                :style="{
                  left: ((s.x + 1) / WINDOW_TICKS) * 100 + '%',
                  top: s.laneY + 2 + 'px',
                  height: LANE_HEIGHT - 4 + 'px',
                }"
              >
                <svg class="st__lock-glyph" viewBox="0 0 8 8" width="7" height="7">
                  <path d="M2.6 3.4 V2.4 a1.4 1.4 0 0 1 2.8 0 V3.4" fill="none" stroke="currentColor" stroke-width="0.9" />
                  <rect x="1.6" y="3.4" width="4.8" height="3.6" rx="0.6" fill="currentColor" />
                </svg>
                {{ s.lockTag }}
              </span>
              <span
                v-if="s.state === 'RUN' && s.coreId !== undefined && cpu.numCores > 1 && s.width >= MIN_LABEL_TICKS"
                class="st__core-tag"
                :style="{
                  left: ((s.x + 1) / WINDOW_TICKS) * 100 + '%',
                  top: s.laneY + 2 + 'px',
                  height: LANE_HEIGHT - 4 + 'px',
                }"
              >C{{ s.coreId }}</span>
            </template>

            <span
              v-for="m in renderedMarks"
              :key="m.key"
              class="st__mark"
              :class="`st__mark--${m.kind}`"
              :style="{
                left: m.leftPct + '%',
                top: m.laneY - 1 + 'px',
                color: m.kind === 'acquired' ? pidColor(m.pid) : undefined,
              }"
              :title="markTooltip(m)"
            >{{ m.glyph }}</span>

            <span
              v-if="deadlockX !== null"
              class="st__deadlock-label"
              :style="{ left: (deadlockX / WINDOW_TICKS) * 100 + '%' }"
            >{{ t('sections.concurrency.syncTimeline.deadlockLabel') }}</span>
          </div>

          <div class="st__axis">
            <span
              v-for="tk in axisMarks"
              :key="`ax-${tk}`"
              class="st__axis-tick"
              :style="{ left: ((tk - windowStart) / WINDOW_TICKS) * 100 + '%' }"
            >{{ tk }}</span>
          </div>
        </div>
      </div>

      <ul class="st__legend">
        <li class="st__legend-item">
          <span class="st__swatch st__swatch--run" aria-hidden="true" />
          {{ t('sections.concurrency.syncTimeline.legendRun') }}
        </li>
        <li class="st__legend-item">
          <span class="st__swatch st__swatch--blocked" aria-hidden="true" />
          {{ t('sections.concurrency.syncTimeline.legendBlocked') }}
        </li>
        <li class="st__legend-item">
          <span class="st__swatch st__swatch--ready" aria-hidden="true" />
          {{ t('sections.concurrency.syncTimeline.legendReady') }}
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.st {
  width: 100%;
}

.st__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
  text-align: center;
}

.st__inner {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: var(--space-2);
  align-items: start;
}

.st__labels {
  position: relative;
  list-style: none;
  margin: 0;
  padding: 0;
}

.st__lane-label {
  position: absolute;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 5px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
  padding-right: 4px;
}

.st__chip {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-radius: var(--radius-sm);
}

.st__lane-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.st__svg-wrap {
  position: relative;
  width: 100%;
}

.st__svg {
  width: 100%;
  display: block;
}

.st__lane-bg {
  fill: var(--surface-2);
  stroke: var(--border);
  stroke-width: 0.5;
  vector-effect: non-scaling-stroke;
}

.st__grid {
  stroke: rgba(var(--accent-rgb), 0.08);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.st__seg {
  stroke: rgba(0, 0, 0, 0.35);
  stroke-width: 0.5;
  vector-effect: non-scaling-stroke;
}

.st__seg--run {
  /* fill set inline via pidColor */
  opacity: 0.8;
}

.st__seg--ready {
  fill: var(--text-muted);
  opacity: 0.18;
}

.st__seg--blocked {
  /* fill set inline via the stripe pattern */
  stroke: rgba(var(--accent-rgb), 0.4);
}

.st__seg--live.st__seg--blocked {
  stroke: var(--amber);
  stroke-dasharray: 2 2;
}

.st__seg:hover {
  opacity: 1;
  stroke: var(--text);
}

.st__blocked-bg {
  fill: rgba(var(--accent-rgb), 0.12);
}

.st__blocked-stripe {
  stroke: rgba(var(--accent-rgb), 0.65);
  stroke-width: 0.7;
}

.st__overlay {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.st__lock-tag,
.st__core-tag {
  position: absolute;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 8px;
  letter-spacing: 0.5px;
  white-space: nowrap;
}

.st__lock-tag {
  color: var(--amber);
}

.st__core-tag {
  color: rgba(0, 0, 0, 0.8);
}

.st__lock-glyph {
  flex: 0 0 auto;
}

.st__mark {
  position: absolute;
  transform: translateX(-50%);
  font-size: 8px;
  line-height: 1;
  pointer-events: auto;
  cursor: help;
  text-shadow: 0 0 3px rgba(0, 0, 0, 0.8);
}

.st__mark--waiting {
  color: var(--amber);
}

.st__mark--released {
  color: var(--green);
}

.st__deadlock-line {
  stroke: var(--red);
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
  filter: drop-shadow(0 0 3px rgba(255, 58, 58, 0.6));
}

.st__deadlock-label {
  position: absolute;
  top: -2px;
  transform: translateX(3px);
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--red-soft);
}

.st__axis {
  position: relative;
  height: 14px;
  margin-top: 2px;
}

.st__axis-tick {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  color: var(--text-muted);
  letter-spacing: 0.5px;
}

.st__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  list-style: none;
  margin: var(--space-2) 0 0;
  padding: 0;
}

.st__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 10px;
  color: var(--text-dim);
}

.st__swatch {
  width: 14px;
  height: 8px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
}

.st__swatch--run {
  background: var(--cyan);
  opacity: 0.8;
}

.st__swatch--blocked {
  background: repeating-linear-gradient(
    45deg,
    rgba(var(--accent-rgb), 0.12),
    rgba(var(--accent-rgb), 0.12) 2px,
    rgba(var(--accent-rgb), 0.65) 2px,
    rgba(var(--accent-rgb), 0.65) 3px
  );
}

.st__swatch--ready {
  background: var(--text-muted);
  opacity: 0.25;
}
</style>
