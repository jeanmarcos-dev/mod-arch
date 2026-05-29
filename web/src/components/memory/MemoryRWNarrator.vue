<script setup lang="ts">
import { ref, onUnmounted, watch, computed } from 'vue';
import { useNarrators, type MemWrite, type MemRead } from '../../lib/narratorBus';
import { useCpuStore } from '../../stores/cpu';
import { useNarratorAnimation } from '../narrators/useNarratorAnimation';
import type { MemoryOwner } from '../../lib/memoryOwners';

// MemoryRWNarrator paints one short-lived line per memory transaction over the
// hex grid in /memoria. Source is the MemoryOwnerChip of the process that
// issued the access (kernel or a PID/thread chip in the left sidebar). Target
// is the byte cell. Writes flow owner→cell (solid). Reads flow cell→owner
// (dashed). The label rides the line so the eye follows the byte. When the
// owner chip is not in the DOM (e.g. scrolled out of the sidebar) the line
// falls back to the row gutter on the left of the grid.

const cpu = useCpuStore();
const animator = useNarratorAnimation();

interface Flight {
  id: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  label: string;
  kind: 'write' | 'read';
  coreId: number;
}

const props = defineProps<{
  baseAddr: number;
  rowCount: number;
  owners: MemoryOwner[];
}>();

const flights = ref<Flight[]>([]);
const root = ref<HTMLElement | null>(null);

const ttlCss = computed(() => `${animator.tickMs.value}ms`);

const MAX_FLIGHTS = 12;

function findCell(addr: number): Element | null {
  return document.querySelector(
    `[data-mem-addr="0x${addr.toString(16).toLowerCase()}"]`,
  );
}

function rectCenter(el: Element | null): { x: number; y: number } | null {
  if (!el || !root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2 - local.left, y: r.top + r.height / 2 - local.top };
}

// Gutter point: a fixed offset to the left of the row holding the target cell.
// Used as fallback when the owner chip is offscreen so the line never starts
// from (0,0) which would look like a glitch.
function rowGutter(cell: Element): { x: number; y: number } | null {
  if (!root.value) return null;
  const row = cell.closest('.hexgrid__row');
  if (!row) return null;
  const local = root.value.getBoundingClientRect();
  const r = row.getBoundingClientRect();
  return { x: r.left - local.left - 8, y: r.top + r.height / 2 - local.top };
}

// Resolve the owner chip for a given pid. Threads share their leader's chip
// because buildOwners groups them under the TGID — so a write from a thread
// pid surfaces as a line from its leader's card. pid=0 means kernel.
function ownerIdForPid(pid: number): string | null {
  if (!pid) return 'kernel';
  for (const o of props.owners) {
    if (o.pid === pid) return o.id;
    if (o.threads.some((t) => t.pid === pid)) return o.id;
  }
  return null;
}

function findOwnerEl(pid: number): Element | null {
  const id = ownerIdForPid(pid);
  if (!id) return null;
  return document.querySelector(`[data-owner-id="${id}"]`);
}

// Source point on the owner chip: anchor on the right edge so the line
// reaches into the grid horizontally instead of starting from the chip's
// centroid (which would force the line to cross the chip body).
function ownerAnchor(el: Element): { x: number; y: number } | null {
  if (!root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.right - local.left, y: r.top + r.height / 2 - local.top };
}

const narrators = useNarrators();

function inWindow(addr: number): boolean {
  return addr >= props.baseAddr && addr < props.baseAddr + props.rowCount * 16;
}

function launch(kind: 'write' | 'read', addr: number, value: number, seq: number, coreId: number, pid: number) {
  if (!cpu.isCoreVisible(coreId)) return;
  if (!inWindow(addr)) return;
  if (!animator.shouldFire()) return;

  const cell = findCell(addr);
  if (!cell) return;
  const target = rectCenter(cell);
  if (!target) return;
  const ownerEl = findOwnerEl(pid);
  const source = (ownerEl && ownerAnchor(ownerEl)) ?? rowGutter(cell);
  if (!source) return;

  const from = kind === 'write' ? source : target;
  const to   = kind === 'write' ? target : source;

  if (flights.value.length >= MAX_FLIGHTS) flights.value.shift();

  flights.value.push({
    id: seq,
    fromX: from.x,
    fromY: from.y,
    toX: to.x,
    toY: to.y,
    label: (kind === 'write' ? 'W ' : 'R ') + '0x' + (value & 0xff).toString(16).toUpperCase().padStart(2, '0'),
    kind,
    coreId,
  });

  const dummy = document.createElement('div');
  const anim = animator.play(dummy, [{ opacity: 0 }, { opacity: 0 }], { fill: 'none' });
  if (anim) {
    anim.finished
      .then(() => { flights.value = flights.value.filter((f) => f.id !== seq); })
      .catch(() => { /* cancelled */ });
  }
}

