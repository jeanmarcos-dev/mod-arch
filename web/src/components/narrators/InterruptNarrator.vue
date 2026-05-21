<script setup lang="ts">
import { onUnmounted, ref } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { useNarrators, type IrqFire, type ModeSwitch } from '../../lib/narratorBus';
import { usePreferencesStore } from '../../stores/preferences';

// InterruptNarrator stages the IRQ-enter / IRET-return arc. On irq_fire it
// draws a curved arrow from the CPU anchor to the IDT entry whose vector was
// just dispatched, and floats a "frame" card at the IDT side listing what
// the hardware just pushed onto the kernel stack (EIP, CS, EFLAGS). On
// mode_switch with cause=iret it draws the reverse arc back to the CPU,
// labelled IRET.
//
// We keep the most-recently-entered vector in a tiny stack so a chain of
// IRET hops can pop in LIFO order. The simulator only nests handlers
// shallowly, but the stack is the honest model.

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const narrators = useNarrators();

interface Hop {
  id: number;
  vec: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  kind: 'enter' | 'iret';
  source: string;
  frame: { EIP: string; CS: string; EFLAGS: string };
}

// IRETArrow is one of the three "stack → CPU" mini-arrows we fire when the
// kernel executes iret. Each arrow re-traces the journey of one saved word
// (EIP / CS / EFLAGS) from the stack panel back to its home register, with a
// small delay so the user reads them as a sequence rather than a single blob.
interface IRETArrow {
  id: number;
  coreId: number;
  label: 'EIP' | 'CS' | 'EFLAGS';
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  delayMs: number;
}

const hops = ref<Hop[]>([]);
const iretArrows = ref<IRETArrow[]>([]);
const root = ref<HTMLElement | null>(null);

const MAX_HOPS = 4;
// Total time the mini-arrow stays on screen, before stagger scaling.
const IRET_ARROW_MS = 700;
// Stagger between the three saved-word arrows (EIP, CS, EFLAGS).
const IRET_STAGGER_MS = 120;

const props = withDefaults(
  defineProps<{
    /** CSS selector for the CPU side (mode chip / register file). */
    cpuSelector?: string;
    /** Lookup for the IDT row of a given vector. */
    idtTarget?: (vec: number) => Element | null;
    /** Time each hop stays on screen in ms (scaled by animationSpeed). */
    ttlMs?: number;
  }>(),
  {
    cpuSelector: '[data-narrator-anchor="cpu"]',
    ttlMs: 1500,
  },
);

function rectCenter(el: Element | null): { x: number; y: number } | null {
  if (!el || !root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2 - local.left, y: r.top + r.height / 2 - local.top };
}

function defaultIdtTarget(vec: number): Element | null {
  return document.querySelector(`[data-narrator-anchor="idt-${vec}"]`) ?? null;
}

