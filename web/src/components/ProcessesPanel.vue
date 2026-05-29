<script setup lang="ts">
import { useCpuStore, type PCBInfo } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Maximize2, Minimize2, Eye, EyeOff, ChevronDown, ChevronRight } from '@lucide/vue';
import CollapsiblePanel from './ui/CollapsiblePanel.vue';
import { useProcessTree } from '../composables/useProcessTree';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

function hex(n: number, w = 6) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function stateClass(state: string) {
  return `state-${state.toLowerCase()}`;
}

// Mirror of pkg/cpu/cpu.go FlagCF/ZF/SF/IF/OF bit positions. Anything outside
// this set is irrelevant for the chip strip; if the backend adds new flags
// the strip degrades silently (no chip rendered) instead of breaking layout.
type FlagName = 'CF' | 'ZF' | 'SF' | 'IF' | 'OF';
const EFLAGS_BITS: Array<{ name: FlagName; mask: number }> = [
  { name: 'CF', mask: 1 << 0  },
  { name: 'ZF', mask: 1 << 6  },
  { name: 'SF', mask: 1 << 7  },
  { name: 'IF', mask: 1 << 9  },
  { name: 'OF', mask: 1 << 11 },
];

// Lista ordenada por PID para que las tarjetas no salten. El PID es secuencial
// al crear cada proceso, así que esto equivale al orden de carga del kernel.
const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

// Depth cap keeps the indent inside the narrow col-right column; deeper
// subtrees still render but stop accumulating horizontal offset and surface
// a ↳ dN chip via realDepth.
const TREE_DEPTH_CAP = 4;
const { treeNodes, initPID } = useProcessTree({ depthCap: TREE_DEPTH_CAP });

const EXPANDED_KEY = 'modos.procs.expandedPids';

interface LoadResult {
  set: Set<number>;
  hadEntry: boolean;
}

function loadExpanded(): LoadResult {
  if (typeof window === 'undefined') return { set: new Set(), hadEntry: false };
  const raw = window.localStorage.getItem(EXPANDED_KEY);
  if (raw === null) return { set: new Set(), hadEntry: false };
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return {
        set: new Set(parsed.filter((x: unknown): x is number => typeof x === 'number')),
        hadEntry: true,
      };
    }
  } catch { /* corrupt entry */ }
  return { set: new Set(), hadEntry: true };
}

function persistExpanded() {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(EXPANDED_KEY, JSON.stringify([...expandedPids.value]));
  } catch { /* quota / private mode → keep in memory only */ }
}

const _loaded = loadExpanded();
const expandedPids = ref<Set<number>>(_loaded.set);
// On first visit (no localStorage entry yet), auto-expand all PCBs once they
// appear. Subsequent visits respect whatever the user has toggled.
let firstSessionAutoExpand = !_loaded.hadEntry;
watch(
  () => sortedProcs.value.map(p => p.pid),
  (pids) => {
    if (firstSessionAutoExpand && pids.length > 0) {
      expandedPids.value = new Set(pids);
      persistExpanded();
      firstSessionAutoExpand = false;
    }
  },
  { immediate: true },
);

function isExpanded(pid: number): boolean {
  return expandedPids.value.has(pid);
}

function toggleExpanded(p: PCBInfo) {
  const next = new Set(expandedPids.value);
  if (next.has(p.pid)) {
    next.delete(p.pid);
  } else {
    next.add(p.pid);
  }
  expandedPids.value = next;
  persistExpanded();
}

function expandAll() {
  expandedPids.value = new Set(sortedProcs.value.map(p => p.pid));
  persistExpanded();
}

function collapseAll() {
  expandedPids.value = new Set();
  persistExpanded();
}

const allExpanded = computed(() =>
  sortedProcs.value.length > 0 && sortedProcs.value.every(p => expandedPids.value.has(p.pid)),
);


