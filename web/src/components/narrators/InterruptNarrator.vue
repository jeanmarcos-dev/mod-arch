<script setup lang="ts">
import { onUnmounted, ref, watch, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import { useNarrators, type IrqFire, type ModeSwitch } from '../../lib/narratorBus';
import { useNarratorAnimation } from './useNarratorAnimation';

const { t } = useI18n();

// InterruptNarrator stages the IRQ-enter / IRET-return arc. On irq_fire it
// draws a curved arrow from the CPU anchor to the IDT entry whose vector was
// just dispatched, and floats a "frame" card at the IDT side listing what
// the hardware just pushed onto the kernel stack (EIP, CS, EFLAGS). On
// mode_switch with cause=iret it draws the reverse arc back to the CPU,
// labelled IRET, plus three mini-arrows for each saved word.
//
// All visible animations last exactly one simulator tick. When cpu.paused
// flips to true (explicit user pause), --anim-state pauses every CSS
// animation; the WAAPI timers used to schedule removal pause through the
// composable. Idle autostops (HALT waiting for IRQ) do not freeze the
// narrator — the interrupt being shown already happened.

const cpu = useCpuStore();
const narrators = useNarrators();
const animator = useNarratorAnimation();

interface Hop {
  id: number;
  vec: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  kind: 'enter' | 'iret';
  source: string;
  coreId: number;
  frame: { EIP: string; CS: string; EFLAGS: string };
}

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

const props = withDefaults(
  defineProps<{
    cpuSelector?: string;
    idtTarget?: (vec: number) => Element | null;
  }>(),
  {
    cpuSelector: '[data-narrator-anchor="cpu"]',
  },
);

const ttlCss = computed(() => `${animator.tickMs.value}ms`);
// Stagger is a fraction of the tick so all three mini-arrows finish within
// one event window even when SPEED is very slow.
const iretStaggerMs = computed(() => Math.max(40, Math.round(animator.tickMs.value * 0.12)));
const iretArrowMs  = computed(() => Math.max(120, Math.round(animator.tickMs.value * 0.7)));

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

  // Pause/resume-aware one-tick timer for cleanup.
  const dummy = document.createElement('div');
  const anim = animator.play(dummy, [{ opacity: 0 }, { opacity: 0 }], { fill: 'none' });
  if (anim) {
    anim.finished
      .then(() => { hops.value = hops.value.filter(x => x.id !== id); })
      .catch(() => { /* cancelled */ });
  }
}

narrators.onIrqFire((ev: IrqFire) => {
  // Bookkeeping first so an IRET later pops the right vector even when the
  // narrator is suppressed for visibility reasons.
  if (ev.fromMode === 'user') {
    pushEnter(ev.coreId, ev.vector);
  }

  if (!cpu.isCoreVisible(ev.coreId)) return;
  if (!animator.shouldFire()) return;

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
    coreId: ev.coreId,
    frame: snapshotFrame(ev.coreId),
  });
});

narrators.onModeSwitch((ev: ModeSwitch) => {
  if (ev.to !== 'user') return;
  if (!/iret/i.test(ev.cause)) return;

  const vec = popEnter(ev.coreId);
  if (vec === undefined) return;

  if (!cpu.isCoreVisible(ev.coreId)) return;
  if (!animator.shouldFire()) return;

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
    coreId: ev.coreId,
    frame: snapshotFrame(ev.coreId),
  });

  spawnIretArrows(ev.coreId);
});

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
      delayMs: i * iretStaggerMs.value,
    });

    const dummy = document.createElement('div');
    const anim = animator.play(
      dummy,
      [{ opacity: 0 }, { opacity: 0 }],
      { fill: 'none', delayMs: i * iretStaggerMs.value, durationMs: iretArrowMs.value },
    );
    if (anim) {
      anim.finished
        .then(() => { iretArrows.value = iretArrows.value.filter(a => a.id !== id); })
        .catch(() => { /* cancelled */ });
    }
  }
}

watch(
  () => cpu.paused,
  (p) => {
    if (root.value) root.value.style.setProperty('--anim-state', p ? 'paused' : 'running');
  },
  { immediate: true },
);

onUnmounted(() => {
  hops.value = [];
  iretArrows.value = [];
  enterStacks.value = {};
});

function curvePath(h: Hop): string {
  const midX = (h.fromX + h.toX) / 2;
  const midY = (h.fromY + h.toY) / 2;
  const lift = Math.min(120, Math.max(40, Math.abs(h.toX - h.fromX) * 0.25));
  const sign = h.kind === 'enter' ? -1 : 1;
  return `M ${h.fromX},${h.fromY} Q ${midX},${midY + sign * lift} ${h.toX},${h.toY}`;
}

function cardPos(h: Hop): { x: number; y: number } {
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
          :style="{
            '--ttl': ttlCss,
            '--core-color': `var(--core-${h.coreId % 4})`,
          }"
        />
      </template>
      <template v-for="a in iretArrows" :key="`ia-${a.id}`">
        <path
          class="int-narrator__mini"
          :d="`M ${a.fromX},${a.fromY} L ${a.toX},${a.toY}`"
          marker-end="url(#int-arrow-mini)"
          :style="{ '--delay': `${a.delayMs}ms`, '--ttl': `${iretArrowMs}ms` }"
        />
        <text
          class="int-narrator__mini-label"
          :x="(a.fromX + a.toX) / 2"
          :y="(a.fromY + a.toY) / 2 - 6"
          text-anchor="middle"
          :style="{ '--delay': `${a.delayMs}ms`, '--ttl': `${iretArrowMs}ms` }"
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
        '--ttl': ttlCss,
        '--core-color': `var(--core-${h.coreId % 4})`,
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
        {{ h.kind === 'enter' ? t('shell.interruptNarrator.pushed') : t('shell.interruptNarrator.popped') }}
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
  animation-play-state: var(--anim-state, running);
}

.int-narrator__arc--enter {
  stroke: var(--core-color, var(--amber));
  color: var(--core-color, var(--amber));
  filter: drop-shadow(0 0 5px color-mix(in oklab, var(--core-color, var(--amber)) 60%, transparent));
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
  animation-play-state: var(--anim-state, running);
}

.int-narrator__card--enter {
  color: var(--core-color, var(--amber));
  box-shadow: 0 0 12px color-mix(in oklab, var(--core-color, var(--amber)) 35%, transparent);
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
  animation-play-state: var(--anim-state, running);
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
  animation-play-state: var(--anim-state, running);
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
