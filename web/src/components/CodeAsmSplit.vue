<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MousePointerClick } from '@lucide/vue';
import { useCpuStore, type SourceLine, type PCBInfo } from '../stores/cpu';
import { useViewerMirrorStore } from '../stores/viewerMirror';

const cpu = useCpuStore();
const mirror = useViewerMirrorStore();
const { t } = useI18n();

function tabTitle(p: PCBInfo): string {
  const key = pinnedPID.value === p.pid
    ? 'panels.codeAsm.tabTitlePinned'
    : 'panels.codeAsm.tabTitleUnpinned';
  return t(key, { pid: p.pid, name: p.name, state: p.state });
}

// PID actually being displayed: follows the focused core's current_pid unless
// the user pins another tab. When a viewer is autofollowing, the mirror takes
// precedence so the audience sees the same pinned tab the host picked.
const localPinnedPID = ref<number | null>(null);
const isViewer = computed(() => cpu.sessionInfo.role === 'viewer');
const pinnedPID = computed<number | null>(() => {
  if (isViewer.value && mirror.pinnedPID !== undefined) {
    return mirror.pinnedPID === 0 ? null : mirror.pinnedPID;
  }
  return localPinnedPID.value;
});
const viewingPID = computed(() => pinnedPID.value ?? cpu.currentPID);

// Mirror-driven sync of the local copy so toggling autofollow off keeps the
// last pinned PID instead of reverting to the auto follow.
watch(() => mirror.pinnedPID, (val) => {
  if (isViewer.value && typeof val === 'number') {
    localPinnedPID.value = val === 0 ? null : val;
  }
});

const source = computed<string>(() => {
  if (!viewingPID.value) return '';
  return cpu.procSources[viewingPID.value] ?? '';
});

const sourceMap = computed<SourceLine[]>(() => {
  if (!viewingPID.value) return [];
  return cpu.procSourceMaps[viewingPID.value] ?? [];
});

// Split the C source by '\n' so we can annotate the active line. We keep the
// numbering 1-based to match SourceLine.c_line.
const sourceLines = computed<string[]>(() => source.value.split('\n'));

const currentProc = computed(() =>
  cpu.procs.find(p => p.pid === viewingPID.value),
);

const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

// PIDs that are RUN on a visible core right now. Used to mark "live" tabs.
const runningOnVisibleCores = computed(() => new Set(
  cpu.cores.filter(c => cpu.isCoreVisible(c.id) && c.current_pid > 0).map(c => c.current_pid),
));

// Inverse mapping: which core currently holds each PID. The tabs render a
// "C{n}" chip in the per-core tint so a student can see at a glance "P1 is
// running on C0, P2 is on C1" without cross-referencing the SimStatusRow.
// Returns undefined when the PID is not assigned to any core (e.g. it is
// READY/BLOCKED), so v-if filters it out cleanly.
const coreOfPID = computed<Record<number, number>>(() => {
  const m: Record<number, number> = {};
  for (const c of cpu.cores) {
    if (c.current_pid > 0) m[c.current_pid] = c.id;
  }
  return m;
});

const multicore = computed(() => cpu.cores.length > 1);

// The offset of the next instruction to execute for the viewing PID. We
// prefer the EIP of the core currently running this PID; if no core has it,
// we fall back to the PCB's saved registers (state captured at the last
// context switch out).
const currentOffset = computed<number | null>(() => {
  const pid = viewingPID.value;
  if (!pid) return null;
  const base = currentProc.value?.code_base ?? 0;
  const liveCore = cpu.cores.find(c => c.current_pid === pid);
  if (liveCore) {
    return (liveCore.regs.EIP ?? 0) - base;
  }
  const saved = currentProc.value?.regs.EIP ?? 0;
  return saved - base;
});

