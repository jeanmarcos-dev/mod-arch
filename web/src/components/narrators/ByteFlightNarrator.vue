<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useNarrators, type MemWrite, type MemRead } from '../../lib/narratorBus';
import { usePreferencesStore } from '../../stores/preferences';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();

// A flight is one animated byte traveling from the source anchor (CPU
// register grid) to the destination anchor (a row inside the memory hex
// view, identified by data-mem-addr). Flights are pruned by their own TTL
// so we never accumulate.
interface Flight {
  id: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  label: string;
  kind: 'write' | 'read';
  born: number;
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
    /** How long each byte stays on screen, in ms. */
    ttlMs?: number;
  }>(),
  {
    sourceSelector: '[data-narrator-anchor="cpu"]',
    ttlMs: 900,
  },
);

const flights = ref<Flight[]>([]);
const root = ref<HTMLElement | null>(null);
const prefs = usePreferencesStore();

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
  // Prefer the strip of the core that actually issued the read/write so the
  // byte appears to launch from there. Fall back to the single-panel anchor
  // for views that do not split CPU state per core (e.g. Memory.vue).
  return (
    document.querySelector(`[data-narrator-anchor="cpu-${coreId}"]`)
    ?? document.querySelector(props.sourceSelector)
  );
}

function launch(kind: 'write' | 'read', addr: number, value: number, seq: number, coreId: number) {
  // Drop bytes from cores the user is filtering out — keeps the animation
  // surface aligned with the rest of the UI.
  if (!cpu.isCoreVisible(coreId)) return;
  const from = rectCenter(sourceFor(coreId));
  const to = rectCenter(props.target ? props.target(addr) : defaultTarget(addr));
  if (!from || !to) return;

  // Slow the flight down in pedagogic and projector modes so the audience
  // can actually trace it.
  const ttl = Math.round(props.ttlMs / prefs.animationSpeed);

  // Cap the in-flight set so a burst (e.g. sys_write reading 8 bytes) doesn't
  // turn into visual noise. Drop the oldest when over the cap.
  if (flights.value.length >= MAX_FLIGHTS) {
    flights.value.shift();
  }

  flights.value.push({
    id: seq,
    fromX: from.x,
    fromY: from.y,
    toX: to.x,
    toY: to.y,
    label: '0x' + (value & 0xff).toString(16).toUpperCase().padStart(2, '0'),
    kind,
    born: performance.now(),
  });

  // Self-pruning: schedule a single removal per flight.
  window.setTimeout(() => {
    flights.value = flights.value.filter((f) => f.id !== seq);
  }, ttl + 50);
}

narrators.onMemWrite((m: MemWrite) => launch('write', m.addr, m.newValue, m.seq, m.coreId));
narrators.onMemRead((m: MemRead) => launch('read', m.addr, m.value, -m.seq, m.coreId));

// rAF tick to expire flights even when the timeout drifts (e.g. tab in
// background returns). Kept lightweight; bails out as soon as the array is
// empty.
let raf = 0;
function tick() {
  const now = performance.now();
  const cutoff = props.ttlMs / Math.max(prefs.animationSpeed, 0.2);
  flights.value = flights.value.filter((f) => now - f.born < cutoff + 60);
  if (flights.value.length > 0) raf = requestAnimationFrame(tick);
  else raf = 0;
}
const stopWatcher = setInterval(() => {
  if (flights.value.length > 0 && raf === 0) raf = requestAnimationFrame(tick);
}, 200);

onMounted(() => { /* root ref already wired via template */ });
onUnmounted(() => {
  if (raf !== 0) cancelAnimationFrame(raf);
  clearInterval(stopWatcher);
});
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
          :style="{ '--ttl': `${props.ttlMs / prefs.animationSpeed}ms` }"
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
        '--ttl': `${props.ttlMs / prefs.animationSpeed}ms`,
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
  stroke-dasharray: 4 3;
  animation: bf-trail var(--ttl) linear forwards;
}

.byte-flight__trail--write {
  stroke: var(--amber);
  color: var(--amber);
  filter: drop-shadow(0 0 4px rgba(255, 179, 71, 0.7));
}

.byte-flight__trail--read {
  stroke: var(--cyan);
  color: var(--cyan);
  filter: drop-shadow(0 0 4px rgba(106, 212, 255, 0.6));
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
}

.byte-flight__byte--write {
  color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}

.byte-flight__byte--read {
  color: var(--cyan);
  box-shadow: 0 0 12px rgba(106, 212, 255, 0.5);
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
