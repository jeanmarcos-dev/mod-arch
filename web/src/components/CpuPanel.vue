<script setup lang="ts">
import { useCpuStore, REGISTERS, type CoreInfo, type PCBInfo } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Maximize2, Minimize2, ChevronDown, ChevronRight } from '@lucide/vue';
import CollapsiblePanel from './ui/CollapsiblePanel.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

function hex(n: number, w = 8) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function regHelp(reg: string): string {
  return `${t(`cpu.regHelp.${reg}` as const)}${t('cpu.glossaryHint')}`;
}
function instrHelp(): string {
  return t('cpu.instrHelp');
}
function plHelp(): string {
  return `${t('cpu.plHelp')}${t('cpu.glossaryHint')}`;
}

// Mirror of pkg/cpu/cpu.go FlagCF/ZF/SF/IF/OF; same chip strip used by
// ProcessesPanel so the visual vocabulary stays consistent.
type FlagName = 'CF' | 'ZF' | 'SF' | 'IF' | 'OF';
const EFLAGS_BITS: Array<{ name: FlagName; mask: number }> = [
  { name: 'CF', mask: 1 << 0  },
  { name: 'ZF', mask: 1 << 6  },
  { name: 'SF', mask: 1 << 7  },
  { name: 'IF', mask: 1 << 9  },
  { name: 'OF', mask: 1 << 11 },
];

function flagOn(eflags: number, mask: number): boolean {
  return (eflags & mask) !== 0;
}
function flagHelp(name: FlagName): string {
  return t(`panels.processes.flagHelp.${name}`);
}
function flagChipTitle(name: FlagName, on: boolean): string {
  return on
    ? t('panels.processes.flagChipOn', { name, help: flagHelp(name) })
    : t('panels.processes.flagChipOff', { name, help: flagHelp(name) });
}

// Data registers shown in the per-core grid. EFLAGS lives in its own chip
// strip further down (CF/ZF/SF/IF/OF), so we drop it here to avoid showing
// the same flags twice.
const DATA_REGISTERS = REGISTERS.filter(r => r !== 'EFLAGS');

const visibleCores = computed<CoreInfo[]>(() => {
  const filtered = cpu.cores.filter(c => cpu.isCoreVisible(c.id));
  if (filtered.length > 0) return filtered;
  return [{
    id: 0,
    regs: { ...cpu.regs },
    segments: { ...cpu.segments },
    mode: cpu.mode,
    halted: cpu.halted,
    tick: cpu.tick,
    current_pid: cpu.currentPID,
  }];
});

function shouldFlash(core: CoreInfo, reg: string): boolean {
  return cpu.changedRegsByCore[core.id]?.has(reg) ?? false;
}

// Per-core expanded state, persisted to localStorage. Empty set + missing
// localStorage entry means "first visit": auto-expand the focused core so
// the panel never opens blank.
const EXPANDED_KEY = 'modos.cores.expandedIds';

function loadExpanded(): Set<number> {
  if (typeof window === 'undefined') return new Set();
  try {
    const raw = window.localStorage.getItem(EXPANDED_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return new Set(parsed.filter((x: unknown): x is number => typeof x === 'number'));
      }
    }
  } catch { /* corrupt entry → fall back to empty */ }
  return new Set();
}

function persistExpanded() {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(EXPANDED_KEY, JSON.stringify([...expandedCores.value]));
  } catch { /* quota / private mode → keep in memory only */ }
}

const hasInteracted = ref<boolean>(
  typeof window !== 'undefined' && window.localStorage.getItem(EXPANDED_KEY) !== null,
);
const expandedCores = ref<Set<number>>(loadExpanded());

function isExpanded(id: number): boolean {
  if (expandedCores.value.has(id)) return true;
  if (hasInteracted.value) return false;
  // Default for fresh sessions: single core → expand it; multi-core →
  // expand only the focused one so the column is not a stack of headers.
  if (visibleCores.value.length === 1) return true;
  return id === cpu.selectedCoreId;
}

