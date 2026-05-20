<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import { useNarrators, type MemWrite, type MemRead } from '../../lib/narratorBus';
import ByteFlightNarrator from '../../components/narrators/ByteFlightNarrator.vue';
import Card from '../../components/ui/Card.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();

// Memory window we paint. Hardcoded to the user-program region for the
// canonical demo (add, sieve, etc.); the picker further down lets the
// audience jump to other regions on the fly.
const baseAddr = ref(0x200000);
const rowCount = 24; // 24 rows × 16 bytes = 384 bytes visible
const bytes = ref<number[]>([]);
const fetchError = ref('');

const wndSize = rowCount * 16;
const wndEnd = computed(() => baseAddr.value + wndSize);

async function loadWindow() {
  try {
    const r = await fetch(`/api/memory?addr=0x${baseAddr.value.toString(16)}&len=${wndSize}`);
    if (!r.ok) throw new Error('HTTP ' + r.status);
    const data = (await r.json()) as { bytes: number[] };
    bytes.value = data.bytes;
    fetchError.value = '';
  } catch (e) {
    fetchError.value = (e as Error).message;
  }
}

// Recent traffic feed: small ring of the last writes/reads inside the window
// so the audience can scan what just happened. Each entry expires on its own
// after a few seconds.
interface FeedEntry {
  id: number;
  addr: number;
  value: number;
  kind: 'write' | 'read';
  ts: number;
}
const feed = ref<FeedEntry[]>([]);
let feedSeq = 0;
function pushFeed(kind: 'write' | 'read', addr: number, value: number) {
  feed.value.unshift({ id: ++feedSeq, addr, value, kind, ts: performance.now() });
  if (feed.value.length > 8) feed.value.length = 8;
}

const narrators = useNarrators();
narrators.onMemWrite((m: MemWrite) => {
  if (m.addr >= baseAddr.value && m.addr < wndEnd.value) {
    pushFeed('write', m.addr, m.newValue);
    flashByte(m.addr);
    // Quick local mutation so the hex view shows the new value even before
    // the next periodic reload finishes.
    const idx = m.addr - baseAddr.value;
    if (idx >= 0 && idx < bytes.value.length) bytes.value[idx] = m.newValue & 0xff;
  }
});
narrators.onMemRead((m: MemRead) => {
  if (m.addr >= baseAddr.value && m.addr < wndEnd.value) {
    pushFeed('read', m.addr, m.value);
  }
});

// Per-byte flash overlay applied when a write lands inside the window. The
// Set keys are the absolute address; the Vue v-bind in the row consumes it.
const flashing = ref<Set<number>>(new Set());
function flashByte(addr: number) {
  flashing.value.add(addr);
  flashing.value = new Set(flashing.value); // reactive bump
  window.setTimeout(() => {
    flashing.value.delete(addr);
    flashing.value = new Set(flashing.value);
  }, 600);
}

// Periodic background reload so addresses that were not touched by mem_*
// (e.g. DMA, kernel internal updates) still refresh. Slower than the
// transitions to keep the narrator readable.
let timer = 0;
onMounted(() => {
  loadWindow();
  timer = window.setInterval(loadWindow, 2000);
});
onUnmounted(() => { if (timer) window.clearInterval(timer); });

watch(baseAddr, loadWindow);

// Preset jumps. Each computes its target on the fly so "Stack" follows
// ESP — keeping the ESP visible in the window matters more than landing
// on a fixed address.
type Preset = { label: string; resolve: () => number };
const presets: Preset[] = [
  { label: 'Programa', resolve: () => 0x200000 },
  { label: 'Stack', resolve: () => Math.max(0, ((cpu.regs.ESP || 0x33fff0) & ~0xff) - 0x80) },
  { label: 'IDT', resolve: () => 0x100000 },
];
function jumpTo(p: Preset) { baseAddr.value = p.resolve(); }

// Auto-follow ESP when a stack-touching demo is loaded. Detection runs on
// the loaded event (procs reset) — when the new program is `pushpop` or
// any stack-heavy demo, jump the window to the stack so the audience does
// not have to click "Stack" manually.
cpu.subscribe((ev) => {
  if (ev.type === 'loaded' && (ev.name === 'pushpop' || ev.name === 'callret')) {
    // Defer one tick so cpu.regs.ESP reflects the post-load value.
    setTimeout(() => { jumpTo(presets[1]); }, 200);
  }
});

