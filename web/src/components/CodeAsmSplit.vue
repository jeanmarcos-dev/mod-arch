<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MousePointerClick } from '@lucide/vue';
import {
  useCpuStore,
  type SourceLine,
  type PCBInfo,
  type VarSnapshot,
  type VarMeta,
  type LocalSnapshot,
  type GlobalSnapshot,
  type ThreadVarSnapshot,
} from '../stores/cpu';
import { useViewerMirrorStore } from '../stores/viewerMirror';
import VarPopover from './VarPopover.vue';
import ColumnResizer from './shell/ColumnResizer.vue';
import LoaderModarch from './brand/LoaderModarch.vue';

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

// A "thread group" is two or more PCBs sharing the same TGID. Single-PCB
// groups (the common case for one-off demos like ADD) are plain processes
// and their cursor chips should read "P4" instead of "T4" so the panel
// doesn't claim a thread group exists where there is none.
const isThreadGroup = computed<boolean>(() => (viewingGroup.value?.members.length ?? 0) > 1);

function chipLabelPrefix(): string {
  return isThreadGroup.value ? 'T' : 'P';
}

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

// ─── Column collapse state ─────────────────────────────────────────────────
// Each of the three panels (C, ASM, VARS) can be collapsed independently and
// the state survives reloads. VARS starts collapsed since it's the new panel
// and users with no declared variables get nothing useful from it open.
const STORAGE_KEYS = {
  c: 'cpu.codeAsm.cCollapsed',
  asm: 'cpu.codeAsm.asmCollapsed',
  vars: 'cpu.codeAsm.varsCollapsed',
} as const;

function loadCollapse(key: string, fallback: boolean): boolean {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    return raw === '1';
  } catch {
    return fallback;
  }
}

const cCollapsed = ref<boolean>(loadCollapse(STORAGE_KEYS.c, false));
const asmCollapsed = ref<boolean>(loadCollapse(STORAGE_KEYS.asm, false));
const varsCollapsed = ref<boolean>(loadCollapse(STORAGE_KEYS.vars, true));

function persistCollapse(key: string, val: boolean) {
  try { localStorage.setItem(key, val ? '1' : '0'); } catch { /* private mode */ }
}

watch(cCollapsed, (v) => persistCollapse(STORAGE_KEYS.c, v));
watch(asmCollapsed, (v) => persistCollapse(STORAGE_KEYS.asm, v));
watch(varsCollapsed, (v) => persistCollapse(STORAGE_KEYS.vars, v));

function toggleC() {
  // Refuse to collapse the last visible panel — at least one must stay open
  // or the layout becomes meaningless and the toggles are unreachable.
  if (!cCollapsed.value && asmCollapsed.value && varsCollapsed.value) return;
  cCollapsed.value = !cCollapsed.value;
}
function toggleAsm() {
  if (!asmCollapsed.value && cCollapsed.value && varsCollapsed.value) return;
  asmCollapsed.value = !asmCollapsed.value;
}
function toggleVars() {
  if (!varsCollapsed.value && cCollapsed.value && asmCollapsed.value) return;
  varsCollapsed.value = !varsCollapsed.value;
}

// ─── Column widths ─────────────────────────────────────────────────────────
// Each open pane occupies a share of the row, persisted across reloads so a
// user who carved the layout once does not have to redo it every time. We
// store flex-grow units (so the math is independent of viewport width) and
// reseat them when the user drags a divider.
const WIDTH_KEYS = {
  c: 'cpu.codeAsm.cWeight',
  asm: 'cpu.codeAsm.asmWeight',
  vars: 'cpu.codeAsm.varsWeight',
} as const;
const MIN_PANE_WEIGHT = 0.2;

function loadWeight(key: string, fallback: number): number {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    const n = Number.parseFloat(raw);
    return Number.isFinite(n) && n >= MIN_PANE_WEIGHT ? n : fallback;
  } catch {
    return fallback;
  }
}

const cWeight = ref<number>(loadWeight(WIDTH_KEYS.c, 1));
const asmWeight = ref<number>(loadWeight(WIDTH_KEYS.asm, 1));
const varsWeight = ref<number>(loadWeight(WIDTH_KEYS.vars, 0.7));

