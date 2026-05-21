<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import { useNarrators, type MemWrite, type MemRead } from '../../lib/narratorBus';
import ByteFlightNarrator from '../../components/narrators/ByteFlightNarrator.vue';
import Term from '../../components/glossary/Term.vue';
import { Target, Pin as PinIcon } from '@lucide/vue';
import MemoryMinimap, { type Pin } from '../../components/memory/MemoryMinimap.vue';
import MemoryCoreChip from '../../components/memory/MemoryCoreChip.vue';
import MemoryHexGrid, { type HeatEntry } from '../../components/memory/MemoryHexGrid.vue';
import MemoryActivityFeed, { type FeedEntry } from '../../components/memory/MemoryActivityFeed.vue';
import { fmtAddr, regionFor, MEM_TOTAL } from '../../lib/memoryLayout';

const cpu = useCpuStore();
const prefs = usePreferencesStore();

const ROW_COUNT = 24;
const WND_SIZE = ROW_COUNT * 16; // 384 B visible

const baseAddr = ref(0x200000);
const bytes = ref<number[]>([]);
const fetchError = ref('');

// When ON, the window re-centers on the most recent mem_write/mem_read so
// the audience always sees the action — useful for kernel-heavy demos
// (race_counter, philos_*, producer_consumer) whose activity lives at
// 0x001FFFE0 area, outside the user_code default window.
const autoFollow = ref(false);
const FOLLOW_THROTTLE_MS = 600;
let lastFollowAt = 0;

const wndEnd = computed(() => baseAddr.value + WND_SIZE);
const currentRegion = computed(() => regionFor(baseAddr.value));

// ─── Live activity state ───────────────────────────────────────────────────
// `heat` carries the "what is hot right now" map, keyed by absolute address.
// It is the source for the per-cell decay animation. `lastWrite`/`lastRead`
// also keep the last event per address but never expire (until GC); the byte
// popover reads from them to show "last write by ... 4.2 s ago" even when
// the heat animation has long ended.
//
// pins is a ring buffer of recent events painted on the minimap. Capped to
// keep DOM nodes bounded — pin elements live for 8 s each via CSS animation.

const heat = ref<Map<number, HeatEntry>>(new Map());
const lastWriteByAddr = ref<Map<number, HeatEntry>>(new Map());
const lastReadByAddr = ref<Map<number, HeatEntry>>(new Map());

const PIN_CAP = 200;
const pins = ref<Pin[]>([]);
let pinSeq = 0;

const FEED_CAP = 12;
const feed = ref<FeedEntry[]>([]);
let feedSeq = 0;

let heatSeqCounter = 0;
const HEAT_TTL_MS = 60_000;

function gcMaps() {
  // Drop entries older than 60 s on every event burst. Cheap; the maps are
  // typically small (<200 entries) and we never hit O(n) over the whole 4 MB.
  const now = performance.now();
  const cutoff = now - HEAT_TTL_MS;
  for (const [addr, entry] of heat.value) {
    if (entry.t0 < cutoff) heat.value.delete(addr);
  }
}

