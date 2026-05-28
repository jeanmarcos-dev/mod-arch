<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import type { CoreInfo } from '../stores/cpu';
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const cpu = useCpuStore();
const { t } = useI18n();

// View tabs are global across the accordion. EIP/ESP make each card center on
// its own core; Custom collapses the accordion into a single full-width dump.
const view = ref<'eip' | 'esp' | 'custom'>('eip');
const customAddr = ref('0x100000');

interface CardData {
  bytes: number[];
  baseAddr: number;
  loading: boolean;
  error: string;
}

// Per-core fetched payloads. Keyed by coreId so we can keep dumps across
// collapse/expand cycles without re-fetching.
const fetchedByCore = ref<Record<number, CardData>>({});
// Single-dump state for the Custom tab. Shared RAM, no point in replicating
// the same bytes once per card.
const customData = ref<CardData>({ bytes: [], baseAddr: 0, loading: false, error: '' });

const EXPANDED_KEY = 'modos.memory.expandedCores';

function loadExpanded(): Set<number> {
  if (typeof window === 'undefined') return new Set([0]);
  try {
    const raw = window.localStorage.getItem(EXPANDED_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return new Set(parsed.filter((x: unknown): x is number => typeof x === 'number'));
      }
    }
  } catch { /* corrupt entry, fall back to default */ }
  return new Set([0]);
}

function persistExpanded() {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(EXPANDED_KEY, JSON.stringify([...expandedCores.value]));
  } catch { /* quota / private mode → keep in memory only */ }
}

const expandedCores = ref<Set<number>>(loadExpanded());

function isExpanded(coreId: number): boolean {
  return expandedCores.value.has(coreId);
}

function toggleExpanded(coreId: number) {
  const next = new Set(expandedCores.value);
  if (next.has(coreId)) next.delete(coreId);
  else next.add(coreId);
  expandedCores.value = next;
  persistExpanded();
  if (next.has(coreId) && view.value !== 'custom') {
    loadCore(coreId);
  }
}

function parseUserAddr(s: string): number {
  s = s.trim();
  if (s.startsWith('0x') || s.startsWith('0X')) {
    const n = parseInt(s.slice(2), 16);
    return isNaN(n) ? -1 : n;
  }
  const n = parseInt(s, 10);
  return isNaN(n) ? -1 : n;
}

function targetAddrFor(c: CoreInfo): number {
  if (view.value === 'esp') return (c.regs.ESP ?? 0) >>> 0;
  // EIP context: show 16 bytes before EIP so the executing instruction sits
  // a few bytes into the dump and the user can also see what was just run.
  return Math.max(0, ((c.regs.EIP ?? 0) >>> 0) - 16);
}

async function loadCore(coreId: number) {
  if (!cpu.connected) return;
  if (view.value === 'custom') return;
  const core = cpu.cores.find(c => c.id === coreId);
  if (!core) return;
  const addr = targetAddrFor(core);
  const prev = fetchedByCore.value[coreId];
  fetchedByCore.value = {
    ...fetchedByCore.value,
    [coreId]: { ...(prev ?? { bytes: [], baseAddr: 0 }), loading: true, error: '' } as CardData,
  };
  try {
    const res = await fetch(`/api/memory?addr=0x${addr.toString(16)}&len=128`);
    if (!res.ok) {
      const text = await res.text();
      fetchedByCore.value = {
        ...fetchedByCore.value,
        [coreId]: { bytes: prev?.bytes ?? [], baseAddr: prev?.baseAddr ?? 0, loading: false, error: text },
      };
      return;
    }
    const data = await res.json();
    fetchedByCore.value = {
      ...fetchedByCore.value,
      [coreId]: { bytes: data.bytes, baseAddr: data.addr, loading: false, error: '' },
    };
  } catch (e) {
    fetchedByCore.value = {
      ...fetchedByCore.value,
      [coreId]: { bytes: prev?.bytes ?? [], baseAddr: prev?.baseAddr ?? 0, loading: false, error: String(e) },
    };
  }
}

