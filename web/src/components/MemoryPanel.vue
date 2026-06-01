<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import type { CoreInfo } from '../stores/cpu';
import { computed, ref, watch, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';

const cpu = useCpuStore();
const { t } = useI18n();

// View tabs: EIP and ESP show one window that follows the selected core's
// pointer (cores do not own memory — they just point into a single shared
// RAM). Custom takes an arbitrary base address.
const view = ref<'eip' | 'esp' | 'custom'>('eip');
const customAddr = ref('0x100000');

// Core to follow in the EIP/ESP views. Null means "track whatever
// cpu.selectedCoreId is right now"; clicking the segmented selector pins
// the panel to that specific core.
const followedCoreId = ref<number | null>(null);

const effectiveCoreId = computed(() => followedCoreId.value ?? cpu.selectedCoreId);
const followedCore = computed<CoreInfo | undefined>(
  () => cpu.cores.find(c => c.id === effectiveCoreId.value),
);

interface DumpData {
  bytes: number[];
  baseAddr: number;
  loading: boolean;
  error: string;
}

// Single window for the EIP/ESP tabs. RAM is global so there is no point in
// fetching one slice per core.
const followed = ref<DumpData>({ bytes: [], baseAddr: 0, loading: false, error: '' });
const customData = ref<DumpData>({ bytes: [], baseAddr: 0, loading: false, error: '' });

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
  // EIP context: pad 16 bytes before EIP so the instruction about to run
  // sits a few rows into the dump and the user also sees what just ran.
  return Math.max(0, ((c.regs.EIP ?? 0) >>> 0) - 16);
}