// Active SourceLine: the entry whose offset matches currentOffset exactly
// (instructions only emit on boundaries). If no exact match, we fall back to
// the largest entry with offset <= currentOffset so users still see SOMETHING
// highlighted while debugging an out-of-bounds EIP.
const activeSourceLine = computed<SourceLine | null>(() => {
  const off = currentOffset.value;
  const map = sourceMap.value;
  if (off === null || map.length === 0) return null;
  const exact = map.find(l => l.offset === off);
  if (exact) return exact;
  let best: SourceLine | null = null;
  for (const l of map) {
    if (l.offset > off) break;
    best = l;
  }
  return best;
});

const activeCLine = computed<number | null>(() => activeSourceLine.value?.c_line ?? null);
const activeOffset = computed<number | null>(() => activeSourceLine.value?.offset ?? null);

function isActiveLine(idx1: number): boolean {
  return activeCLine.value === idx1;
}

function isActiveAsm(offset: number): boolean {
  return activeOffset.value === offset;
}

function pickProc(pid: number) {
  const next = localPinnedPID.value === pid ? null : pid;
  localPinnedPID.value = next;
  if (!isViewer.value) {
    // Encode "unpinned" as 0 on the wire because Focus.PinnedPID is a *int
    // and 0 / no pinned PID are equivalent in practice.
    cpu.sendFocus({ pinned_pid: next ?? 0 });
  }
}

// Auto-scroll: every time the active C line or ASM offset changes, bring the
// matching rows into view inside their own scroll containers. We use
// `block: 'center'` so the highlighted row sits in the middle of the panel
// instead of jumping to the edges on each step.
const cListRef = ref<HTMLElement | null>(null);
const asmListRef = ref<HTMLElement | null>(null);