async function loadCustom() {
  if (!cpu.connected || view.value !== 'custom') return;
  const addr = parseUserAddr(customAddr.value);
  if (addr < 0) {
    customData.value = { ...customData.value, error: t('panels.memory.invalidAddr') };
    return;
  }
  customData.value = { ...customData.value, loading: true, error: '' };
  try {
    const res = await fetch(`/api/memory?addr=0x${addr.toString(16)}&len=128`);
    if (!res.ok) {
      customData.value = { ...customData.value, loading: false, error: await res.text() };
      return;
    }
    const data = await res.json();
    customData.value = { bytes: data.bytes, baseAddr: data.addr, loading: false, error: '' };
  } catch (e) {
    customData.value = { ...customData.value, loading: false, error: String(e) };
  }
}

// Refetch every expanded card whenever the relevant inputs change. The
// dependency string covers EIP/ESP/current_pid per visible core so any
// instruction stepped on any core triggers an update — without this, a card
// for C1 would freeze while the user focused C0.
watch(
  () => [
    view.value,
    cpu.tick,
    cpu.connected,
    cpu.visibleCores
      .map(c => `${c.id}:${c.regs.EIP ?? 0}:${c.regs.ESP ?? 0}:${c.current_pid}`)
      .join('|'),
  ],
  () => {
    if (view.value === 'custom') { loadCustom(); return; }
    for (const c of cpu.visibleCores) {
      if (expandedCores.value.has(c.id)) loadCore(c.id);
    }
  },
);

watch(() => customAddr.value, () => {
  if (view.value === 'custom') loadCustom();
});

// External "jump to address" intent (currently fired by ProcessesPanel when
// the user clicks → on a PCB row). We switch to the Custom view because that
// is the only mode that accepts an arbitrary base address; EIP/ESP would
// snap back to the live registers on the next tick.
watch(() => cpu.pendingMemoryJump, (addr) => {
  if (addr === null || addr === undefined) return;
  view.value = 'custom';
  customAddr.value = `0x${addr.toString(16).toUpperCase()}`;
  cpu.clearMemoryJump();
});

onMounted(() => {
  if (view.value === 'custom') loadCustom();
  else for (const c of cpu.visibleCores) if (expandedCores.value.has(c.id)) loadCore(c.id);
});

