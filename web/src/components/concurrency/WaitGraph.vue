<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import { useLockGraph, GRAPH_W, GRAPH_H, type GraphEdge, type GraphNode } from '../../composables/useLockGraph';
import { pidColor } from '../../lib/pidColors';
import Term from '../glossary/Term.vue';

const { t } = useI18n();
const cpu = useCpuStore();
const { nodes, edges, deadlockCycle } = useLockGraph();

const PROC_W = 66;
const PROC_H = 26;
const LOCK_R = 11;

const byId = computed(() => new Map(nodes.value.map(n => [n.id, n])));
const blockedPids = computed(
  () => new Set(cpu.procs.filter(p => p.state === 'BLOCKED').map(p => p.pid)),
);

const hasLocks = computed(() => nodes.value.some(n => n.kind !== 'proc'));
const hasWaits = computed(() => edges.value.some(e => e.kind === 'waits'));

// Distance from a node's center to its boundary along a unit direction, so
// arrowheads land just outside the shape instead of underneath it.
function boundaryDist(n: GraphNode, ux: number, uy: number): number {
  if (n.kind === 'proc') {
    const hx = PROC_W / 2 + 3;
    const hy = PROC_H / 2 + 3;
    const tx = ux !== 0 ? hx / Math.abs(ux) : Infinity;
    const ty = uy !== 0 ? hy / Math.abs(uy) : Infinity;
    return Math.min(tx, ty);
  }
  return LOCK_R + 4;
}

interface RenderedEdge extends GraphEdge {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

const renderedEdges = computed<RenderedEdge[]>(() => {
  const out: RenderedEdge[] = [];
  for (const e of edges.value) {
    const a = byId.value.get(e.from);
    const b = byId.value.get(e.to);
    if (!a || !b) continue;
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const len = Math.hypot(dx, dy) || 1;
    const ux = dx / len;
    const uy = dy / len;
    const padA = boundaryDist(a, ux, uy);
    const padB = boundaryDist(b, ux, uy) + 5; // room for the arrowhead
    out.push({
      ...e,
      x1: a.x + ux * padA,
      y1: a.y + uy * padA,
      x2: b.x - ux * padB,
      y2: b.y - uy * padB,
    });
  }
  return out;
});

function markerFor(e: GraphEdge): string {
  if (e.inCycle) return 'url(#wg-arrow-cycle)';
  return e.kind === 'holds' ? 'url(#wg-arrow-holds)' : 'url(#wg-arrow-waits)';
}

const procNodes = computed(() => nodes.value.filter(n => n.kind === 'proc'));
const lockNodes = computed(() => nodes.value.filter(n => n.kind !== 'proc'));

// At 8px mono the proc rect fits ~12 characters; longer names would spill
// over the edges. The full label stays available via the <title> tooltip.
const PROC_LABEL_MAX = 12;
function shortLabel(n: GraphNode): string {
  if (n.label.length <= PROC_LABEL_MAX) return n.label;
  return `${n.label.slice(0, PROC_LABEL_MAX - 1)}…`;
}

const cycleText = computed(() => {
  const c = deadlockCycle.value;
  if (!c) return '';
  const procs = c.filter(id => id.startsWith('p')).map(id => `P${id.slice(1)}`);
  return [...procs, procs[0]].join(' → ');
});
</script>

<template>
  <div class="wg">
    <div v-if="!hasLocks" class="wg__empty">
      <p>{{ t('sections.concurrency.waitGraph.noWaiters') }}</p>
      <p class="wg__empty-hint">{{ t('sections.concurrency.waitGraph.noLocksHint') }}</p>
    </div>

    <template v-else>
      <svg
        class="wg__svg"
        :viewBox="`0 0 ${GRAPH_W} ${GRAPH_H}`"
        xmlns="http://www.w3.org/2000/svg"
        role="img"
      >
        <defs>
          <marker id="wg-arrow-holds" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto" markerUnits="userSpaceOnUse">
            <path d="M0 0 L6 3 L0 6 Z" class="wg__arrow wg__arrow--holds" />
          </marker>
          <marker id="wg-arrow-waits" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto" markerUnits="userSpaceOnUse">
            <path d="M0 0 L6 3 L0 6 Z" class="wg__arrow wg__arrow--waits" />
          </marker>
          <marker id="wg-arrow-cycle" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto" markerUnits="userSpaceOnUse">
            <path d="M0 0 L6 3 L0 6 Z" class="wg__arrow wg__arrow--cycle" />
          </marker>
        </defs>

        <TransitionGroup tag="g" name="wg-fade">
          <line
            v-for="e in renderedEdges"
            :key="e.id"
            :x1="e.x1"
            :y1="e.y1"
            :x2="e.x2"
            :y2="e.y2"
            class="wg__edge"
            :class="[`wg__edge--${e.kind}`, { 'wg__edge--cycle': e.inCycle }]"
            :marker-end="markerFor(e)"
          />
        </TransitionGroup>

        <TransitionGroup tag="g" name="wg-fade">
        <g
          v-for="n in lockNodes"
          :key="n.id"
          class="wg__lock"
          :class="{ 'wg__lock--cycle': n.inCycle }"
        >
          <rect
            :x="n.x - LOCK_R"
            :y="n.y - LOCK_R"
            :width="LOCK_R * 2"
            :height="LOCK_R * 2"
            rx="2"
            class="wg__lock-shape"
            :transform="n.kind === 'mutex' ? `rotate(45 ${n.x} ${n.y})` : undefined"
          />
          <text :x="n.x" :y="n.y - LOCK_R - 6" class="wg__lock-label" text-anchor="middle">{{ n.label }}</text>
        </g>
        </TransitionGroup>