watch(activeCLine, async () => {
  await nextTick();
  cListRef.value?.querySelector('.line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

watch(activeOffset, async () => {
  await nextTick();
  asmListRef.value?.querySelector('.asm-line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

function fmtHex(n: number): string {
  return '0x' + (n >>> 0).toString(16).toUpperCase().padStart(4, '0');
}
</script>

<template>
  <div class="panel code-asm" data-pizarra-anchor="code-panel">
    <div class="panel-title">
      <span class="title-text">{{ t('panels.codeAsm.title') }}</span>
      <span class="title-hint" v-if="cpu.procs.length > 1">
        <MousePointerClick :size="12" :stroke-width="2" />
        {{ t('panels.codeAsm.titleHint') }}
      </span>
    </div>

    <div v-if="sortedProcs.length === 0" class="empty">
      {{ t('panels.codeAsm.empty') }}
    </div>

    <template v-else>
      <div class="proc-tabs">
        <button
          v-for="p in sortedProcs"
          :key="p.pid"
          class="proc-tab"
          :class="{
            current: runningOnVisibleCores.has(p.pid),
            pinned: pinnedPID === p.pid,
            viewing: p.pid === viewingPID,
          }"
          @click="pickProc(p.pid)"
          :title="tabTitle(p)"
        >
          <span class="pt-pid">P{{ p.pid }}</span>
          <span class="pt-name">{{ p.name }}</span>
          <!-- Multicore identity: small chip with the core's color so the
               student maps PID → core at a glance. Hidden in single-core
               sessions where the dot is enough. -->
          <span
            v-if="multicore && coreOfPID[p.pid] !== undefined"
            class="pt-core"
            :style="{ background: `var(--core-${coreOfPID[p.pid] % 4})`, color: `var(--core-fg)` }"
            :title="t('panels.codeAsm.coreRunning', { core: coreOfPID[p.pid] })"
          >C{{ coreOfPID[p.pid] }}</span>
          <span v-if="runningOnVisibleCores.has(p.pid)" class="pt-current">●</span>
          <span v-if="pinnedPID === p.pid" class="pt-pin">📌</span>
        </button>
      </div>

      <div class="code-meta" v-if="currentProc">
        <span class="meta-tag" :title="t('panels.codeAsm.stateTitle')">{{ currentProc.state }}</span>
        <span class="meta-info">
          {{ currentProc.name }} · {{ t('panels.codeAsm.loadedAt') }}
          <span class="addr" :title="t('panels.codeAsm.codeBaseAddrTitle')">0x{{ (currentProc.code_base || 0).toString(16).toUpperCase() }}</span>
          · {{ t('panels.codeAsm.stackTop') }}
          <span class="addr">0x{{ (currentProc.stack_top || 0).toString(16).toUpperCase() }}</span>
          <template v-if="activeOffset !== null">
            · {{ t('panels.codeAsm.executing') }}
            <span class="addr">{{ fmtHex(activeOffset) }}</span>
          </template>
        </span>
      </div>

      <div class="split" :class="{ 'split--no-asm': sourceMap.length === 0 }">
        <section class="pane pane--c">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.cTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.cLinesCount', { n: sourceLines.length }) }}</span>
          </header>
          <ol v-if="source" ref="cListRef" class="c-list">
            <li
              v-for="(line, idx) in sourceLines"
              :key="idx"
              :class="['line', { 'line--current': isActiveLine(idx + 1) }]"
            >
              <span class="line-no">{{ idx + 1 }}</span>
              <span class="line-text">{{ line || ' ' }}</span>
            </li>
          </ol>
          <div v-else class="pane-empty">{{ t('panels.codeAsm.emptyC') }}</div>
        </section>

        <section v-if="sourceMap.length > 0" class="pane pane--asm">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.asmTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.asmInstrCount', { n: sourceMap.length }) }}</span>
          </header>
          <ol ref="asmListRef" class="asm-list">
            <li
              v-for="(sl, i) in sourceMap"
              :key="`${sl.offset}-${i}`"
              :class="['asm-line', { 'asm-line--current': isActiveAsm(sl.offset) }]"
              :title="t('panels.codeAsm.asmLineTitle', { n: sl.c_line })"
            >
              <span class="asm-off">{{ fmtHex(sl.offset) }}</span>
              <span class="asm-mnem">{{ sl.mnemonic }}</span>
              <span class="asm-cline">L{{ sl.c_line }}</span>
            </li>
          </ol>
        </section>

        <section v-else class="pane pane--asm pane--asm-fallback">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.asmTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.noMappingSub') }}</span>
          </header>
          <div class="pane-empty">
            {{ t('panels.codeAsm.noMappingBody') }}
          </div>
        </section>
      </div>

      <div class="explain">
        <details>
          <summary>
            <span class="exp-q">{{ t('panels.codeAsm.explainSummary') }}</span>
          </summary>
          <p>
            {{ t('panels.codeAsm.explainP1Pre') }}<strong>{{ t('panels.codeAsm.explainP1Bold') }}</strong>{{ t('panels.codeAsm.explainP1Mid') }}<em>{{ t('panels.codeAsm.explainP1Em') }}</em>{{ t('panels.codeAsm.explainP1Post') }}
          </p>
          <p>
            {{ t('panels.codeAsm.explainP2') }}
          </p>
          <p>
            {{ t('panels.codeAsm.explainP3Pre') }}<strong>{{ t('panels.codeAsm.explainP3Bold') }}</strong>{{ t('panels.codeAsm.explainP3Post') }}
          </p>
        </details>
      </div>
    </template>
  </div>
</template>

<style scoped>
.code-asm {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 10px;
}

.title-text { color: var(--amber); }

.title-hint {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-dim);
  font-style: italic;
  text-transform: none;
  letter-spacing: 0;
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 24px;
  text-align: center;
  font-size: 13px;
}

.proc-tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.proc-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 11px;
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-dim);
  border-color: var(--border);
  cursor: pointer;
  background: var(--surface-overlay);
}
.proc-tab:hover { border-color: var(--text-dim); color: var(--text); }
.proc-tab.current { border-color: var(--green); color: var(--green); }
.proc-tab.viewing {
  background: rgba(var(--accent-rgb), 0.15);
  border-color: var(--amber);
  color: var(--amber);
}
.proc-tab.pinned { border-style: double; border-width: 3px; }
.pt-pid { font-weight: 700; }
.pt-current { color: var(--green); font-size: 9px; }
.pt-pin { font-size: 9px; }

/* Per-core ownership badge inside a proc tab. Background and fg are set
   inline via --core-${id}; this rule only owns geometry and typography so
   the chip lines up with the rest of the tab content. */
.pt-core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
}

.code-meta {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--border);
  font-size: 12px;
  flex-wrap: wrap;
}
.meta-tag {
  padding: 2px 8px;
  background: rgba(106, 212, 255, 0.08);
  color: var(--cyan);
  border: 1px solid var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
}
.meta-info { color: var(--text-dim); }
.addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}