function persistWeight(key: string, val: number) {
  try { localStorage.setItem(key, String(val)); } catch { /* private mode */ }
}

watch(cWeight, (v) => persistWeight(WIDTH_KEYS.c, v));
watch(asmWeight, (v) => persistWeight(WIDTH_KEYS.asm, v));
watch(varsWeight, (v) => persistWeight(WIDTH_KEYS.vars, v));

interface DragState {
  // Which pair of panes the divider sits between. We need both weights to
  // be live for the drag math to feel natural — when you pull right, the
  // left pane grows by the same amount the right pane shrinks.
  leftRef: Ref<number>;
  rightRef: Ref<number>;
  startX: number;
  startLeftWeight: number;
  startRightWeight: number;
  containerWidth: number;
}
// Plain mutable variable (not ref) so Vue does not auto-unwrap the nested
// Ref<number> fields. We still need reactivity for the divider highlight,
// which we drive through a separate identifier (one divider at a time).
let dragState: DragState | null = null;
type DividerKey = 'c-asm' | 'asm-vars';
const activeDivider = ref<DividerKey | null>(null);
const splitRef = ref<HTMLElement | null>(null);

function startDrag(
  which: DividerKey,
  event: PointerEvent,
) {
  const container = splitRef.value;
  if (!container) return;
  const containerWidth = container.getBoundingClientRect().width;
  if (containerWidth <= 0) return;

  let leftRef: Ref<number>;
  let rightRef: Ref<number>;
  if (which === 'c-asm') {
    leftRef = cWeight;
    rightRef = asmWeight;
  } else {
    leftRef = asmWeight;
    rightRef = varsWeight;
  }

  dragState = {
    leftRef,
    rightRef,
    startX: event.clientX,
    startLeftWeight: leftRef.value,
    startRightWeight: rightRef.value,
    containerWidth,
  };
  activeDivider.value = which;
  // Window listeners + capture phase so the drag survives the cursor leaving
  // the divider element. Same pattern Simulator.vue uses with ColumnResizer.
  window.addEventListener('pointermove', onDrag);
  window.addEventListener('pointerup', endDrag);
  window.addEventListener('pointercancel', endDrag);
  event.preventDefault();
}

function onDrag(event: PointerEvent) {
  const st = dragState;
  if (!st) return;
  const totalWeight = st.startLeftWeight + st.startRightWeight;
  const dx = event.clientX - st.startX;
  // Convert pixel delta to a weight delta proportional to the share of the
  // container width the dragged pair occupies. This keeps the cursor stuck
  // to the divider regardless of how many panes are open.
  const pairShareWidth = (totalWeight / sumOfWeights()) * st.containerWidth;
  if (pairShareWidth <= 0) return;
  const deltaWeight = (dx / pairShareWidth) * totalWeight;

  let nextLeft = st.startLeftWeight + deltaWeight;
  let nextRight = st.startRightWeight - deltaWeight;
  if (nextLeft < MIN_PANE_WEIGHT) {
    nextRight -= (MIN_PANE_WEIGHT - nextLeft);
    nextLeft = MIN_PANE_WEIGHT;
  }
  if (nextRight < MIN_PANE_WEIGHT) {
    nextLeft -= (MIN_PANE_WEIGHT - nextRight);
    nextRight = MIN_PANE_WEIGHT;
  }
  if (nextLeft < MIN_PANE_WEIGHT || nextRight < MIN_PANE_WEIGHT) return;
  st.leftRef.value = nextLeft;
  st.rightRef.value = nextRight;
}

function endDrag() {
  if (!dragState) return;
  dragState = null;
  activeDivider.value = null;
  window.removeEventListener('pointermove', onDrag);
  window.removeEventListener('pointerup', endDrag);
  window.removeEventListener('pointercancel', endDrag);
}

// sumOfWeights returns the sum across panes that are currently *open*. A
// collapsed pane contributes nothing to the layout grow share because its
// CSS treats it as a fixed-width handle.
function sumOfWeights(): number {
  let total = 0;
  if (!cCollapsed.value) total += cWeight.value;
  if (!asmCollapsed.value) total += asmWeight.value;
  if (!varsCollapsed.value) total += varsWeight.value;
  return total > 0 ? total : 1;
}