async function loadFollowed() {
  if (!cpu.connected) return;
  if (view.value === 'custom') return;
  const c = followedCore.value;
  if (!c) {
    followed.value = { bytes: [], baseAddr: 0, loading: false, error: '' };
    return;
  }
  const addr = targetAddrFor(c);
  const prev = followed.value;
  followed.value = { ...prev, loading: true, error: '' };
  try {
    const res = await fetch(`/api/memory?addr=0x${addr.toString(16)}&len=128`);
    if (!res.ok) {
      followed.value = { bytes: prev.bytes, baseAddr: prev.baseAddr, loading: false, error: await res.text() };
      return;
    }
    const data = await res.json();
    followed.value = { bytes: data.bytes, baseAddr: data.addr, loading: false, error: '' };
  } catch (e) {
    followed.value = { bytes: prev.bytes, baseAddr: prev.baseAddr, loading: false, error: String(e) };
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

// Refetch when the active view, tick, connection, followed core or its
// EIP/ESP/current_pid changes. The dependency string compresses the
// followed-core registers so a step on the followed core triggers a refresh
// without thrashing on unrelated cores.
watch(
  () => {
    const c = followedCore.value;
    return [
      view.value,
      cpu.tick,
      cpu.connected,
      effectiveCoreId.value,
      c ? `${c.regs.EIP ?? 0}:${c.regs.ESP ?? 0}:${c.current_pid}` : '',
    ];
  },
  () => {
    if (view.value === 'custom') { loadCustom(); return; }
    loadFollowed();
  },
);

watch(() => customAddr.value, () => {
  if (view.value === 'custom') loadCustom();
});

// If the followed core gets filtered out via the SimBar visibility toggle,
// snap to the first still-visible core so the panel never points at a
// hidden core.
watch(
  () => cpu.visibleCores.map(c => c.id).join(','),
  () => {
    if (followedCoreId.value === null) return;
    if (!cpu.isCoreVisible(followedCoreId.value)) {
      followedCoreId.value = cpu.visibleCores[0]?.id ?? null;
    }
  },
);

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
  else loadFollowed();
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

function linesFor(data: DumpData) {
  const out: { addr: number; bytes: number[]; ascii: string }[] = [];
  for (let i = 0; i < data.bytes.length; i += 16) {
    const chunk = data.bytes.slice(i, i + 16);
    const ascii = chunk.map(b => (b >= 0x20 && b < 0x7F) ? String.fromCharCode(b) : '.').join('');
    out.push({ addr: data.baseAddr + i, bytes: chunk, ascii });
  }
  return out;
}

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

function pidNameFor(coreId: number): string {
  const pid = cpu.cores.find(c => c.id === coreId)?.current_pid ?? 0;
  if (!pid) return '';
  return cpu.procs.find(p => p.pid === pid)?.name ?? '';
}

const followedAddr = computed(() => {
  const c = followedCore.value;
  if (!c) return 0;
  return view.value === 'esp' ? espFor(c.id) : eipFor(c.id);
});

const followedRegLabel = computed(() => (view.value === 'esp' ? 'ESP' : 'EIP'));

// Trap frame readout: when the followed core is parked in PL0, the HW just
// pushed EFLAGS/CS/EIP from the trapped process onto the kernel stack. ESP
// points at EIP, so the three dwords at [+0/+4/+8] are the live "where the
// process will resume on IRET". Decoded from the ESP dump bytes the panel
// already fetched (little-endian uint32 each).
const trapFrame = computed(() => {
  if (view.value !== 'esp') return null;
  const c = followedCore.value;
  if (!c || c.mode !== 0) return null;
  const bytes = followed.value.bytes;
  if (bytes.length < 12) return null;
  const u32 = (off: number) =>
    ((bytes[off] | (bytes[off + 1] << 8) | (bytes[off + 2] << 16) | (bytes[off + 3] << 24)) >>> 0);
  return {
    eip: u32(0),
    cs: u32(4) & 0xffff, // CS is zero-extended to 32 bits on push, low 16 are the selector
    eflags: u32(8),
    espBase: followed.value.baseAddr,
  };
});

// Switch to Custom and jump the dump to the start of a memory region. Used by
// the clickable legend chips below the dump — EIP/ESP would snap back to the
// live pointers on the next tick so Custom is the only honest target.
function jumpToRegion(addr: number) {
  view.value = 'custom';
  customAddr.value = `0x${addr.toString(16).toUpperCase()}`;
}
</script>

<template>
  <div class="panel mem" data-pizarra-anchor="memory-panel" data-narrator-anchor="memory-panel">
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

      <div
        v-if="view !== 'custom' && cpu.visibleCores.length > 1"
        class="core-selector"
        :title="t('panels.memory.coreSelectorHint')"
        role="radiogroup"
        :aria-label="t('panels.memory.coreSelectorLabel')"
      >
        <span class="core-selector__label">{{ t('panels.memory.coreSelectorLabel') }}</span>
        <button
          v-for="c in cpu.visibleCores"
          :key="c.id"
          type="button"
          :class="['core-selector__btn', { active: c.id === effectiveCoreId }]"
          :style="{ '--core-color': `var(--core-${c.id % 4})` }"
          :aria-checked="c.id === effectiveCoreId"
          role="radio"
          @click="followedCoreId = c.id"
        >C{{ c.id }}</button>
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

    <div v-if="view !== 'custom' && followedCore" class="following-line">
      <span class="following-line__label">{{ t('panels.memory.followingLabel') }}</span>
      <span
        class="following-line__core"
        :style="{ color: `var(--core-${followedCore.id % 4})`, borderColor: `var(--core-${followedCore.id % 4})` }"
      >C{{ followedCore.id }}:{{ followedRegLabel }}</span>
      <span class="following-line__addr">@ {{ fmtAddr(followedAddr) }}</span>
      <span v-if="followedCore.current_pid" class="following-line__pid">
        · {{ t('panels.memory.followingPid', { pid: followedCore.current_pid, name: pidNameFor(followedCore.id) || '—' }) }}
      </span>
      <span class="following-line__region">{{ regionLabel(followedAddr) }}</span>
    </div>

    <!-- Trap frame readout: visible only on the ESP tab when the followed
         core is in PL0. Decodes the three dwords the HW pushed at IRQ entry
         (EIP / CS / EFLAGS of the trapped process) into a labeled strip so
         the alumno can see "this is where P5 will resume on IRET". -->
    <div v-if="trapFrame" class="trap-frame" :title="t('panels.memory.trapFrameTitle')">
      <div class="trap-frame__title">{{ t('panels.memory.trapFrameTitleLabel') }}</div>
      <div class="trap-frame__row">
        <span class="trap-frame__off">[ESP+0]</span>
        <span class="trap-frame__label">{{ t('panels.memory.trapFrameSavedEip') }}</span>
        <span class="trap-frame__val">0x{{ trapFrame.eip.toString(16).toUpperCase().padStart(8, '0') }}</span>
      </div>
      <div class="trap-frame__row">
        <span class="trap-frame__off">[ESP+4]</span>
        <span class="trap-frame__label">{{ t('panels.memory.trapFrameSavedCs') }}</span>
        <span class="trap-frame__val">0x{{ trapFrame.cs.toString(16).toUpperCase().padStart(4, '0') }}</span>
      </div>
      <div class="trap-frame__row">
        <span class="trap-frame__off">[ESP+8]</span>
        <span class="trap-frame__label">{{ t('panels.memory.trapFrameSavedEflags') }}</span>
        <span class="trap-frame__val">0x{{ trapFrame.eflags.toString(16).toUpperCase().padStart(8, '0') }}</span>
      </div>
    </div>
    <div v-else-if="view !== 'custom'" class="following-line following-line--empty">
      {{ t('panels.memory.followingNoCore') }}
    </div>

    <!-- Custom: single full-width dump. -->
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

    <!-- EIP / ESP: a single hex window that follows the selected core. RAM is
         global; cores are just pointers into it. -->
    <div v-else class="memory-stack">
      <section
        v-if="followedCore"
        class="memory-card"
        :class="`memory-card--core-${followedCore.id % 4}`"
      >
        <div v-if="followed.error" class="error">{{ followed.error }}</div>
        <pre v-else class="hex-dump">
          <div
            v-for="(line, i) in linesFor(followed)"
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
                  'hex-byte--eip': isEipHit(followedCore.id, line.addr + j),
                  'hex-byte--esp': isEspHit(followedCore.id, line.addr + j) && !isEipHit(followedCore.id, line.addr + j),
                }"
                :style="hitStyle(followedCore.id, line.addr + j)"
                :data-mem-addr="`0x${(line.addr + j).toString(16).toLowerCase()}`"
              >{{ hex(b) }}</span>
            </span>
            <span class="hex-ascii">{{ line.ascii }}</span>
          </div>
        </pre>
      </section>
    </div>

    <div class="legend">
      <button
        type="button"
        class="leg leg-reserved"
        :title="t('shell.memoryRegionChip.jumpHint', { name: t('panels.memory.legend.reserved') })"
        @click="jumpToRegion(0x000000)"
      >{{ t('panels.memory.legend.reserved') }}</button>
      <button
        type="button"
        class="leg leg-kernel"
        :title="t('shell.memoryRegionChip.jumpHint', { name: t('panels.memory.legend.kernel') })"
        @click="jumpToRegion(0x100000)"
      >{{ t('panels.memory.legend.kernel') }}</button>
      <button
        type="button"
        class="leg leg-user_code"
        :title="t('shell.memoryRegionChip.jumpHint', { name: t('panels.memory.legend.userCode') })"
        @click="jumpToRegion(0x200000)"
      >{{ t('panels.memory.legend.userCode') }}</button>
      <button
        type="button"
        class="leg leg-user_stack"
        :title="t('shell.memoryRegionChip.jumpHint', { name: t('panels.memory.legend.userStack') })"
        @click="jumpToRegion(0x300000)"
      >{{ t('panels.memory.legend.userStack') }}</button>
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
  gap: 2px;
  border-bottom: 1px solid var(--border);
}