function hex(n: number, w = 2) {
  return (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function fmtAddr(n: number): string {
  return `0x${hex(n, 8)}`;
}

// Identifies the region of a given address. Used to tint the address column
// per row (kernel = red, user_code = green, user_stack = yellow).
function regionOf(addr: number): string {
  if (addr < 0x100000) return 'reserved';
  if (addr < 0x200000) return 'kernel';
  if (addr < 0x300000) return 'user_code';
  if (addr < 0x400000) return 'user_stack';
  return 'oob';
}

function linesFor(data: CardData) {
  const out: { addr: number; bytes: number[]; ascii: string }[] = [];
  for (let i = 0; i < data.bytes.length; i += 16) {
    const chunk = data.bytes.slice(i, i + 16);
    const ascii = chunk.map(b => (b >= 0x20 && b < 0x7F) ? String.fromCharCode(b) : '.').join('');
    out.push({ addr: data.baseAddr + i, bytes: chunk, ascii });
  }
  return out;
}

// Per-card hit detection: each card only highlights its own EIP/ESP. Two
// cards can sit next to each other and each paints its own cursor on its own
// bytes — no ambiguity about which core a highlight belongs to.
function isEipHit(coreId: number, addr: number): boolean {
  const c = cpu.cores.find(x => x.id === coreId);
  if (!c) return false;
  return ((c.regs.EIP ?? 0) >>> 0) === addr;
}

function isEspHit(coreId: number, addr: number): boolean {
  const c = cpu.cores.find(x => x.id === coreId);
  if (!c) return false;
  const esp = (c.regs.ESP ?? 0) >>> 0;
  return addr >= esp && addr < esp + 4;
}

function hitStyle(coreId: number, addr: number): Record<string, string> | undefined {
  if (isEipHit(coreId, addr)) {
    return { background: `var(--core-${coreId % 4})`, color: 'var(--core-fg)' };
  }
  if (isEspHit(coreId, addr)) {
    return { boxShadow: `inset 0 -2px 0 var(--core-${coreId % 4})` };
  }
  return undefined;
}

function pidFor(coreId: number): number {
  return cpu.cores.find(c => c.id === coreId)?.current_pid ?? 0;
}

function eipFor(coreId: number): number {
  return (cpu.cores.find(c => c.id === coreId)?.regs.EIP ?? 0) >>> 0;
}

function espFor(coreId: number): number {
  return (cpu.cores.find(c => c.id === coreId)?.regs.ESP ?? 0) >>> 0;
}

function regionLabel(addr: number): string {
  const r = regionOf(addr);
  if (r === 'reserved') return t('panels.memory.regions.reserved');
  if (r === 'kernel') return t('panels.memory.regions.kernel');
  if (r === 'user_code') return t('panels.memory.regions.userCode');
  if (r === 'user_stack') return t('panels.memory.regions.userStack');
  return t('panels.memory.regions.none');
}

function cardData(coreId: number): CardData {
  return fetchedByCore.value[coreId] ?? { bytes: [], baseAddr: 0, loading: false, error: '' };
}

// When only one core is visible the chevron and click-to-collapse are not
// useful — the body should be permanently visible like in the pre-multicore
// design. With 2+ cores the accordion behavior kicks in.
const collapsibleMode = computed(() => cpu.visibleCores.length > 1);

// Effective expansion: in single-core mode every card is forced expanded; in
// multicore the user's persisted set decides.
function effectivelyExpanded(coreId: number): boolean {
  return !collapsibleMode.value || isExpanded(coreId);
}
</script>

<template>
  <div class="panel mem" data-pizarra-anchor="memory-panel">
    <div class="panel-title">{{ t('panels.memory.title') }}</div>

    <div class="controls">
      <div class="view-tabs">
        <button
          @click="view = 'eip'"
          :class="{ active: view === 'eip' }"
          :title="t('panels.memory.tabHints.eip')"
        >{{ t('panels.memory.tabs.eip') }}</button>
        <button
          @click="view = 'esp'"
          :class="{ active: view === 'esp' }"
          :title="t('panels.memory.tabHints.esp')"
        >{{ t('panels.memory.tabs.esp') }}</button>
        <button
          @click="view = 'custom'"
          :class="{ active: view === 'custom' }"
          :title="t('panels.memory.tabHints.custom')"
        >{{ t('panels.memory.tabs.custom') }}</button>
      </div>
      <input
        v-if="view === 'custom'"
        v-model="customAddr"
        name="memory_address"
        :aria-label="t('panels.memory.addrAria')"
        class="addr-input"
        placeholder="0x100000"
      />
    </div>

    <!-- Custom: single full-width dump. Shared RAM ⇒ replicating across cards
         would just show the same bytes N times. -->
    <div v-if="view === 'custom'" class="memory-stack">
      <section class="memory-card memory-card--custom">
        <header class="memory-card__head">
          <span class="memory-card__label">{{ t('panels.memory.customLabel') }}</span>
          <span class="memory-card__reg">addr {{ fmtAddr(customData.baseAddr) }}</span>
          <span class="memory-card__region">{{ regionLabel(customData.baseAddr) }}</span>
        </header>
        <div v-if="customData.error" class="error">{{ customData.error }}</div>
        <pre v-else class="hex-dump">
          <div
            v-for="(line, i) in linesFor(customData)"
            :key="i"
            class="hex-line"
            :class="regionOf(line.addr)"
          >
            <span class="hex-addr">{{ hex(line.addr, 6) }}</span>
            <span class="hex-bytes">
              <span
                v-for="(b, j) in line.bytes"
                :key="j"
                class="hex-byte"
                :data-mem-addr="`0x${(line.addr + j).toString(16).toLowerCase()}`"
              >{{ hex(b) }}</span>
            </span>
            <span class="hex-ascii">{{ line.ascii }}</span>
          </div>
        </pre>
      </section>
    </div>

    <!-- EIP / ESP: one card per visible core. Header is always visible so a
         collapsed card still tells the user where C<id> is pointing. -->
    <div v-else class="memory-stack">
      <section
        v-for="c in cpu.visibleCores"
        :key="c.id"
        class="memory-card"
        :class="[
          `memory-card--core-${c.id % 4}`,
          { 'memory-card--collapsed': collapsibleMode && !effectivelyExpanded(c.id) },
        ]"
      >
        <header
          class="memory-card__head"
          :class="{ 'memory-card__head--clickable': collapsibleMode }"
          :role="collapsibleMode ? 'button' : undefined"
          :tabindex="collapsibleMode ? 0 : undefined"
          :aria-expanded="collapsibleMode ? effectivelyExpanded(c.id) : undefined"
          @click="collapsibleMode && toggleExpanded(c.id)"
          @keydown.enter.prevent="collapsibleMode && toggleExpanded(c.id)"
          @keydown.space.prevent="collapsibleMode && toggleExpanded(c.id)"
        >
          <span v-if="collapsibleMode" class="memory-card__chevron">{{ effectivelyExpanded(c.id) ? '▾' : '▸' }}</span>
          <span
            class="memory-card__label"
            :style="{ color: `var(--core-${c.id % 4})`, borderColor: `var(--core-${c.id % 4})` }"
          >C{{ c.id }}</span>
          <span class="memory-card__pid">PID {{ pidFor(c.id) || '—' }}</span>
          <span class="memory-card__reg">EIP {{ fmtAddr(eipFor(c.id)) }}</span>
          <span class="memory-card__reg">ESP {{ fmtAddr(espFor(c.id)) }}</span>
          <span class="memory-card__region">{{ regionLabel(view === 'esp' ? espFor(c.id) : eipFor(c.id)) }}</span>
        </header>

        <div v-if="effectivelyExpanded(c.id)" class="memory-card__body">
          <div v-if="cardData(c.id).error" class="error">{{ cardData(c.id).error }}</div>
          <pre v-else class="hex-dump">
            <div
              v-for="(line, i) in linesFor(cardData(c.id))"
              :key="i"
              class="hex-line"
              :class="regionOf(line.addr)"
            >
              <span class="hex-addr">{{ hex(line.addr, 6) }}</span>
              <span class="hex-bytes">
                <span
                  v-for="(b, j) in line.bytes"
                  :key="j"
                  class="hex-byte"
                  :class="{
                    'hex-byte--eip': isEipHit(c.id, line.addr + j),
                    'hex-byte--esp': isEspHit(c.id, line.addr + j) && !isEipHit(c.id, line.addr + j),
                  }"
                  :style="hitStyle(c.id, line.addr + j)"
                  :data-mem-addr="`0x${(line.addr + j).toString(16).toLowerCase()}`"
                >{{ hex(b) }}</span>
              </span>
              <span class="hex-ascii">{{ line.ascii }}</span>
            </div>
          </pre>
        </div>
      </section>
    </div>

    <div class="legend">
      <span class="leg leg-reserved">{{ t('panels.memory.legend.reserved') }}</span>
      <span class="leg leg-kernel">{{ t('panels.memory.legend.kernel') }}</span>
      <span class="leg leg-user_code">{{ t('panels.memory.legend.userCode') }}</span>
      <span class="leg leg-user_stack">{{ t('panels.memory.legend.userStack') }}</span>
    </div>
  </div>
