<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useCpuStore } from '../../stores/cpu';
import {
  CONSOLE_DRAWER_MIN_PX,
  consoleDrawerMaxPx,
  usePreferencesStore,
} from '../../stores/preferences';
import { useBreakpoint } from '../../composables/useBreakpoint';
import { useResizableDrawer } from '../../composables/useResizableDrawer';
import RowResizer from '../shell/RowResizer.vue';
import ConsoleDrawerHandle from './ConsoleDrawerHandle.vue';
import ConsolePanel from '../ConsolePanel.vue';

interface Props {
  forceExpanded?: boolean;
}
const props = defineProps<Props>();

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { isMobile } = useBreakpoint();

const { consoleChunks, kbdReaders, lastIrq21At } = storeToRefs(cpu);

const expanded = computed(() => props.forceExpanded || prefs.consoleDrawer.expanded);
const heightPx = computed(() => prefs.consoleDrawer.heightPx);

const lastOpenedAt = ref(Date.now());
watch(expanded, (next) => {
  if (next) lastOpenedAt.value = Date.now();
});

const irqStillFresh = ref(false);
let irqFreshTimer: number | null = null;
watch(lastIrq21At, (ts) => {
  if (!ts) return;
  irqStillFresh.value = true;
  if (irqFreshTimer !== null) window.clearTimeout(irqFreshTimer);
  irqFreshTimer = window.setTimeout(() => { irqStillFresh.value = false; }, 2000);
});
onUnmounted(() => {
  if (irqFreshTimer !== null) window.clearTimeout(irqFreshTimer);
});

const unseenStdout = computed(() => {
  if (expanded.value) return 0;
  return consoleChunks.value.filter(c => c.kind === 'out' && c.ts > lastOpenedAt.value).length;
});
const kbdReading = computed(() => kbdReaders.value.length > 0);
const irqRecent = computed(() => irqStillFresh.value && lastIrq21At.value > lastOpenedAt.value);

const { start: startResize, isResizing } = useResizableDrawer({
  getHeight: () => prefs.consoleDrawer.heightPx,
  setHeight: (px) => prefs.setConsoleDrawerHeight(px),
  minPx: CONSOLE_DRAWER_MIN_PX,
  maxPx: () => consoleDrawerMaxPx(),
});

const drawerStyle = computed(() => {
  if (props.forceExpanded || isMobile.value) return { flex: '1 1 auto', minHeight: '0' };
  if (!expanded.value) return { flex: '0 0 auto' };
  return { flexBasis: `${heightPx.value}px` };
});

function toggle() {
  if (props.forceExpanded) return;
  prefs.toggleConsoleDrawer();
}
</script>

<template>
  <div
    class="console-drawer"
    :class="{
      'is-expanded': expanded,
      'is-collapsed': !expanded,
      'is-resizing': isResizing,
      'is-mobile': isMobile || forceExpanded,
    }"
    :style="drawerStyle"
  >
    <RowResizer
      v-if="expanded && !isMobile && !forceExpanded"
      class="console-drawer__resizer"
      :active="isResizing"
      @pointerdown="startResize"
    />

    <ConsoleDrawerHandle
      v-if="!forceExpanded"
      :expanded="expanded"
      :unseen-stdout="unseenStdout"
      :kbd-reading="kbdReading"
      :irq-recent="irqRecent"
      @toggle="toggle"
    />

    <div v-show="expanded" class="console-drawer__body">
      <ConsolePanel class="console-drawer__panel" />
    </div>
  </div>
</template>

<style scoped>
.console-drawer {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  transition: flex-basis 180ms ease;
  width: 100%;
  background: var(--bg, transparent);
}

.console-drawer.is-expanded {
  padding-top: 0;
}

.console-drawer.is-resizing {
  transition: none;
}

.console-drawer__resizer {
  flex: 0 0 auto;
}

.console-drawer__body {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  padding: 4px 6px 6px;
}

.console-drawer__panel {
  flex: 1 1 auto;
  min-height: 0;
}

.console-drawer.is-mobile .console-drawer__body {
  padding: 0;
}
</style>