function recordEvent(kind: 'write' | 'read', addr: number, value: number, coreId: number, pid: number) {
  heatSeqCounter += 1;
  const entry: HeatEntry = {
    seq: heatSeqCounter,
    coreId,
    kind,
    t0: performance.now(),
    pid,
    value,
  };

  // Heatmap layer (write resets read, write resets write — always newest wins).
  if (addr >= baseAddr.value && addr < wndEnd.value) {
    heat.value.set(addr, entry);
    // Reactive bump: Vue Map mutations require a new reference for templates
    // depending on Map.get to re-evaluate. Cheaper than `new Map(...)` on
    // every event because we only touch the affected key.
    heat.value = new Map(heat.value);
  }

  // Persistent "last X" maps for the popover (no decay).
  if (kind === 'write') {
    lastWriteByAddr.value.set(addr, entry);
    lastWriteByAddr.value = new Map(lastWriteByAddr.value);
  } else {
    lastReadByAddr.value.set(addr, entry);
    lastReadByAddr.value = new Map(lastReadByAddr.value);
  }

  // Mini-map pin ring buffer.
  pinSeq += 1;
  pins.value.push({ id: pinSeq, addr, kind, coreId, t0: entry.t0 });
  if (pins.value.length > PIN_CAP) pins.value.splice(0, pins.value.length - PIN_CAP);

  // Activity feed (per-core filtering happens in the feed component).
  feedSeq += 1;
  feed.value.unshift({
    id: feedSeq, addr, value, kind, coreId, pid, ts: entry.t0,
  });
  if (feed.value.length > FEED_CAP) feed.value.length = FEED_CAP;

  // Speculative local mutation so the grid shows the new byte even before
  // the next periodic reload finishes.
  if (kind === 'write' && addr >= baseAddr.value && addr < wndEnd.value) {
    const idx = addr - baseAddr.value;
    if (idx >= 0 && idx < bytes.value.length) {
      bytes.value[idx] = value & 0xff;
    }
  }

  // Auto-follow: keep the window centered on the most recent activity. The
  // throttle keeps the view from re-centering many times per second when
  // multiple events arrive in a burst (which would be disorienting). We
  // skip the jump if the event already falls inside the visible window —
  // no point moving when the user already sees it pulse.
  if (autoFollow.value && (addr < baseAddr.value || addr >= wndEnd.value)) {
    const now = performance.now();
    if (now - lastFollowAt > FOLLOW_THROTTLE_MS) {
      lastFollowAt = now;
      jumpTo(addr - WND_SIZE / 2);
    }
  }

  gcMaps();
}

// ─── Memory fetch ──────────────────────────────────────────────────────────

async function loadWindow() {
  try {
    const r = await fetch(`/api/memory?addr=0x${baseAddr.value.toString(16)}&len=${WND_SIZE}`);
    if (!r.ok) throw new Error('HTTP ' + r.status);
    const data = (await r.json()) as { bytes: number[] };
    bytes.value = data.bytes;
    fetchError.value = '';
  } catch (e) {
    fetchError.value = (e as Error).message;
  }
}

// ─── Narrator subscription ─────────────────────────────────────────────────

const narrators = useNarrators();
narrators.onMemWrite((m: MemWrite) => {
  // Honor the global SimBar core filter so hidden cores leave no trace
  // here either (feed/heat/pins). Matches the model documented in
  // cpu.ts: "selectedCoreIds rules every pane in the app."
  if (!cpu.isCoreVisible(m.coreId)) return;
  recordEvent('write', m.addr, m.newValue, m.coreId, m.pid);
});
narrators.onMemRead((m: MemRead) => {
  if (!cpu.isCoreVisible(m.coreId)) return;
  recordEvent('read', m.addr, m.value, m.coreId, m.pid);
});

let timer = 0;
onMounted(() => {
  loadWindow();
  timer = window.setInterval(loadWindow, 2000);
});
onUnmounted(() => { if (timer) window.clearInterval(timer); });

watch(baseAddr, () => {
  // Switching the window invalidates the heatmap for the previous range.
  // Keep lastWrite/lastRead so jumping back still shows accurate history.
  heat.value = new Map();
  loadWindow();
});

// ─── Navigation: jump / scroll / input ─────────────────────────────────────

function clampAddr(a: number): number {
  if (!Number.isFinite(a)) return baseAddr.value;
  const max = MEM_TOTAL - WND_SIZE;
  return Math.max(0, Math.min(max, a)) & ~0xf;
}

function jumpTo(a: number) {
  baseAddr.value = clampAddr(a);
}

function scrollBy(delta: number) {
  jumpTo(baseAddr.value + delta);
}

// Highlight pulse on the row holding the jumped-to byte. Used for explicit
// user actions on a *specific* address (chip EIP/ESP, feed entry, minimap
// pin, hex input). Bulk jumps (presets, autofocus, region buttons) skip
// the pulse to avoid visual noise.
const highlightedAddr = ref<number | null>(null);
const highlightSeq = ref(0);
let highlightTimer = 0;