// PID → core that is currently executing it (if any). Used to surface the
// C<id> chip on RUN cards and to pick the live register source.
function coreOfPid(pid: number): number | null {
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    if (c.current_pid === pid) return c.id;
  }
  return null;
}

// PCB-visible register file: always the kernel-saved snapshot from the last
// context switch. While the process is RUN the live regs tick in the assigned
// core (see Cores panel); these numbers stay frozen until the scheduler emits
// the next `pcb_save`, which mirrors the real "context save on dispatch out"
// mechanic of the OS.
function pcbRegs(p: PCBInfo): Record<string, number> {
  return p.regs;
}

function pcbEIP(p: PCBInfo): number {
  return (pcbRegs(p).EIP ?? 0) >>> 0;
}

function regsList(regs: Record<string, number>): Array<{ name: string; val: number }> {
  const order = ['EAX', 'EBX', 'ECX', 'EDX', 'ESP', 'EBP', 'EIP'];
  return order.filter(k => k in regs).map(k => ({ name: k, val: regs[k] }));
}

function flagOn(eflags: number, mask: number): boolean {
  return (eflags & mask) !== 0;
}

function regsSource(p: PCBInfo): string {
  return coreOfPid(p.pid) !== null
    ? t('panels.processes.regsSrcPreDispatch')
    : t('panels.processes.regsSrcSaved');
}

function stateTitle(p: PCBInfo): string {
  switch (p.state) {
    case 'RUN': {
      const core = coreOfPid(p.pid);
      return core === null
        ? t('panels.processes.states.runUnknownCore')
        : t('panels.processes.states.run', { core });
    }
    case 'READY':   return t('panels.processes.states.ready');
    case 'BLOCKED': return p.blocked_on
      ? t('panels.processes.states.blockedOn', { reason: p.blocked_on })
      : t('panels.processes.states.blocked');
    case 'NEW':     return t('panels.processes.states.new');
    case 'ZOMBIE':  return t('panels.processes.states.zombie');
    default:        return p.state;
  }
}

function flagHelp(name: FlagName): string {
  return t(`panels.processes.flagHelp.${name}`);
}

function flagChipTitle(name: FlagName, on: boolean): string {
  return on
    ? t('panels.processes.flagChipOn', { name, help: flagHelp(name) })
    : t('panels.processes.flagChipOff', { name, help: flagHelp(name) });
}

function regCellTitle(reg: string, p: PCBInfo): string {
  const cell = coreOfPid(p.pid) !== null
    ? t('panels.processes.regCellPreDispatch', { reg })
    : t('panels.processes.regCellSaved', { reg });
  return t('panels.processes.regCellTitle', { cell, pid: p.pid });
}

function regsSrcTitle(p: PCBInfo): string {
  const desc = coreOfPid(p.pid) !== null
    ? t('panels.processes.regsHintPreDispatch')
    : t('panels.processes.regsHintSaved');
  return t('panels.processes.regsValuesAre', { desc });
}
</script>