function hex(n: number, w = 6): string {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

// snapshotFrame reads EIP/CS/EFLAGS from the core that actually triggered the
// event, not the one the user happens to be inspecting. In single-core mode
// the two collapse to the same core; in multicore an IRQ on C1 must show C1's
// frame regardless of whether the SimBar is parked on C0.
function snapshotFrame(coreId: number) {
  const core = cpu.cores.find(c => c.id === coreId);
  const regs = core?.regs ?? cpu.regs;
  const segs = core?.segments ?? cpu.segments;
  return {
    EIP: hex(regs.EIP || 0),
    CS: hex(segs.CS || 0, 4),
    EFLAGS: hex(regs.EFLAGS || 0, 4),
  };
}

let seq = 0;
// One enter-stack per core so an IRET on C1 pops the IRQ that C1 took, not
// whichever vector landed last on any core. Without the per-core split, IRET
// arcs in a 2-core demo would chase the wrong vector half the time.
const enterStacks = ref<Record<number, number[]>>({});
function pushEnter(coreId: number, vec: number) {
  (enterStacks.value[coreId] ??= []).push(vec);
}
function popEnter(coreId: number): number | undefined {
  return enterStacks.value[coreId]?.pop();
}

function spawn(h: Omit<Hop, 'id'>) {
  seq++;
  if (hops.value.length >= MAX_HOPS) hops.value.shift();
  const id = seq;
  hops.value.push({ ...h, id });

  const ttl = Math.round(props.ttlMs / prefs.animationSpeed);
  window.setTimeout(() => {
    hops.value = hops.value.filter(x => x.id !== id);
  }, ttl + 100);
}

narrators.onIrqFire((ev: IrqFire) => {
  // Bookkeeping first, rendering second. The enterStack must stay balanced
  // even if the user toggles the core off mid-flight, otherwise a later IRET
  // pops the wrong vector. Nested kernel→kernel IRQs never produce a mode
  // flip on IRET, so we skip the push entirely to keep enter/pop paired.
  if (ev.fromMode === 'user') {
    pushEnter(ev.coreId, ev.vector);
  }

  if (!cpu.isCoreVisible(ev.coreId)) return;
  const cpuEl = document.querySelector(props.cpuSelector);
  const idtEl = props.idtTarget ? props.idtTarget(ev.vector) : defaultIdtTarget(ev.vector);
  if (!cpuEl || !idtEl) return;
  const from = rectCenter(cpuEl);
  const to = rectCenter(idtEl);
  if (!from || !to) return;

  spawn({
    vec: ev.vector,
    fromX: from.x, fromY: from.y,
    toX: to.x, toY: to.y,
    kind: 'enter', source: ev.source,
    frame: snapshotFrame(ev.coreId),
  });
});

narrators.onModeSwitch((ev: ModeSwitch) => {
  // Only model the kernel→user return triggered by an IRET. Other causes
  // (a fault elevating into kernel) are handled by ModeFlipNarrator.
  if (ev.to !== 'user') return;
  if (!/iret/i.test(ev.cause)) return;

  // Pop unconditionally so the per-core stack stays balanced even when the
  // core is hidden in the SimBar. Render is what gates on visibility.
  const vec = popEnter(ev.coreId);
  if (vec === undefined) return;

  if (!cpu.isCoreVisible(ev.coreId)) return;

  const cpuEl = document.querySelector(props.cpuSelector);
  const idtEl = props.idtTarget ? props.idtTarget(vec) : defaultIdtTarget(vec);
  if (!cpuEl || !idtEl) return;
  const cpuCenter = rectCenter(cpuEl);
  const idtCenter = rectCenter(idtEl);
  if (!cpuCenter || !idtCenter) return;

  spawn({
    vec,
    fromX: idtCenter.x, fromY: idtCenter.y,
    toX: cpuCenter.x, toY: cpuCenter.y,
    kind: 'iret', source: ev.cause,
    frame: snapshotFrame(ev.coreId),
  });

  spawnIretArrows(ev.coreId);
});

// spawnIretArrows fires three small arrows from the stack-frame panel rows
// (EIP / CS / EFLAGS) toward the CPU anchor, each delayed slightly so they
// read as a sequence. If the stack panel is not mounted (e.g. the user is on
// a different route, the narrator still runs because the bus is global) we
// silently skip — there is nothing to animate from.
function spawnIretArrows(coreId: number) {
  const stackRoot = document.querySelector(`[data-irq-stack="C${coreId}"]`);
  if (!stackRoot) return;
  const cpuEl = document.querySelector(props.cpuSelector);
  if (!cpuEl) return;
  const cpuTarget = rectCenter(cpuEl);
  if (!cpuTarget) return;

  const labels: Array<{ slot: string; label: 'EIP' | 'CS' | 'EFLAGS' }> = [
    { slot: 'eip', label: 'EIP' },
    { slot: 'cs', label: 'CS' },
    { slot: 'eflags', label: 'EFLAGS' },
  ];

  for (let i = 0; i < labels.length; i++) {
    const { slot, label } = labels[i];
    const slotEl = stackRoot.querySelector(`[data-stack-slot="${slot}"]`);
    if (!slotEl) continue;
    const from = rectCenter(slotEl);
    if (!from) continue;
    seq++;
    const id = seq;
    iretArrows.value.push({
      id,
      coreId,
      label,
      fromX: from.x, fromY: from.y,
      toX: cpuTarget.x, toY: cpuTarget.y,
      delayMs: i * IRET_STAGGER_MS,
    });
    const lifetime = (IRET_ARROW_MS + i * IRET_STAGGER_MS) / prefs.animationSpeed + 50;
    window.setTimeout(() => {
      iretArrows.value = iretArrows.value.filter(a => a.id !== id);
    }, lifetime);
  }
}

onUnmounted(() => {
  hops.value = [];
  iretArrows.value = [];
  enterStacks.value = {};
});

// The arrow path is a quadratic bezier whose control point sits above the
// midpoint to draw a noticeable arc rather than a straight line.
function curvePath(h: Hop): string {
  const midX = (h.fromX + h.toX) / 2;
  const midY = (h.fromY + h.toY) / 2;
  const lift = Math.min(120, Math.max(40, Math.abs(h.toX - h.fromX) * 0.25));
  // Lift the control point UP for ENTER (going right), DOWN for IRET so
  // the two paths don't overlap.
  const sign = h.kind === 'enter' ? -1 : 1;
  return `M ${h.fromX},${h.fromY} Q ${midX},${midY + sign * lift} ${h.toX},${h.toY}`;
}

// The card sits near the arc's destination so the student reads it after
// the arrow lands.
function cardPos(h: Hop): { x: number; y: number } {
  // ENTER lands on the IDT row — drop the card to the right of it.
  // IRET lands on the CPU anchor — drop the card BELOW it so the card
  // never overlaps section titles or sticky chrome sitting above the CPU
  // anchor (the mode chip on /interrupciones has the page title to its
  // right and only empty body below).
  if (h.kind === 'enter') {
    return { x: h.toX + 40, y: h.toY };
  }
  return { x: h.toX, y: h.toY + 70 };
}

function vecLabel(v: number): string {
  return '0x' + v.toString(16).padStart(2, '0').toUpperCase();
}
</script>

<template>
  <div ref="root" class="int-narrator" aria-hidden="true">
    <svg v-if="hops.length || iretArrows.length" class="int-narrator__svg">
      <defs>
        <marker id="int-arrow-enter" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="currentColor" />
        </marker>
        <marker id="int-arrow-iret" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="currentColor" />
        </marker>
        <marker id="int-arrow-mini" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="currentColor" />
        </marker>
      </defs>
      <template v-for="h in hops" :key="h.id">
        <path
          :class="['int-narrator__arc', `int-narrator__arc--${h.kind}`]"
          :d="curvePath(h)"
          :marker-end="h.kind === 'enter' ? 'url(#int-arrow-enter)' : 'url(#int-arrow-iret)'"
          :style="{ '--ttl': `${props.ttlMs / prefs.animationSpeed}ms` }"
        />
      </template>
      <template v-for="a in iretArrows" :key="`ia-${a.id}`">
        <path
          class="int-narrator__mini"
          :d="`M ${a.fromX},${a.fromY} L ${a.toX},${a.toY}`"
          marker-end="url(#int-arrow-mini)"
          :style="{ '--delay': `${a.delayMs / prefs.animationSpeed}ms`, '--ttl': `${IRET_ARROW_MS / prefs.animationSpeed}ms` }"
        />
        <text
          class="int-narrator__mini-label"
          :x="(a.fromX + a.toX) / 2"
          :y="(a.fromY + a.toY) / 2 - 6"
          text-anchor="middle"
          :style="{ '--delay': `${a.delayMs / prefs.animationSpeed}ms`, '--ttl': `${IRET_ARROW_MS / prefs.animationSpeed}ms` }"
        >{{ a.label }}</text>
      </template>
    </svg>

    <div
      v-for="h in hops"
      :key="h.id + 'card'"
      :class="['int-narrator__card', `int-narrator__card--${h.kind}`]"
      :style="{
        '--x': `${cardPos(h).x}px`,
        '--y': `${cardPos(h).y}px`,
        '--ttl': `${props.ttlMs / prefs.animationSpeed}ms`,
      }"
    >
      <header class="int-narrator__card-head">
        <span class="int-narrator__verb">{{ h.kind === 'enter' ? 'INT' : 'IRET' }}</span>
        <span class="int-narrator__vec">{{ vecLabel(h.vec) }}</span>
        <span class="int-narrator__source">{{ h.source }}</span>
      </header>
      <ul class="int-narrator__frame">
        <li>
          <span class="int-narrator__frame-name">EIP</span>
          <span class="int-narrator__frame-val">{{ h.frame.EIP }}</span>
        </li>
        <li>
          <span class="int-narrator__frame-name">CS</span>
          <span class="int-narrator__frame-val">{{ h.frame.CS }}</span>
        </li>
        <li>
          <span class="int-narrator__frame-name">EFLAGS</span>
          <span class="int-narrator__frame-val">{{ h.frame.EFLAGS }}</span>
        </li>
      </ul>
      <footer class="int-narrator__frame-foot">
        {{ h.kind === 'enter' ? 'pushed → kernel stack' : 'popped ← kernel stack' }}
      </footer>
    </div>
  </div>