.view-tabs button {
  position: relative;
  font-size: 11px;
  padding: 5px 12px;
  letter-spacing: 0;
  text-transform: none;
  min-height: 0;
  background: var(--panel-soft);
  color: var(--text-muted);
  border: 1px solid var(--border);
  border-bottom: none;
  margin-bottom: -1px;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
}

.view-tabs button:hover:not(.active) {
  background: color-mix(in oklab, var(--accent) 8%, var(--panel-soft));
  color: var(--text);
  border-color: var(--border-strong);
  box-shadow: none;
}

.view-tabs button.active {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--accent);
  border-color: var(--border);
}

.view-tabs button.active::before {
  content: '';
  position: absolute;
  top: -1px;
  left: -1px;
  right: -1px;
  height: 2px;
  background: var(--accent);
  pointer-events: none;
}

/* Trap frame readout. Sits between the "following" line and the hex dump
 * when the followed core is in PL0, so the eye reads top-to-bottom:
 *   "following C0:ESP @ 0x..."
 *   "TRAP FRAME pushed by HW"          ← new strip
 *   [hex dump showing those same bytes]
 * The amber accent reuses the kernel-mode hue from the Event Log so the
 * three panels speak the same visual language. */
.trap-frame {
  margin: 4px 0 6px;
  padding: 6px 8px;
  border: 1px solid var(--amber);
  border-radius: 3px;
  background: rgba(var(--accent-rgb), 0.06);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}