<template>
  <CollapsiblePanel
    class="panel procs"
    data-pizarra-anchor="procs-panel"
    :title="t('panels.processes.title')"
    :title-hint="t('panels.processes.titleHint')"
    glossary-key="PCB"
    :collapsed="prefs.panelsCollapsed.processes"
    @update:collapsed="prefs.togglePanel('processes', $event)"
  >
    <template #header-actions>
      <button
        v-if="sortedProcs.length > 0"
        type="button"
        class="proc-toolbar__btn proc-toolbar__btn--expand"
        :title="allExpanded ? t('panels.processes.collapseHint') : t('panels.processes.expandHint')"
        :aria-label="allExpanded ? t('panels.processes.collapseAll') : t('panels.processes.expandAll')"
        :aria-expanded="allExpanded"
        @click.stop="allExpanded ? collapseAll() : expandAll()"
      >
        <component
          :is="allExpanded ? Minimize2 : Maximize2"
          :size="13"
          :stroke-width="2"
        />
        <span class="proc-toolbar__btn-label">
          {{ allExpanded ? t('panels.processes.collapseAll') : t('panels.processes.expandAll') }}
        </span>
      </button>
      <button
        v-if="initPID > 0"
        type="button"
        class="proc-toolbar__btn"
        :title="prefs.showInit ? t('panels.processes.hideInit') : t('panels.processes.showInit')"
        :aria-label="prefs.showInit ? t('panels.processes.hideInit') : t('panels.processes.showInit')"
        :aria-pressed="prefs.showInit"
        @click.stop="prefs.setShowInit(!prefs.showInit)"
      >
        <Eye v-if="prefs.showInit" :size="14" :stroke-width="2" />
        <EyeOff v-else :size="14" :stroke-width="2" />
      </button>
    </template>

    <div v-if="sortedProcs.length === 0" class="empty">
      {{ t('panels.processes.emptyDemo') }}
    </div>

    <div v-if="treeNodes.length > 0" class="proc-list">
      <section
        v-for="node in treeNodes"
        :key="node.proc.pid"
        class="proc-card"
        :class="[
          stateClass(node.proc.state),
          {
            'proc-card--running': coreOfPid(node.proc.pid) !== null,
            'proc-card--expanded': isExpanded(node.proc.pid),
            'proc-card--flash': cpu.flashingPIDs.has(node.proc.pid),
            'proc-card--child': node.depth > 0,
          },
        ]"
        :style="node.depth > 0 ? { marginLeft: (node.depth * 14) + 'px', borderLeft: '1px solid var(--border)' } : undefined"
        :data-narrator-anchor="`pcb-${node.proc.pid}`"
      >
        <header
          class="proc-card__head"
          role="button"
          tabindex="0"
          :aria-expanded="isExpanded(node.proc.pid)"
          :title="stateTitle(node.proc)"
          @click="toggleExpanded(node.proc)"
          @keydown.enter.prevent="toggleExpanded(node.proc)"
          @keydown.space.prevent="toggleExpanded(node.proc)"
        >
          <component
            :is="isExpanded(node.proc.pid) ? ChevronDown : ChevronRight"
            :size="12"
            :stroke-width="2"
            class="proc-card__chevron"
          />
          <span class="proc-card__pid">{{ node.isThread ? 'T' : 'P' }}{{ node.proc.pid }}</span>
          <span class="proc-card__name" :title="t('panels.processes.nameTooltip', { name: node.proc.name, eip: hex(pcbEIP(node.proc), 8) })">{{ node.proc.name }}</span>
          <span class="proc-card__head-spacer" />
          <span
            v-if="coreOfPid(node.proc.pid) !== null"
            class="proc-card__core"
            :style="{ color: `var(--core-${(coreOfPid(node.proc.pid) as number) % 4})` }"
            :title="t('panels.processes.coreRunning', { core: coreOfPid(node.proc.pid) ?? 0 })"
          >C{{ coreOfPid(node.proc.pid) }}</span>
          <span class="proc-card__state" :class="stateClass(node.proc.state)">{{ node.proc.state }}</span>
          <span
            v-if="node.isThread"
            class="proc-card__thread-chip"
            :title="t('panels.processes.threadChipTitle', { tgid: node.proc.tgid })"
          >TGID {{ node.proc.tgid }}</span>
          <span
            v-else-if="(node.proc.parent_pid ?? 0) > 0"
            class="proc-card__parent-chip"
            :title="t('panels.processes.parentChipTitle', { pid: node.proc.parent_pid })"
          >↑P{{ node.proc.parent_pid }}</span>
          <span
            v-if="node.realDepth > TREE_DEPTH_CAP"
            class="proc-card__depth-chip"
            :title="t('panels.processes.depthChipTitle', { depth: node.realDepth, cap: TREE_DEPTH_CAP })"
          >↳ d{{ node.realDepth }}</span>
        </header>

        <div v-if="isExpanded(node.proc.pid)" class="proc-card__body">
          <div class="proc-card__meta">
            <span class="proc-card__addr" :title="t('panels.processes.addrHint')">
              @ {{ hex(node.proc.code_base, 8) }}
            </span>
            <span class="proc-card__regs-src" :title="regsSrcTitle(node.proc)">
              {{ regsSource(node.proc) }}
            </span>
            <span
              v-if="node.proc.state === 'ZOMBIE' && node.proc.exit_code !== undefined"
              class="proc-card__exit"
              :title="t('panels.processes.exitCodeTitle')"
            >exit={{ node.proc.exit_code }}</span>
          </div>

          <div v-if="node.proc.state === 'BLOCKED' && node.proc.blocked_on" class="proc-card__blocked">
            {{ t('panels.processes.blockedWaiting', { reason: node.proc.blocked_on }) }}
          </div>

          <div class="proc-card__regs">
            <span
              v-for="r in regsList(pcbRegs(node.proc))"
              :key="r.name"
              :title="regCellTitle(r.name, node.proc)"
            >
              <span class="r-name">{{ r.name }}</span>
              <span class="r-val">{{ hex(r.val, 8) }}</span>
            </span>
          </div>

          <div class="proc-card__flags">
            <span
              class="proc-card__flags-label clickable-label"
              :title="t('panels.processes.eflagsHint')"
              @click="cpu.openGlossary('EFLAGS')"
            >{{ t('panels.processes.eflagsTitle') }}</span>
            <span
              v-for="f in EFLAGS_BITS"
              :key="f.name"
              class="pcb-flag"
              :class="{ 'pcb-flag--on': flagOn(pcbRegs(node.proc).EFLAGS ?? 0, f.mask) }"
              :title="flagChipTitle(f.name, flagOn(pcbRegs(node.proc).EFLAGS ?? 0, f.mask))"
            >{{ f.name }}</span>
          </div>
        </div>
      </section>
    </div>

  </CollapsiblePanel>