</template>

<style scoped>
.mem {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.controls {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.view-tabs {
  display: flex;
  gap: 4px;
}

.view-tabs button {
  font-size: 11px;
  padding: 4px 10px;
  letter-spacing: 0;
  text-transform: none;
}

.view-tabs button.active {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  border-color: var(--amber);
}

.addr-input {
  background: var(--surface-overlay);
  border: 1px solid var(--border);
  color: var(--amber);
  padding: 4px 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  width: 110px;
}

.memory-stack {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.memory-card {
  border: 1px solid var(--border);
  border-left-width: 3px;
  background: var(--surface-overlay);
  display: flex;
  flex-direction: column;
}

.memory-card--core-0 { border-left-color: var(--core-0); }
.memory-card--core-1 { border-left-color: var(--core-1); }
.memory-card--core-2 { border-left-color: var(--core-2); }
.memory-card--core-3 { border-left-color: var(--core-3); }

.memory-card--custom {
  border-left-color: var(--amber);
}

.memory-card__head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-dim);
  flex-wrap: wrap;
}

.memory-card__head--clickable {
  cursor: pointer;
  user-select: none;
}

.memory-card__head--clickable:hover {
  background: rgba(var(--accent-rgb), 0.06);
}

.memory-card__head--clickable:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.memory-card__chevron {
  width: 10px;
  color: var(--text-dim);
  flex-shrink: 0;
}

.memory-card__label {
  font-weight: 700;
  border: 1px solid;
  padding: 1px 6px;
  font-size: 11px;
}

.memory-card__pid,
.memory-card__reg {
  color: var(--text);
}

.memory-card__region {
  margin-left: auto;
  color: var(--text-muted);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.memory-card__body {
  padding: 4px 10px 8px;
  border-top: 1px dashed var(--border);
}

.hex-dump {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.4;
  margin: 0;
}

.hex-line {
  display: flex;
  gap: 12px;
  padding: 1px 0;
}

.hex-addr {
  color: var(--cyan);
  flex-shrink: 0;
}

.hex-bytes {
  flex: 1;
}

.hex-byte {
  display: inline-block;
  padding: 0 2px;
  margin-right: 1px;
  color: var(--text);
}

.hex-byte--eip {
  font-weight: 700;
}

.hex-ascii {
  color: var(--text-dim);
  flex-shrink: 0;
  font-family: 'VT323', monospace;
  font-size: 14px;
  letter-spacing: 1px;
}

.hex-line.kernel .hex-addr { color: var(--red-soft); }
.hex-line.user_code .hex-addr { color: var(--green); }
.hex-line.user_stack .hex-addr { color: var(--yellow); }

.legend {
  display: flex;
  gap: 8px;
  margin-top: 6px;
  font-size: 11px;
  flex-wrap: wrap;
}

.leg {
  padding: 1px 6px;
  border: 1px solid;
  font-family: 'JetBrains Mono', monospace;
}

.leg-reserved { color: var(--text-dim); border-color: var(--text-dim); }
.leg-kernel { color: var(--red-soft); border-color: var(--red-soft); }
.leg-user_code { color: var(--green); border-color: var(--green); }
.leg-user_stack { color: var(--yellow); border-color: var(--yellow); }

.error {
  color: var(--red);
  font-size: 12px;
  padding: 8px;
}

@media (max-width: 768px) {
  .view-tabs button {
    padding: 10px 12px;
    font-size: 13px;
    min-height: 40px;
  }

  .hex-dump {
    overflow-x: auto;
    font-size: 13px;
  }

  .hex-line {
    white-space: nowrap;
  }

  .hex-ascii {
    font-size: 15px;
  }

  .addr-input {
    width: 130px;
    font-size: 14px;
    padding: 10px;
  }

  .legend {
    gap: 6px;
    font-size: 10px;
  }

  .memory-card__head {
    font-size: 12px;
    padding: 8px 10px;
  }
}
</style>
