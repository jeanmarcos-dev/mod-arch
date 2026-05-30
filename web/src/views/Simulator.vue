<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, type ComponentPublicInstance } from 'vue';
import { useI18n } from 'vue-i18n';
import { Cpu, Code2, ListTree, MemoryStick, Boxes, Terminal, Lock } from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { useViewerMirrorStore } from '../stores/viewerMirror';
import { useAppConfigStore } from '../stores/appConfig';
import { usePreferencesStore, COLUMN_MIN_PX, COLUMN_MAX_PERCENT } from '../stores/preferences';
import { useSimBarStore } from '../stores/simBar';
import { useIsStudent } from '../composables/useIsStudent';
import { useResizableColumn } from '../composables/useResizableColumn';
import { useResizableRow } from '../composables/useResizableRow';
import CpuPanel from '../components/CpuPanel.vue';
import EventsLog from '../components/EventsLog.vue';
import ProcessesPanel from '../components/ProcessesPanel.vue';
import QueuesPanel from '../components/QueuesPanel.vue';
import IpcPanel from '../components/IpcPanel.vue';
import ConsolePanel from '../components/ConsolePanel.vue';
import MemoryPanel from '../components/MemoryPanel.vue';
import CodeAsmSplit from '../components/CodeAsmSplit.vue';
import LockedPanel from '../components/shell/LockedPanel.vue';
import ColumnResizer from '../components/shell/ColumnResizer.vue';
import RowResizer from '../components/shell/RowResizer.vue';
import ByteFlightNarrator from '../components/narrators/ByteFlightNarrator.vue';
import ModeFlipNarrator from '../components/narrators/ModeFlipNarrator.vue';
import TrapFrameNarrator from '../components/narrators/TrapFrameNarrator.vue';
import PCBCopyNarrator from '../components/narrators/PCBCopyNarrator.vue';

const cpu = useCpuStore();
const mirror = useViewerMirrorStore();
const appCfg = useAppConfigStore();
const prefs = usePreferencesStore();
const bar = useSimBarStore();
const isStudent = useIsStudent();
const { t } = useI18n();

// Two resizable side columns: the cores rail on the left, and the
// procesos/colas/IPC stack on the right. Widths are persisted in preferences
// and clamped to [minPx, COLUMN_MAX_PERCENT * viewport]. Snap targets land on
// 25% and 33% of the viewport so the layout can converge on the two presets
// the design discussed without forcing the user there.
const COLUMN_SNAP_PERCENTS = [0.25, COLUMN_MAX_PERCENT];

const {
  width: leftWidth,
  isResizing: leftResizing,
  start: startLeftResize,
} = useResizableColumn({
  side: 'left',
  initialPx: prefs.columnWidths.leftPx,
  minPx: COLUMN_MIN_PX.left,
  maxPercent: COLUMN_MAX_PERCENT,
  snapPercents: COLUMN_SNAP_PERCENTS,
  onChange: (px) => prefs.setColumnWidth('left', px),
});

const {
  width: rightWidth,
  isResizing: rightResizing,
  start: startRightResize,
} = useResizableColumn({
  side: 'right',
  initialPx: prefs.columnWidths.rightPx,
  minPx: COLUMN_MIN_PX.right,
  maxPercent: COLUMN_MAX_PERCENT,
  snapPercents: COLUMN_SNAP_PERCENTS,
  onChange: (px) => prefs.setColumnWidth('right', px),
});

const leftStyle = computed(() => ({ width: `${leftWidth.value}px` }));
const rightStyle = computed(() => ({ width: `${rightWidth.value}px` }));

// Column widths live inside useResizableColumn, but the preferences store
// can also rewrite them externally (e.g. Reset UI). Mirror store → ref so a
// programmatic change reaches the layout; the equality guard prevents the
// resizer's onChange from echoing back into a write loop.
watch(() => prefs.columnWidths.leftPx, (px) => {
  if (leftWidth.value !== px) leftWidth.value = px;
});
watch(() => prefs.columnWidths.rightPx, (px) => {
  if (rightWidth.value !== px) rightWidth.value = px;
});

