<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Cpu, Code2, ListTree, MemoryStick, Boxes, Terminal } from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { useViewerMirrorStore } from '../stores/viewerMirror';
import CpuPanel from '../components/CpuPanel.vue';
import EventsLog from '../components/EventsLog.vue';
import ProcessesPanel from '../components/ProcessesPanel.vue';
import ConsolePanel from '../components/ConsolePanel.vue';
import MemoryPanel from '../components/MemoryPanel.vue';
import CodeAsmSplit from '../components/CodeAsmSplit.vue';
import ByteFlightNarrator from '../components/narrators/ByteFlightNarrator.vue';
import ModeFlipNarrator from '../components/narrators/ModeFlipNarrator.vue';
import PCBCopyNarrator from '../components/narrators/PCBCopyNarrator.vue';

const cpu = useCpuStore();
const mirror = useViewerMirrorStore();
const { t } = useI18n();

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
  localMiddleTab.value = val;
  if (val === 'code' || val === 'events' || val === 'memory') {
    localMobileTab.value = val;
  }
  if (!isViewer.value) {
    cpu.sendFocus({ middle_tab: val, mobile_tab: val });
  }
}

function setMobileTab(val: MobileTab) {
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

onMounted(() => {
  cpu.connect();
  checkMobile();
  window.addEventListener('resize', checkMobile);
});

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile);
});
</script>

<template>
  <div :class="{ shake: cpu.faultShake }" class="root" data-pizarra-anchor="view-overview">
    <!-- Tab bar mobile-only — controla todo el layout. Sólo visible <= 768px (CSS) -->
    <nav class="mobile-tabs" :aria-hidden="!isMobile">
      <button :class="{ active: activeMobileTab === 'cpu' }" @click="setMobileTab('cpu')" :title="t('tabs.cpuTitle')"><Cpu :size="14" :stroke-width="2" /><span>{{ t('tabs.cpu') }}</span></button>
      <button :class="{ active: activeMobileTab === 'code' }" @click="setMobileTab('code')" :title="t('tabs.codeTitle')"><Code2 :size="14" :stroke-width="2" /><span>{{ t('tabs.cod') }}</span></button>
      <button :class="{ active: activeMobileTab === 'events' }" @click="setMobileTab('events')" :title="t('tabs.eventsTitle')"><ListTree :size="14" :stroke-width="2" /><span>{{ t('tabs.log') }}</span></button>
      <button :class="{ active: activeMobileTab === 'memory' }" @click="setMobileTab('memory')" :title="t('tabs.memoryTitle')"><MemoryStick :size="14" :stroke-width="2" /><span>{{ t('tabs.mem') }}</span></button>
      <button :class="{ active: activeMobileTab === 'procs' }" @click="setMobileTab('procs')" :title="t('tabs.procsTitle')"><Boxes :size="14" :stroke-width="2" /><span>{{ t('tabs.proc') }}</span></button>
      <button :class="{ active: activeMobileTab === 'console' }" @click="setMobileTab('console')" :title="t('tabs.consoleTitle')"><Terminal :size="14" :stroke-width="2" /><span>{{ t('tabs.cons') }}</span></button>
    </nav>

    <main class="layout">
      <CpuPanel v-show="visible.cpu" class="col-left" />
      <div v-show="visible.middle" class="middle col-center">
        <div class="middle-tabs desktop-only">
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'code' }"
            @click="setMiddleTab('code')"
            :title="t('tabs.codeC_long')"
          ><Code2 :size="14" :stroke-width="2" /><span>{{ t('tabs.codeC') }}</span></button>
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'events' }"
            @click="setMiddleTab('events')"
            :title="t('tabs.eventsLong')"
          ><ListTree :size="14" :stroke-width="2" /><span>{{ t('tabs.eventLog') }}</span></button>
          <button
            class="tab"
            :class="{ active: activeMiddleTab === 'memory' }"
            @click="setMiddleTab('memory')"
            :title="t('tabs.memoryLong')"
          ><MemoryStick :size="14" :stroke-width="2" /><span>{{ t('tabs.memory') }}</span></button>
        </div>
        <CodeAsmSplit v-show="visible.code" />
        <EventsLog v-show="visible.events" />
        <MemoryPanel v-show="visible.memory" />
        <ConsolePanel v-show="visible.console" />
      </div>
      <ProcessesPanel v-show="visible.procs" class="col-right" />
    </main>

    <!-- Narrators are mounted at the Overview root so byte-flight, PCB-copy
         and mode-flip animations are visible in the general view, not only in
         /memoria, /procesos and /interrupciones. They are pure overlays
         (pointer-events: none, position: absolute) and only render when their
         events arrive, so they cost nothing when idle. InterruptNarrator is
         intentionally omitted because the IDT anchors live exclusively in
         the /interrupciones view. -->
    <ByteFlightNarrator />
    <ModeFlipNarrator />
    <PCBCopyNarrator />

    <div class="credit" :title="t('credit.hint')">
      <span class="credit-tag">{{ t('credit.tag') }}</span>
      <span class="credit-name">{{ t('credit.name') }}</span>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  flex: 1;
  gap: 8px;
  padding: 8px;
  overflow: hidden;
}

.middle {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
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
  background: var(--surface-overlay);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
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
  background: rgba(var(--accent-rgb), 0.1);
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.2);
}

.credit {
  position: fixed;
  bottom: 6px;
  right: 10px;
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 1px;
  pointer-events: none;
  user-select: none;
  font-family: 'JetBrains Mono', monospace;
  line-height: 1.1;
  opacity: 0.65;
  transition: opacity 0.2s ease;
}

.credit:hover {
  opacity: 1;
}

.credit-tag {
  font-family: var(--font-display);
  font-size: 11px;
  color: var(--accent);
  letter-spacing: 3px;
  text-shadow: var(--shadow-glow-amber);
}

.credit-name {
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 1px;
  text-transform: uppercase;
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

  /* Credit más discreto en mobile, no estorba */
  .credit {
    bottom: 4px;
    right: 6px;
    opacity: 0.35;
  }

  .credit-tag { font-size: 9px; letter-spacing: 2px; }
  .credit-name { font-size: 8px; }
}
</style>