</template>

<style scoped>
.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 16px 0;
  text-align: center;
}

.clickable-label {
  cursor: pointer;
  transition: color 0.15s ease;
}

.clickable-label:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

/* Container query scope: the panel root advertises its inline size so the
   "Expand all" button in the header can drop its label when the column is
   too narrow to fit "PROCESSES" + the action chips comfortably. */
.procs {
  container-type: inline-size;
}

.proc-toolbar__btn {
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

.proc-toolbar__btn :deep(svg) {
  transition: opacity 0.2s ease;
  flex-shrink: 0;
}

.proc-toolbar__btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.proc-toolbar__btn:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}

.proc-toolbar__btn--expand {
  width: auto;
  padding: 0 7px;
  gap: 5px;
  font-family: inherit;
  font-size: 8px;
  letter-spacing: 0.5px;
  text-transform: uppercase;
}

.proc-toolbar__btn-label {
  line-height: 1;
  white-space: nowrap;
}

@container (max-width: 280px) {
  .proc-toolbar__btn--expand {
    width: 22px;
    padding: 0;
    gap: 0;
  }
  .proc-toolbar__btn-label {
    display: none;
  }
}

.proc-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}

.proc-card {
  border: 1px solid var(--border);
  border-left-width: 3px;
  border-left-color: var(--border);
  background: var(--surface-overlay);
  transition: background 0.15s ease;
}

.proc-card.state-run     { border-left-color: var(--green); }
.proc-card.state-ready   { border-left-color: var(--yellow); }
.proc-card.state-blocked { border-left-color: var(--red-soft); }
.proc-card.state-new     { border-left-color: var(--cyan); }
.proc-card.state-zombie  { border-left-color: var(--text-muted); }

.proc-card--running {
  background: color-mix(in oklab, var(--green) 8%, transparent);
}

