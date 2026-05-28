<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';
import ModeFlipNarrator from '../../components/narrators/ModeFlipNarrator.vue';
import InterruptNarrator from '../../components/narrators/InterruptNarrator.vue';
import Term from '../../components/glossary/Term.vue';

const { t } = useI18n();
const cpu = useCpuStore();
const prefs = usePreferencesStore();

// Mode label + colour hook for the big current-mode chip.
const modeLabel = computed(() => (cpu.mode === 3 ? 'USER' : 'KERNEL'));
const modeKey = computed<'user' | 'kernel'>(() => (cpu.mode === 3 ? 'user' : 'kernel'));

// IDT entries. We hard-code the well-known vectors the simulator implements
// so the table is meaningful even before a program runs. Each entry remembers
// the timestamp of its last activation so the row pulses briefly.
type IdtNameKey = 'divide' | 'gp' | 'pageFault' | 'irqTimer' | 'irqKeyboard' | 'syscall' | 'mutexSem';

interface IDTRow {
  vec: number;
  hex: string;
  nameKey: IdtNameKey;
  source: 'HW' | 'SW' | 'fault';
  lastFired: number;
}

const idt = reactive<IDTRow[]>([
  { vec: 0x00, hex: '0x00', nameKey: 'divide',      source: 'fault', lastFired: 0 },
  { vec: 0x0d, hex: '0x0D', nameKey: 'gp',          source: 'fault', lastFired: 0 },
  { vec: 0x0e, hex: '0x0E', nameKey: 'pageFault',   source: 'fault', lastFired: 0 },
  { vec: 0x20, hex: '0x20', nameKey: 'irqTimer',    source: 'HW',    lastFired: 0 },
  { vec: 0x21, hex: '0x21', nameKey: 'irqKeyboard', source: 'HW',    lastFired: 0 },
  { vec: 0x80, hex: '0x80', nameKey: 'syscall',     source: 'SW',    lastFired: 0 },
  { vec: 0x81, hex: '0x81', nameKey: 'mutexSem',    source: 'SW',    lastFired: 0 },
]);

function idtName(r: IDTRow): string {
  return t(`sections.interrupts.idtNames.${r.nameKey}`);
}

// Recent mode switches feed. We could parse the log but a structured ring
// buffer is cheaper and keeps the markup clean. coreId tracks which core
// produced the switch so multicore demos do not look like a single thread
// flipping back and forth.
interface ModeSwitchRow {
  id: number;
  coreId: number;
  from: 'USER' | 'KERNEL';
  to: 'USER' | 'KERNEL';
  cause: string;
  ts: number;
}

const switches = ref<ModeSwitchRow[]>([]);
let nextId = 1;
const MAX_SWITCHES = 12;

// Mode-switch feed honors the SimBar core filter so transitions from hidden
// cores stop polluting the timeline.
const visibleSwitches = computed(() => switches.value.filter(s => cpu.isCoreVisible(s.coreId)));

const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
  if (ev.type === 'mode_switch') {
    switches.value.unshift({
      id: nextId++,
      coreId: ev.core_id,
      from: ev.from === 3 ? 'USER' : 'KERNEL',
      to: ev.to === 3 ? 'USER' : 'KERNEL',
      cause: ev.cause,
      ts: Date.now(),
    });
    if (switches.value.length > MAX_SWITCHES) {
      switches.value = switches.value.slice(0, MAX_SWITCHES);
    }
  } else if (ev.type === 'irq_fire') {
    const row = idt.find(r => r.vec === ev.vector);
    if (row) row.lastFired = Date.now();
  } else if (ev.type === 'page_fault') {
    const row = idt.find(r => r.vec === 0x0e);
    if (row) row.lastFired = Date.now();
  } else if (ev.type === 'reset') {
    switches.value = [];
    for (const r of idt) r.lastFired = 0;
  }
});

onBeforeUnmount(unsubscribe);

// A row pulses for ~1.2s after firing. Force a redraw via a 100ms tick so the
// computed class flips off when the window expires.
const now = ref(Date.now());
const tickHandle = window.setInterval(() => { now.value = Date.now(); }, 200);
onBeforeUnmount(() => window.clearInterval(tickHandle));