// Refs to the panels stacked in each side column. useResizableRow reads
// their offsetHeight at drag start to translate pointer deltas into weight
// changes. Template refs on Vue components resolve to a public instance, not
// a DOM node — useResizableRow walks $el internally.
type PanelRef = ComponentPublicInstance | HTMLElement | null;
const cpuEl = ref<PanelRef>(null);
const ipcEl = ref<PanelRef>(null);
const processesEl = ref<PanelRef>(null);
const queuesEl = ref<PanelRef>(null);
const middleTabsEl = ref<PanelRef>(null);
const consolePanelEl = ref<PanelRef>(null);

// CSS variable, not `flex` shorthand: CollapsiblePanel reads --cpanel-weight
// from this inline style and applies it only to flex-grow. The collapsed
// override targets the long-hand properties with !important, so a toggle
// always wins over the inline weight without depending on shorthand cascade.
const cpuStyle = computed(() => ({ '--cpanel-weight': String(prefs.panelHeights.cpu) }));
const ipcStyle = computed(() => ({ '--cpanel-weight': String(prefs.panelHeights.ipc) }));
const processesStyle = computed(() => ({ '--cpanel-weight': String(prefs.panelHeights.processes) }));
const queuesStyle = computed(() => ({ '--cpanel-weight': String(prefs.panelHeights.queues) }));
// Center column: tabs pile (Código/Bitácora/Memoria) vs ConsolePanel below.
const middleTabsStyle = computed(() => ({ '--mc-weight': String(prefs.panelHeights.middleTabs) }));
const middleConsoleStyle = computed(() => ({ '--mc-weight': String(prefs.panelHeights.middleConsole) }));

const { isResizing: cpuIpcResizing, start: startCpuIpcResize } = useResizableRow({
  topEl: cpuEl,
  bottomEl: ipcEl,
  getTopWeight: () => prefs.panelHeights.cpu,
  getBottomWeight: () => prefs.panelHeights.ipc,
  setTopWeight: (w) => prefs.setPanelHeight('cpu', w),
  setBottomWeight: (w) => prefs.setPanelHeight('ipc', w),
});

const { isResizing: processesQueuesResizing, start: startProcessesQueuesResize } = useResizableRow({
  topEl: processesEl,
  bottomEl: queuesEl,
  getTopWeight: () => prefs.panelHeights.processes,
  getBottomWeight: () => prefs.panelHeights.queues,
  setTopWeight: (w) => prefs.setPanelHeight('processes', w),
  setBottomWeight: (w) => prefs.setPanelHeight('queues', w),
});

// Center column: splits vertical space between the active tab (Código /
// Bitácora / Memoria, all v-show siblings inside .middle__tabs-stack) and the
// ConsolePanel directly below. Without this handle the console grows with its
// content until it pushes the tabs out of the viewport.
const { isResizing: middleTabsConsoleResizing, start: startMiddleTabsConsoleResize } = useResizableRow({
  topEl: middleTabsEl,
  bottomEl: consolePanelEl,
  getTopWeight: () => prefs.panelHeights.middleTabs,
  getBottomWeight: () => prefs.panelHeights.middleConsole,
  setTopWeight: (w) => prefs.setPanelHeight('middleTabs', w),
  setBottomWeight: (w) => prefs.setPanelHeight('middleConsole', w),
});

// Row resizers only make sense when both neighbours are expanded — a
// collapsed panel is fixed at its header height and has no slack to trade.
const showCpuIpcResizer = computed(() =>
  !prefs.panelsCollapsed.cores && !prefs.panelsCollapsed.ipc,
);
const showProcessesQueuesResizer = computed(() =>
  !prefs.panelsCollapsed.processes && !prefs.panelsCollapsed.queues,
);

// Panel IDs disabled by the admin, scoped to students. Privileged users
// (admin / moderator) always see every panel — the simulator must remain a
// debugging surface for the people configuring the experience.
const lockedPanels = computed<Set<string>>(() => {
  if (!isStudent.value) return new Set();
  return new Set(appCfg.disabledPanels);
});

function isLocked(panel: string): boolean {
  return lockedPanels.value.has(panel);
}