.trap-frame__title {
  font-size: 9px;
  letter-spacing: 0.5px;
  font-weight: 700;
  color: var(--amber);
  margin-bottom: 4px;
  text-transform: uppercase;
}
.trap-frame__row {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.trap-frame__off {
  color: var(--text-muted);
  font-size: 10px;
  min-width: 56px;
}
.trap-frame__label {
  color: var(--text);
  min-width: 110px;
}
.trap-frame__val {
  color: var(--amber);
  font-variant-numeric: tabular-nums;
}

.core-selector {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 2px 4px 2px 6px;
  background: var(--surface-overlay);
}

.core-selector__label {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
  margin-right: 2px;
}

.core-selector__btn {
  background: transparent;
  border: 1px solid var(--core-color, var(--border));
  border-radius: var(--radius-sm);
  color: var(--core-color, var(--text-dim));
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  padding: 1px 6px;
  cursor: pointer;
  min-height: unset;
  letter-spacing: 0.5px;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}

.core-selector__btn:hover {
  background: color-mix(in oklab, var(--core-color) 18%, transparent);
  color: var(--core-color);
}

.core-selector__btn.active {
  background: var(--core-color);
  color: var(--core-fg, var(--surface-base));
  border-color: var(--core-color);
  box-shadow: 0 0 6px var(--core-color);
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

.following-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  padding: 4px 6px;
  margin-bottom: 6px;
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
  background: color-mix(in oklab, var(--surface-overlay) 60%, transparent);
}

.following-line__label {
  font-size: 9px;
  letter-spacing: 1.5px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.following-line__core {
  border: 1px solid;
  padding: 0 6px;
  font-weight: 700;
}

.following-line__addr {
  color: var(--amber);
}

.following-line__pid {
  color: var(--text);
}

.following-line__region {
  margin-left: auto;
  font-size: 9.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.following-line--empty {
  color: var(--text-muted);
  font-style: italic;
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

.memory-card__label {
  font-weight: 700;
  border: 1px solid var(--amber);
  color: var(--amber);
  padding: 1px 6px;
  font-size: 11px;
}

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

.hex-dump {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.4;
  margin: 0;
  padding: 4px 10px 8px;
}

.hex-line {
  display: flex;
  gap: 12px;
  padding: 1px 0;
  align-items: center;
}

.hex-addr {
  color: var(--cyan);
  flex-shrink: 0;
}

.hex-bytes {
  display: flex;
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
  background: transparent;
  font-size: 11px;
  cursor: pointer;
  min-height: unset;
  letter-spacing: 0;
  transition: background var(--transition-fast), box-shadow var(--transition-fast);
}

.leg:hover {
  background: color-mix(in oklab, currentColor 14%, transparent);
  box-shadow: 0 0 6px color-mix(in oklab, currentColor 50%, transparent);
}

.leg:focus-visible {
  outline: 1px solid currentColor;
  outline-offset: 1px;
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

  .core-selector__btn {
    padding: 6px 10px;
    font-size: 12px;
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
