<script setup lang="ts">
import { computed, ref } from 'vue';
import { MEMORY_REGIONS, MEM_TOTAL, fmtAddr, coreColorVar } from '../../lib/memoryLayout';

export interface Pin {
  id: number;
  addr: number;
  kind: 'write' | 'read';
  coreId: number;
  t0: number;
}

const props = defineProps<{
  baseAddr: number;
  wndSize: number;
  pins: Pin[];
}>();

const emit = defineEmits<{
  (e: 'jump', addr: number): void;
}>();

const trackRef = ref<HTMLDivElement | null>(null);

const viewfinder = computed(() => {
  const leftPct = (props.baseAddr / MEM_TOTAL) * 100;
  const widthPct = Math.max(0.4, (props.wndSize / MEM_TOTAL) * 100);
  return { left: `${leftPct}%`, width: `${widthPct}%` };
});

const regionsLayout = computed(() =>
  MEMORY_REGIONS.map(r => ({
    ...r,
    leftPct: (r.start / MEM_TOTAL) * 100,
    widthPct: ((r.end - r.start + 1) / MEM_TOTAL) * 100,
  })),
);

const pinElements = computed(() =>
  props.pins.map(p => ({
    ...p,
    leftPct: (p.addr / MEM_TOTAL) * 100,
    color: coreColorVar(p.coreId),
  })),
);

const hoverAddr = ref<number | null>(null);
const hoverPct = ref(0);
const hoverPin = ref<Pin | null>(null);

// Pins are 1.5 px wide on screen — too narrow to click reliably. The track
// covers 4 MB so each pixel ≈ several KB. When the cursor lands within this
// many pixels of a pin, we snap to that pin's exact address instead of
// using the raw cursor offset, and surface a tooltip with its details.
const PIN_SNAP_PX = 6;

function trackPixelRange(): { rect: DOMRect; widthPx: number } | null {
  const el = trackRef.value;
  if (!el) return null;
  const rect = el.getBoundingClientRect();
  return { rect, widthPx: rect.width };
}

function pctFromEvent(e: MouseEvent): number {
  const t = trackPixelRange();
  if (!t) return 0;
  return Math.max(0, Math.min(1, (e.clientX - t.rect.left) / t.widthPx));
}

function addrFromEvent(e: MouseEvent): number {
  const pct = pctFromEvent(e);
  return Math.floor(pct * MEM_TOTAL) & ~0xf;
}

function nearestPin(e: MouseEvent): Pin | null {
  const t = trackPixelRange();
  if (!t || props.pins.length === 0) return null;
  const cursorPx = e.clientX - t.rect.left;
  let best: Pin | null = null;
  let bestDx = PIN_SNAP_PX + 1;
  for (const p of props.pins) {
    const px = (p.addr / MEM_TOTAL) * t.widthPx;
    const dx = Math.abs(px - cursorPx);
    if (dx < bestDx) {
      bestDx = dx;
      best = p;
    }
  }
  return best;
}

function onTrackMove(e: MouseEvent) {
  hoverAddr.value = addrFromEvent(e);
  hoverPct.value = pctFromEvent(e) * 100;
  hoverPin.value = nearestPin(e);
}

function onTrackLeave() {
  hoverAddr.value = null;
  hoverPin.value = null;
}

function onTrackClick(e: MouseEvent) {
  // Prefer snapping to a nearby pin's exact address so the user can land on
  // the byte they actually saw flash. Fall back to the raw cursor offset.
  const pin = nearestPin(e);
  emit('jump', pin ? pin.addr & ~0xf : addrFromEvent(e));
}

function jumpRegion(start: number) { emit('jump', start); }