// Style helpers — each open pane reads its flex-grow from the corresponding
// ref. Collapsed panes get a 28px handle from CSS.
const cPaneStyle = computed(() => ({ flexGrow: cWeight.value, flexShrink: 1, flexBasis: '0' }));
const asmPaneStyle = computed(() => ({ flexGrow: asmWeight.value, flexShrink: 1, flexBasis: '0' }));
const varsPaneStyle = computed(() => ({ flexGrow: varsWeight.value, flexShrink: 1, flexBasis: '0' }));

const showDividerCAsm = computed(() => !cCollapsed.value && !asmCollapsed.value);
const showDividerAsmVars = computed(() => !asmCollapsed.value && !varsCollapsed.value);

// ─── VARS payload ─────────────────────────────────────────────────────────
// procVars / procVarMeta are keyed by the leader PID. viewingTGID already
// returns that PID for the active tab, so we just look it up.
const varSnapshot = computed<VarSnapshot | null>(() => {
  const tgid = viewingTGID.value;
  if (!tgid) return null;
  return cpu.procVars[tgid] ?? null;
});

const varMeta = computed<VarMeta | null>(() => {
  const tgid = viewingTGID.value;
  if (!tgid) return null;
  return cpu.procVarMeta[tgid] ?? null;
});

const hasVarMetadata = computed<boolean>(() => {
  const m = varMeta.value;
  if (!m) return false;
  return Boolean((m.globals && m.globals.length) || (m.locals && m.locals.length));
});

// Filter threads to those whose PID actually exists in the current process
// table — a stale snapshot occasionally references a TID that has already
// been reaped. Keep them in source order (backend sorted by PID).
const varThreads = computed<ThreadVarSnapshot[]>(() => {
  const snap = varSnapshot.value;
  if (!snap) return [];
  return snap.threads;
});

const varGlobals = computed<GlobalSnapshot[]>(() => varSnapshot.value?.globals ?? []);

// Track previous value to flash on change. Keyed by "<scope>:<name>" so
// globals and per-thread locals do not collide.
const previousValues = ref<Record<string, string>>({});
const flashingKeys = ref<Set<string>>(new Set());

function valueKey(scope: string, name: string): string { return `${scope}:${name}`; }

watch(varSnapshot, (snap, prev) => {
  if (!snap) return;
  const next: Record<string, string> = { ...previousValues.value };
  const justChanged = new Set<string>();

  for (const g of snap.globals) {
    const key = valueKey('global', g.name);
    const v = g.value_hex ?? '';
    if (prev && previousValues.value[key] !== undefined && previousValues.value[key] !== v) {
      justChanged.add(key);
    }
    next[key] = v;
  }
  for (const th of snap.threads) {
    for (const l of th.locals) {
      const key = valueKey(`T${th.tid}`, l.name);
      const v = l.value_hex ?? '';
      if (prev && previousValues.value[key] !== undefined && previousValues.value[key] !== v) {
        justChanged.add(key);
      }
      next[key] = v;
    }
  }
  previousValues.value = next;
  if (justChanged.size === 0) return;
  flashingKeys.value = justChanged;
  setTimeout(() => {
    if (flashingKeys.value === justChanged) flashingKeys.value = new Set();
  }, 350);
}, { deep: true });

function isFlashing(scope: string, name: string): boolean {
  return flashingKeys.value.has(valueKey(scope, name));
}

function localLocLabel(l: LocalSnapshot): string {
  if (l.loc.kind === 'reg') return `[${l.loc.reg}]`;
  if (l.loc.kind === 'stack') {
    const off = l.loc.offset ?? 0;
    if (off === 0) return '[EBP]';
    if (off < 0) return `[EBP${off}]`;
    return `[EBP+${off}]`;
  }
  if (l.loc.kind === 'mem' && l.loc.addr !== undefined) {
    return `[0x${l.loc.addr.toString(16).toUpperCase()}]`;
  }
  return '';
}

function threadChipStyle(tid: number): Record<string, string> {
  const coreId = visibleCoreOfPID.value[tid];
  if (coreId !== undefined) {
    return { background: `var(--core-${coreId % 4})`, color: 'var(--core-fg)' };
  }
  return {};
}