narrators.onMemWrite((m: MemWrite) => launch('write', m.addr, m.newValue, m.seq, m.coreId, m.pid));
narrators.onMemRead((m: MemRead) => launch('read', m.addr, m.value, -m.seq, m.coreId, m.pid));

watch(
  () => cpu.paused,
  (p) => {
    if (root.value) root.value.style.setProperty('--anim-state', p ? 'paused' : 'running');
  },
  { immediate: true },
);

onUnmounted(() => { flights.value = []; });
</script>

<template>
  <div ref="root" class="mem-rw" aria-hidden="true">
    <svg class="mem-rw__svg" v-if="flights.length">
      <defs>
        <marker id="mem-rw-head" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="currentColor" />
        </marker>
      </defs>
      <template v-for="f in flights" :key="f.id">
        <line
          :class="['mem-rw__trail', `mem-rw__trail--${f.kind}`]"
          :x1="f.fromX"
          :y1="f.fromY"
          :x2="f.toX"
          :y2="f.toY"
          marker-end="url(#mem-rw-head)"
          :style="{ '--ttl': ttlCss, '--core-color': `var(--core-${f.coreId % 4})` }"
        />
      </template>
    </svg>

    <span
      v-for="f in flights"
      :key="f.id + 'l'"
      :class="['mem-rw__byte', `mem-rw__byte--${f.kind}`]"
      :style="{
        '--from-x': `${f.fromX}px`,
        '--from-y': `${f.fromY}px`,
        '--to-x': `${f.toX}px`,
        '--to-y': `${f.toY}px`,
        '--ttl': ttlCss,
        '--core-color': `var(--core-${f.coreId % 4})`,
      }"
    >
      {{ f.label }}
    </span>
  </div>
</template>

<style scoped>
.mem-rw {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
  z-index: var(--z-overlay);
}

.mem-rw__svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.mem-rw__trail {
  stroke-width: 1.5;
  fill: none;
  animation: mem-rw-trail var(--ttl) linear forwards;
  animation-play-state: var(--anim-state, running);
  stroke: var(--core-color, var(--amber));
  color: var(--core-color, var(--amber));
  filter: drop-shadow(0 0 4px color-mix(in oklab, var(--core-color, var(--amber)) 70%, transparent));
}

.mem-rw__trail--write {
  stroke-dasharray: none;
}

.mem-rw__trail--read {
  stroke-dasharray: 4 3;
}

@keyframes mem-rw-trail {
  0%   { stroke-dashoffset: 0; opacity: 0; }
  20%  { opacity: 1; }
  100% { stroke-dashoffset: -120; opacity: 0; }
}

.mem-rw__byte {
  position: absolute;
  left: 0;
  top: 0;
  transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%));
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  padding: 1px 5px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  background: color-mix(in oklab, var(--surface-2) 90%, transparent);
  white-space: nowrap;
  animation: mem-rw-fly var(--ttl) cubic-bezier(0.25, 0.5, 0.25, 1) forwards;
  animation-play-state: var(--anim-state, running);
  color: var(--core-color, var(--amber));
  box-shadow: 0 0 10px color-mix(in oklab, var(--core-color, var(--amber)) 45%, transparent);
  line-height: 1.1;
}

.mem-rw__byte--read {
  border-style: dashed;
  background: color-mix(in oklab, var(--surface-2) 70%, transparent);
}

@keyframes mem-rw-fly {
  0% {
    transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)) scale(0.85);
    opacity: 0;
  }
  15% {
    opacity: 1;
    transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)) scale(1);
  }
  85% {
    opacity: 1;
  }
  100% {
    transform: translate(calc(var(--to-x) - 50%), calc(var(--to-y) - 50%)) scale(1.05);
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .mem-rw__trail,
  .mem-rw__byte { animation-duration: 0.001ms !important; }
}
</style>