function toggleExpanded(id: number) {
  hasInteracted.value = true;
  const next = new Set(expandedCores.value);
  // Materialize the current default for cores we touch for the first time
  // so toggling does the obvious thing instead of acting on an empty set.
  if (!next.has(id) && isExpanded(id)) {
    for (const c of visibleCores.value) {
      if (isExpanded(c.id)) next.add(c.id);
    }
    next.delete(id);
  } else if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  expandedCores.value = next;
  persistExpanded();
}

function expandAll() {
  hasInteracted.value = true;
  expandedCores.value = new Set(visibleCores.value.map(c => c.id));
  persistExpanded();
}

function collapseAll() {
  hasInteracted.value = true;
  expandedCores.value = new Set();
  persistExpanded();
}

const allExpanded = computed(() =>
  visibleCores.value.length > 0 && visibleCores.value.every(c => isExpanded(c.id)),
);

type CoreStateKind = 'run' | 'idle' | 'idle-task' | 'halt';
function coreState(c: CoreInfo): CoreStateKind {
  // idle-task wins over halt: when the swapper is bound, halted=true is the
  // expected resting state between IRQs, not a terminal stop. The dedicated
  // badge tells the student the core is *intentionally* sleeping, not dead.
  if (c.running_idle) return 'idle-task';
  if (c.halted) return 'halt';
  if ((c.current_pid ?? 0) > 0) return 'run';
  return 'idle';
}
function stateClass(kind: CoreStateKind) {
  return `state-${kind}`;
}
function stateLabel(kind: CoreStateKind): string {
  switch (kind) {
    case 'run':       return t('panels.cores.stateRun');
    case 'idle':      return t('panels.cores.stateIdle');
    case 'idle-task': return t('panels.cores.stateIdleTask');
    case 'halt':      return t('panels.cores.stateHalt');
  }
}
function stateTitle(kind: CoreStateKind, coreId: number): string {
  switch (kind) {
    case 'run':       return t('panels.cores.stateRunTitle',      { core: coreId });
    case 'idle':      return t('panels.cores.stateIdleTitle',     { core: coreId });
    case 'idle-task': return t('panels.cores.stateIdleTaskTitle', { core: coreId });
    case 'halt':      return t('panels.cores.stateHaltTitle',     { core: coreId });
  }
}

function pcbOfCore(c: CoreInfo): PCBInfo | null {
  if (!c.current_pid) return null;
  return cpu.procs.find(p => p.pid === c.current_pid) ?? null;
}
function isThreadPCB(p: PCBInfo): boolean {
  const tgid = p.tgid ?? p.pid;
  return tgid !== p.pid;
}
function pcbChipTitle(c: CoreInfo, p: PCBInfo): string {
  return isThreadPCB(p)
    ? t('panels.cores.tidChipTitle', { tid: p.pid, tgid: p.tgid ?? p.pid, core: c.id })
    : t('panels.cores.pidChipTitle', { pid: p.pid, core: c.id });
}
</script>

