<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import Card from '../ui/Card.vue';
import { fmtAddr, fmtByte, regionFor, coreColorVar } from '../../lib/memoryLayout';

export interface HeatEntry {
  /** Monotonic id used to force animation restart on each new event. */
  seq: number;
  coreId: number;
  kind: 'write' | 'read';
  /** performance.now() of the originating event, used for "x s ago" labels. */
  t0: number;
  pid: number;
  /** Last byte value as known when the event landed (useful for popover). */
  value: number;
}

const props = defineProps<{
  baseAddr: number;
  rowCount: number;
  bytes: number[];
  heat: Map<number, HeatEntry>;
  lastReadByAddr: Map<number, HeatEntry>;
  lastWriteByAddr: Map<number, HeatEntry>;
  errorMessage?: string;
}>();

const emit = defineEmits<{
  (e: 'scroll-by', delta: number): void;
}>();

const rows = computed(() => {
  const out: { addr: number; cells: number[] }[] = [];
  for (let r = 0; r < props.rowCount; r++) {
    const rowAddr = props.baseAddr + r * 16;
    out.push({ addr: rowAddr, cells: props.bytes.slice(r * 16, r * 16 + 16) });
  }
  return out;
});

// ─── Popover state ──────────────────────────────────────────────────────────
// Click on a cell opens a small floating panel anchored to it. Keyed by
// absolute address; only one open at a time.

const popoverAddr = ref<number | null>(null);
const popoverAnchorRect = ref<DOMRect | null>(null);
const containerRef = ref<HTMLDivElement | null>(null);
const tickRef = ref(0);

let pressDownAddr: number | null = null;
let pressDownXY: { x: number; y: number } | null = null;

function onCellMouseDown(addr: number, e: MouseEvent) {
  pressDownAddr = addr;
  pressDownXY = { x: e.clientX, y: e.clientY };
}

function onCellMouseUp(addr: number, e: MouseEvent) {
  if (pressDownAddr === addr && pressDownXY) {
    const dx = Math.abs(e.clientX - pressDownXY.x);
    const dy = Math.abs(e.clientY - pressDownXY.y);
    if (dx < 4 && dy < 4) {
      openPopover(addr, e.currentTarget as HTMLElement);
    }
  }
  pressDownAddr = null;
  pressDownXY = null;
}

function openPopover(addr: number, el: HTMLElement) {
  popoverAddr.value = addr;
  popoverAnchorRect.value = el.getBoundingClientRect();
}

function closePopover() {
  popoverAddr.value = null;
  popoverAnchorRect.value = null;
}

function onDocumentClick(e: MouseEvent) {
  if (popoverAddr.value === null) return;
  const popoverEl = document.querySelector('.hexgrid__popover');
  if (popoverEl && e.target instanceof Node && popoverEl.contains(e.target)) return;
  const cell = (e.target instanceof Element) ? e.target.closest('.hexgrid__cell') : null;
  if (cell) return; // mouseup handler will manage re-opening
  closePopover();
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { closePopover(); return; }
  if (e.key === 'PageDown') {
    e.preventDefault();
    emit('scroll-by', e.shiftKey ? props.rowCount * 16 : 16);
  } else if (e.key === 'PageUp') {
    e.preventDefault();
    emit('scroll-by', e.shiftKey ? -props.rowCount * 16 : -16);
  } else if (e.key === 'ArrowDown') {
    e.preventDefault();
    emit('scroll-by', 16);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    emit('scroll-by', -16);
  }
}

// Throttle wheel scrolls so a fast spin does not warp 8 KB at once.
let wheelLockedUntil = 0;
function onWheel(e: WheelEvent) {
  e.preventDefault();
  const now = performance.now();
  if (now < wheelLockedUntil) return;
  wheelLockedUntil = now + 60;
  emit('scroll-by', e.deltaY > 0 ? 16 : -16);
}

// rAF tick to refresh "x s ago" labels in the popover while it is open.
let raf = 0;
function tick() {
  if (popoverAddr.value !== null) {
    tickRef.value = (tickRef.value + 1) % 1000;
    raf = requestAnimationFrame(tick);
  } else {
    raf = 0;
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentClick);
});
onUnmounted(() => {
  document.removeEventListener('mousedown', onDocumentClick);
  if (raf) cancelAnimationFrame(raf);
});

function popoverStyleVars() {
  const rect = popoverAnchorRect.value;
  const container = containerRef.value?.getBoundingClientRect();
  if (!rect || !container) return {};
  const left = Math.min(container.right - 240, Math.max(container.left + 4, rect.left));
  const top = rect.bottom + 6;
  return { left: `${left}px`, top: `${top}px` };
}

function cellHeatStyle(addr: number): Record<string, string> {
  const h = props.heat.get(addr);
  if (!h) return {};
  return { '--core-color': coreColorVar(h.coreId) };
}

