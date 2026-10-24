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
import { useConnectionBanner } from '../composables/useConnectionBanner';
import MobileMenuButton from '../components/shell/MobileMenuButton.vue';
import GlossaryModal from '../components/GlossaryModal.vue';
import TutorLauncher from '../components/tutor/TutorLauncher.vue';
import ReportModal from '../components/ReportModal.vue';
import WhiteboardCursor from '../components/whiteboard/WhiteboardCursor.vue';
import WhiteboardClickRipple from '../components/whiteboard/WhiteboardClickRipple.vue';
import WhiteboardFocusPill from '../components/whiteboard/WhiteboardFocusPill.vue';
import EventAnnouncer from '../components/a11y/EventAnnouncer.vue';
import AccessibilityMenu from '../components/a11y/AccessibilityMenu.vue';
import { useI18n } from 'vue-i18n';

const cpu = useCpuStore();
const viewer = useViewerStore();
const prefs = usePreferencesStore();
const toast = useToastStore();
const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const { visible: bannerVisible } = useConnectionBanner();

function focusA11y(ev: Event) {
  ev.preventDefault();
  document.getElementById('a11y-menu')?.focus();
}

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
  // SimStatusRow. setVisibleCores replaces the set atomically (commitFilter
  // path) so reactivity, persistence and mirrorFocus run exactly like for
  // a local toggle.
  if (Array.isArray(p.cores) && p.cores.length > 0) {
    cpu.setVisibleCores(p.cores);
  }
  // Reflect the host's mode + narrators into the volatile mirror layer instead
  // of writing the persisted preference. This drives the live view while
  // following without clobbering the spectator's own choice, which must survive
  // a reset or refresh (see preferences store: effectiveMode / setMirror).
  prefs.setMirror({ mode: p.mode, narrators: Boolean(p.narrators) });
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
  { deep: true },
);

// Drop the mirror as soon as autofollow turns off so the spectator falls back
// to their own persisted mode + narrators preference.
watch(
  () => viewer.autofollow,
  (on) => { if (!on) prefs.clearMirror(); },
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
  <div :class="['shell', `shell--mode-${prefs.effectiveMode}`, { 'shell--disconnected': bannerVisible }]">
    <a class="skip-link" href="#a11y-menu" @click="focusA11y">{{ t('a11y.skipLink') }}</a>
    <ConnectionBanner />
    <MobileMenuButton />
    <Sidebar :items="items" />
    <div class="shell__main">
      <ViewerTopBar />
      <SimStatusRow v-if="isSimRoute" class="shell__status-row" :readonly="true" />
      <main id="main-content" class="shell__content" data-pizarra-anchor="shell-content">
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

    <TutorLauncher />
    <ReportModal />
    <WhiteboardCursor />
    <WhiteboardClickRipple />
    <WhiteboardFocusPill />
    <EventAnnouncer />
    <AccessibilityMenu />
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

/* Mobile: the content area becomes the single vertical scroll container so a
   section taller than the viewport stays reachable. Desktop keeps the fixed
   layout that fills the viewport with per-panel scrolls. */
@media (max-width: 768px) {
  .shell__content {
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    padding-bottom: env(safe-area-inset-bottom);
  }
}

.shell--mode-pedagogic { --row-density: 1.15; font-size: calc(16px * var(--a11y-text-scale, 1)); }
.shell--mode-projector { --row-density: 1.4; font-size: calc(18px * var(--a11y-text-scale, 1)); }

.route-enter-from { opacity: 0; transform: translateY(8px); }
.route-leave-to   { opacity: 0; transform: translateY(-6px); }
.route-enter-active,
.route-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
</style>
