<script setup lang="ts">
import { onUnmounted, ref } from 'vue';
import { useNarrators, type PcbSave, type PcbRestore } from '../../lib/narratorBus';
import { usePreferencesStore } from '../../stores/preferences';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();

// PCBCopyNarrator animates the kernel's pcb_save / pcb_restore work. On
// pcb_save a card flies from the CPU anchor to the row of the PID whose
// context was just frozen. On pcb_restore the card flies in the opposite
// direction. The card displays the most relevant registers (EIP, ESP, EAX,
// EBX) so the student can see "this is the chunk of state being copied"
// rather than a vague glow.
//
// Place inside a position: relative parent — it covers it via inset: 0 and
// lets pointer events fall through.

interface Trip {
  id: number;
  pid: number;
  coreId: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  pairs: { name: string; val: string }[];
  kind: 'save' | 'restore';
}

const props = withDefaults(
  defineProps<{
    /**
     * CSS selector for the CPU side (register file) when no per-core anchor
     * exists. Kept as a fallback so single-core demos and views without core
     * panels still light up.
     */
    cpuSelector?: string;
    /** Function returning the PCB row element for a given PID. */
    pcbTarget?: (pid: number) => Element | null;
    /** Time each card stays on screen in ms (scaled by animationSpeed). */
    ttlMs?: number;
  }>(),
  {
    cpuSelector: '[data-narrator-anchor="cpu"]',
    ttlMs: 1300,
  },
);

// Resolve the CPU-side anchor for a given core. Multicore views render one
// `data-narrator-anchor="cpu-<id>"` per strip; falling back to the generic
// "cpu" anchor keeps the legacy single-core panels working unchanged.
function cpuAnchorFor(coreId: number): Element | null {
  return (
    document.querySelector(`[data-narrator-anchor="cpu-${coreId}"]`)
    ?? document.querySelector(props.cpuSelector)
  );
}

const trips = ref<Trip[]>([]);
const root = ref<HTMLElement | null>(null);
const prefs = usePreferencesStore();

const MAX_TRIPS = 4;

function rectCenter(el: Element | null): { x: number; y: number } | null {
  if (!el || !root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2 - local.left, y: r.top + r.height / 2 - local.top };
}

function defaultPcbTarget(pid: number): Element | null {
  return document.querySelector(`[data-narrator-anchor="pcb-${pid}"]`) ?? null;
}