function ageLabel(t0: number | undefined): string {
  if (t0 === undefined) return '—';
  const ms = performance.now() - t0;
  if (ms < 1000) return `hace ${Math.round(ms)} ms`;
  if (ms < 60_000) return `hace ${(ms / 1000).toFixed(1)} s`;
  return `hace ${(ms / 60_000).toFixed(1)} min`;
}

const popoverInfo = computed(() => {
  const addr = popoverAddr.value;
  if (addr === null) return null;
  // Touch tickRef so the "x s ago" labels recompute every frame.
  void tickRef.value;
  const idx = addr - props.baseAddr;
  const value = (idx >= 0 && idx < props.bytes.length) ? props.bytes[idx] : undefined;
  const region = regionFor(addr);
  const lastW = props.lastWriteByAddr.get(addr);
  const lastR = props.lastReadByAddr.get(addr);
  const printable = value !== undefined && value >= 0x20 && value <= 0x7e
    ? `'${String.fromCharCode(value)}'`
    : '';
  return {
    addr,
    value,
    valueDec: value ?? 0,
    printable,
    region,
    lastW,
    lastR,
  };
});

function ensureTicker() {
  if (raf === 0 && popoverAddr.value !== null) raf = requestAnimationFrame(tick);
}
</script>

<template>
  <Card class="hexgrid-card" :elevation="1">
    <div v-if="errorMessage" class="hexgrid__err">{{ errorMessage }}</div>
    <div
      ref="containerRef"
      class="hexgrid"
      tabindex="0"
      role="grid"
      aria-label="Vista hexadecimal de la memoria"
      @keydown="onKeydown"
      @wheel.passive.prevent="onWheel"
    >
      <div class="hexgrid__head">
        <span class="hexgrid__addr-h">addr</span>
        <span v-for="c in 16" :key="c" class="hexgrid__col-h">{{ (c - 1).toString(16).toUpperCase() }}</span>
      </div>
      <div v-for="row in rows" :key="row.addr" class="hexgrid__row">
        <span class="hexgrid__addr">{{ fmtAddr(row.addr).slice(2) }}</span>
        <span
          v-for="(byte, idx) in row.cells"
          :key="idx"
          class="hexgrid__cell"
          :style="cellHeatStyle(row.addr + idx)"
          :data-mem-addr="`0x${(row.addr + idx).toString(16).toLowerCase()}`"
          @mousedown="onCellMouseDown(row.addr + idx, $event)"
          @mouseup="onCellMouseUp(row.addr + idx, $event)"
        >
          <span
            v-if="heat.get(row.addr + idx)"
            :key="heat.get(row.addr + idx)!.seq"
            :class="['hexgrid__heat', `hexgrid__heat--${heat.get(row.addr + idx)!.kind}`]"
          />
          <transition name="digit-fade">
            <span :key="byte" class="hexgrid__digit">{{ fmtByte(byte) }}</span>
          </transition>
        </span>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="popoverInfo"
        class="hexgrid__popover"
        :style="popoverStyleVars()"
        @mouseenter="ensureTicker"
        role="dialog"
        aria-label="Detalle del byte"
      >
        <button type="button" class="hexgrid__popover-close" @click="closePopover" aria-label="Cerrar">×</button>
        <div class="hexgrid__popover-head">
          <code>{{ fmtAddr(popoverInfo.addr) }}</code>
          <span class="hexgrid__popover-region">{{ popoverInfo.region?.label ?? 'fuera de mapa' }}</span>
        </div>
        <div class="hexgrid__popover-value">
          <span>valor:</span>
          <code v-if="popoverInfo.value !== undefined">0x{{ fmtByte(popoverInfo.value) }}</code>
          <span v-else>—</span>
          <span v-if="popoverInfo.value !== undefined" class="hexgrid__popover-dec">
            ({{ popoverInfo.valueDec }}<template v-if="popoverInfo.printable"> · {{ popoverInfo.printable }}</template>)
          </span>
        </div>
        <dl class="hexgrid__popover-stats">
          <div>
            <dt>último W</dt>
            <dd>
              <template v-if="popoverInfo.lastW">
                C{{ popoverInfo.lastW.coreId }} / P{{ popoverInfo.lastW.pid || '—' }} · {{ ageLabel(popoverInfo.lastW.t0) }}
              </template>
              <template v-else>—</template>
            </dd>
          </div>
          <div>
            <dt>último R</dt>
            <dd>
              <template v-if="popoverInfo.lastR">
                C{{ popoverInfo.lastR.coreId }} / P{{ popoverInfo.lastR.pid || '—' }} · {{ ageLabel(popoverInfo.lastR.t0) }}
              </template>
              <template v-else>—</template>
            </dd>
          </div>
          <div v-if="popoverInfo.region">
            <dt>permisos</dt>
            <dd>
              user {{ popoverInfo.region.userPerm }} · kernel {{ popoverInfo.region.kernPerm }}
            </dd>
          </div>
        </dl>
      </div>
    </Teleport>
  </Card>
