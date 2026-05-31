<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MousePointerClick } from '@lucide/vue';
import { useCpuStore, type SourceLine, type PCBInfo } from '../stores/cpu';
import { useViewerMirrorStore } from '../stores/viewerMirror';

const cpu = useCpuStore();
const mirror = useViewerMirrorStore();
const { t } = useI18n();

// Each TGID renders as one tab. The leader owns the source/sourceMap; siblings
// contribute their EIP as additional cursors so race conditions are visible
// over the shared code instead of forcing the student to swap tabs.
interface TGIDGroup {
  leader: PCBInfo;
  members: PCBInfo[]; // includes the leader; sorted by pid
}

interface ThreadCursor {
  pid: number;
  tid: number;
  isLeader: boolean;
  state: string;
  coreId: number | null; // null when the thread is not currently RUN
  offset: number; // EIP - code_base, can be negative for not-yet-loaded threads
}

const localPinnedPID = ref<number | null>(null);
const isViewer = computed(() => cpu.sessionInfo.role === 'viewer');
const pinnedPID = computed<number | null>(() => {
  if (isViewer.value && mirror.pinnedPID !== undefined) {
    return mirror.pinnedPID === 0 ? null : mirror.pinnedPID;
  }
  return localPinnedPID.value;
});

// Mirror-driven sync of the local copy so toggling autofollow off keeps the
// last pinned PID instead of reverting to the auto follow.
watch(() => mirror.pinnedPID, (val) => {
  if (isViewer.value && typeof val === 'number') {
    localPinnedPID.value = val === 0 ? null : val;
  }
});

function tgidOf(p: PCBInfo): number {
  return p.tgid ?? p.pid;
}

// Kernel-side PCBs (swapper, init, kthreadd, per-core kernel/N) have no C
// source attached and would render empty tabs in a panel dedicated to user
// programs. Mirrors the "hide kernel machinery" filter in useProcessTree.
function isUserProc(p: PCBInfo): boolean {
  if (p.is_idle === true) return false;
  if (p.is_kthread === true) return false;
  if (p.name === 'init') return false;
  return true;
}

// Group every visible user PCB by TGID. The leader is the member whose
// pid == tgid (or is_leader true); when a snapshot arrives mid-clone and no
// leader is present yet, the smallest-pid member acts as the temporary
// leader so the tab still renders.
const tgidGroups = computed<TGIDGroup[]>(() => {
  const byTgid = new Map<number, PCBInfo[]>();
  for (const p of cpu.procs) {
    if (!isUserProc(p)) continue;
    const key = tgidOf(p);
    const bucket = byTgid.get(key);
    if (bucket) bucket.push(p);
    else byTgid.set(key, [p]);
  }
  const groups: TGIDGroup[] = [];
  for (const [tgid, members] of byTgid) {
    members.sort((a, b) => a.pid - b.pid);
    const leader = members.find(m => m.is_leader || m.pid === tgid) ?? members[0];
    groups.push({ leader, members });
  }
  groups.sort((a, b) => a.leader.pid - b.leader.pid);
  return groups;
});

// The TGID actually being displayed: follows the focused core's current_pid
// (resolved to its TGID) unless the user pins a leader. `pinnedPID` stores the
// leader's pid, so an explicit pin always maps to exactly one group. When the
// focused core is running a kernel-side PCB (filtered out of tgidGroups), we
// fall back to the first user group so the panel keeps showing user code
// instead of blanking on every HLT/idle transition.
const viewingTGID = computed<number | null>(() => {
  if (pinnedPID.value) {
    const group = tgidGroups.value.find(g => g.leader.pid === pinnedPID.value);
    if (group) return tgidOf(group.leader);
    return pinnedPID.value;
  }
  if (cpu.currentPID) {
    const proc = cpu.procs.find(p => p.pid === cpu.currentPID);
    const tgid = proc ? tgidOf(proc) : cpu.currentPID;
    if (tgidGroups.value.some(g => tgidOf(g.leader) === tgid)) return tgid;
  }
  return tgidGroups.value[0] ? tgidOf(tgidGroups.value[0].leader) : null;
});

const viewingGroup = computed<TGIDGroup | null>(() => {
  const tgid = viewingTGID.value;
  if (!tgid) return null;
  return tgidGroups.value.find(g => tgidOf(g.leader) === tgid) ?? null;
});

const source = computed<string>(() => {
  const tgid = viewingTGID.value;
  if (!tgid) return '';
  // Source lives on the leader's pid because that is the PID the backend
  // populated procSources with on `loaded`. Threads created later via clone()
  // share AddressSpace.CodeBase in the kernel but never get their own entry.
  return cpu.procSources[tgid] ?? '';
});