function jumpToAndHighlight(addr: number) {
  jumpTo(addr);
  highlightedAddr.value = clampAddr(addr);
  highlightSeq.value++;
  if (highlightTimer) window.clearTimeout(highlightTimer);
  highlightTimer = window.setTimeout(() => { highlightedAddr.value = null; }, 1700);
}

// Quick shortcuts left over from the previous design — kept as a small row
// below the address input. "Stack" is dynamic so a stack-heavy demo keeps
// ESP visible without manual scrolling.
type Preset = { label: string; resolve: () => number };
const presets: Preset[] = [
  { label: 'Programa', resolve: () => 0x200000 },
  { label: 'Stack',    resolve: () => Math.max(0, ((cpu.regs.ESP || 0x33fff0) & ~0xff) - 0x80) },
  { label: 'IDT',      resolve: () => 0x100000 },
];

// Free-form hex / decimal input.
const inputAddr = ref('');
const inputError = ref(false);

function commitInput() {
  const raw = inputAddr.value.trim();
  if (!raw) return;
  const n = raw.startsWith('0x') || raw.startsWith('0X')
    ? parseInt(raw.slice(2), 16)
    : Number(raw);
  if (!Number.isFinite(n) || n < 0) {
    inputError.value = true;
    return;
  }
  inputError.value = false;
  jumpToAndHighlight(n);
}

watch(inputAddr, () => { inputError.value = false; });

// Auto-follow ESP when a stack-touching demo is loaded.
cpu.subscribe((ev) => {
  if (ev.type === 'loaded' && (ev.name === 'pushpop' || ev.name === 'callret')) {
    setTimeout(() => { jumpTo(presets[1].resolve()); }, 200);
  }
});
</script>

<template>
  <div class="memory-page">
    <ByteFlightNarrator class="memory-page__overlay" />

    <header class="memory-page__head">
      <div class="memory-page__title">
        <Term term="MMU" mode="title" class="memory-page__title-main">Memoria</Term>
        <span class="memory-page__title-sub">
          Ventana <code>{{ fmtAddr(baseAddr) }}</code> · <code>{{ WND_SIZE }} B</code>
          <template v-if="currentRegion">
            · <span class="memory-page__title-region">{{ currentRegion.label }}</span>
            <span class="memory-page__title-perm">user {{ currentRegion.userPerm }} · kernel {{ currentRegion.kernPerm }}</span>
          </template>
        </span>
      </div>

      <MemoryMinimap
        :base-addr="baseAddr"
        :wnd-size="WND_SIZE"
        :pins="pins"
        @jump="jumpToAndHighlight"
      />

      <div class="memory-page__nav">
        <label class="memory-page__input">
          <span class="memory-page__input-prefix">addr</span>
          <input
            v-model="inputAddr"
            type="text"
            inputmode="text"
            placeholder="0x200000"
            spellcheck="false"
            autocomplete="off"
            :class="{ 'is-error': inputError }"
            @keydown.enter="commitInput"
            @blur="commitInput"
          />
        </label>
        <div class="memory-page__presets">
          <button
            v-for="p in presets"
            :key="p.label"
            type="button"
            @click="jumpTo(p.resolve())"
          >{{ p.label }}</button>
          <button
            type="button"
            :class="['memory-page__follow', { 'memory-page__follow--on': autoFollow }]"
            :title="autoFollow
              ? 'Autofoco activo — la ventana de memoria salta sola al lugar donde el simulador escribe o lee. Toque para fijarla.'
              : 'Autofoco — toque para que la ventana de memoria salte sola al lugar donde el simulador escribe o lee.'"
            @click="autoFollow = !autoFollow"
          >
            <component :is="autoFollow ? Target : PinIcon" :size="13" :stroke-width="2" />
            <span>Autofoco</span>
          </button>
        </div>
      </div>
    </header>

    <div
      v-if="!prefs.narratorsEnabled"
      class="memory-page__hint"
      title="Active 'Narradores visuales' en Preferencias para ver los bytes viajando"
    >
      Narradores apagados — actívelos en Preferencias para ver los bytes viajando.
    </div>

    <div class="memory-page__body">
      <aside class="memory-page__cores">
        <MemoryCoreChip
          v-for="c in cpu.cores"
          :key="c.id"
          :core="c"
          :selected="c.id === cpu.selectedCoreId"
          :hidden-by-filter="!cpu.isCoreVisible(c.id)"
          @select="cpu.setSelectedCore"
          @jump="jumpToAndHighlight"
        />
      </aside>

      <MemoryHexGrid
        class="memory-page__grid"
        :base-addr="baseAddr"
        :row-count="ROW_COUNT"
        :bytes="bytes"
        :heat="heat"
        :last-read-by-addr="lastReadByAddr"
        :last-write-by-addr="lastWriteByAddr"
        :error-message="fetchError"
        :highlighted-addr="highlightedAddr"
        :highlight-seq="highlightSeq"
        @scroll-by="scrollBy"
      />

      <MemoryActivityFeed
        class="memory-page__feed"
        :entries="feed"
        :num-cores="cpu.numCores"
        :visible-core-ids="cpu.selectedCoreIds"
        @jump="jumpToAndHighlight"
      />
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
  /* Narrator sits above grid but below modals/topbar. */
}