const rows = computed(() => {
  const out: { addr: number; cells: number[] }[] = [];
  for (let r = 0; r < rowCount; r++) {
    const rowAddr = baseAddr.value + r * 16;
    out.push({ addr: rowAddr, cells: bytes.value.slice(r * 16, r * 16 + 16) });
  }
  return out;
});

function fmtAddr(a: number): string {
  return '0x' + (a >>> 0).toString(16).toUpperCase().padStart(8, '0');
}
function fmtByte(b: number | undefined): string {
  if (b === undefined) return '··';
  return (b & 0xff).toString(16).toUpperCase().padStart(2, '0');
}
function ageOf(ts: number): number {
  return performance.now() - ts;
}
</script>

<template>
  <div class="memory-page">
    <!-- The narrator overlay covers the whole page so flights can cross
         between the CPU chip and any byte cell. -->
    <ByteFlightNarrator class="memory-page__overlay" />

    <header class="memory-page__head">
      <div class="memory-page__title">
        <span class="memory-page__title-main">Memoria</span>
        <span class="memory-page__title-sub">
          Ventana <code>{{ fmtAddr(baseAddr) }}</code> · <code>{{ wndSize }} B</code>
        </span>
      </div>
      <div class="memory-page__presets">
        <button
          v-for="p in presets"
          :key="p.label"
          type="button"
          @click="jumpTo(p)"
        >{{ p.label }}</button>
      </div>

      <div
        v-if="!prefs.narratorsEnabled"
        class="memory-page__hint"
        title="Active 'Narradores visuales' en Preferencias para ver los bytes viajando"
      >
        ℹ Narradores apagados — actívelos en Preferencias para ver los bytes viajando
      </div>
    </header>

    <div class="memory-page__body">
      <!-- CPU side: a single chip that acts as the source anchor for byte
           flights. Shows the current EIP + EAX as context. -->
      <aside class="memory-page__cpu" data-narrator-anchor="cpu">
        <div class="cpu-chip">
          <div class="cpu-chip__label">CPU</div>
          <div class="cpu-chip__pid">PID {{ cpu.currentPID || '—' }}</div>
          <dl class="cpu-chip__regs">
            <div><dt>EIP</dt><dd>{{ fmtAddr(cpu.regs.EIP) }}</dd></div>
            <div><dt>EAX</dt><dd>{{ fmtAddr(cpu.regs.EAX) }}</dd></div>
            <div><dt>EBX</dt><dd>{{ fmtAddr(cpu.regs.EBX) }}</dd></div>
            <div><dt>ESP</dt><dd>{{ fmtAddr(cpu.regs.ESP) }}</dd></div>
          </dl>
        </div>
      </aside>

      <!-- Memory grid -->
      <Card class="memory-page__grid" :elevation="1">
        <div v-if="fetchError" class="memory-page__err">{{ fetchError }}</div>
        <div class="hexgrid">
          <div class="hexgrid__head">
            <span class="hexgrid__addr-h">addr</span>
            <span v-for="c in 16" :key="c" class="hexgrid__col-h">{{ (c - 1).toString(16).toUpperCase() }}</span>
          </div>
          <div v-for="row in rows" :key="row.addr" class="hexgrid__row">
            <span class="hexgrid__addr">{{ fmtAddr(row.addr).slice(2) }}</span>
            <span
              v-for="(byte, idx) in row.cells"
              :key="idx"
              :class="['hexgrid__cell', flashing.has(row.addr + idx) && 'hexgrid__cell--flash']"
              :data-mem-addr="`0x${(row.addr + idx).toString(16).toLowerCase()}`"
            >{{ fmtByte(byte) }}</span>
          </div>
        </div>
      </Card>

      <!-- Right rail: activity feed -->
      <aside class="memory-page__feed">
        <Card title="Actividad" :elevation="1" class="memory-page__feed-card">
          <p v-if="!feed.length" class="memory-page__feed-empty">
            Cargue un demo y ejecute para ver el tráfico.
          </p>
          <ol v-else class="memory-page__feed-list">
            <li
              v-for="e in feed"
              :key="e.id"
              :class="['feed-row', `feed-row--${e.kind}`]"
              :style="{ opacity: Math.max(0.4, 1 - ageOf(e.ts) / 5000) }"
            >
              <span class="feed-row__kind">{{ e.kind === 'write' ? '◣ W' : '◤ R' }}</span>
              <code class="feed-row__addr">{{ fmtAddr(e.addr) }}</code>
              <code class="feed-row__val">{{ fmtByte(e.value) }}</code>
            </li>
          </ol>
        </Card>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.memory-page {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.memory-page__overlay {
  /* The narrator sits above everything but below modals/topbar. */
}

.memory-page__head {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-5);
  border-bottom: 1px dashed var(--border);
  flex-shrink: 0;
}