/* Split layout: 2 equal columns when ASM is available, single column on
   fallback. Each pane scrolls independently so the highlighted row can be
   centered without dragging the other side. */
.split {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  overflow: hidden;
  min-height: 0;
}
.split--no-asm { grid-template-columns: 1fr; }
.split--no-asm .pane--asm-fallback { display: none; }

.pane {
  display: flex;
  flex-direction: column;
  background: var(--surface-overlay);
  border: 1px solid var(--border);
  min-height: 0;
}

.pane-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 4px 10px;
  border-bottom: 1px dashed var(--border);
  background: rgba(var(--accent-rgb), 0.04);
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
}
.pane-title { color: var(--amber); }
.pane-sub { color: var(--text-muted); font-size: 9px; letter-spacing: 1px; }

.pane-empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 18px 14px;
  font-size: 12px;
  text-align: center;
}

.c-list,
.asm-list {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  flex: 1;
  overflow-y: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.55;
  counter-reset: none;
}

.line,
.asm-line {
  display: grid;
  align-items: baseline;
  padding: 0 10px;
  white-space: pre;
  border-left: 2px solid transparent;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}

.line {
  grid-template-columns: 28px 1fr;
  gap: 10px;
  color: var(--text);
}
.line-no {
  color: var(--text-muted);
  font-size: 10px;
  text-align: right;
  user-select: none;
}
.line-text { white-space: pre-wrap; }

.line--current {
  background: rgba(var(--accent-rgb), 0.14);
  border-left-color: var(--amber);
  box-shadow: inset 6px 0 18px -10px rgba(var(--accent-rgb), 0.55);
}
.line--current .line-no { color: var(--amber); font-weight: 700; }
.line--current .line-text { color: var(--amber); }

.asm-line {
  grid-template-columns: 60px 1fr 32px;
  gap: 8px;
  color: var(--text-dim);
}
.asm-off { color: var(--cyan); font-size: 11px; }
.asm-mnem { color: var(--text); }
.asm-cline {
  color: var(--text-muted);
  font-size: 10px;
  text-align: right;
}

.asm-line--current {
  background: rgba(127, 255, 170, 0.10);
  border-left-color: var(--green);
  box-shadow: inset 6px 0 18px -10px rgba(127, 255, 170, 0.45);
}
.asm-line--current .asm-off { color: var(--green); font-weight: 700; }
.asm-line--current .asm-mnem { color: var(--green); font-weight: 700; }
.asm-line--current .asm-cline { color: var(--green); }

.explain { margin-top: 8px; }
.explain summary {
  cursor: pointer;
  color: var(--amber-dim);
  font-size: 12px;
  padding: 4px;
  user-select: none;
}
.explain summary:hover { color: var(--amber); }
.exp-q { font-style: italic; }
.explain[open] {
  padding: 8px;
  background: rgba(var(--accent-rgb), 0.04);
  border-left: 2px solid var(--amber);
}
.explain p {
  font-size: 12px;
  line-height: 1.55;
  color: var(--text-dim);
  margin: 6px 0;
}
.explain p:first-of-type { margin-top: 4px; }
.explain strong { color: var(--text); }
.explain em { color: var(--amber-dim); }

@media (max-width: 768px) {
  /* Narrow viewports: stack panes vertically so each one keeps a usable
     reading width. The C panel stays on top because the lesson starts there. */
  .split { grid-template-columns: 1fr; }
  .split .pane--asm { min-height: 260px; }
}
</style>