.memory-page__head {
  display: grid;
  grid-template-columns: auto 1fr auto;
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
  min-width: 200px;
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
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}

.memory-page__title-sub code {
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
}

.memory-page__title-region {
  color: var(--green);
  text-transform: uppercase;
  letter-spacing: 1px;
  font-size: 10px;
}

.memory-page__title-perm {
  font-size: 9.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.memory-page__nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
}

.memory-page__input {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  padding-left: 6px;
}

.memory-page__input-prefix {
  font-size: 9.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.memory-page__input input {
  background: transparent;
  border: none;
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 4px 8px;
  width: 110px;
  outline: none;
  min-height: unset;
}

.memory-page__input input.is-error {
  color: var(--red-soft);
}

.memory-page__input input::placeholder {
  color: var(--text-muted);
}

.memory-page__presets {
  display: flex;
  gap: 0;
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
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 3px 8px;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
  min-height: unset;
}

.memory-page__presets button:last-child { border-right: none; }
.memory-page__presets button:hover { background: rgba(var(--accent-rgb), 0.10); color: var(--amber); }

.memory-page__follow {
  display: inline-flex !important;
  align-items: center;
  gap: 4px;
  border-left: 1px dashed var(--border-strong) !important;
}
.memory-page__follow svg { display: block; }
.memory-page__follow--on {
  background: color-mix(in oklab, var(--amber) 22%, transparent);
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}
.memory-page__follow--on svg {
  filter: drop-shadow(0 0 4px var(--amber));
  animation: autofocus-pulse 1.4s ease-in-out infinite;
  transform-origin: center;
}

@keyframes autofocus-pulse {
  0%, 100% {
    transform: scale(1);
    filter: drop-shadow(0 0 3px var(--amber));
  }
  50% {
    transform: scale(1.18);
    filter: drop-shadow(0 0 9px var(--amber));
  }
}

@media (prefers-reduced-motion: reduce) {
  .memory-page__follow--on svg { animation: none; }
}

.memory-page__hint {
  font-size: 11px;
  color: var(--text-dim);
  letter-spacing: 0.5px;
  padding: 6px var(--space-5);
  border-bottom: 1px dashed var(--border);
  flex-shrink: 0;
}

.memory-page__body {
  display: grid;
  grid-template-columns: 200px 1fr 270px;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  overflow: hidden;
  flex: 1;
  min-height: 0;
}

.memory-page__cores {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  overflow-y: auto;
  min-height: 0;
}

.memory-page__grid {
  min-height: 0;
}

.memory-page__feed {
  min-height: 0;
}

@media (max-width: 1100px) {
  .memory-page__head {
    grid-template-columns: 1fr;
    grid-template-rows: auto auto auto;
  }
  .memory-page__nav { align-items: flex-start; }
  .memory-page__body {
    grid-template-columns: 1fr;
    grid-template-rows: auto 1fr auto;
  }
  .memory-page__cores {
    flex-direction: row;
    overflow-x: auto;
  }
  .memory-page__cores > * {
    min-width: 180px;
  }
}
</style>