// Tab state lives locally as the source of truth for the user that owns the
// tab UI. Viewers receive overrides through the focus mirror: when the host
// publishes a focus.middle_tab, mirroredMiddleTab below points at it and
// activeMiddleTab returns the mirrored value. Owners ignore the mirror and
// emit a focus delta on every change.
type MiddleTab = 'events' | 'memory' | 'code';
type MobileTab = 'cpu' | 'code' | 'events' | 'memory' | 'procs' | 'console';
const localMiddleTab = ref<MiddleTab>('events');
const localMobileTab = ref<MobileTab>('events');

const isViewer = computed(() => cpu.sessionInfo.role === 'viewer');

const activeMiddleTab = computed<MiddleTab>(() => {
  if (isViewer.value && mirror.middleTab) return mirror.middleTab as MiddleTab;
  return localMiddleTab.value;
});
const activeMobileTab = computed<MobileTab>(() => {
  if (isViewer.value && mirror.mobileTab) return mirror.mobileTab as MobileTab;
  return localMobileTab.value;
});

function setMiddleTab(val: MiddleTab) {
  if (isLocked(val)) return;
  localMiddleTab.value = val;
  if (val === 'code' || val === 'events' || val === 'memory') {
    localMobileTab.value = val;
  }
  if (!isViewer.value) {
    cpu.sendFocus({ middle_tab: val, mobile_tab: val });
  }
}

function setMobileTab(val: MobileTab) {
  if (isLocked(val)) return;
  localMobileTab.value = val;
  if (val === 'code' || val === 'events' || val === 'memory') {
    localMiddleTab.value = val;
  }
  if (!isViewer.value) {
    cpu.sendFocus({
      mobile_tab: val,
      middle_tab: (val === 'code' || val === 'events' || val === 'memory') ? val : undefined,
    });
  }
}

// If the currently-active tab falls into the locked set (admin just disabled
// it, or the user lands on the page while it's already disabled), pick the
// first still-available tab so the center column never renders a locked
// panel as the active view.
const MIDDLE_TAB_FALLBACK: MiddleTab[] = ['events', 'code', 'memory'];
const MOBILE_TAB_FALLBACK: MobileTab[] = ['events', 'cpu', 'code', 'memory', 'procs', 'console'];
watch(lockedPanels, (locked) => {
  if (locked.has(localMiddleTab.value)) {
    const next = MIDDLE_TAB_FALLBACK.find((t) => !locked.has(t));
    if (next) localMiddleTab.value = next;
  }
  if (locked.has(localMobileTab.value)) {
    const next = MOBILE_TAB_FALLBACK.find((t) => !locked.has(t));
    if (next) localMobileTab.value = next;
  }
}, { immediate: true });

// Mirror-driven local copies: when the viewer receives a new focus, sync the
// local refs so a later autofollow-off does not snap the tab back.
watch(() => mirror.middleTab, (val) => {
  if (isViewer.value && val) localMiddleTab.value = val as MiddleTab;
});
watch(() => mirror.mobileTab, (val) => {
  if (isViewer.value && val) localMobileTab.value = val as MobileTab;
});

// "Jump to memory of P<n>" intents emitted from ProcessesPanel land here. We
// only own the tab switch — MemoryPanel handles re-centering and clears the
// flag, which is fine because <MemoryPanel> stays mounted via v-show.
watch(() => cpu.pendingMemoryJump, (addr) => {
  if (addr === null || addr === undefined) return;
  setMiddleTab('memory');
});

const isMobile = ref(false);
function checkMobile() {
  isMobile.value = window.matchMedia('(max-width: 768px)').matches;
}

// Visibilidad de cada panel resuelta en un solo lugar. En desktop, CPU/Procs/Console
// siempre visibles + middleTab para code/events/memory. En mobile, mobileTab decide todo.
const visible = computed(() => {
  if (isMobile.value) {
    const t = activeMobileTab.value;
    return {
      cpu: t === 'cpu',
      code: t === 'code',
      events: t === 'events',
      memory: t === 'memory',
      procs: t === 'procs',
      console: t === 'console',
      middle: t === 'code' || t === 'events' || t === 'memory' || t === 'console',
    };
  }
  return {
    cpu: true,
    code: activeMiddleTab.value === 'code',
    events: activeMiddleTab.value === 'events',
    memory: activeMiddleTab.value === 'memory',
    procs: true,
    console: true,
    middle: true,
  };
});