function rowActive(r: IDTRow): boolean {
  return r.lastFired > 0 && now.value - r.lastFired < 1200;
}

function timeAgo(ts: number): string {
  const diff = Math.max(0, now.value - ts);
  if (diff < 1000) return t('sections.shared.timeAgoNow');
  if (diff < 60_000) return t('sections.shared.timeAgoSec', { n: Math.floor(diff / 1000) });
  return t('sections.shared.timeAgoMin', { n: Math.floor(diff / 60_000) });
}

function hex(n: number, width = 8): string {
  return '0x' + (n >>> 0).toString(16).toUpperCase().padStart(width, '0');
}

// Last handler entry for an IDT row, respecting the SimBar core filter: when
// the core that fired the vector is hidden the cell stays empty so the table
// stops reflecting invisible activity.
function handlerHitForRow(r: IDTRow) {
  const hit = cpu.lastFiredHandlerByVector[r.vec];
  if (!hit) return null;
  if (!cpu.isCoreVisible(hit.coreId)) return null;
  return hit;
}

// Cores currently shown in the SimBar filter. The stack-frame grid uses this
// to mount one card per visible core in a stable order.
const visibleCores = computed(() => cpu.cores.filter(c => cpu.isCoreVisible(c.id)));

function activeFrame(coreId: number) {
  return cpu.activeFrameByCore[coreId] ?? null;
}

const narratorHint = computed(() =>
  prefs.narratorsEnabled
    ? null
    : t('sections.interrupts.narratorHint'),
);
</script>

