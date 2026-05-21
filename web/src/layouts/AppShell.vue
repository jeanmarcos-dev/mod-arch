<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import {
  LayoutDashboard,
  ListOrdered,
  MemoryStick,
  Zap,
  Layers,
  FolderClock,
  BookOpen,
  Settings,
  UserRound,
} from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import SimBar from '../components/shell/SimBar.vue';
import ConnectionBanner from '../components/shell/ConnectionBanner.vue';
import GlossaryModal from '../components/GlossaryModal.vue';
import ViewerShell from './ViewerShell.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const route = useRoute();

// Any cpu.openGlossary('EIP') call from a panel opens the popup right where
// the student clicked. The full /glosario page is still reachable from the
// sidebar for browse-the-whole-thing scenarios.
const glossaryOpen = ref(false);
watch(() => cpu.glossaryOpenSignal, (n, prev) => {
  if (n === prev) return;
  glossaryOpen.value = true;
});

// Connect once when the shell mounts. The shell lives for the whole student
// session, so child route changes don't trigger a reconnect. cpu.connect is
// idempotent — calls from Simulator.vue (for the viewer route) are no-ops
// here.
onMounted(() => {
  cpu.connect();
});

// The breadcrumb was removed from the topbar, so the section name now lives
// only in document.title (and in the active sidebar item). This avoids a
// redundant chrome row.
watch(
  () => route.meta.title,
  (title) => {
    const t = typeof title === 'string' && title ? `${title} · modos-cpu` : 'modos-cpu';
    if (typeof document !== 'undefined') document.title = t;
  },
  { immediate: true },
);

const items = [
  { to: '/', label: 'Vista general', icon: LayoutDashboard, group: 'SIMULACIÓN' },
  { to: '/procesos', label: 'Procesos', icon: ListOrdered, group: 'SIMULACIÓN' },
  { to: '/memoria', label: 'Memoria', icon: MemoryStick, group: 'SIMULACIÓN' },
  { to: '/interrupciones', label: 'Interrupciones', icon: Zap, group: 'SIMULACIÓN' },
  { to: '/concurrencia', label: 'Concurrencia', icon: Layers, group: 'SIMULACIÓN' },
  { to: '/sesiones', label: 'Mis sesiones', icon: FolderClock, group: 'HERRAMIENTAS' },
  { to: '/glosario', label: 'Glosario', icon: BookOpen, group: 'HERRAMIENTAS' },
  { to: '/preferencias', label: 'Preferencias', icon: Settings, group: 'HERRAMIENTAS' },
  { to: '/perfil', label: 'Perfil', icon: UserRound, group: 'HERRAMIENTAS' },
];

// Routes where the SimBar (controls + status chips) is meaningful. Tool
// routes like /perfil, /preferencias, /glosario, /sesiones don't drive the
// simulator, so the bar is hidden to give them full vertical space.
const isSimRoute = computed(() => {
  const p = route.path;
  return p === '/'
    || p === '/procesos'
    || p === '/memoria'
    || p === '/interrupciones'
    || p === '/concurrencia';
});

// Host-side presence: every time the owner navigates inside a sim route,
// changes the visible-cores filter, switches the presentation mode preset
// or toggles narrators, we publish so autofollowing viewers mirror the
// same view. Tool routes are skipped — the host's "Preferences" page is
// irrelevant to a spectator and broadcasting it would yank the viewer out
// of whatever section they were actually following.
//
// The role is part of the key so that the first session_info that flips us
// to "owner" also triggers an initial broadcast.
watch(
  [
    () => cpu.sessionInfo.role,
    () => route.path,
    () => Array.from(cpu.selectedCoreIds).sort((a, b) => a - b).join(','),
    () => prefs.mode,
    () => prefs.narratorsEnabled,
  ],
  () => {
    if (cpu.sessionInfo.role !== 'owner') return;
    if (!isSimRoute.value) return;
    const cores = Array.from(cpu.selectedCoreIds).sort((a, b) => a - b);
    cpu.sendPresence({
      section: route.path,
      cores,
      mode: prefs.mode,
      narrators: prefs.narratorsEnabled,
    });
  },
  { immediate: true },
);
</script>

<template>
  <!-- Viewer role gets its own shell (read-only topbar, sidebar with only
       sim-relevant items, presence-driven autofollow). Owners keep the host
       shell below. The role flips on session_info events from the WS. -->
  <ViewerShell v-if="cpu.isViewer()" />
  <div v-else :class="['shell', `shell--mode-${prefs.mode}`, { 'shell--disconnected': !cpu.connected }]">
    <ConnectionBanner />
    <Sidebar :items="items" />
    <div class="shell__main">
      <SimBar v-if="isSimRoute" />
      <Topbar v-else />
      <main class="shell__content">
        <RouterView v-slot="{ Component }">
          <Transition name="route" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>
    </div>

    <GlossaryModal
      :open="glossaryOpen"
      :initial-search="cpu.glossarySearch"
      @close="glossaryOpen = false"
    />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
  box-sizing: border-box;
  transition: padding-top var(--transition-base);
}

/* When the ConnectionBanner is visible we reserve 36px at the top of the
   whole shell so it does not occlude the sidebar header or the SimBar. */
.shell--disconnected {
  padding-top: 36px;
}

.shell__main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.shell__content {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* Mode-driven density and font-size knobs. Sections opt in by referencing the
   --row-density variable and font-size: inherit. */
.shell--mode-pedagogic { --row-density: 1.15; font-size: 16px; }
.shell--mode-projector { --row-density: 1.4; font-size: 18px; }

/* Route transition: gentle slide+fade so changing sections feels light but
   noticeable. Honors prefers-reduced-motion via the global rule in style.css. */
.route-enter-from { opacity: 0; transform: translateY(8px); }
.route-leave-to   { opacity: 0; transform: translateY(-6px); }
.route-enter-active,
.route-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
</style>