function tipAge(t0: number): string {
  const ms = performance.now() - t0;
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${(ms / 60_000).toFixed(1)} min`;
}
</script>

<template>
  <div class="minimap">
    <div class="minimap__head">
      <span class="minimap__title">Mapa físico</span>
      <span class="minimap__range">
        <code>0x000000</code> ─ <code>0x3FFFFF</code>
      </span>
      <span v-if="hoverPin" class="minimap__hover-tip" :style="{ '--core-color': coreColorVar(hoverPin.coreId) }">
        <span class="minimap__hover-tag">[C{{ hoverPin.coreId }}]</span>
        <span class="minimap__hover-kind">{{ hoverPin.kind === 'write' ? 'W' : 'R' }}</span>
        <code>{{ fmtAddr(hoverPin.addr) }}</code>
        <span class="minimap__hover-age">hace {{ tipAge(hoverPin.t0) }}</span>
      </span>
      <span v-else-if="hoverAddr !== null" class="minimap__hover-addr">
        <code>{{ fmtAddr(hoverAddr) }}</code>
      </span>
      <span v-else-if="pins.length > 0" class="minimap__hint">
        {{ pins.length }} {{ pins.length === 1 ? 'evento' : 'eventos' }} · click para saltar
      </span>
    </div>

    <div
      ref="trackRef"
      class="minimap__track"
      :class="{ 'minimap__track--snap': hoverPin }"
      @mousemove="onTrackMove"
      @mouseleave="onTrackLeave"
      @click="onTrackClick"
    >
      <button
        v-for="r in regionsLayout"
        :key="r.name"
        type="button"
        class="minimap__region"
        :class="`minimap__region--${r.name}`"
        :style="{ left: `${r.leftPct}%`, width: `${r.widthPct}%` }"
        :title="`${r.label} · ${fmtAddr(r.start)}–${fmtAddr(r.end)} · user ${r.userPerm} · kernel ${r.kernPerm}`"
        @click.stop="jumpRegion(r.start)"
      >
        <span class="minimap__region-label">{{ r.label }}</span>
      </button>

      <span
        v-for="p in pinElements"
        :key="p.id"
        :class="['minimap__pin', `minimap__pin--${p.kind}`, { 'minimap__pin--hover': hoverPin && hoverPin.id === p.id }]"
        :style="{ left: `${p.leftPct}%`, '--pin-color': p.color }"
      />

      <span class="minimap__viewfinder" :style="viewfinder" />

      <span
        v-if="hoverAddr !== null"
        class="minimap__cursor"
        :style="{ left: `${hoverPct}%` }"
      />
    </div>
  </div>
</template>

<style scoped>
.minimap {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.minimap__head {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  font-size: 10px;
  letter-spacing: 1px;
  /* Pin the header to the height of the largest variant (the hover tip
     with its border + padding) so the layout never jumps when the user
     mouses over the track. */
  min-height: 22px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.minimap__title {
  color: var(--amber);
  letter-spacing: 2px;
}

.minimap__range code {
  color: var(--amber-dim);
  font-family: 'JetBrains Mono', monospace;
}

.minimap__hover-addr,
.minimap__hover-tip,
.minimap__hint {
  margin-left: auto;
  font-size: 10px;
  letter-spacing: 0.5px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  /* Equal box on every state so the head row never resizes when one
     variant swaps with another. */
  height: 20px;
  padding: 0 6px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  box-sizing: border-box;
}
.minimap__hover-addr code,
.minimap__hover-tip code {
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}
.minimap__hover-tip {
  color: var(--core-color, var(--amber));
  border-color: color-mix(in oklab, var(--core-color) 60%, transparent);
  background: color-mix(in oklab, var(--core-color) 10%, transparent);
}
.minimap__hover-tip code { color: var(--core-color); }
.minimap__hover-tag { font-weight: 600; letter-spacing: 1px; }
.minimap__hover-kind {
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
}
.minimap__hover-age { color: var(--text-muted); font-size: 9.5px; }
.minimap__hint {
  color: var(--text-muted);
  text-transform: lowercase;
  font-style: italic;
}

.minimap__track {
  position: relative;
  height: 26px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  cursor: crosshair;
  overflow: hidden;
}

.minimap__region {
  position: absolute;
  top: 0;
  bottom: 0;
  background: transparent;
  border: none;
  border-right: 1px dashed var(--border);
  color: var(--text-muted);
  padding: 0 var(--space-1);
  margin: 0;
  cursor: pointer;
  transition: background var(--transition-fast);
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 0;
  text-transform: lowercase;
  letter-spacing: 1px;
  font-size: 9.5px;
  font-family: inherit;
}

.minimap__region:last-child { border-right: none; }

.minimap__region:hover {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--amber);
}

.minimap__region--reserved   { background: rgba(125, 120, 104, 0.10); }
.minimap__region--kernel     { background: rgba(255, 58, 58, 0.06); }
.minimap__region--user_code  { background: rgba(127, 255, 170, 0.06); }
.minimap__region--user_stack { background: rgba(106, 212, 255, 0.05); }

.minimap__region-label {
  pointer-events: none;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.minimap__viewfinder {
  position: absolute;
  top: -2px;
  bottom: -2px;
  border: 1.5px solid var(--amber);
  background: rgba(255, 181, 71, 0.18);
  box-shadow: 0 0 8px rgba(255, 181, 71, 0.45), inset 0 0 4px rgba(255, 181, 71, 0.25);
  transition: left var(--transition-base), width var(--transition-base);
  pointer-events: none;
  z-index: 3;
  min-width: 3px;
}

.minimap__pin {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1.5px;
  background: var(--pin-color);
  pointer-events: none;
  z-index: 1;
  animation: pin-fade 8s linear forwards;
}

.minimap__pin--write {
  box-shadow: 0 0 4px var(--pin-color);
}

.minimap__pin--read {
  opacity: 0.55;
  width: 1px;
}

.minimap__pin--hover {
  width: 3px;
  opacity: 1;
  z-index: 4;
  box-shadow: 0 0 8px var(--pin-color), 0 0 14px var(--pin-color);
  animation: none;
}

.minimap__track--snap {
  cursor: pointer;
}

@keyframes pin-fade {
  0%   { opacity: 1; }
  100% { opacity: 0; }
}

.minimap__cursor {
  position: absolute;
  top: -3px;
  bottom: -3px;
  width: 1px;
  background: var(--amber);
  opacity: 0.55;
  pointer-events: none;
  z-index: 2;
}

@media (prefers-reduced-motion: reduce) {
  .minimap__pin { animation: none; opacity: 1; }
  .minimap__viewfinder { transition: none; }
}
</style>
