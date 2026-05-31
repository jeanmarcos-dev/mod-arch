<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useRoute, useRouter } from 'vue-router';
import { LogIn } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import {
  CONSOLE_DRAWER_MIN_PX,
  consoleDrawerMaxPx,
  usePreferencesStore,
} from '../../stores/preferences';
import { useTutorStore, type TutorCitation } from '../../stores/tutor';
import { useToastStore } from '../../stores/toast';
import { useAuthStore } from '../../stores/auth';
import { useBreakpoint } from '../../composables/useBreakpoint';
import { useResizableDrawer } from '../../composables/useResizableDrawer';
import RowResizer from '../shell/RowResizer.vue';
import ConsoleDrawerHandle, { type ConsoleDrawerTab } from './ConsoleDrawerHandle.vue';
import ConsolePanel from '../ConsolePanel.vue';
import TutorPanel from '../tutor/TutorPanel.vue';
import TutorBotIcon from '../tutor/TutorBotIcon.vue';

interface Props {
  forceExpanded?: boolean;
}
const props = defineProps<Props>();

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const tutor = useTutorStore();
const toast = useToastStore();
const auth = useAuthStore();
const router = useRouter();
const route = useRoute();
const { isMobile } = useBreakpoint();

const { consoleChunks, kbdReaders, lastIrq21At } = storeToRefs(cpu);

const expanded = computed(() => props.forceExpanded || prefs.consoleDrawer.expanded);
const heightPx = computed(() => prefs.consoleDrawer.heightPx);

const activeTab = ref<ConsoleDrawerTab>('console');

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

const tutorActive = computed(() => expanded.value && activeTab.value === 'modarchi');
watch(tutorActive, (active) => {
  tutor.setOpen(active);
}, { immediate: true });

const unseenStdout = computed(() => {
  if (expanded.value && activeTab.value === 'console') return 0;
  return consoleChunks.value.filter(c => c.kind === 'out' && c.ts > lastOpenedAt.value).length;
});
const kbdReading = computed(() => kbdReaders.value.length > 0);
const irqRecent = computed(() => irqStillFresh.value && lastIrq21At.value > lastOpenedAt.value);
const unreadModarchi = computed(() => (tutorActive.value ? 0 : tutor.unreadCount));

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

function selectTab(tab: ConsoleDrawerTab) {
  activeTab.value = tab;
}

const needsLogin = computed(() => !auth.user);

function goToLogin() {
  router.push({ name: 'login', query: { next: route.fullPath } });
  activeTab.value = 'console';
}

function onTutorClose() {
  activeTab.value = 'console';
}

function handleCite(citation: TutorCitation) {
  switch (citation.source) {
    case 'glossary':
      cpu.openGlossary(citation.id);
      return;
    case 'demos':
      cpu.load(citation.id);
      toast.ok(`Cargando demo "${citation.id}"`);
      return;
    case 'isa':
      toast.info(`ISA — ${citation.label.replace(/^\[ISA:\s*/, '').replace(/\]$/, '')}`);
      return;
    case 'docs':
      toast.info(`Documentación — ${citation.label.replace(/^\[Docs:\s*/, '').replace(/\]$/, '')}`);
      return;
    case 'silberschatz':
    default: {
      const refLabel = citation.page ? `página ${citation.page}` : citation.id;
      toast.info(`Referencia: Silberschatz, ${refLabel}`);
    }
  }
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
      :active-tab="activeTab"
      :unseen-stdout="unseenStdout"
      :kbd-reading="kbdReading"
      :irq-recent="irqRecent"
      :unread-modarchi="unreadModarchi"
      @toggle="toggle"
      @select="selectTab"
    />

    <div v-show="expanded" class="console-drawer__body">
      <ConsolePanel
        v-show="activeTab === 'console'"
        class="console-drawer__panel"
      />
      <template v-if="activeTab === 'modarchi'">
        <div v-if="needsLogin" class="modarchi-gate" role="dialog" aria-label="Inicie sesión">
          <div class="modarchi-gate__icon"><TutorBotIcon :size="32" :stroke-width="1.4" /></div>
          <h3 class="modarchi-gate__title">MODARCHI protegido</h3>
          <p class="modarchi-gate__text">
            Para conversar con MODARCHI sobre Sistemas Operativos debe
            iniciar sesión con su correo institucional.
          </p>
          <button class="modarchi-gate__cta" type="button" @click="goToLogin">
            <LogIn :size="14" :stroke-width="2" />
            Iniciar sesión
          </button>
          <p class="modarchi-gate__hint">El simulador sigue siendo de uso libre.</p>
        </div>
        <TutorPanel
          v-else
          class="console-drawer__panel"
          @close="onTutorClose"
          @cite="handleCite"
        />
      </template>
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

.modarchi-gate {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  text-align: center;
  gap: 0.6rem;
  background: var(--surface-1);
  border: 1px solid rgba(var(--accent-rgb), 0.45);
}

.modarchi-gate__icon {
  color: var(--accent);
  opacity: 0.85;
  filter: drop-shadow(0 0 12px rgba(var(--accent-rgb), 0.35));
}

.modarchi-gate__title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 1rem;
  letter-spacing: 3px;
  color: var(--accent);
  text-transform: uppercase;
}

.modarchi-gate__text {
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--text);
  max-width: 28rem;
}

.modarchi-gate__cta {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  margin-top: 0.4rem;
  padding: 0.55rem 1.1rem;
  background: rgba(var(--accent-rgb), 0.22);
  border: 1px solid var(--accent);
  color: var(--accent);
  border-radius: var(--radius-sm);
  font-family: inherit;
  font-size: 0.85rem;
  letter-spacing: 0.5px;
  cursor: pointer;
  transition: background 0.15s;
}

.modarchi-gate__cta:hover {
  background: rgba(var(--accent-rgb), 0.34);
}

.modarchi-gate__hint {
  margin: 0;
  font-size: 0.7rem;
  color: var(--text-muted);
}
</style>