</template>

<style scoped>
.int-narrator {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
  z-index: var(--z-overlay);
}

.int-narrator__svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.int-narrator__arc {
  fill: none;
  stroke-width: 2;
  stroke-dasharray: 6 4;
  animation: int-arc var(--ttl) ease-out forwards;
}

.int-narrator__arc--enter {
  stroke: var(--amber);
  color: var(--amber);
  filter: drop-shadow(0 0 5px rgba(var(--accent-rgb), 0.6));
}

.int-narrator__arc--iret {
  stroke: var(--green);
  color: var(--green);
  filter: drop-shadow(0 0 5px rgba(127, 255, 170, 0.6));
}

@keyframes int-arc {
  0%   { stroke-dashoffset: 0; opacity: 0; }
  20%  { opacity: 1; }
  100% { stroke-dashoffset: -240; opacity: 0; }
}

.int-narrator__card {
  position: absolute;
  left: 0;
  top: 0;
  min-width: 168px;
  padding: 6px 10px;
  background: var(--surface-2);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  transform: translate(calc(var(--x) - 50%), calc(var(--y) - 50%));
  animation: int-card var(--ttl) cubic-bezier(0.25, 0.5, 0.25, 1) forwards;
}

.int-narrator__card--enter {
  color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}

.int-narrator__card--iret {
  color: var(--green);
  box-shadow: 0 0 12px rgba(127, 255, 170, 0.45);
}