// ─── C-source tokenisation for the popover ────────────────────────────────
// Build the set of declared variable names so the renderer can mark each
// token in the C source. Locals and globals share the namespace because the
// popover only needs a key; resolution to a specific scope happens at hover
// time using `viewingGroup`'s threads.
const declaredVarNames = computed<Set<string>>(() => {
  const names = new Set<string>();
  const meta = varMeta.value;
  if (!meta) return names;
  for (const g of meta.globals ?? []) names.add(g.name);
  for (const l of meta.locals ?? []) names.add(l.name);
  return names;
});

interface CToken { text: string; isVar: boolean }

// Tokenise each C line by identifier vs anything-else. Keeping the
// non-identifier chunks intact preserves whitespace, punctuation and the
// look of the original source.
function tokenizeLine(line: string): CToken[] {
  if (!line) return [];
  const out: CToken[] = [];
  const re = /[A-Za-z_]\w*/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(line)) !== null) {
    if (m.index > last) {
      out.push({ text: line.slice(last, m.index), isVar: false });
    }
    out.push({ text: m[0], isVar: declaredVarNames.value.has(m[0]) });
    last = re.lastIndex;
  }
  if (last < line.length) out.push({ text: line.slice(last), isVar: false });
  return out;
}

// Lazily memoise the tokenisation per current source so we don't redo regex
// on every render of the v-for. Re-runs only when source or var names change.
const tokenisedLines = computed<CToken[][]>(() => sourceLines.value.map(tokenizeLine));

// ─── Popover state ───────────────────────────────────────────────────────
interface PopoverState {
  visible: boolean;
  name: string;
  anchorRect: DOMRect | null;
}
const popover = ref<PopoverState>({ visible: false, name: '', anchorRect: null });
let popoverTimer: number | null = null;

function showPopover(name: string, ev: MouseEvent) {
  if (!declaredVarNames.value.has(name)) return;
  if (popoverTimer !== null) { clearTimeout(popoverTimer); popoverTimer = null; }
  const target = ev.currentTarget as HTMLElement | null;
  popover.value = {
    visible: true,
    name,
    anchorRect: target?.getBoundingClientRect() ?? null,
  };
}

function hidePopover() {
  popoverTimer = window.setTimeout(() => {
    popover.value = { visible: false, name: '', anchorRect: null };
    popoverTimer = null;
  }, 150);
}

function keepPopover() {
  if (popoverTimer !== null) { clearTimeout(popoverTimer); popoverTimer = null; }
}

// On mount, re-validate the persisted collapse state: if all three slots are
// collapsed (e.g. someone hand-edited localStorage), reopen C so the panel
// is usable.
onMounted(() => {
  if (cCollapsed.value && asmCollapsed.value && varsCollapsed.value) {
    cCollapsed.value = false;
  }
});

// Defensive cleanup: an unmount mid-drag would otherwise leave the window
// listeners alive and respond to future pointer events from a stale closure.
onBeforeUnmount(() => {
  if (dragState !== null) endDrag();
});
</script>

