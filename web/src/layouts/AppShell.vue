<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import {
  LayoutDashboard,
  ListOrdered,
  MemoryStick,
  Zap,
  Layers,
  BookOpen,
  Settings,
  UserRound,
  ShieldUser,
} from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import { useAuthStore } from '../stores/auth';
import { useAppConfigStore } from '../stores/appConfig';
import { useIsStudent } from '../composables/useIsStudent';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import SimBar from '../components/shell/SimBar.vue';
import ConnectionBanner from '../components/shell/ConnectionBanner.vue';
import { useConnectionBanner } from '../composables/useConnectionBanner';
import MobileMenuButton from '../components/shell/MobileMenuButton.vue';
import GlossaryModal from '../components/GlossaryModal.vue';
import ConsoleDrawer from '../components/console/ConsoleDrawer.vue';
import TutorLauncher from '../components/tutor/TutorLauncher.vue';
import { useBreakpoint } from '../composables/useBreakpoint';
import WhiteboardCursor from '../components/whiteboard/WhiteboardCursor.vue';
import WhiteboardClickRipple from '../components/whiteboard/WhiteboardClickRipple.vue';
import WhiteboardFocusPill from '../components/whiteboard/WhiteboardFocusPill.vue';
import ViewerShell from './ViewerShell.vue';
import { useWhiteboardStore } from '../stores/whiteboard';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const auth = useAuthStore();
const appCfg = useAppConfigStore();
const isStudent = useIsStudent();
const whiteboard = useWhiteboardStore();
const route = useRoute();
const { t, locale } = useI18n();
const { visible: bannerVisible } = useConnectionBanner();
const { isMobile } = useBreakpoint();

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
  // Fetch the identity so the sidebar can surface the "Panel admin" link to
  // moderators / DB-admins on first load. If the user is anonymous fetchMe
  // returns false and the entry stays hidden.
  if (!auth.user) auth.fetchMe();
  // Pull the admin-curated UI lockdowns so the sidebar and simulator can
  // render the right padlocks on entry. Idempotent (the store guards
  // against double-fetch via `loaded`).
  appCfg.load();
});

// The breadcrumb was removed from the topbar, so the section name now lives
// only in document.title (and in the active sidebar item). This avoids a
// redundant chrome row. Reacts both to navigation and to locale changes so a
// user who switches ES → EN sees the tab title update immediately.
watch(
  [() => route.meta.titleKey, locale],
  ([titleKey]) => {
    const label = typeof titleKey === 'string' && titleKey ? t(titleKey) : '';
    const doc = label ? `${label} · MODARCH` : 'MODARCH';
    if (typeof document !== 'undefined') document.title = doc;
  },
  { immediate: true },
);

// Close the mobile drawer on every navigation so a tap that ends up on the
// same route also clears the overlay.
watch(() => route.path, () => {
  if (prefs.sidebarMobileOpen) prefs.closeSidebarMobile();
});

// The "Panel admin" item only appears for users whose backend-reported role
// gives them /admin access. The link itself navigates out of the AppShell —
// AdminShell handles its own gating, so this entry is purely a discovery
// affordance, not the authorization boundary.
const items = computed(() => {
  const sim = t('shell.groups.simulation');
  const tools = t('shell.groups.tools');
  // The route `name` here mirrors web/src/router/index.ts and the
  // backend's uiValidRouteNames whitelist. Keep all three in sync.
  const base: Array<{
    to: string;
    name: string;
    label: string;
    icon: typeof LayoutDashboard;
    group: string;
    locked?: boolean;
  }> = [
    { to: '/', name: 'overview', label: t('router.titles.overview'), icon: LayoutDashboard, group: sim },
    { to: '/procesos', name: 'processes', label: t('router.titles.processes'), icon: ListOrdered, group: sim },
    { to: '/memoria', name: 'memory', label: t('router.titles.memory'), icon: MemoryStick, group: sim },
    { to: '/interrupciones', name: 'interrupts', label: t('router.titles.interrupts'), icon: Zap, group: sim },
    { to: '/concurrencia', name: 'concurrency', label: t('router.titles.concurrency'), icon: Layers, group: sim },
    { to: '/glosario', name: 'glossary', label: t('router.titles.glossary'), icon: BookOpen, group: tools },
    { to: '/preferencias', name: 'preferences', label: t('router.titles.preferences'), icon: Settings, group: tools },
    { to: '/perfil', name: 'profile', label: t('router.titles.profile'), icon: UserRound, group: tools },
  ];
  if (isStudent.value) {
    for (const item of base) {
      if (appCfg.disabledRoutes.has(item.name)) item.locked = true;
    }
  }
  const role = auth.user?.role;
  if (role === 'admin' || role === 'moderator') {
    base.push({ to: '/admin', name: 'admin-dashboard', label: t('router.titles.adminPanel'), icon: ShieldUser, group: tools });
  }
  return base;
});

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

// Whiteboard: while the owner has the toggle on, sample the pointer at 20fps
// and broadcast DOM-anchored coords. The handler resolves the element under
// the mouse and walks up to the nearest [data-pizarra-anchor] so the viewer
// can reproject the cursor onto the same panel even when its own viewport
// is smaller or its sidebar is collapsed. The watcher binds/unbinds the
// listener based on role + toggle so non-owners never carry the cost.
const POINTER_HZ = 20;
const POINTER_INTERVAL_MS = Math.round(1000 / POINTER_HZ);
const SHELL_CONTENT_FALLBACK = 'shell-content';
let pointerLastSent = 0;
let pointerLastAnchor = '';
let pointerLastOX = -1;
let pointerLastOY = -1;