.int-narrator__card-head {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  border-bottom: 1px dashed currentColor;
  padding-bottom: 3px;
  margin-bottom: 4px;
}

.int-narrator__verb {
  font-weight: 700;
  letter-spacing: 1px;
}

.int-narrator__vec {
  color: var(--text);
}

.int-narrator__source {
  font-size: 9px;
  color: var(--text-dim);
  margin-left: auto;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.int-narrator__frame {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.int-narrator__frame li {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
}

.int-narrator__frame-name { color: var(--cyan); }
.int-narrator__frame-val  { color: inherit; }

.int-narrator__frame-foot {
  margin-top: 4px;
  padding-top: 3px;
  border-top: 1px dashed var(--border);
  font-size: 9px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
  text-align: center;
}

@keyframes int-card {
  0% { opacity: 0; transform: translate(calc(var(--x) - 50%), calc(var(--y) - 50%)) scale(0.85); }
  20% { opacity: 1; transform: translate(calc(var(--x) - 50%), calc(var(--y) - 50%)) scale(1); }
  85% { opacity: 1; }
  100% { opacity: 0; transform: translate(calc(var(--x) - 50%), calc(var(--y) - 50%)) scale(1.05); }
}

/* Mini-arrows for the IRET: one per saved word flying from the stack panel
   back to the CPU. They share a stagger driven by the per-arrow --delay so
   the three reads as a sequence. */
.int-narrator__mini {
  fill: none;
  stroke: var(--green);
  color: var(--green);
  stroke-width: 1.5;
  stroke-dasharray: 4 3;
  opacity: 0;
  filter: drop-shadow(0 0 4px rgba(127, 255, 170, 0.55));
  animation: int-mini-arc var(--ttl) ease-out forwards;
  animation-delay: var(--delay);
}

.int-narrator__mini-label {
  fill: var(--green);
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  opacity: 0;
  text-shadow: 0 0 6px rgba(127, 255, 170, 0.6);
  animation: int-mini-label var(--ttl) ease-out forwards;
  animation-delay: var(--delay);
}

@keyframes int-mini-arc {
  0%   { opacity: 0; stroke-dashoffset: 0; }
  20%  { opacity: 1; }
  100% { opacity: 0; stroke-dashoffset: -60; }
}

@keyframes int-mini-label {
  0%   { opacity: 0; }
  25%  { opacity: 1; }
  85%  { opacity: 1; }
  100% { opacity: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .int-narrator__arc,
  .int-narrator__card,
  .int-narrator__mini,
  .int-narrator__mini-label { animation-duration: 0.001ms !important; }
}
</style>