// Forward "console panel is the active mobile tab" and "keyboard HW is taking
// keystrokes" into the SimBar store so TutorLauncher can dim the FAB while
// either condition holds. Resetting on unmount avoids a stuck-dim state if
// the user navigates away with the console open.
watch(
  () => isMobile.value && visible.value.console,
  (active) => bar.setConsoleMobileActive(active),
  { immediate: true },
);
watch(
  () => isMobile.value && visible.value.console && cpu.kbdReaders.length > 0,
  (active) => bar.setKeyboardHwActive(active),
  { immediate: true },
);

onMounted(() => {
  cpu.connect();
  checkMobile();
  window.addEventListener('resize', checkMobile);
});

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile);
  bar.setConsoleMobileActive(false);
  bar.setKeyboardHwActive(false);
});
</script>

<template>
  <div :class="{ shake: cpu.faultShake }" class="root" data-pizarra-anchor="view-overview">
    <!-- Tab bar mobile-only — controla todo el layout. Sólo visible <= 768px (CSS) -->
    <nav class="mobile-tabs" :aria-hidden="!isMobile">
      <button :class="{ active: activeMobileTab === 'cpu', locked: isLocked('cpu') }" :disabled="isLocked('cpu')" @click="setMobileTab('cpu')" :title="isLocked('cpu') ? t('simulator.lockedPanel.title') : t('tabs.cpuTitle')"><Cpu :size="14" :stroke-width="2" /><span>{{ t('tabs.cpu') }}</span><Lock v-if="isLocked('cpu')" class="tab-lock" :size="10" :stroke-width="2" /></button>
      <button :class="{ active: activeMobileTab === 'code', locked: isLocked('code') }" :disabled="isLocked('code')" @click="setMobileTab('code')" :title="isLocked('code') ? t('simulator.lockedPanel.title') : t('tabs.codeTitle')"><Code2 :size="14" :stroke-width="2" /><span>{{ t('tabs.cod') }}</span><Lock v-if="isLocked('code')" class="tab-lock" :size="10" :stroke-width="2" /></button>
      <button :class="{ active: activeMobileTab === 'events', locked: isLocked('events') }" :disabled="isLocked('events')" @click="setMobileTab('events')" :title="isLocked('events') ? t('simulator.lockedPanel.title') : t('tabs.eventsTitle')"><ListTree :size="14" :stroke-width="2" /><span>{{ t('tabs.log') }}</span><Lock v-if="isLocked('events')" class="tab-lock" :size="10" :stroke-width="2" /></button>
      <button :class="{ active: activeMobileTab === 'memory', locked: isLocked('memory') }" :disabled="isLocked('memory')" @click="setMobileTab('memory')" :title="isLocked('memory') ? t('simulator.lockedPanel.title') : t('tabs.memoryTitle')"><MemoryStick :size="14" :stroke-width="2" /><span>{{ t('tabs.mem') }}</span><Lock v-if="isLocked('memory')" class="tab-lock" :size="10" :stroke-width="2" /></button>
      <button :class="{ active: activeMobileTab === 'procs', locked: isLocked('procs') }" :disabled="isLocked('procs')" @click="setMobileTab('procs')" :title="isLocked('procs') ? t('simulator.lockedPanel.title') : t('tabs.procsTitle')"><Boxes :size="14" :stroke-width="2" /><span>{{ t('tabs.proc') }}</span><Lock v-if="isLocked('procs')" class="tab-lock" :size="10" :stroke-width="2" /></button>
      <button :class="{ active: activeMobileTab === 'console', locked: isLocked('console') }" :disabled="isLocked('console')" @click="setMobileTab('console')" :title="isLocked('console') ? t('simulator.lockedPanel.title') : t('tabs.consoleTitle')"><Terminal :size="14" :stroke-width="2" /><span>{{ t('tabs.cons') }}</span><Lock v-if="isLocked('console')" class="tab-lock" :size="10" :stroke-width="2" /></button>
    </nav>

    <main class="layout">
      <div
        v-show="visible.cpu"
        class="col-left col-left--stack"
        :style="!isMobile ? leftStyle : undefined"
      >
        <CpuPanel
          v-if="!isLocked('cpu')"
          ref="cpuEl"
          class="col-left__panel"
          :style="!isMobile ? cpuStyle : undefined"
        />
        <LockedPanel
          v-else
          ref="cpuEl"
          class="col-left__panel"
          :style="!isMobile ? cpuStyle : undefined"
        />
        <RowResizer
          v-if="!isMobile && showCpuIpcResizer"
          :active="cpuIpcResizing"
          @pointerdown="startCpuIpcResize"
        />
        <IpcPanel
          v-if="!isLocked('ipc')"
          ref="ipcEl"
          class="col-left__panel"
          :style="!isMobile ? ipcStyle : undefined"
        />
        <LockedPanel
          v-else
          ref="ipcEl"
          class="col-left__panel"
          :style="!isMobile ? ipcStyle : undefined"
        />
      </div>
      <ColumnResizer
        v-if="!isMobile && visible.cpu"
        side="left"
        :active="leftResizing"
        @pointerdown="startLeftResize"
      />
      <div v-show="visible.middle" class="middle col-center">
        <div class="middle-tabs desktop-only">
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'code', locked: isLocked('code') }"
            :disabled="isLocked('code')"
            @click="setMiddleTab('code')"
            :title="isLocked('code') ? t('simulator.lockedPanel.title') : t('tabs.codeC_long')"
          ><Code2 :size="14" :stroke-width="2" /><span>{{ t('tabs.codeC') }}</span><Lock v-if="isLocked('code')" class="tab-lock" :size="10" :stroke-width="2" /></button>
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'events', locked: isLocked('events') }"
            :disabled="isLocked('events')"
            @click="setMiddleTab('events')"
            :title="isLocked('events') ? t('simulator.lockedPanel.title') : t('tabs.eventsLong')"
          ><ListTree :size="14" :stroke-width="2" /><span>{{ t('tabs.eventLog') }}</span><Lock v-if="isLocked('events')" class="tab-lock" :size="10" :stroke-width="2" /></button>
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'memory', locked: isLocked('memory') }"
            :disabled="isLocked('memory')"
            @click="setMiddleTab('memory')"
            :title="isLocked('memory') ? t('simulator.lockedPanel.title') : t('tabs.memoryLong')"
          ><MemoryStick :size="14" :stroke-width="2" /><span>{{ t('tabs.memory') }}</span><Lock v-if="isLocked('memory')" class="tab-lock" :size="10" :stroke-width="2" /></button>
        </div>
        <div
          ref="middleTabsEl"
          class="middle__tabs-stack"
          v-show="visible.code || visible.events || visible.memory"
          :style="!isMobile ? middleTabsStyle : undefined"
        >
          <CodeAsmSplit v-if="!isLocked('code')" v-show="visible.code" />
          <LockedPanel v-else v-show="visible.code" />
          <EventsLog v-if="!isLocked('events')" v-show="visible.events" />
          <LockedPanel v-else v-show="visible.events" />
          <MemoryPanel v-if="!isLocked('memory')" v-show="visible.memory" />
          <LockedPanel v-else v-show="visible.memory" />
        </div>
        <RowResizer
          v-if="!isMobile && visible.console"
          class="middle__row-resizer desktop-only"
          :active="middleTabsConsoleResizing"
          @pointerdown="startMiddleTabsConsoleResize"
        />
        <ConsolePanel
          v-if="!isLocked('console')"
          ref="consolePanelEl"
          v-show="visible.console"
          :style="!isMobile ? middleConsoleStyle : undefined"
        />
        <LockedPanel
          v-else
          ref="consolePanelEl"
          v-show="visible.console"
          :style="!isMobile ? middleConsoleStyle : undefined"
        />
      </div>
      <ColumnResizer
        v-if="!isMobile && visible.procs"
        side="right"
        :active="rightResizing"
        @pointerdown="startRightResize"
      />
      <div
        v-show="visible.procs"
        class="col-right col-right--stack"
        :style="!isMobile ? rightStyle : undefined"
      >
        <ProcessesPanel
          v-if="!isLocked('procs')"
          ref="processesEl"
          class="col-right__panel"
          :style="!isMobile ? processesStyle : undefined"
        />
        <LockedPanel
          v-else
          ref="processesEl"
          class="col-right__panel"
          :style="!isMobile ? processesStyle : undefined"
        />
        <RowResizer
          v-if="!isMobile && showProcessesQueuesResizer"
          :active="processesQueuesResizing"
          @pointerdown="startProcessesQueuesResize"
        />
        <QueuesPanel
          ref="queuesEl"
          class="col-right__panel"
          :style="!isMobile ? queuesStyle : undefined"
        />
      </div>
    </main>

    <!-- Narrators are mounted at the Overview root so byte-flight, PCB-copy
         and mode-flip animations are visible in the general view, not only in
         /memoria, /procesos and /interrupciones. They are pure overlays
         (pointer-events: none, position: absolute) and only render when their
         events arrive, so they cost nothing when idle. InterruptNarrator is
         intentionally omitted because the IDT anchors live exclusively in
         the /interrupciones view. -->
    <ByteFlightNarrator />
    <ModeFlipNarrator :suppress-banner="activeMiddleTab === 'events'" />
    <TrapFrameNarrator />
    <PCBCopyNarrator />
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  flex: 1;
  /* No gap here: the ColumnResizer between columns is the only separator —
     adding gap on top of the resizer width would waste horizontal space. */
  gap: 0;
  padding: 8px;
  overflow: hidden;
}