<template>
  <div class="panel code-asm" data-pizarra-anchor="code-panel">
    <div v-if="sortedProcs.length > 1" class="panel-hint">
      <MousePointerClick :size="12" :stroke-width="2" />
      {{ t('panels.codeAsm.titleHint') }}
    </div>

    <div v-if="tgidGroups.length === 0" class="empty">
      <LoaderModarch class="empty__watermark" size="260px" compact aria-hidden="true" />
      <span class="empty__text">{{ t('panels.codeAsm.empty') }}</span>
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
              {{ chipLabelPrefix() }}{{ c.tid }}<template v-if="c.asmOffset !== null">@{{ fmtHex(c.asmOffset) }}</template><template v-else>·{{ c.state }}</template><span v-if="i < resolvedCursors.length - 1">, </span>
            </span>
          </template>
        </span>
      </div>

      <div
        ref="splitRef"
        class="split"
        :class="{
          'split--c-collapsed': cCollapsed,
          'split--asm-collapsed': asmCollapsed,
          'split--vars-collapsed': varsCollapsed,
          'split--no-asm': sourceMap.length === 0,
        }"
      >
        <!-- C pane: collapsed handle is a thin vertical bar with the rotated
             title; user clicks the chevron / handle to expand. -->
        <section v-if="cCollapsed" class="pane-handle" @click="toggleC" :title="t('panels.codeAsm.expandC')">
          <span class="pane-handle-title">{{ t('panels.codeAsm.cTitle') }}</span>
        </section>
        <section v-else class="pane pane--c" :style="cPaneStyle">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.cTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.cLinesCount', { n: sourceLines.length }) }}</span>
            <button class="pane-collapse" :title="t('panels.codeAsm.collapseC')" @click="toggleC">−</button>
          </header>
          <ol v-if="source" ref="cListRef" class="c-list">
            <li
              v-for="(line, idx) in sourceLines"
              :key="idx"
              :class="['line', { 'line--current': isActiveLine(idx + 1) }]"
            >
              <span class="line-no">{{ idx + 1 }}</span>
              <span class="line-text">
                <template v-for="(tok, ti) in tokenisedLines[idx]" :key="ti">
                  <span
                    v-if="tok.isVar"
                    class="c-token c-token--var"
                    @mouseenter="showPopover(tok.text, $event)"
                    @mouseleave="hidePopover"
                  >{{ tok.text }}</span>
                  <template v-else>{{ tok.text }}</template>
                </template>
                <template v-if="!tokenisedLines[idx] || tokenisedLines[idx].length === 0">{{ line || ' ' }}</template>
              </span>
              <span v-if="isActiveLine(idx + 1)" class="line-cursors">
                <span
                  v-for="c in cursorsAtCLine(idx + 1)"
                  :key="c.pid"
                  class="thread-chip"
                  :class="{ 'thread-chip--idle': c.coreId === null }"
                  :style="chipStyle(c)"
                  :title="cursorTooltip(c)"
                >{{ chipLabelPrefix() }}{{ c.tid }}</span>
              </span>
            </li>
          </ol>
          <div v-else class="pane-empty">{{ t('panels.codeAsm.emptyC') }}</div>
        </section>

        <!-- Drag handle between C and ASM. Reuses the global ColumnResizer
             so the visual treatment (grip pill, hover/active highlight) is
             identical to the layout-level resizers in Simulator.vue. -->
        <ColumnResizer
          v-if="showDividerCAsm"
          side="left"
          :active="activeDivider === 'c-asm'"
          @pointerdown="(e) => startDrag('c-asm', e)"
        />

        <!-- ASM pane: same collapse pattern. The legacy "no mapping" fallback
             is folded into the open pane body so the collapsed handle is the
             same regardless. -->
        <section v-if="asmCollapsed" class="pane-handle" @click="toggleAsm" :title="t('panels.codeAsm.expandAsm')">
          <span class="pane-handle-title">{{ t('panels.codeAsm.asmTitle') }}</span>
        </section>
        <section v-else-if="sourceMap.length > 0" class="pane pane--asm" :style="asmPaneStyle">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.asmTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.asmInstrCount', { n: sourceMap.length }) }}</span>
            <button class="pane-collapse" :title="t('panels.codeAsm.collapseAsm')" @click="toggleAsm">−</button>
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
                >{{ chipLabelPrefix() }}{{ c.tid }}</span>
              </span>
            </li>
          </ol>
        </section>
        <section v-else class="pane pane--asm pane--asm-fallback" :style="asmPaneStyle">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.asmTitle') }}</span>
            <span class="pane-sub">{{ t('panels.codeAsm.noMappingSub') }}</span>
            <button class="pane-collapse" :title="t('panels.codeAsm.collapseAsm')" @click="toggleAsm">−</button>
          </header>
          <div class="pane-empty">{{ t('panels.codeAsm.noMappingBody') }}</div>
        </section>

        <!-- Drag handle between ASM and VARS. -->
        <ColumnResizer
          v-if="showDividerAsmVars"
          side="right"
          :active="activeDivider === 'asm-vars'"
          @pointerdown="(e) => startDrag('asm-vars', e)"
        />

        <!-- VARS pane: opt-in column. Empty body when the demo declared no
             variables, so the toggle is still discoverable. -->
        <section v-if="varsCollapsed" class="pane-handle pane-handle--vars" @click="toggleVars" :title="t('panels.codeAsm.expandVars')">
          <span class="pane-handle-title">{{ t('panels.codeAsm.varsTitle') }}</span>
        </section>
        <section v-else class="pane pane--vars" :style="varsPaneStyle">
          <header class="pane-head">
            <span class="pane-title">{{ t('panels.codeAsm.varsTitle') }}</span>
            <span class="pane-sub" v-if="hasVarMetadata">{{ t('panels.codeAsm.varsLiveSub') }}</span>
            <span class="pane-sub" v-else>{{ t('panels.codeAsm.varsEmptySub') }}</span>
            <button class="pane-collapse" :title="t('panels.codeAsm.collapseVars')" @click="toggleVars">−</button>
          </header>
          <div v-if="!hasVarMetadata" class="pane-empty">
            {{ t('panels.codeAsm.varsNoMeta') }}
          </div>
          <div v-else class="vars-body">
            <div v-if="varGlobals.length > 0" class="vars-section">
              <div class="vars-section-title">{{ t('panels.codeAsm.globals') }}</div>
              <ul class="vars-list">
                <li
                  v-for="g in varGlobals"
                  :key="g.name"
                  class="var-row"
                  :class="{ 'var-row--flash': isFlashing('global', g.name), 'var-row--unmapped': g.unmapped }"
                >
                  <span class="var-name">{{ g.name }}</span>
                  <span class="var-loc">[{{ g.addr }}]</span>
                  <span class="var-type">{{ g.type }}</span>
                  <span v-if="g.unmapped" class="var-val var-val--unmapped">{{ t('panels.codeAsm.unmapped') }}</span>
                  <span v-else class="var-val">{{ g.value_dec }} <span class="var-val-hex">{{ g.value_hex }}</span></span>
                </li>
              </ul>
            </div>
            <div
              v-for="th in varThreads"
              :key="th.tid"
              class="vars-section vars-section--thread"
              :class="{ 'vars-section--out': !th.in_scope }"
            >
              <div class="vars-section-title vars-section-title--thread">
                <span class="thread-chip" :style="threadChipStyle(th.tid)">{{ th.label }}</span>
                <span class="vars-fn" v-if="th.func">{{ th.func }}</span>
                <span class="vars-fn vars-fn--none" v-else>{{ t('panels.codeAsm.outOfScope') }}</span>
                <span class="vars-ebp">EBP={{ th.ebp }}</span>
              </div>
              <ul v-if="th.locals.length > 0" class="vars-list">
                <li
                  v-for="l in th.locals"
                  :key="l.name"
                  class="var-row"
                  :class="{
                    'var-row--flash': isFlashing(`T${th.tid}`, l.name),
                    'var-row--unset': l.unset,
                    'var-row--unmapped': l.unmapped,
                  }"
                >
                  <span class="var-name">{{ l.name }}</span>
                  <span class="var-loc">{{ localLocLabel(l) }}</span>
                  <span class="var-type">{{ l.type }}</span>
                  <span v-if="l.unset" class="var-val var-val--unset">{{ t('panels.codeAsm.unset') }}</span>
                  <span v-else-if="l.unmapped" class="var-val var-val--unmapped">{{ t('panels.codeAsm.unmapped') }}</span>
                  <span v-else class="var-val">{{ l.value_dec }} <span class="var-val-hex">{{ l.value_hex }}</span></span>
                </li>
              </ul>
              <div v-else class="vars-no-locals">{{ t('panels.codeAsm.noLocalsDeclared') }}</div>
            </div>
          </div>
        </section>
      </div>

      <VarPopover
        v-if="popover.visible"
        :name="popover.name"
        :anchor-rect="popover.anchorRect"
        :var-meta="varMeta"
        :var-snapshot="varSnapshot"
        @stay="keepPopover"
        @leave="hidePopover"
      />

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