<template>
  <div class="ints-page" data-pizarra-anchor="view-interrupciones">
    <ModeFlipNarrator />
    <InterruptNarrator />

    <div class="ints-page__content">
      <header class="ints-page__hero">
        <div
          :class="['ints-page__mode-chip', `ints-page__mode-chip--${modeKey}`]"
          data-narrator-anchor="cpu"
        >
          <span class="ints-page__mode-caption">{{ t('sections.interrupts.cpuIn') }}</span>
          <Term
            :term="cpu.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'"
            mode="label"
            class="ints-page__mode-value"
          >{{ modeLabel }}</Term>
        </div>
        <div class="ints-page__hero-text">
          <h2 class="ints-page__title">
            <Term term="IRQ" mode="title">{{ t('sections.interrupts.title') }}</Term>
          </h2>
          <p class="ints-page__tagline">{{ t('sections.interrupts.tagline') }}</p>
          <p v-if="narratorHint" class="ints-page__hint">{{ narratorHint }}</p>
        </div>
      </header>

      <div class="ints-page__grid">
        <section class="ints-page__panel">
          <header class="ints-page__panel-head">
            <h3><Term term="IDT" mode="title">{{ t('sections.interrupts.idtTitle') }}</Term></h3>
            <span class="ints-page__panel-hint">{{ t('sections.interrupts.idtHint') }}</span>
          </header>
          <div class="ints-page__panel-body">
          <table class="ints-table">
            <thead>
              <tr>
                <th><Term term="INT N" mode="label">{{ t('sections.interrupts.colVec') }}</Term></th>
                <th>{{ t('sections.interrupts.colName') }}</th>
                <th><Term term="IRQ" mode="label">{{ t('sections.interrupts.colSource') }}</Term></th>
                <th>{{ t('sections.interrupts.colHandler') }}</th>
                <th>{{ t('sections.interrupts.colLast') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="r in idt"
                :key="r.vec"
                :class="['ints-table__row', { 'ints-table__row--active': rowActive(r) }]"
                :data-narrator-anchor="`idt-${r.vec}`"
              >
                <td class="ints-table__vec">
                  <Term :term="`IRQ ${r.hex}`" as="code">{{ r.hex }}</Term>
                </td>
                <td>{{ idtName(r) }}</td>
                <td>
                  <span :class="['ints-table__src', `ints-table__src--${r.source.toLowerCase()}`]">
                    {{ r.source }}
                  </span>
                </td>
                <td class="ints-table__handler">
                  <template v-if="handlerHitForRow(r)">
                    <code
                      class="ints-table__handler-addr"
                      :title="t('sections.interrupts.handlerTooltip', { core: handlerHitForRow(r)!.coreId, time: timeAgo(handlerHitForRow(r)!.ts) })"
                    >{{ hex(handlerHitForRow(r)!.handler) }}</code>
                    <span v-if="cpu.numCores > 1" class="ints-table__handler-core">C{{ handlerHitForRow(r)!.coreId }}</span>
                  </template>
                  <span v-else class="ints-table__time">—</span>
                </td>
                <td class="ints-table__time">
                  {{ r.lastFired ? timeAgo(r.lastFired) : '—' }}
                </td>
              </tr>
            </tbody>
          </table>
          </div>
        </section>

        <section class="ints-page__panel">
          <header class="ints-page__panel-head">
            <h3><Term term="Mode switch" mode="title">{{ t('sections.interrupts.transitionsTitle') }}</Term></h3>
            <span class="ints-page__panel-hint">{{ t('sections.interrupts.transitionsHint') }}</span>
          </header>
          <div class="ints-page__panel-body">
          <div v-if="visibleSwitches.length === 0" class="ints-feed__empty">
            {{ switches.length === 0
              ? t('sections.interrupts.transitionsEmpty')
              : t('sections.interrupts.transitionsEmptyVisible') }}
          </div>
          <ol v-else :class="['ints-feed', { 'ints-feed--multi': cpu.numCores > 1 }]">
            <li v-for="s in visibleSwitches" :key="s.id" class="ints-feed__row">
              <span v-if="cpu.numCores > 1" class="ints-feed__core" :title="`Disparado por el núcleo C${s.coreId}`">C{{ s.coreId }}</span>
              <Term
                :term="s.from === 'USER' ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'"
                mode="label"
                :class="['ints-feed__chip', `ints-feed__chip--${s.from.toLowerCase()}`]"
              >{{ s.from }}</Term>
              <span class="ints-feed__arrow">→</span>
              <Term
                :term="s.to === 'USER' ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'"
                mode="label"
                :class="['ints-feed__chip', `ints-feed__chip--${s.to.toLowerCase()}`]"
              >{{ s.to }}</Term>
              <span class="ints-feed__cause">{{ s.cause }}</span>
              <span class="ints-feed__time">{{ timeAgo(s.ts) }}</span>
            </li>
          </ol>
          </div>
        </section>
      </div>

      <section class="ints-page__panel ints-page__stacks">
        <header class="ints-page__panel-head">
          <h3><Term term="Interrupt frame" mode="title">{{ t('sections.interrupts.stackTitle') }}</Term></h3>
          <span class="ints-page__panel-hint">{{ t('sections.interrupts.stackHint') }}</span>
        </header>
        <div class="ints-stacks__grid">
          <article
            v-for="c in visibleCores"
            :key="`stack-${c.id}`"
            :class="['ints-stack', { 'ints-stack--active': activeFrame(c.id), 'ints-stack--idle': !activeFrame(c.id) }]"
            :data-irq-stack="`C${c.id}`"
          >
            <header class="ints-stack__head">
              <span class="ints-stack__core">C{{ c.id }}</span>
              <template v-if="activeFrame(c.id)">
                <span class="ints-stack__vec">{{ t('sections.interrupts.stackVector', { hex: hex(activeFrame(c.id)!.vector, 2) }) }}</span>
                <span class="ints-stack__handler" :title="t('sections.interrupts.stackHandlerHint')">{{ t('sections.interrupts.stackHandler', { hex: hex(activeFrame(c.id)!.handler) }) }}</span>
              </template>
              <span v-else class="ints-stack__idle-tag">{{ t('sections.interrupts.stackIdleTag') }}</span>
            </header>
            <ol v-if="activeFrame(c.id)" class="ints-stack__rows">
              <li class="ints-stack__row" data-stack-slot="eip">
                <span class="ints-stack__esp">ESP+0</span>
                <Term term="EIP" mode="label" class="ints-stack__name">EIP</Term>
                <code class="ints-stack__val">{{ hex(activeFrame(c.id)!.savedEIP, 8) }}</code>
              </li>
              <li class="ints-stack__row" data-stack-slot="cs">
                <span class="ints-stack__esp">ESP+4</span>
                <Term term="CS" mode="label" class="ints-stack__name">CS</Term>
                <code class="ints-stack__val">{{ hex(activeFrame(c.id)!.savedCS, 4) }}</code>
              </li>
              <li class="ints-stack__row" data-stack-slot="eflags">
                <span class="ints-stack__esp">ESP+8</span>
                <Term term="EFLAGS" mode="label" class="ints-stack__name">EFLAGS</Term>
                <code class="ints-stack__val">{{ hex(activeFrame(c.id)!.savedEFLAGS, 8) }}</code>
              </li>
            </ol>
            <div v-else class="ints-stack__empty">
              {{ t('sections.interrupts.stackEmptyPrefix') }}<Term :term="c.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'">{{ c.mode === 3 ? 'USER' : 'KERNEL' }}</Term>{{ t('sections.interrupts.stackEmptySuffix') }}
            </div>
          </article>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.ints-page {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ints-page__content {
  position: relative;
  z-index: 1;
  flex: 1;
  /* Page-level scroll disabled on purpose: the IDT/feed/stack panels each
     own their internal scroll so the layout never grows past the viewport
     and the user does not have to chase a frame that just appeared. */
  overflow: hidden;
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
}

.ints-page__hero {
  display: flex;
  align-items: center;
  gap: var(--space-5);
  flex-shrink: 0;
}

.ints-page__mode-chip {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-width: 160px;
  padding: var(--space-3) var(--space-4);
  border: 2px solid currentColor;
  border-radius: var(--radius-md);
  background: var(--surface-2);
  font-family: var(--font-display);
  letter-spacing: 4px;
  transition: color var(--transition-base), box-shadow var(--transition-base);
}

.ints-page__mode-chip--user {
  color: var(--cyan);
  box-shadow: 0 0 12px rgba(106, 212, 255, 0.25);
}

.ints-page__mode-chip--kernel {
  color: var(--amber);
  box-shadow: var(--shadow-glow-amber-strong);
}

.ints-page__mode-caption {
  font-size: 10px;
  letter-spacing: 3px;
  color: var(--text-dim);
}

.ints-page__mode-value {
  font-size: 22px;
  margin-top: var(--space-1);
  text-shadow: 0 0 10px currentColor;
}

.ints-page__hero-text {
  flex: 1;
  min-width: 0;
}

.ints-page__title {
  font-family: var(--font-display);
  font-size: 18px;
  letter-spacing: 5px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
  margin-bottom: var(--space-3);
}

.ints-page__tagline {
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
}

.ints-page__tagline code {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px;
  font-size: 11px;
  color: var(--amber);
}

.ints-page__hint {
  margin-top: var(--space-3);
  font-size: 11px;
  color: var(--text-dim);
  padding: var(--space-2) var(--space-3);
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
}

.ints-page__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
  /* Grid claims the leftover height so the stack panel sits at the true
     bottom edge without dancing as the feed grows. */
  flex: 1;
  min-height: 0;
}

@media (max-width: 768px) {
  .ints-page__hero { flex-direction: column; align-items: flex-start; }
  .ints-page__grid { grid-template-columns: 1fr; }
  .ints-feed__row { grid-template-columns: auto 1fr; row-gap: 2px; }
  .ints-feed--multi .ints-feed__row { grid-template-columns: auto 1fr; }
}

.ints-page__panel {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  overflow: hidden;
}

.ints-page__panel-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
  padding-bottom: var(--space-2);
  flex-shrink: 0;
}

.ints-page__panel-head h3 {
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 3px;
  color: var(--amber);
  font-weight: normal;
}

.ints-page__panel-hint {
  font-size: 10px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

/* Each panel's scrollable body. Without this the table and feed simply got
   clipped by .ints-page__panel's overflow:hidden, so longer-than-the-pane
   content (256-vector IDT, growing transitions feed) became unreachable. */
.ints-page__panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

.ints-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.ints-table thead th {
  text-align: left;
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-dim);
  text-transform: uppercase;
  padding: var(--space-2) var(--space-2);
  border-bottom: 1px dashed var(--border);
  font-weight: normal;
}

.ints-table__row {
  transition: background var(--transition-base), box-shadow var(--transition-base);
}

.ints-table__row td {
  padding: var(--space-2);
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.06);
}