<template>
  <CollapsiblePanel
    class="panel cpu"
    data-pizarra-anchor="cpu-panel"
    data-narrator-anchor="cpu"
    :title="t('panels.cores.title')"
    :collapsed="prefs.panelsCollapsed.cores"
    @update:collapsed="prefs.togglePanel('cores', $event)"
  >
    <template #header-actions>
      <button
        v-if="visibleCores.length > 0"
        type="button"
        class="cpu-toolbar__btn cpu-toolbar__btn--expand"
        :title="allExpanded ? t('panels.cores.collapseHint') : t('panels.cores.expandHint')"
        :aria-label="allExpanded ? t('panels.cores.collapseAll') : t('panels.cores.expandAll')"
        :aria-expanded="allExpanded"
        @click.stop="allExpanded ? collapseAll() : expandAll()"
      >
        <component
          :is="allExpanded ? Minimize2 : Maximize2"
          :size="13"
          :stroke-width="2"
        />
        <span class="cpu-toolbar__btn-label">
          {{ allExpanded ? t('panels.cores.collapseAll') : t('panels.cores.expandAll') }}
        </span>
      </button>
    </template>

    <div class="core-list">
      <section
        v-for="core in visibleCores"
        :key="`core-${core.id}`"
        :class="[
          'core-card',
          stateClass(coreState(core)),
          {
            'core-card--focus': core.id === cpu.selectedCoreId,
            'core-card--expanded': isExpanded(core.id),
          },
        ]"
        :data-narrator-anchor="`cpu-${core.id}`"
      >
        <header
          class="core-card__head"
          role="button"
          tabindex="0"
          :aria-expanded="isExpanded(core.id)"
          :title="t('panels.cores.cardTitle', { core: core.id })"
          @click="toggleExpanded(core.id)"
          @keydown.enter.prevent="toggleExpanded(core.id)"
          @keydown.space.prevent="toggleExpanded(core.id)"
        >
          <component
            :is="isExpanded(core.id) ? ChevronDown : ChevronRight"
            :size="12"
            :stroke-width="2"
            class="core-card__chevron"
          />
          <span
            class="core-card__id"
            :style="{ color: `var(--core-${core.id % 4})` }"
            :title="core.id === cpu.selectedCoreId ? t('panels.cores.focusBadgeTitle') : undefined"
          >C{{ core.id }}</span>
          <template v-if="pcbOfCore(core)">
            <span
              class="core-card__pid"
              :title="pcbChipTitle(core, pcbOfCore(core) as PCBInfo)"
            >{{ isThreadPCB(pcbOfCore(core) as PCBInfo) ? 'T' : 'P' }}{{ core.current_pid }}</span>
            <span
              v-if="isThreadPCB(pcbOfCore(core) as PCBInfo)"
              class="core-card__tgid-chip"
              :title="t('panels.cores.tgidChipTitle', { tgid: (pcbOfCore(core) as PCBInfo).tgid ?? (pcbOfCore(core) as PCBInfo).pid })"
            >TGID {{ (pcbOfCore(core) as PCBInfo).tgid }}</span>
          </template>
          <span class="core-card__head-spacer" />
          <span
            class="core-card__mode"
            :class="core.mode === 3 ? 'mode-user' : 'mode-kernel'"
            :title="core.mode === 3 ? t('panels.cores.modeUserTitle') : t('panels.cores.modeKernelTitle')"
          >{{ core.mode === 3 ? t('panels.cores.modeUser') : t('panels.cores.modeKernel') }}</span>
          <span
            class="core-card__state"
            :class="stateClass(coreState(core))"
            :title="stateTitle(coreState(core), core.id)"
          >{{ stateLabel(coreState(core)) }}</span>
        </header>

        <div v-if="isExpanded(core.id)" class="core-card__body">
          <div class="core-card__regs">
            <span
              v-for="r in DATA_REGISTERS"
              :key="r"
              :class="{ flash: shouldFlash(core, r) }"
              :title="regHelp(r)"
              @click="cpu.openGlossary(r)"
            >
              <span class="r-name">{{ r }}</span>
              <span class="r-val">{{ hex(core.regs[r] ?? 0) }}</span>
            </span>
          </div>

          <div class="core-card__instr" :title="instrHelp()">
            <span class="r-name">{{ t('panels.cores.instrLabel') }}</span>
            <span class="r-val instr-val">{{ cpu.lastMnemonic[core.id] ?? '—' }}</span>
          </div>

          <div class="core-card__segments">
            <span
              class="core-card__segments-label clickable-label"
              :title="t('panels.cores.segmentsHint')"
            >{{ t('panels.cores.segmentsLabel') }}</span>
            <span
              class="core-card__seg"
              :title="regHelp('CS')"
              @click="cpu.openGlossary('CS')"
            ><span class="seg-name">CS</span><span class="seg-val">{{ hex(core.segments.CS, 4) }}</span></span>
            <span
              class="core-card__seg"
              :title="regHelp('DS')"
              @click="cpu.openGlossary('DS')"
            ><span class="seg-name">DS</span><span class="seg-val">{{ hex(core.segments.DS, 4) }}</span></span>
            <span
              class="core-card__seg"
              :title="regHelp('SS')"
              @click="cpu.openGlossary('SS')"
            ><span class="seg-name">SS</span><span class="seg-val">{{ hex(core.segments.SS, 4) }}</span></span>
          </div>

          <div class="core-card__flags">
            <span
              class="core-card__flags-label clickable-label"
              :title="t('panels.processes.eflagsHint')"
              @click="cpu.openGlossary('EFLAGS')"
            >{{ t('panels.processes.eflagsTitle') }}</span>
            <span
              v-for="f in EFLAGS_BITS"
              :key="f.name"
              class="pcb-flag"
              :class="{ 'pcb-flag--on': flagOn(core.regs.EFLAGS ?? 0, f.mask) }"
              :title="flagChipTitle(f.name, flagOn(core.regs.EFLAGS ?? 0, f.mask))"
            >{{ f.name }}</span>
          </div>

          <div class="core-card__pl clickable" :title="plHelp()" @click="cpu.openGlossary('PL')">
            <span class="r-name">PL</span>
            <span class="r-val" :style="{ color: core.mode === 3 ? 'var(--green)' : 'var(--red-soft)' }">
              {{ core.mode }} ({{ core.mode === 3 ? t('cpu.plUser') : t('cpu.plKernel') }})
            </span>
          </div>
        </div>
      </section>
    </div>
  </CollapsiblePanel>