const sourceMap = computed<SourceLine[]>(() => {
  const tgid = viewingTGID.value;
  if (!tgid) return [];
  return cpu.procSourceMaps[tgid] ?? [];
});

const sourceLines = computed<string[]>(() => source.value.split('\n'));

const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

// PIDs that are RUN on a visible core right now. Used to mark "live" tabs and
// to colour the cursor chips with the owning core's tint.
const coreOfPID = computed<Record<number, number>>(() => {
  const m: Record<number, number> = {};
  for (const c of cpu.cores) {
    if (c.current_pid > 0) m[c.current_pid] = c.id;
  }
  return m;
});

const visibleCoreOfPID = computed<Record<number, number>>(() => {
  const m: Record<number, number> = {};
  for (const c of cpu.cores) {
    if (c.current_pid > 0 && cpu.isCoreVisible(c.id)) m[c.current_pid] = c.id;
  }
  return m;
});

const multicore = computed(() => cpu.cores.length > 1);

// Compute one cursor per group member. We prefer the live core's EIP when the
// thread is currently scheduled; otherwise we use the PCB's saved EIP. Offset
// is relative to the leader's code_base because source/sourceMap are indexed
// by offset, and all threads of the group share that base.
const threadCursors = computed<ThreadCursor[]>(() => {
  const group = viewingGroup.value;
  if (!group) return [];
  const base = group.leader.code_base ?? 0;
  const cursors: ThreadCursor[] = [];
  for (const m of group.members) {
    const coreId = visibleCoreOfPID.value[m.pid] ?? null;
    let eip: number;
    if (coreId !== null) {
      const live = cpu.cores.find(c => c.id === coreId);
      eip = live?.regs.EIP ?? 0;
    } else {
      eip = m.regs?.EIP ?? 0;
    }
    cursors.push({
      pid: m.pid,
      tid: m.tid ?? m.pid,
      isLeader: m.is_leader === true || m.pid === tgidOf(m),
      state: m.state,
      coreId,
      offset: eip - base,
    });
  }
  return cursors;
});

// Resolve each cursor to the C line / ASM offset it maps to. We accept any
// cursor whose offset falls inside the source map (exact match preferred,
// largest-prior offset as fallback) so out-of-bounds EIPs (e.g. a thread
// still executing the leader-only prologue) still highlight something.
function resolveCursor(c: ThreadCursor): { cLine: number | null; offset: number | null } {
  const map = sourceMap.value;
  if (map.length === 0) return { cLine: null, offset: null };
  const exact = map.find(l => l.offset === c.offset);
  if (exact) return { cLine: exact.c_line, offset: exact.offset };
  let best: SourceLine | null = null;
  for (const l of map) {
    if (l.offset > c.offset) break;
    best = l;
  }
  return best ? { cLine: best.c_line, offset: best.offset } : { cLine: null, offset: null };
}

interface ResolvedCursor extends ThreadCursor {
  cLine: number | null;
  asmOffset: number | null;
}

const resolvedCursors = computed<ResolvedCursor[]>(() =>
  threadCursors.value.map(c => {
    const r = resolveCursor(c);
    return { ...c, cLine: r.cLine, asmOffset: r.offset };
  }),
);

const cursorsByCLine = computed<Map<number, ResolvedCursor[]>>(() => {
  const m = new Map<number, ResolvedCursor[]>();
  for (const c of resolvedCursors.value) {
    if (c.cLine === null) continue;
    const bucket = m.get(c.cLine);
    if (bucket) bucket.push(c);
    else m.set(c.cLine, [c]);
  }
  return m;
});

const cursorsByAsmOffset = computed<Map<number, ResolvedCursor[]>>(() => {
  const m = new Map<number, ResolvedCursor[]>();
  for (const c of resolvedCursors.value) {
    if (c.asmOffset === null) continue;
    const bucket = m.get(c.asmOffset);
    if (bucket) bucket.push(c);
    else m.set(c.asmOffset, [c]);
  }
  return m;
});

function cursorsAtCLine(line: number): ResolvedCursor[] {
  return cursorsByCLine.value.get(line) ?? [];
}

function cursorsAtAsmOffset(off: number): ResolvedCursor[] {
  return cursorsByAsmOffset.value.get(off) ?? [];
}

function isActiveLine(line: number): boolean {
  return cursorsByCLine.value.has(line);
}