</template>

<style scoped>
.hexgrid-card {
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.hexgrid-card :deep(.card__body) {
  padding: 0;
  overflow: auto;
  flex: 1;
  min-height: 0;
}

.hexgrid {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  padding: var(--space-3);
  outline: none;
}

.hexgrid:focus-visible {
  box-shadow: inset 0 0 0 1px var(--amber);
  border-radius: var(--radius-sm);
}

.hexgrid__head {
  display: grid;
  grid-template-columns: 80px repeat(16, 1fr);
  gap: 6px;
  padding-bottom: var(--space-2);
  border-bottom: 1px dashed var(--border);
  margin-bottom: var(--space-2);
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.hexgrid__head .hexgrid__col-h {
  text-align: center;
}

.hexgrid__row {
  display: grid;
  grid-template-columns: 80px repeat(16, 1fr);
  gap: 6px;
  padding: 3px 0;
}

.hexgrid__addr {
  color: var(--text-muted);
  font-size: 11px;
  letter-spacing: 0.5px;
}

.hexgrid__cell {
  position: relative;
  text-align: center;
  color: var(--text);
  padding: 2px 0;
  border-radius: 2px;
  cursor: pointer;
  overflow: hidden;
}

.hexgrid__cell:hover {
  outline: 1px solid var(--border-strong);
}

.hexgrid__digit {
  position: relative;
  z-index: 2;
  display: inline-block;
}

.hexgrid__heat {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.hexgrid__heat--write {
  animation: heat-write 6s linear forwards;
}

.hexgrid__heat--read {
  animation: heat-read 3s linear forwards;
}

@keyframes heat-write {
  0%   { background-color: color-mix(in oklab, var(--core-color) 65%, transparent);
         box-shadow: 0 0 8px var(--core-color), inset 0 0 4px var(--core-color);
         transform: scale(1.15); }
  6%   { background-color: color-mix(in oklab, var(--core-color) 55%, transparent);
         transform: scale(1.04);
         box-shadow: 0 0 4px var(--core-color); }
  15%  { background-color: color-mix(in oklab, var(--core-color) 40%, transparent);
         transform: scale(1);
         box-shadow: none; }
  100% { background-color: transparent; box-shadow: none; }
}

@keyframes heat-read {
  0%   { background-color: color-mix(in oklab, var(--core-color) 30%, transparent);
         box-shadow: inset 0 0 0 1.5px var(--core-color); }
  20%  { background-color: color-mix(in oklab, var(--core-color) 22%, transparent);
         box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--core-color) 60%, transparent); }
  100% { background-color: transparent; box-shadow: none; }
}

.digit-fade-enter-active,
.digit-fade-leave-active {
  position: absolute;
  inset: 2px 0;
  transition: opacity 220ms ease-out, transform 220ms ease-out;
}

.digit-fade-enter-from { opacity: 0; transform: translateY(-3px); }
.digit-fade-leave-to   { opacity: 0; transform: translateY(3px);  }

.hexgrid__err {
  padding: var(--space-3);
  color: var(--red-soft);
  font-size: 12px;
}

/* ─── Popover ──────────────────────────────────────────────────────────── */

.hexgrid__popover {
  position: fixed;
  z-index: var(--z-floating-toolbar);
  width: 240px;
  background: var(--surface-3);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-elevation-floating);
  padding: var(--space-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text);
}

.hexgrid__popover-close {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 22px;
  height: 22px;
  min-height: unset;
  background: transparent;
  border: none;
  color: var(--text-dim);
  font-size: 18px;
  line-height: 1;
  padding: 0;
  cursor: pointer;
}
.hexgrid__popover-close:hover { color: var(--amber); background: transparent; box-shadow: none; }

.hexgrid__popover-head {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--border);
  margin-bottom: 6px;
}
.hexgrid__popover-head code {
  color: var(--amber);
  font-size: 12px;
}
.hexgrid__popover-region {
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: lowercase;
}

.hexgrid__popover-value {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 6px;
}
.hexgrid__popover-value code {
  color: var(--amber);
}
.hexgrid__popover-dec {
  color: var(--text-muted);
  font-size: 10px;
}

.hexgrid__popover-stats {
  display: grid;
  gap: 4px;
}

.hexgrid__popover-stats > div {
  display: grid;
  grid-template-columns: 60px 1fr;
  gap: var(--space-2);
  align-items: baseline;
}

.hexgrid__popover-stats dt {
  font-size: 9.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.hexgrid__popover-stats dd {
  color: var(--text);
  font-size: 10.5px;
}

@media (prefers-reduced-motion: reduce) {
  .hexgrid__heat--write,
  .hexgrid__heat--read { animation: none; opacity: 0; }
  .digit-fade-enter-active,
  .digit-fade-leave-active { transition: none; }
}
</style>