        <TransitionGroup tag="g" name="wg-fade">
        <g
          v-for="n in procNodes"
          :key="n.id"
          class="wg__proc"
          :class="{
            'wg__proc--blocked': blockedPids.has(n.refId),
            'wg__proc--cycle': n.inCycle,
          }"
          :style="{ color: pidColor(n.refId) }"
        >
          <rect
            :x="n.x - PROC_W / 2"
            :y="n.y - PROC_H / 2"
            :width="PROC_W"
            :height="PROC_H"
            rx="8"
            class="wg__proc-shape"
          />
          <text :x="n.x" :y="n.y + 3" class="wg__proc-label" text-anchor="middle">{{ shortLabel(n) }}</text>
          <title>{{ n.label }}</title>
        </g>
        </TransitionGroup>
      </svg>

      <p v-if="!hasWaits" class="wg__quiet">{{ t('sections.concurrency.waitGraph.noWaiters') }}</p>

      <div v-if="cycleText" class="wg__banner" role="alert">
        <strong class="wg__banner-head"><Term term="Deadlock" mode="label">{{ t('sections.concurrency.waitGraph.deadlockTitle') }}</Term></strong>
        <span>{{ t('sections.concurrency.waitGraph.deadlockBody', { cycle: cycleText }) }}</span>
      </div>

      <ul class="wg__legend">
        <li>
          <svg viewBox="0 0 24 6" class="wg__legend-swatch"><line x1="0" y1="3" x2="24" y2="3" class="wg__edge wg__edge--holds" /></svg>
          {{ t('sections.concurrency.waitGraph.legendHolds') }}
        </li>
        <li>
          <svg viewBox="0 0 24 6" class="wg__legend-swatch"><line x1="0" y1="3" x2="24" y2="3" class="wg__edge wg__edge--waits" /></svg>
          {{ t('sections.concurrency.waitGraph.legendWaits') }}
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.wg {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.wg__svg {
  width: 100%;
  display: block;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}

.wg__empty {
  font-size: 12px;
  color: var(--text-dim);
  border: 1px dashed var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}
.wg__empty p { margin: 0; }
.wg__empty-hint {
  margin-top: var(--space-2) !important;
  color: var(--text-muted);
  font-size: 11px;
  line-height: 1.5;
}

.wg__quiet {
  margin: 0;
  font-size: 11px;
  font-style: italic;
  color: var(--text-muted);
  text-align: center;
}

.wg__edge {
  fill: none;
  stroke-width: 1.4;
  vector-effect: non-scaling-stroke;
  transition: opacity var(--transition-base), stroke var(--transition-base);
}
.wg__edge--holds { stroke: var(--green); }
.wg__edge--waits { stroke: var(--amber); stroke-dasharray: 4 3; }
/* Marching ants on the deadlock circuit. The global prefers-reduced-motion
   rule in style.css collapses this animation for sensitive users. */
.wg__edge--cycle {
  stroke: var(--red);
  stroke-dasharray: 6 4;
  animation: wg-march 0.8s linear infinite;
}
@keyframes wg-march {
  to { stroke-dashoffset: -10; }
}

.wg__arrow--holds { fill: var(--green); }
.wg__arrow--waits { fill: var(--amber); }
.wg__arrow--cycle { fill: var(--red); }

.wg__lock-shape {
  fill: var(--surface-1);
  stroke: var(--text-dim);
  stroke-width: 1.2;
  vector-effect: non-scaling-stroke;
  transition: stroke var(--transition-base);
}
.wg__lock--cycle .wg__lock-shape { stroke: var(--red); }
.wg__lock-label {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  fill: var(--text-dim);
  letter-spacing: 1px;
}
.wg__lock--cycle .wg__lock-label { fill: var(--red); }

.wg__proc-shape {
  fill: currentColor;
  fill-opacity: 0.12;
  stroke: currentColor;
  stroke-width: 1.4;
  vector-effect: non-scaling-stroke;
  transition: stroke var(--transition-base), filter var(--transition-base);
}
.wg__proc-label {
  font-family: 'JetBrains Mono', monospace;
  font-size: 8px;
  fill: var(--text);
}
.wg__proc--blocked .wg__proc-shape {
  filter: drop-shadow(0 0 4px color-mix(in oklab, var(--yellow, #ffd54f) 55%, transparent));
}
.wg__proc--cycle { color: var(--red) !important; }
.wg__proc--cycle .wg__proc-shape {
  filter: drop-shadow(0 0 5px color-mix(in oklab, var(--red) 60%, transparent));
}

.wg__banner {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--red);
  border-radius: var(--radius-md);
  background: color-mix(in oklab, var(--red) 10%, transparent);
  padding: var(--space-2) var(--space-3);
  font-size: 11px;
  line-height: 1.5;
  color: var(--text);
}
.wg__banner-head {
  color: var(--red);
  letter-spacing: 1.5px;
  font-family: var(--font-display);
  font-size: 11px;
}

.wg__legend {
  list-style: none;
  display: flex;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  font-size: 10px;
  color: var(--text-muted);
}
.wg__legend li {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.wg__legend-swatch {
  width: 24px;
  height: 6px;
}

/* Fade for graph elements entering/leaving as locks change hands. */
.wg-fade-enter-active,
.wg-fade-leave-active {
  transition: opacity 0.25s ease;
}
.wg-fade-enter-from,
.wg-fade-leave-to {
  opacity: 0;
}
</style>