.proc-card--flash {
  background: rgba(var(--accent-rgb), 0.18);
  animation: flash-amber 800ms ease-out;
}

@keyframes flash-amber {
  0%   { background: rgba(var(--accent-rgb), 0.30); }
  100% { background: rgba(var(--accent-rgb), 0.04); }
}

.proc-card__head {
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

.proc-card__head:hover {
  background: rgba(var(--accent-rgb), 0.06);
}

.proc-card__head:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.proc-card__chevron {
  width: 14px;
  color: var(--amber);
  flex-shrink: 0;
  font-size: 14px;
  line-height: 1;
  text-align: center;
}

/* In-header tags are plain coloured text — no boxes, no backgrounds. The
   exception is RUN: the only state that demands "CPU is on this PCB right
   now" priority, so it gets a solid green chip that the eye lands on first. */
.proc-card__state {
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}

.proc-card__state.state-run {
  background: var(--green);
  color: var(--bg-0);
  padding: 1px 5px;
  border-radius: 2px;
}
.proc-card__state.state-ready   { color: var(--yellow); }
.proc-card__state.state-blocked { color: var(--red-soft); }
.proc-card__state.state-new     { color: var(--cyan); }
.proc-card__state.state-zombie  { color: var(--text-muted); }

.proc-card__core {
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
  flex-shrink: 0;
}

.proc-card__pid {
  color: var(--amber);
  font-weight: 700;
  font-size: 11px;
  flex-shrink: 0;
}

.proc-card__name {
  color: var(--text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.proc-card__head-spacer {
  flex: 1 1 auto;
  min-width: 0;
}

/* Tree-view chips: parent pointer and depth-cap badge ride at the end of
   the card header. Tight typography so two of them still fit in the
   narrow col-right column on desktop. */
.proc-card__parent-chip,
.proc-card__depth-chip,
.proc-card__thread-chip {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-dim);
  padding: 0 4px;
  border: 1px solid var(--border);
  border-radius: 2px;
  flex-shrink: 0;
}

.proc-card__depth-chip {
  color: var(--amber);
  border-color: var(--amber);
}

.proc-card__thread-chip {
  color: var(--cyan);
  border-color: var(--cyan);
}

.proc-card__exit {
  font-size: 10px;
  color: var(--text-muted);
  letter-spacing: 1px;
}

/* Zombies should read as "ya no están corriendo": faded card, no flash
   border, and the state chip stays muted. */
.proc-card.state-zombie {
  opacity: 0.6;
}
.proc-card.state-zombie .proc-card__name {
  text-decoration: line-through;
  text-decoration-color: var(--text-muted);
}

/* Child cards in the tree get a subtle border-left guideline. The
   indent is applied inline via :style so we can cap it at depth 4. */
.proc-card--child {
  border-left-color: var(--accent) !important;
  border-left-style: dotted !important;
}

.proc-card__body {
  padding: 6px 10px 8px;
  border-top: 1px dashed var(--border);
  background: rgba(var(--accent-rgb), 0.03);
}

.proc-card__meta {
  display: flex;
  gap: 10px;
  font-size: 10px;
  color: var(--text-muted);
  margin-bottom: 6px;
  flex-wrap: wrap;
  align-items: baseline;
}

.proc-card__addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}

.proc-card__regs-src {
  margin-left: auto;
  font-style: italic;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--text-muted);
}

.proc-card__blocked {
  font-size: 11px;
  color: var(--red-soft);
  margin-bottom: 6px;
  font-style: italic;
}

.proc-card__regs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 3px 12px;
  font-size: 12px;
}

.proc-card__regs > span {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  min-width: 0;
  cursor: help;
}
.proc-card__regs .r-val {
  flex-shrink: 0;
}

.r-name {
  color: var(--cyan);
}

.r-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}

.proc-card__flags-label {
  color: var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.proc-card__flags {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  flex-wrap: wrap;
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
</style>