function resolveAnchor(clientX: number, clientY: number): { anchor: string; offsetX: number; offsetY: number } | null {
  const target = document.elementFromPoint(clientX, clientY);
  let host: Element | null = target ? target.closest('[data-pizarra-anchor]') : null;
  if (!host) {
    // Fallback to the shared content area present in both shells. Tagged via
    // the same data attribute on .shell__content below.
    host = document.querySelector(`[data-pizarra-anchor="${SHELL_CONTENT_FALLBACK}"]`);
  }
  if (!host) return null;
  const anchor = host.getAttribute('data-pizarra-anchor') || SHELL_CONTENT_FALLBACK;
  const rect = host.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  const offsetX = (clientX - rect.left) / rect.width;
  const offsetY = (clientY - rect.top) / rect.height;
  if (offsetX < 0 || offsetX > 1 || offsetY < 0 || offsetY > 1) return null;
  return { anchor, offsetX, offsetY };
}

function onPointerMove(ev: PointerEvent) {
  const now = performance.now();
  if (now - pointerLastSent < POINTER_INTERVAL_MS) return;
  const resolved = resolveAnchor(ev.clientX, ev.clientY);
  if (!resolved) return;
  // Skip identical-coord echoes (cursor parked) to spare the wire.
  if (
    resolved.anchor === pointerLastAnchor &&
    resolved.offsetX === pointerLastOX &&
    resolved.offsetY === pointerLastOY
  ) {
    return;
  }
  pointerLastSent = now;
  pointerLastAnchor = resolved.anchor;
  pointerLastOX = resolved.offsetX;
  pointerLastOY = resolved.offsetY;
  cpu.sendPointer(resolved.anchor, resolved.offsetX, resolved.offsetY, route.path);
}

// Click capture: while broadcasting, every click the owner makes also emits
// a ripple so viewers see what the owner is signaling. We use the capture
// phase so the listener fires before any local handler can preventDefault,
// and we never preventDefault ourselves — the click still does its normal
// thing locally.
function onPointerClick(ev: MouseEvent) {
  const resolved = resolveAnchor(ev.clientX, ev.clientY);
  if (!resolved) return;
  cpu.sendClick(resolved.anchor, resolved.offsetX, resolved.offsetY, route.path);
}

function attachPointer() {
  window.addEventListener('pointermove', onPointerMove, { passive: true });
  window.addEventListener('click', onPointerClick, true);
}
function detachPointer() {
  window.removeEventListener('pointermove', onPointerMove);
  window.removeEventListener('click', onPointerClick, true);
}

watch(
  [() => cpu.sessionInfo.role, () => whiteboard.broadcasting],
  ([role, broadcasting], [prevRole, prevBroadcasting] = [undefined, undefined]) => {
    const active = role === 'owner' && broadcasting;
    const wasActive = prevRole === 'owner' && prevBroadcasting;
    if (active && !wasActive) attachPointer();
    if (!active && wasActive) {
      detachPointer();
      // If we just turned off (or lost the owner role), make sure viewers
      // drop the cursor promptly instead of waiting for the staleness timer.
      cpu.sendPointerOff();
    }
  },
  { immediate: true },
);

onBeforeUnmount(detachPointer);

const shellRef = ref<HTMLElement | null>(null);
let topChromeObserver: ResizeObserver | null = null;

function observeTopChrome() {
  if (!shellRef.value) return;
  const target = shellRef.value.querySelector('.sim-bar, .topbar') as HTMLElement | null;
  if (!target) {
    shellRef.value.style.setProperty('--shell-top-h', '48px');
    return;
  }
  topChromeObserver?.disconnect();
  topChromeObserver = new ResizeObserver(([entry]) => {
    const h = Math.round(entry.contentRect.height);
    shellRef.value?.style.setProperty('--shell-top-h', `${h}px`);
  });
  topChromeObserver.observe(target);
  shellRef.value.style.setProperty('--shell-top-h', `${Math.round(target.getBoundingClientRect().height)}px`);
}

watch(() => route.path, () => nextTick(observeTopChrome), { flush: 'post' });
onMounted(() => nextTick(observeTopChrome));
onBeforeUnmount(() => topChromeObserver?.disconnect());
</script>

<template>
  <!-- Viewer role gets its own shell (read-only topbar, sidebar with only
       sim-relevant items, presence-driven autofollow). Owners keep the host
       shell below. The role flips on session_info events from the WS. -->
  <ViewerShell v-if="cpu.isViewer()" />
  <div
    v-else
    ref="shellRef"
    :class="['shell', `shell--mode-${prefs.mode}`, { 'shell--disconnected': bannerVisible }]"
  >
    <ConnectionBanner />
    <MobileMenuButton />
    <Sidebar :items="items" />
    <div class="shell__main">
      <SimBar v-if="isSimRoute" />
      <Topbar v-else />
      <main class="shell__content" data-pizarra-anchor="shell-content">
        <RouterView v-slot="{ Component }">
          <Transition name="route" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>
      <ConsoleDrawer v-if="!isMobile" />
    </div>

    <GlossaryModal
      :open="glossaryOpen"
      :initial-search="cpu.glossarySearch"
      @close="glossaryOpen = false"
    />

    <TutorLauncher />

    <!-- Whiteboard overlay: paints the owner's pointer for viewers. Cheap
         to keep mounted (renders nothing until an owner_pointer arrives). -->
    <WhiteboardCursor />
    <WhiteboardClickRipple />
    <WhiteboardFocusPill />
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

@media (max-width: 768px) {
  .shell--disconnected { padding-top: 28px; }
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