.middle {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  /* Breathing room only on the two edges that face a resizer. */
  margin: 0;
  /* Without this min-height the children expand by their intrinsic size,
     which lets ConsolePanel push the tabs out of the viewport. With min-height
     at 0 the column honors the flex weights instead. */
  min-height: 0;
}

/* Wrapper around the v-show'd tab panels (Código/Bitácora/Memoria). Only one
   tab is visible at a time, so its single visible child stretches to fill the
   wrapper. The wrapper itself participates in the .middle flex split against
   ConsolePanel via --mc-weight, with the RowResizer in between. */
.middle__tabs-stack {
  flex: var(--mc-weight, 1) 1 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.middle__tabs-stack > * {
  flex: 1;
  min-height: 0;
}

/* Mobile already stacks every panel vertically with its own height rules; the
   resizer makes no sense there. */
@media (max-width: 768px) {
  .middle__row-resizer { display: none; }
  .middle__tabs-stack { flex: 1 1 auto; }
}

.col-left {
  flex-shrink: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* Left column now stacks CPU + IPC, mirroring the right column shape so the
   simulator surfaces both "what is each core running" (top) and "how do
   procs talk to each other" (bottom) on the same rail. */
.col-left--stack {
  display: flex;
  flex-direction: column;
  gap: 0;
  min-width: 0;
  flex-shrink: 0;
}

.col-left--stack > .col-left__panel + .col-left__panel {
  margin-top: 6px;
}

/* Right column stacks two collapsible panels: Procesos and Colas. IPC used
   to live here too but moved to the left rail so the queues panel can grow
   into the extra vertical real estate — animating wake/dispatch arrivals
   needs room to breathe. */
.col-right--stack {
  display: flex;
  flex-direction: column;
  /* RowResizer (when present) provides the only visual gap. Adjacent
     collapsed headers sit flush against each other, which makes the
     "collapsed stack" feel like a single sidebar of titles. */
  gap: 0;
  min-width: 0;
  flex-shrink: 0;
}

/* The flex sizing of each panel inside .col-right--stack lives on
   .cpanel inside CollapsiblePanel — scoped CSS here cannot reach the
   child component's root. We only handle the column itself. */

/* Adjacent panels with no RowResizer between them (i.e. one or both
   collapsed) would otherwise sit flush against each other. A tiny margin
   restores visual separation without reintroducing the resizer's grip. */
.col-right--stack > .col-right__panel + .col-right__panel {
  margin-top: 6px;
}

@media (max-width: 768px) {
  /* When the layout flips to flex-direction: column on mobile, the side
     columns lose their flex sizing and fall back to intrinsic height — the
     stacked CollapsiblePanel children then render as zero-height shells
     because their parents do not own any vertical real estate. Forcing
     `min-height: 0` + `flex: 1 1 auto` (so they participate in the column
     flex split) and a minimum per-panel height restores visible content. */
  .col-left {
    width: 100% !important;
    min-height: 0;
    flex: 1 1 auto;
  }
  .col-right--stack {
    width: 100% !important;
    min-height: 0;
    flex: 1 1 auto;
  }
  .col-left--stack > *,
  .col-right--stack > * { min-height: 160px; }
}

.middle-tabs {
  display: flex;
  gap: 4px;
  /* Container query anchor so the tabs collapse to icon-only when their own
     width gets cramped — without introducing a new viewport breakpoint. */
  container-type: inline-size;
}

.tab {
  flex: 1 1 0;
  min-width: 0;
  font-size: 12px;
  letter-spacing: 1px;
  padding: 6px 10px;
  color: var(--text-dim);
  border-color: var(--border);
  background: var(--panel-soft);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
}

.tab:hover:not(.active):not(.locked):not(:disabled) {
  color: var(--text);
  border-color: var(--border-strong);
  background: color-mix(in oklab, var(--accent) 6%, var(--panel-soft));
}
.tab > svg { flex-shrink: 0; }
.tab > span {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

/* Below this container width the labels do not fit comfortably next to the
   icons. Hide the text and let the title attribute carry meaning — the
   accent on the active tab still signals which view is open. */
@container (max-width: 360px) {
  .tab > span { display: none; }
  .tab { padding: 6px 8px; }
}

.tab.active {
  color: var(--amber);
  border-color: var(--amber);
  background: color-mix(in oklab, var(--accent) 14%, var(--panel-soft));
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.2);
}

.tab.locked,
.mobile-tabs button.locked {
  opacity: 0.5;
  cursor: not-allowed;
  background: var(--panel-soft);
}

.tab.locked:hover,
.mobile-tabs button.locked:hover {
  background: var(--panel-soft);
  color: var(--text-dim);
  border-color: var(--border);
  box-shadow: none;
}

.tab-lock {
  margin-left: 4px;
  color: var(--text-muted);
  opacity: 0.85;
  flex-shrink: 0;
}

.root {
  display: flex;
  flex-direction: column;
  height: 100%;
  flex: 1;
  /* Narrators are absolutely-positioned overlays anchored to this root —
     without an explicit positioning context they would float up to the
     viewport and miss the underlying panel coordinates. */
  position: relative;
}

@keyframes shake-anim {
  0%, 100% { transform: translateX(0); }
  10%, 30%, 50%, 70%, 90% { transform: translateX(-6px); }
  20%, 40%, 60%, 80% { transform: translateX(6px); }
}

.shake {
  animation: shake-anim 500ms cubic-bezier(.36,.07,.19,.97) both;
}

/* Mobile tab bar — oculta en desktop, visible vía media query mobile */
.mobile-tabs {
  display: none;
}

/* ─── MOBILE (≤ 768px) ─────────────────────────────────────────────────────
   En mobile sólo se muestra un panel a la vez, controlado por mobileTab.
   El layout pasa a columna vertical y la mobile-tabs bar es la única forma
   de navegar entre paneles. */
@media (max-width: 768px) {
  .layout {
    flex-direction: column;
    padding: 6px;
    gap: 6px;
    min-height: 0;
  }

  .mobile-tabs {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: 2px;
    padding: 6px;
    background: var(--surface-overlay);
    border-bottom: 1px solid var(--border-strong);
    position: sticky;
    top: 0;
    z-index: 50;
  }

  .mobile-tabs button {
    font-size: 10px;
    padding: 8px 2px;
    letter-spacing: 0.5px;
    color: var(--text-dim);
    border: 1px solid var(--border);
    background: var(--surface-overlay-strong);
    display: inline-flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    min-height: 42px;
    text-transform: uppercase;
  }

  .mobile-tabs button.active {
    color: var(--amber);
    border-color: var(--amber);
    background: rgba(var(--accent-rgb), 0.15);
    box-shadow: 0 0 6px rgba(var(--accent-rgb), 0.3);
  }

  /* Las tabs internas del middle desaparecen — su función la cubre la mobile-tabs bar. */
  .desktop-only {
    display: none !important;
  }
}
</style>
