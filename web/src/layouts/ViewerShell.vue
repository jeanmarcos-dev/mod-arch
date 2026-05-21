<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  LayoutDashboard,
  ListOrdered,
  MemoryStick,
  Zap,
  Layers,
  BookOpen,
} from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { useViewerStore } from '../stores/viewer';
import { usePreferencesStore } from '../stores/preferences';
import { useToastStore } from '../stores/toast';
import Sidebar from '../components/shell/Sidebar.vue';
import ViewerTopBar from '../components/shell/ViewerTopBar.vue';
import SimStatusRow from '../components/shell/SimStatusRow.vue';
import ConnectionBanner from '../components/shell/ConnectionBanner.vue';
import MobileMenuButton from '../components/shell/MobileMenuButton.vue';
import GlossaryModal from '../components/GlossaryModal.vue';
import WhiteboardCursor from '../components/whiteboard/WhiteboardCursor.vue';
import WhiteboardClickRipple from '../components/whiteboard/WhiteboardClickRipple.vue';

const cpu = useCpuStore();
const viewer = useViewerStore();
const prefs = usePreferencesStore();
const toast = useToastStore();
const route = useRoute();
const router = useRouter();

// Sidebar items reflect the section routes the viewer can navigate to.
// Tool-only routes (preferencias, perfil, sesiones) are hidden — they have no
// meaning for a spectator who does not own the session.
const items = [
  { to: '/', label: 'Vista general', icon: LayoutDashboard, group: 'SIMULACIÓN' },
  { to: '/procesos', label: 'Procesos', icon: ListOrdered, group: 'SIMULACIÓN' },
  { to: '/memoria', label: 'Memoria', icon: MemoryStick, group: 'SIMULACIÓN' },
  { to: '/interrupciones', label: 'Interrupciones', icon: Zap, group: 'SIMULACIÓN' },
  { to: '/concurrencia', label: 'Concurrencia', icon: Layers, group: 'SIMULACIÓN' },
  { to: '/glosario', label: 'Glosario', icon: BookOpen, group: 'HERRAMIENTAS' },
];

// The SimStatusRow only makes sense on sim-bound routes — toolish routes
// (glosario) would benefit from extra vertical space instead.
const isSimRoute = computed(() => {
  const p = route.path;
  return p === '/'
    || p === '/procesos'
    || p === '/memoria'
    || p === '/interrupciones'
    || p === '/concurrencia';
});

// Autofollow: when the host's presence updates, follow them. Mirrors the
// section, the visible-cores filter, the presentation mode preset and the
// narrators toggle. If the viewer navigates manually we treat that as an
// opt-out (see watch on $route below) and stop following until they
// re-enable it.
const knownSections = new Set<string>(items.map((i) => i.to));
type ApplyArgs = {
  section: string;
  cores: number[];
  mode: string;
  narrators: boolean;
};
function applyPresence(p: ApplyArgs) {
  if (!knownSections.has(p.section)) return;
  if (route.path !== p.section) {
    router.push(p.section).catch(() => undefined);
  }
  // Mirror visible cores so the host's filter selection drives the viewer's
  // SimStatusRow. The replace API is symmetric to what selectOnlyCore does.
  if (Array.isArray(p.cores) && p.cores.length > 0) {
    cpu.selectedCoreIds.clear();
    for (const id of p.cores) cpu.selectedCoreIds.add(id);
  }
  // Assign the mode ref directly instead of calling setMode — setMode has a
  // side effect that flips narrators on for non-compact presets, which would
  // fight against the explicit narrators flag the host just broadcast.
  if (p.mode === 'compact' || p.mode === 'pedagogic' || p.mode === 'projector') {
    prefs.mode = p.mode;
  }
  prefs.narratorsEnabled = Boolean(p.narrators);
}

watch(
  () => cpu.lastPresence,
  (p) => {
    if (!viewer.autofollow) return;
    if (!p.section) return;
    applyPresence({
      section: p.section,
      cores: p.cores,
      mode: p.mode,
      narrators: p.narrators,
    });
  },
  { deep: true, immediate: true },
);

// Manual navigation while autofollow is on: respect the user's intent by
// turning autofollow off (with a discreet toast so they know). Track the
// last presence-driven navigation so it does not count as a manual intent.
const lastAutoSection = ref<string>('');
watch(() => cpu.lastPresence.section, (s) => { lastAutoSection.value = s; });

watch(
  () => route.path,
  (newPath, oldPath) => {
    // Close the mobile drawer on any navigation so the overlay does not
    // linger after the user picked a section.
    if (prefs.sidebarMobileOpen) prefs.closeSidebarMobile();

    if (!oldPath || newPath === oldPath) return;
    if (!viewer.autofollow) return;
    // If we landed here because of an applyPresence call, the path will match
    // the latest broadcasted section. Anything else is the user clicking on
    // their own.
    if (newPath === lastAutoSection.value) return;
    viewer.setAutofollow(false);
    toast.push('Autofollow desactivado — clic en el botón Autofollow para volver a seguir al profesor', 'info');
  },
);

// On mount: if the initial presence has a section we are not on, snap there
// so a late-joining viewer lands where the professor is presenting.
onMounted(() => {
  cpu.connect();
  if (viewer.autofollow && cpu.lastPresence.section) {
    applyPresence({
      section: cpu.lastPresence.section,
      cores: cpu.lastPresence.cores,
      mode: cpu.lastPresence.mode,
      narrators: cpu.lastPresence.narrators,
    });
  }
});

// Hand raise toasts are owner-side, but the viewer also benefits from
// seeing their own gesture acknowledged. The cpu store bumps handRaiseSignal
// every time the host's payload reaches *anyone* — for the viewer this is
// effectively a no-op since the broadcast is owner-only.

// Open the glossary modal when any component calls cpu.openGlossary().
const glossaryOpen = ref(false);
watch(() => cpu.glossaryOpenSignal, (n, prev) => {
  if (n === prev) return;
  glossaryOpen.value = true;
});
</script>

<template>
  <div :class="['shell', `shell--mode-${prefs.mode}`, { 'shell--disconnected': !cpu.connected }]">
    <ConnectionBanner />
    <MobileMenuButton />
    <Sidebar :items="items" />
    <div class="shell__main">
      <ViewerTopBar />
      <SimStatusRow v-if="isSimRoute" class="shell__status-row" :readonly="true" />
      <main class="shell__content" data-pizarra-anchor="shell-content">
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

    <WhiteboardCursor />
    <WhiteboardClickRipple />
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

.shell--disconnected {
  padding-top: 36px;
}

@media (max-width: 768px) {
  .shell--disconnected { padding-top: 28px; }
}

.shell__main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.shell__status-row {
  border-bottom: 1px solid var(--border);
  background: var(--surface-1);
}

.shell__content {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.shell--mode-pedagogic { --row-density: 1.15; font-size: 16px; }
.shell--mode-projector { --row-density: 1.4; font-size: 18px; }

.route-enter-from { opacity: 0; transform: translateY(8px); }
.route-leave-to   { opacity: 0; transform: translateY(-6px); }
.route-enter-active,
.route-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
</style>
