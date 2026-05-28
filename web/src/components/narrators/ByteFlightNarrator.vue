<script setup lang="ts">
import { ref, onUnmounted, watch, computed } from 'vue';
import { useNarrators, type MemWrite, type MemRead } from '../../lib/narratorBus';
import { useCpuStore } from '../../stores/cpu';
import { useNarratorAnimation } from './useNarratorAnimation';

const cpu = useCpuStore();
const animator = useNarratorAnimation();

// A flight is one animated byte traveling from the source anchor (CPU
// register grid) to the destination anchor (a row inside the memory hex
// view, identified by data-mem-addr). Each flight lasts exactly one
// simulator tick; below narratorMinTickMs they don't fire at all.
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

const props = withDefaults(
  defineProps<{
    /** CSS selector for the source anchor (typically the CPU registers panel). */
    sourceSelector?: string;
    /**
     * Function returning the destination element for a given address. The
     * Memory view registers data-mem-addr="0xNNNN" attributes on each row
     * so the byte lands on the exact byte being written.
     */
    target?: (addr: number) => Element | null;
  }>(),
  {
    sourceSelector: '[data-narrator-anchor="cpu"]',
  },
);

const flights = ref<Flight[]>([]);
const root = ref<HTMLElement | null>(null);

const ttlCss = computed(() => `${animator.tickMs.value}ms`);

function rectCenter(el: Element | null): { x: number; y: number } | null {
  if (!el || !root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2 - local.left, y: r.top + r.height / 2 - local.top };
}

function defaultTarget(addr: number): Element | null {
  return document.querySelector(
    `[data-mem-addr="0x${addr.toString(16).toLowerCase()}"]`,
  ) ?? null;
}

const narrators = useNarrators();

const MAX_FLIGHTS = 12;

function sourceFor(coreId: number): Element | null {
  return (
    document.querySelector(`[data-narrator-anchor="cpu-${coreId}"]`)
    ?? document.querySelector(props.sourceSelector)
  );
}

function launch(kind: 'write' | 'read', addr: number, value: number, seq: number, coreId: number) {
  if (!cpu.isCoreVisible(coreId)) return;
  if (!animator.shouldFire()) return;

  const chipCenter = rectCenter(sourceFor(coreId));
  const cellCenter = rectCenter(props.target ? props.target(addr) : defaultTarget(addr));
  if (!chipCenter || !cellCenter) return;

  // Direction is semantic: write moves data CPU → memory, read moves data
  // memory → CPU. Swapping endpoints for reads makes the demo match the
  // mental model students learn (arrows follow the data).
  const from = kind === 'write' ? chipCenter : cellCenter;
  const to   = kind === 'write' ? cellCenter : chipCenter;

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

  // One-tick pause/resume-aware timer driving removal of this flight.
  const dummy = document.createElement('div');
  const anim = animator.play(dummy, [{ opacity: 0 }, { opacity: 0 }], { fill: 'none' });
  if (anim) {
    anim.finished
      .then(() => { flights.value = flights.value.filter((f) => f.id !== seq); })
      .catch(() => { /* cancelled */ });
  }
}

narrators.onMemWrite((m: MemWrite) => launch('write', m.addr, m.newValue, m.seq, m.coreId));
narrators.onMemRead((m: MemRead) => launch('read', m.addr, m.value, -m.seq, m.coreId));

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
  <div ref="root" class="byte-flight" aria-hidden="true">
    <svg class="byte-flight__svg" v-if="flights.length">
      <defs>
        <marker id="bf-head" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="currentColor" />
        </marker>
      </defs>
      <template v-for="f in flights" :key="f.id">
        <line
          :class="['byte-flight__trail', `byte-flight__trail--${f.kind}`]"
          :x1="f.fromX"
          :y1="f.fromY"
          :x2="f.toX"
          :y2="f.toY"
          marker-end="url(#bf-head)"
          :style="{ '--ttl': ttlCss, '--core-color': `var(--core-${f.coreId % 4})` }"
        />
      </template>
    </svg>

    <span
      v-for="f in flights"
      :key="f.id + 'l'"
      :class="['byte-flight__byte', `byte-flight__byte--${f.kind}`]"
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
.byte-flight {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
  z-index: var(--z-overlay);
}

.byte-flight__svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.byte-flight__trail {
  stroke-width: 1.5;
  fill: none;
  animation: bf-trail var(--ttl) linear forwards;
  animation-play-state: var(--anim-state, running);
  stroke: var(--core-color, var(--amber));
  color: var(--core-color, var(--amber));
  filter: drop-shadow(0 0 4px color-mix(in oklab, var(--core-color, var(--amber)) 70%, transparent));
}

/* Writes look solid (data is being placed). Reads look dashed (the byte is
   being grabbed and pulled back to the chip). Same color: the core that
   issued the access. */
.byte-flight__trail--write {
  stroke-dasharray: none;
}

.byte-flight__trail--read {
  stroke-dasharray: 4 3;
}

@keyframes bf-trail {
  0% { stroke-dashoffset: 0; opacity: 0; }
  20% { opacity: 1; }
  100% { stroke-dashoffset: -120; opacity: 0; }
}

.byte-flight__byte {
  position: absolute;
  left: 0;
  top: 0;
  transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%));
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 2px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  white-space: nowrap;
  animation: bf-fly var(--ttl) cubic-bezier(0.25, 0.5, 0.25, 1) forwards;
  animation-play-state: var(--anim-state, running);
  color: var(--core-color, var(--amber));
  box-shadow: 0 0 12px color-mix(in oklab, var(--core-color, var(--amber)) 50%, transparent);
}

/* Reads carry a dashed border to mirror the dashed line that connects them
   to the CPU, so the W/R distinction is visible at a glance without
   needing the W/R prefix in the label (kept anyway as redundancy). */
.byte-flight__byte--read {
  border-style: dashed;
  background: color-mix(in oklab, var(--surface-2) 80%, transparent);
}

@keyframes bf-fly {
  0% {
    transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)) scale(0.8);
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
  .byte-flight__trail,
  .byte-flight__byte { animation-duration: 0.001ms !important; }
}
</style>