</template>

<style scoped>
.cpu {
  container-type: inline-size;
}

.cpu-toolbar__btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  box-sizing: border-box;
  width: 22px;
  height: 22px;
  min-height: 0;
  padding: 0;
  font-size: 0;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: color 0.18s ease, border-color 0.18s ease, background 0.18s ease;
}
.cpu-toolbar__btn :deep(svg) {
  transition: opacity 0.2s ease;
  flex-shrink: 0;
}
.cpu-toolbar__btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}
.cpu-toolbar__btn:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}
.cpu-toolbar__btn--expand {
  width: auto;
  padding: 0 7px;
  gap: 5px;
  font-family: inherit;
  font-size: 8px;
  letter-spacing: 0.5px;
  text-transform: uppercase;
}
.cpu-toolbar__btn-label {
  line-height: 1;
  white-space: nowrap;
}
@container (max-width: 280px) {
  .cpu-toolbar__btn--expand {
    width: 22px;
    padding: 0;
    gap: 0;
  }
  .cpu-toolbar__btn-label {
    display: none;
  }
}

.core-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}

.core-card {
  border: 1px solid var(--border);
  border-left-width: 3px;
  border-left-color: var(--border);
  background: var(--surface-overlay);
  transition: background 0.15s ease, border-color 0.15s ease;
  container-type: inline-size;
}