function isActiveAsm(off: number): boolean {
  return cursorsByAsmOffset.value.has(off);
}

// Pick the cursor the auto-scroll should follow. Priority: first RUN cursor
// (lowest coreId for determinism). When none is RUN, keep the current scroll
// position by returning null.
function pickScrollTarget(cursors: ResolvedCursor[]): ResolvedCursor | null {
  const running = cursors.filter(c => c.state === 'RUN' && c.coreId !== null);
  if (running.length === 0) return null;
  running.sort((a, b) => (a.coreId ?? 0) - (b.coreId ?? 0));
  return running[0];
}

const scrollTargetCLine = computed<number | null>(() =>
  pickScrollTarget(resolvedCursors.value)?.cLine ?? null,
);

const scrollTargetOffset = computed<number | null>(() =>
  pickScrollTarget(resolvedCursors.value)?.asmOffset ?? null,
);

function tabTitle(g: TGIDGroup): string {
  const leader = g.leader;
  if (g.members.length > 1) {
    const tids = g.members.map(m => m.tid ?? m.pid).join(',');
    return t('panels.codeAsm.tabTitleGroup', {
      pid: leader.pid,
      name: leader.name,
      state: leader.state,
      tids,
    });
  }
  const key = pinnedPID.value === leader.pid
    ? 'panels.codeAsm.tabTitlePinned'
    : 'panels.codeAsm.tabTitleUnpinned';
  return t(key, { pid: leader.pid, name: leader.name, state: leader.state });
}

function groupHasRunningMember(g: TGIDGroup): boolean {
  for (const m of g.members) {
    if (visibleCoreOfPID.value[m.pid] !== undefined) return true;
  }
  return false;
}

function siblingTids(g: TGIDGroup): string {
  if (g.members.length <= 1) return '';
  // Render every member's TID — including the leader's — so the chip reads
  // "T2,T3,T4" rather than hiding the head of the group.
  return g.members.map(m => m.tid ?? m.pid).join(',');
}

function pickGroup(g: TGIDGroup) {
  const leaderPid = g.leader.pid;
  const next = localPinnedPID.value === leaderPid ? null : leaderPid;
  localPinnedPID.value = next;
  if (!isViewer.value) {
    cpu.sendFocus({ pinned_pid: next ?? 0 });
  }
}

function cursorTooltip(c: ResolvedCursor): string {
  if (c.coreId !== null) {
    return t('panels.codeAsm.threadCursorTitle', { tid: c.tid, core: c.coreId });
  }
  return t('panels.codeAsm.threadCursorIdleTitle', { tid: c.tid, state: c.state });
}

// Auto-scroll: every time the target C line or ASM offset changes, bring the
// matching rows into view inside their own scroll containers. `block: center`
// keeps the highlighted row centred instead of snapping to the edges.
const cListRef = ref<HTMLElement | null>(null);
const asmListRef = ref<HTMLElement | null>(null);