.memory-page__title {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.memory-page__title-main {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}

.memory-page__title-sub {
  font-size: 11px;
  color: var(--text-dim);
}

.memory-page__title-sub code {
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
}

.memory-page__presets {
  display: flex;
  gap: var(--space-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.memory-page__presets button {
  background: transparent;
  border: none;
  border-right: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 6px 10px;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.memory-page__presets button:last-child { border-right: none; }
.memory-page__presets button:hover { background: rgba(255, 179, 71, 0.08); color: var(--amber); }
.memory-page__presets button.on {
  background: rgba(255, 179, 71, 0.18);
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}

.memory-page__hint {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-dim);
  letter-spacing: 0.5px;
}

.memory-page__body {
  display: grid;
  grid-template-columns: 200px 1fr 240px;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  overflow: hidden;
  flex: 1;
  min-height: 0;
}

.memory-page__cpu {
  display: flex;
  align-items: center;
  justify-content: center;
}

.cpu-chip {
  width: 100%;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-4);
  background: var(--surface-2);
  box-shadow: var(--shadow-glow-amber);
  position: relative;
}

.cpu-chip::before,
.cpu-chip::after {
  content: '';
  position: absolute;
  width: 6px;
  height: 16px;
  background: var(--amber);
  border-radius: 2px;
  opacity: 0.6;
}
.cpu-chip::before { left: -3px; top: 14px; }
.cpu-chip::after  { right: -3px; top: 14px; }

.cpu-chip__label {
  font-family: 'Major Mono Display', monospace;
  font-size: 13px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  text-align: center;
  margin-bottom: var(--space-2);
}

.cpu-chip__pid {
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-dim);
  text-align: center;
  text-transform: uppercase;
  margin-bottom: var(--space-3);
}

.cpu-chip__regs {
  display: grid;
  gap: var(--space-1);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.cpu-chip__regs > div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
}

.cpu-chip__regs dt {
  color: var(--text-dim);
  letter-spacing: 1px;
}

.cpu-chip__regs dd {
  color: var(--text);
}

.memory-page__grid {
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.memory-page__grid :deep(.card__body) {
  padding: 0;
  overflow: auto;
  flex: 1;
  min-height: 0;
}

.hexgrid {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  padding: var(--space-3);
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
  text-align: center;
  color: var(--text);
  padding: 2px 0;
  border-radius: 2px;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.hexgrid__cell--flash {
  background: rgba(255, 179, 71, 0.30);
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  animation: hex-pulse 600ms ease-out;
}

@keyframes hex-pulse {
  0% { background: rgba(255, 179, 71, 0.6); transform: scale(1.18); }
  100% { background: rgba(255, 179, 71, 0); transform: scale(1); }
}

.memory-page__err {
  padding: var(--space-3);
  color: var(--red-soft);
  font-size: 12px;
}

.memory-page__feed-card { display: flex; flex-direction: column; min-height: 0; }

.memory-page__feed-empty {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.memory-page__feed-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.feed-row {
  display: grid;
  grid-template-columns: 40px 1fr 40px;
  gap: var(--space-2);
  align-items: center;
  padding: 4px var(--space-2);
  border-left: 2px solid;
  background: var(--surface-2);
  transition: opacity var(--transition-base);
}

.feed-row--write {
  border-left-color: var(--amber);
  color: var(--amber);
}

.feed-row--read {
  border-left-color: var(--cyan);
  color: var(--cyan);
}

.feed-row__kind {
  font-weight: bold;
  letter-spacing: 1px;
}

.feed-row__addr,
.feed-row__val {
  color: inherit;
}

@media (max-width: 1100px) {
  .memory-page__body {
    grid-template-columns: 1fr;
  }
  .memory-page__cpu { order: -1; }
}
</style>