.core-card.state-run       { border-left-color: var(--green); }
.core-card.state-idle      { border-left-color: var(--text-muted); }
.core-card.state-idle-task { border-left-color: var(--cyan, #6cf); }
.core-card.state-halt      { border-left-color: var(--red-soft); }

.core-card--focus {
  box-shadow: inset 0 0 0 1px rgba(var(--accent-rgb), 0.25);
}

.core-card.state-halt {
  opacity: 0.85;
}

/* idle-task badge breathes slowly to suggest "asleep, not dead". The
 * pulse only animates the state chip background, not the whole card,
 * so the rest of the regs stay legible while the core HLTs. */
.core-card.state-idle-task {
  opacity: 0.92;
}
.core-card.state-idle-task .core-card__state {
  animation: idleTaskPulse 2.4s ease-in-out infinite;
}
@keyframes idleTaskPulse {
  0%, 100% { opacity: 0.55; }
  50%      { opacity: 1; }
}

.core-card__head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  padding: 5px 8px;
  cursor: pointer;
  user-select: none;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  min-width: 0;
}

.core-card__head:hover {
  background: rgba(var(--accent-rgb), 0.06);
}

.core-card__head:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.core-card__chevron {
  width: 14px;
  color: var(--amber);
  flex-shrink: 0;
  font-size: 14px;
  line-height: 1;
  text-align: center;
}

.core-card__state {
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}
.core-card__state.state-run       { color: var(--green); }
.core-card__state.state-idle      { color: var(--text-muted); }
.core-card__state.state-idle-task { color: var(--cyan, #6cf); }
.core-card__state.state-halt      { color: var(--red-soft); }

.core-card__id {
  font-size: 11px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
  color: var(--amber);
}
.core-card--focus .core-card__id {
  text-decoration: underline dotted currentColor;
  text-underline-offset: 2px;
}

.core-card__mode {
  font-size: 9px;
  letter-spacing: 1px;
  line-height: 1.6;
  padding: 1px 6px;
  border: 1px solid var(--border);
  border-radius: 2px;
  flex-shrink: 0;
}
.core-card__mode.mode-user {
  color: var(--green);
  border-color: var(--green);
}
.core-card__mode.mode-kernel {
  color: var(--red-soft);
  border-color: var(--red-soft);
}

.core-card__pid {
  color: var(--amber);
  font-weight: 700;
  font-size: 11px;
  flex-shrink: 0;
}

.core-card__head-spacer {
  flex: 1 1 auto;
  min-width: 0;
}

.core-card__tgid-chip {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--cyan);
  border: 1px solid var(--cyan);
  padding: 0 4px;
  border-radius: 2px;
  flex-shrink: 0;
}

.core-card__body {
  padding: 6px 10px 8px;
  border-top: 1px dashed var(--border);
  background: rgba(var(--accent-rgb), 0.03);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.core-card__regs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 3px 12px;
  font-size: 12px;
}
.core-card__regs > span {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  cursor: pointer;
  padding: 1px 2px;
  min-width: 0;
  transition: background 0.18s ease, outline-color 0.18s ease;
}
.core-card__regs .r-val {
  flex-shrink: 0;
}
.core-card__regs > span:hover {
  background: rgba(106, 212, 255, 0.1);
  outline: 1px dashed var(--cyan);
}

.r-name {
  color: var(--cyan);
  font-weight: 500;
}
.r-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}

.flash {
  animation: flash-amber 250ms ease-out;
}
@keyframes flash-amber {
  0%   { background: rgba(var(--accent-rgb), 0.30); }
  100% { background: rgba(var(--accent-rgb), 0.04); }
}

.core-card__instr {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  font-size: 12px;
  padding: 1px 2px;
  cursor: help;
}
.instr-val {
  color: var(--amber);
  font-size: 12px;
  text-align: right;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 70%;
}

.core-card__segments {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
.core-card__segments-label {
  color: var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
}
.core-card__seg {
  display: inline-flex;
  gap: 4px;
  padding: 0 4px;
  border: 1px solid var(--border);
  border-radius: 2px;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}
.core-card__seg:hover {
  border-color: var(--cyan);
  background: rgba(106, 212, 255, 0.08);
}
.seg-name {
  color: var(--cyan);
  font-weight: 500;
}
.seg-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}

.core-card__flags {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.core-card__flags-label {
  color: var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.pcb-flag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  padding: 1px 5px;
  border: 1px solid var(--border);
  color: var(--text-muted);
  background: transparent;
  cursor: help;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}
.pcb-flag--on {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.12);
  text-shadow: 0 0 4px rgba(var(--accent-rgb), 0.45);
}

.core-card__pl {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  font-size: 12px;
  padding: 1px 2px;
}
.core-card__pl.clickable {
  cursor: pointer;
}
.core-card__pl.clickable:hover {
  background: rgba(106, 212, 255, 0.1);
  outline: 1px dashed var(--cyan);
}

.clickable-label {
  cursor: pointer;
  transition: color 0.15s ease;
}
.clickable-label:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

@container (max-width: 230px) {
  .core-card__head {
    gap: 4px 5px;
    font-size: 11px;
    padding: 5px 6px;
  }
  .core-card__state,
  .core-card__mode,
  .core-card__tgid-chip {
    font-size: 9px;
    letter-spacing: 0.5px;
  }
  .core-card__mode {
    padding: 1px 4px;
  }
  .core-card__tgid-chip {
    padding: 0 3px;
  }
}

@media (max-width: 768px) {
  .core-card__regs {
    font-size: 13px;
  }
  .core-card__head {
    font-size: 13px;
  }
}

@media (max-width: 360px) {
  /* On a 320px viewport the auto-fit grid collapses to a single column and
     the long hex values would push the panel into horizontal scroll. Force
     two columns and let the value ellipsize — the full value remains in
     the row's title attribute already used for click-to-copy. */
  .core-card__regs {
    grid-template-columns: 1fr 1fr;
    gap: 2px 8px;
    font-size: 12px;
  }
  .core-card__regs > span {
    min-width: 0;
  }
  .core-card__regs .r-val {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .core-card__flags { row-gap: 4px; }
}
</style>