watch(scrollTargetCLine, async (line) => {
  if (line === null) return;
  await nextTick();
  cListRef.value?.querySelector('.line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

watch(scrollTargetOffset, async (off) => {
  if (off === null) return;
  await nextTick();
  asmListRef.value?.querySelector('.asm-line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

function fmtHex(n: number): string {
  return '0x' + (n >>> 0).toString(16).toUpperCase().padStart(4, '0');
}

// Inline style for a cursor chip: tint by core when RUN, neutral surface
// otherwise. Modulo by 4 mirrors how SimStatusRow assigns --core-N tokens.
function chipStyle(c: ResolvedCursor): Record<string, string> {
  if (c.coreId !== null) {
    return {
      background: `var(--core-${c.coreId % 4})`,
      color: 'var(--core-fg)',
    };
  }
  return {};
}
</script>

<template>
  <div class="panel code-asm" data-pizarra-anchor="code-panel">
    <div class="panel-title">
      <span class="title-text">{{ t('panels.codeAsm.title') }}</span>
      <span class="title-hint" v-if="sortedProcs.length > 1">
        <MousePointerClick :size="12" :stroke-width="2" />
        {{ t('panels.codeAsm.titleHint') }}
      </span>
    </div>

    <div v-if="tgidGroups.length === 0" class="empty">
      {{ t('panels.codeAsm.empty') }}
    </div>

    <template v-else>
      <div class="proc-tabs">
        <button
          v-for="g in tgidGroups"
          :key="g.leader.pid"
          class="proc-tab"
          :class="{
            current: groupHasRunningMember(g),
            pinned: pinnedPID === g.leader.pid,
            viewing: tgidOf(g.leader) === viewingTGID,
          }"
          @click="pickGroup(g)"
          :title="tabTitle(g)"
        >
          <span class="pt-pid">P{{ g.leader.pid }}</span>
          <span class="pt-name">{{ g.leader.name }}</span>
          <span
            v-if="g.members.length > 1"
            class="pt-tids"
            :title="t('panels.codeAsm.tabGroupTidsTitle', { count: g.members.length })"
          >T{{ siblingTids(g) }}</span>
          <!-- Multicore identity: small chip with the core's color so the
               student maps PID → core at a glance. Hidden in single-core
               sessions where the dot is enough. The leader's chip is shown
               when the leader itself is running; otherwise we surface the
               first member's core so the tab still signals "where is this
               group". -->
          <span
            v-if="multicore && coreOfPID[g.leader.pid] !== undefined"
            class="pt-core"
            :style="{ background: `var(--core-${coreOfPID[g.leader.pid] % 4})`, color: `var(--core-fg)` }"
            :title="t('panels.codeAsm.coreRunning', { core: coreOfPID[g.leader.pid] })"
          >C{{ coreOfPID[g.leader.pid] }}</span>
          <span v-if="groupHasRunningMember(g)" class="pt-current">●</span>
          <span v-if="pinnedPID === g.leader.pid" class="pt-pin">📌</span>
        </button>
      </div>

      <div class="code-meta" v-if="viewingGroup">
        <span class="meta-tag" :title="t('panels.codeAsm.stateTitle')">{{ viewingGroup.leader.state }}</span>
        <span class="meta-info">
          {{ viewingGroup.leader.name }} · {{ t('panels.codeAsm.loadedAt') }}
          <span class="addr" :title="t('panels.codeAsm.codeBaseAddrTitle')">0x{{ (viewingGroup.leader.code_base || 0).toString(16).toUpperCase() }}</span>
          · {{ t('panels.codeAsm.stackTop') }}
          <span class="addr">0x{{ (viewingGroup.leader.stack_top || 0).toString(16).toUpperCase() }}</span>
          <template v-if="resolvedCursors.length > 0">
            ·
            <span
              v-for="(c, i) in resolvedCursors"
              :key="c.pid"
              class="meta-cursor"
              :style="chipStyle(c)"
            >
              T{{ c.tid }}<template v-if="c.asmOffset !== null">@{{ fmtHex(c.asmOffset) }}</template><template v-else>·{{ c.state }}</template><span v-if="i < resolvedCursors.length - 1">, </span>
            </span>
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
              <span class="line-text">{{ line || ' ' }}</span>
              <span v-if="isActiveLine(idx + 1)" class="line-cursors">
                <span
                  v-for="c in cursorsAtCLine(idx + 1)"
                  :key="c.pid"
                  class="thread-chip"
                  :class="{ 'thread-chip--idle': c.coreId === null }"
                  :style="chipStyle(c)"
                  :title="cursorTooltip(c)"
                >T{{ c.tid }}</span>
              </span>
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
              <span v-if="isActiveAsm(sl.offset)" class="asm-cursors">
                <span
                  v-for="c in cursorsAtAsmOffset(sl.offset)"
                  :key="c.pid"
                  class="thread-chip"
                  :class="{ 'thread-chip--idle': c.coreId === null }"
                  :style="chipStyle(c)"
                  :title="cursorTooltip(c)"
                >T{{ c.tid }}</span>
              </span>
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

.pt-tids {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--cyan);
  padding: 1px 5px;
  border: 1px solid var(--cyan);
  border-radius: var(--radius-sm);
}

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
.meta-info { color: var(--text-dim); display: inline-flex; flex-wrap: wrap; gap: 4px; align-items: baseline; }
.addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}
.meta-cursor {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  padding: 1px 4px;
  border-radius: var(--radius-sm);
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
  grid-template-columns: 28px 1fr auto;
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
  grid-template-columns: 60px 1fr 32px auto;
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

/* Cursor chips: one per thread sitting on the same line/offset. RUN chips
   carry the core tint; idle ones get a neutral border so the student can
   still tell who is parked where without stealing visual weight from the
   live thread. */
.line-cursors,
.asm-cursors {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  padding-left: 6px;
  flex-wrap: wrap;
}

.thread-chip {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  background: var(--surface-overlay);
  color: var(--text);
  border: 1px solid var(--border);
}
.thread-chip--idle {
  opacity: 0.6;
  border-style: dashed;
}

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