.ints-table__row--active {
  background: rgba(var(--accent-rgb), 0.12);
  box-shadow: inset 2px 0 0 var(--amber);
  animation: ints-row-pulse 1.2s ease-out;
}

@keyframes ints-row-pulse {
  0%   { background: rgba(var(--accent-rgb), 0.32); }
  100% { background: rgba(var(--accent-rgb), 0.05); }
}

.ints-table__vec {
  font-family: 'JetBrains Mono', monospace;
  color: var(--amber);
}

.ints-table__src {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
}

.ints-table__src--hw    { color: var(--cyan); }
.ints-table__src--sw    { color: var(--amber); }
.ints-table__src--fault { color: #ff6b6b; }

.ints-table__time {
  color: var(--text-dim);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  text-align: right;
}

.ints-feed {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0;
  margin: 0;
  /* The feed scrolls inside its panel: as new transitions land, the page
     itself never grows past the viewport. */
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.ints-feed__empty {
  font-size: 12px;
  color: var(--text-dim);
  font-style: italic;
  padding: var(--space-3);
  flex: 1;
  min-height: 0;
}

.ints-feed__row {
  display: grid;
  grid-template-columns: auto auto auto 1fr auto;
  align-items: center;
  gap: var(--space-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 4px var(--space-2);
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.06);
}
.ints-feed--multi .ints-feed__row {
  grid-template-columns: auto auto auto auto 1fr auto;
}
.ints-feed__core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 1px 5px;
  white-space: nowrap;
}

.ints-feed__chip {
  padding: 1px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
}

.ints-feed__chip--user   { color: var(--cyan); }
.ints-feed__chip--kernel { color: var(--amber); }

.ints-feed__arrow { color: var(--text-dim); }

.ints-feed__cause {
  color: var(--text);
  font-size: 11px;
}

.ints-feed__time {
  color: var(--text-dim);
  font-size: 10px;
  text-align: right;
}

.ints-table__handler {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ints-table__handler-addr {
  color: var(--cyan);
  background: rgba(106, 212, 255, 0.06);
  border: 1px solid rgba(106, 212, 255, 0.25);
  border-radius: var(--radius-sm);
  padding: 1px 5px;
}

.ints-table__row--active .ints-table__handler-addr {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.12);
  border-color: var(--amber);
}

.ints-table__handler-core {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-muted);
  padding: 0 5px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

/* Stack-frame grid: one card per visible core. Cards reserve a fixed
   height so the layout never jitters between "idle" and "active" — the
   ESP+0/4/8 slots show up in place instead of stretching the page. */
.ints-page__stacks {
  flex-shrink: 0;
  margin-top: 0;
}

.ints-stacks__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-3);
}