function hex(n: number, w = 6): string {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function pickRegs(regs: Record<string, number>): { name: string; val: string }[] {
  return ['EIP', 'ESP', 'EAX', 'EBX']
    .filter(k => k in regs)
    .map(k => ({ name: k, val: hex(regs[k]) }));
}

const narrators = useNarrators();

function launch(kind: 'save' | 'restore', payload: PcbSave | PcbRestore) {
  // Skip events from cores hidden by the SimBar filter so the user does not
  // see arcs flying from off-screen anchors.
  if (!cpu.isCoreVisible(payload.coreId)) return;
  const cpuEl = cpuAnchorFor(payload.coreId);
  const pcbEl = props.pcbTarget ? props.pcbTarget(payload.pid) : defaultPcbTarget(payload.pid);
  if (!cpuEl || !pcbEl) return;

  const cpuCenter = rectCenter(cpuEl);
  const pcbCenter = rectCenter(pcbEl);
  if (!cpuCenter || !pcbCenter) return;

  const from = kind === 'save' ? cpuCenter : pcbCenter;
  const to = kind === 'save' ? pcbCenter : cpuCenter;

  if (trips.value.length >= MAX_TRIPS) trips.value.shift();

  trips.value.push({
    id: payload.seq,
    pid: payload.pid,
    coreId: payload.coreId,
    fromX: from.x,
    fromY: from.y,
    toX: to.x,
    toY: to.y,
    pairs: pickRegs(payload.regs),
    kind,
  });

  const ttl = Math.round(props.ttlMs / prefs.animationSpeed);
  window.setTimeout(() => {
    trips.value = trips.value.filter(t => t.id !== payload.seq);
  }, ttl + 80);
}

narrators.onPcbSave((p) => launch('save', p));
narrators.onPcbRestore((p) => launch('restore', p));

onUnmounted(() => { trips.value = []; });
</script>

<template>
  <div ref="root" class="pcb-copy" aria-hidden="true">
    <svg v-if="trips.length" class="pcb-copy__svg">
      <template v-for="t in trips" :key="t.id">
        <line
          :class="['pcb-copy__trail', `pcb-copy__trail--${t.kind}`]"
          :x1="t.fromX"
          :y1="t.fromY"
          :x2="t.toX"
          :y2="t.toY"
          :style="{
            '--ttl': `${props.ttlMs / prefs.animationSpeed}ms`,
            '--core-color': `var(--core-${t.coreId % 4})`,
          }"
        />
      </template>
    </svg>

    <div
      v-for="t in trips"
      :key="t.id + 'c'"
      :class="['pcb-copy__card', `pcb-copy__card--${t.kind}`]"
      :style="{
        '--from-x': `${t.fromX}px`,
        '--from-y': `${t.fromY}px`,
        '--to-x':   `${t.toX}px`,
        '--to-y':   `${t.toY}px`,
        '--ttl':    `${props.ttlMs / prefs.animationSpeed}ms`,
        '--core-color': `var(--core-${t.coreId % 4})`,
      }"
    >
      <header class="pcb-copy__card-head">
        <span class="pcb-copy__verb">{{ t.kind === 'save' ? 'SAVE' : 'RESTORE' }}</span>
        <span class="pcb-copy__pid">P{{ t.pid }}</span>
      </header>
      <ul class="pcb-copy__regs">
        <li v-for="r in t.pairs" :key="r.name">
          <span class="pcb-copy__reg-name">{{ r.name }}</span>
          <span class="pcb-copy__reg-val">{{ r.val }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.pcb-copy {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
  z-index: var(--z-overlay);
}

.pcb-copy__svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.pcb-copy__trail {
  stroke-width: 1.5;
  fill: none;
  stroke-dasharray: 6 4;
  animation: pcb-trail var(--ttl) linear forwards;
}

/* SAVE picks up the source core's tint (matches the InterruptNarrator
   convention). RESTORE stays green — semantically "this PID is now running
   again" — regardless of which core received the context. */
.pcb-copy__trail--save {
  stroke: var(--core-color, var(--amber));
  filter: drop-shadow(0 0 5px color-mix(in oklab, var(--core-color, var(--amber)) 60%, transparent));
}

.pcb-copy__trail--restore {
  stroke: var(--green);
  filter: drop-shadow(0 0 5px rgba(127, 255, 170, 0.6));
}

@keyframes pcb-trail {
  0%   { stroke-dashoffset: 0; opacity: 0; }
  18%  { opacity: 1; }
  100% { stroke-dashoffset: -180; opacity: 0; }
}

.pcb-copy__card {
  position: absolute;
  left: 0;
  top: 0;
  min-width: 132px;
  padding: 6px 8px;
  background: var(--surface-2);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%));
  animation: pcb-fly var(--ttl) cubic-bezier(0.25, 0.5, 0.25, 1) forwards;
}

.pcb-copy__card--save {
  color: var(--core-color, var(--amber));
  box-shadow: 0 0 12px color-mix(in oklab, var(--core-color, var(--amber)) 35%, transparent);
}

.pcb-copy__card--restore {
  color: var(--green);
  box-shadow: 0 0 12px rgba(127, 255, 170, 0.4);
}

.pcb-copy__card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
  border-bottom: 1px dashed currentColor;
  padding-bottom: 3px;
}

.pcb-copy__verb {
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
}

.pcb-copy__pid {
  font-weight: 700;
  font-size: 12px;
}

.pcb-copy__regs {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 2px 8px;
}

.pcb-copy__regs li {
  display: flex;
  justify-content: space-between;
  gap: 4px;
  font-size: 10px;
}

.pcb-copy__reg-name {
  color: var(--text-dim);
}

.pcb-copy__reg-val {
  color: inherit;
}

@keyframes pcb-fly {
  0% {
    transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)) scale(0.85);
    opacity: 0;
  }
  18% {
    opacity: 1;
    transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)) scale(1);
  }
  88% { opacity: 1; }
  100% {
    transform: translate(calc(var(--to-x) - 50%), calc(var(--to-y) - 50%)) scale(1.04);
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pcb-copy__trail,
  .pcb-copy__card { animation-duration: 0.001ms !important; }
}
</style>