.panel-hint {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3) 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-dim);
  font-style: italic;
}

.empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-4);
  padding: 24px;
  text-align: center;
  pointer-events: none;
}

.empty__watermark {
  opacity: 0.12;
  max-width: 60%;
  max-height: 50%;
}

.empty__text {
  color: var(--text-muted);
  font-style: italic;
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

/* Split layout: up to three flex columns (C, ASM, VARS) with drag handles
   between any two open panes. Each open pane reads its own flex-grow value
   from a reactive ref so the user can carve the row to their liking; the
   weights live in localStorage. Collapsed panes degrade to a 28px handle
   that is not draggable. */
.split {
  flex: 1;
  display: flex;
  flex-direction: row;
  gap: 0;
  overflow: hidden;
  min-height: 0;
}
.split--no-asm .pane--asm-fallback { display: none; }

/* Open panes obey the flex style applied inline; a min-width keeps them
   readable when the divider gets dragged close to a sibling. */
.pane {
  min-width: 120px;
}

/* Pane dividers use the shared <ColumnResizer> so their look matches the
   layout-level resizers in Simulator.vue. No local divider CSS lives here
   anymore. */

/* Collapsed handle: thin clickable bar with the title rotated 90deg so the
   pane is still identifiable without taking horizontal space. flex: 0 0 28px
   pins the width so the handle never steals space from the open panes. */
.pane-handle {
  flex: 0 0 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-overlay);
  border: 1px solid var(--border);
  cursor: pointer;
  user-select: none;
  transition: background 120ms ease;
}
.pane-handle:hover { background: rgba(var(--accent-rgb), 0.08); }
.pane-handle-title {
  writing-mode: vertical-rl;
  transform: rotate(180deg);
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
}
.pane-collapse {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-muted);
  width: 18px;
  height: 18px;
  margin-left: 6px;
  line-height: 14px;
  font-size: 14px;
  cursor: pointer;
  padding: 0;
  font-family: inherit;
}
.pane-collapse:hover { color: var(--amber); border-color: var(--amber); }

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