.ints-stack {
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
  background: var(--surface-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  /* Calibrated for: head row (~20px) + 3 frame rows (~24px each) + padding.
     Keeps active/idle cards visually identical so the page never reflows. */
  min-height: 124px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.ints-stack--active {
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
  background: rgba(var(--accent-rgb), 0.04);
}

.ints-stack--idle {
  opacity: 0.6;
}

.ints-stack__head {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  flex-wrap: wrap;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.ints-stack__core {
  font-weight: 700;
  color: var(--amber);
  letter-spacing: 1px;
}

.ints-stack__vec {
  color: var(--text);
}

.ints-stack__handler {
  color: var(--cyan);
  font-size: 10px;
  margin-left: auto;
}

.ints-stack__idle-tag {
  color: var(--text-muted);
  font-style: italic;
  font-size: 11px;
}

.ints-stack__rows {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-family: 'JetBrains Mono', monospace;
}

.ints-stack__row {
  display: grid;
  grid-template-columns: 56px 64px 1fr;
  align-items: center;
  gap: var(--space-2);
  padding: 3px 6px;
  border-radius: var(--radius-sm);
  background: rgba(0, 0, 0, 0.25);
}

.ints-stack__empty {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
  flex: 1;
  display: flex;
  align-items: center;
}

.ints-stack__esp {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-muted);
}

.ints-stack__name {
  color: var(--cyan);
  font-size: 11px;
  letter-spacing: 1px;
}

.ints-stack__val {
  color: var(--amber);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

</style>