/* ─── VARS pane ───────────────────────────────────────────────────────── */
.pane--vars { min-width: 220px; }
.vars-body {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px 10px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.45;
}
.vars-section {
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--border);
}
.vars-section:last-child { border-bottom: none; }
.vars-section--out { opacity: 0.5; }
.vars-section-title {
  color: var(--amber-dim);
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  margin-bottom: 4px;
}
.vars-section-title--thread {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.vars-fn { color: var(--text); text-transform: none; letter-spacing: 0; font-family: 'JetBrains Mono', monospace; font-size: 10px; }
.vars-fn--none { color: var(--text-muted); font-style: italic; }
.vars-ebp { color: var(--text-muted); font-family: 'JetBrains Mono', monospace; font-size: 10px; margin-left: auto; }
.vars-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.var-row {
  display: grid;
  grid-template-columns: minmax(40px, auto) minmax(60px, auto) 32px 1fr;
  gap: 6px;
  align-items: baseline;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  transition: background 350ms ease;
}
.var-row--flash { background: rgba(var(--accent-rgb), 0.18); }
.var-row--unset, .var-row--unmapped { opacity: 0.55; }
.var-name { color: var(--text); }
.var-loc { color: var(--text-muted); font-size: 10px; }
.var-type { color: var(--amber-dim); font-size: 10px; text-transform: uppercase; letter-spacing: 1px; }
.var-val { color: var(--text); justify-self: end; }
.var-val-hex { color: var(--text-muted); font-size: 10px; margin-left: 4px; }
.var-val--unset, .var-val--unmapped { color: var(--text-muted); font-style: italic; font-size: 10px; }
.vars-no-locals { color: var(--text-muted); font-style: italic; font-size: 10px; padding: 2px 4px; }

/* ─── Identifier tokens in the C pane ─────────────────────────────────── */
.c-token--var {
  border-bottom: 1px dotted rgba(var(--accent-rgb), 0.5);
  cursor: help;
  transition: background 120ms ease, color 120ms ease;
}
.c-token--var:hover {
  background: rgba(var(--accent-rgb), 0.12);
  color: var(--amber);
}

@media (max-width: 768px) {
  /* Narrow viewports: stack panes vertically so each one keeps a usable
     reading width. The C panel stays on top because the lesson starts there. */
  .split { grid-template-columns: 1fr; }
  .split .pane--asm { min-height: 260px; }
  /* Inside the mobile page-scroll the editor keeps a bounded height so each
     pane scrolls internally instead of stretching the page to the full source
     length. */
  .code-asm {
    height: 70vh;
  }
}
</style>
