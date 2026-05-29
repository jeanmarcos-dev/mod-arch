<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { LogIn, Minus } from '@lucide/vue';

import TutorPanel from './TutorPanel.vue';
import TutorBotIcon from './TutorBotIcon.vue';
import { useTutorStore, type TutorCitation } from '../../stores/tutor';
import { useCpuStore } from '../../stores/cpu';
import { useToastStore } from '../../stores/toast';
import { useAuthStore } from '../../stores/auth';
import { useSimBarStore } from '../../stores/simBar';
import { usePreferencesStore } from '../../stores/preferences';
import { useBreakpoint } from '../../composables/useBreakpoint';
import { RESET_FAB_EVENT } from '../../composables/useResetUi';

const tutor = useTutorStore();
const cpu = useCpuStore();
const toast = useToastStore();
const auth = useAuthStore();
const bar = useSimBarStore();
const prefs = usePreferencesStore();
const { isMobile } = useBreakpoint();
const router = useRouter();
const route = useRoute();

// Dim the FAB whenever a foreground tool needs the bottom of the viewport on
// mobile. Opacity + pointer-events only — keeping z-index avoids a sudden
// shadow pop when the overlay closes.
const isDimmed = computed(() =>
  isMobile.value && (
    bar.toolsDrawerOpen
    || bar.programPickerOpen
    || bar.consoleMobileActive
    || bar.keyboardHwActive
    || prefs.sidebarMobileOpen
  ),
);

const needsLogin = computed(() => !auth.user);

function goToLogin() {
  // Preserve the current page so the user lands back here after login.
  router.push({ name: 'login', query: { next: route.fullPath } });
  tutor.setOpen(false);
}

const unreadCount = computed(() => (tutor.open ? 0 : tutor.unreadCount));
const unreadLabel = computed(() => (unreadCount.value > 9 ? '9+' : String(unreadCount.value)));

const isExpanded = computed(() => tutor.displayMode === 'expanded');

function handleCite(citation: TutorCitation) {
  switch (citation.source) {
    case 'glossary':
      cpu.openGlossary(citation.id);
      return;
    case 'demos':
      // Loading the demo names the citation id directly. The student sees
      // the simulator switch to the demo it just heard about.
      cpu.load(citation.id);
      toast.ok(`Cargando demo "${citation.id}"`);
      return;
    case 'isa':
      // ISA chunks are project-internal spec; we surface the topic in a
      // toast for now. A future iteration could open a side panel.
      toast.info(`ISA — ${citation.label.replace(/^\[ISA:\s*/, '').replace(/\]$/, '')}`);
      return;
    case 'docs':
      toast.info(`Documentación — ${citation.label.replace(/^\[Docs:\s*/, '').replace(/\]$/, '')}`);
      return;
    case 'silberschatz':
    default: {
      const ref = citation.page ? `página ${citation.page}` : citation.id;
      toast.info(`Referencia: Silberschatz, ${ref}`);
    }
  }
}

// Temporary drag: the FAB can be dragged out of the way to expose UI it
// overlaps (the IPC panel's collapse caret, for example). The offset is
// kept in a local ref so it is wiped on reload — by design, this is not a
// persisted preference.
const fabRef = ref<HTMLButtonElement | null>(null);
const dragOffset = ref<{ x: number; y: number } | null>(null);
const isDragging = ref(false);

const DRAG_THRESHOLD_PX = 5;
const VIEWPORT_MARGIN_PX = 4;

let dragPointerId = -1;
let dragStartMouseX = 0;
let dragStartMouseY = 0;
let dragStartOffsetX = 0;
let dragStartOffsetY = 0;
let fabAnchorLeft = 0;
let fabAnchorTop = 0;
let fabWidth = 0;
let fabHeight = 0;

const dragStyle = computed(() => {
  if (!dragOffset.value) return undefined;
  return {
    transform: `translate(${dragOffset.value.x}px, ${dragOffset.value.y}px)`,
    animation: 'none',
    transition: isDragging.value ? 'none' : undefined,
  } as Record<string, string | undefined>;
});

function clampOffset(x: number, y: number) {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const minX = VIEWPORT_MARGIN_PX - fabAnchorLeft;
  const maxX = vw - fabAnchorLeft - fabWidth - VIEWPORT_MARGIN_PX;
  const minY = VIEWPORT_MARGIN_PX - fabAnchorTop;
  const maxY = vh - fabAnchorTop - fabHeight - VIEWPORT_MARGIN_PX;
  return {
    x: Math.min(Math.max(x, minX), Math.max(minX, maxX)),
    y: Math.min(Math.max(y, minY), Math.max(minY, maxY)),
  };
}

function onFabPointerDown(ev: PointerEvent) {
  if (ev.button !== 0 || !fabRef.value) return;
  const rect = fabRef.value.getBoundingClientRect();
  const currentOffset = dragOffset.value ?? { x: 0, y: 0 };
  // The anchor is where the FAB would render with no transform applied —
  // subtract the active offset from the live rect to recover it.
  fabAnchorLeft = rect.left - currentOffset.x;
  fabAnchorTop = rect.top - currentOffset.y;
  fabWidth = rect.width;
  fabHeight = rect.height;
  dragStartMouseX = ev.clientX;
  dragStartMouseY = ev.clientY;
  dragStartOffsetX = currentOffset.x;
  dragStartOffsetY = currentOffset.y;
  dragPointerId = ev.pointerId;
  isDragging.value = false;
  window.addEventListener('pointermove', onFabPointerMove);
  window.addEventListener('pointerup', onFabPointerUp);
  window.addEventListener('pointercancel', onFabPointerUp);
}

function onFabPointerMove(ev: PointerEvent) {
  if (ev.pointerId !== dragPointerId) return;
  const dx = ev.clientX - dragStartMouseX;
  const dy = ev.clientY - dragStartMouseY;
  if (!isDragging.value) {
    if (Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return;
    isDragging.value = true;
  }
  dragOffset.value = clampOffset(dragStartOffsetX + dx, dragStartOffsetY + dy);
}

function onFabPointerUp(ev: PointerEvent) {
  if (ev.pointerId !== dragPointerId) return;
  const wasDragging = isDragging.value;
  dragPointerId = -1;
  isDragging.value = false;
  window.removeEventListener('pointermove', onFabPointerMove);
  window.removeEventListener('pointerup', onFabPointerUp);
  window.removeEventListener('pointercancel', onFabPointerUp);
  if (wasDragging) {
    // Swallow the synthetic click that follows a drag-release so we do not
    // open the panel right when the user lets go. The safety raf drops the
    // listener if no click arrives (touch, cancelled gesture).
    const suppressClick = (e: MouseEvent) => {
      e.stopPropagation();
      e.preventDefault();
    };
    window.addEventListener('click', suppressClick, { capture: true, once: true });
    requestAnimationFrame(() => {
      window.removeEventListener('click', suppressClick, { capture: true });
    });
  }
}

function onWindowResize() {
  if (!dragOffset.value || !fabRef.value) return;
  const rect = fabRef.value.getBoundingClientRect();
  fabAnchorLeft = rect.left - dragOffset.value.x;
  fabAnchorTop = rect.top - dragOffset.value.y;
  fabWidth = rect.width;
  fabHeight = rect.height;
  dragOffset.value = clampOffset(dragOffset.value.x, dragOffset.value.y);
}

function onResetFabPosition() {
  dragOffset.value = null;
}

onMounted(() => {
  window.addEventListener('resize', onWindowResize);
  window.addEventListener(RESET_FAB_EVENT, onResetFabPosition);
});

// Esc collapses an expanded modal back to docked without closing the panel,
// so the conversation is preserved. If already docked, Esc closes the panel.
function onKeydown(ev: KeyboardEvent) {
  if (ev.key !== 'Escape' || !tutor.open) return;
  if (isExpanded.value) {
    tutor.setDisplayMode('docked');
    ev.preventDefault();
  }
}

watch(
  () => tutor.open,
  (isOpen) => {
    if (isOpen) {
      window.addEventListener('keydown', onKeydown);
    } else {
      window.removeEventListener('keydown', onKeydown);
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown);
  window.removeEventListener('pointermove', onFabPointerMove);
  window.removeEventListener('pointerup', onFabPointerUp);
  window.removeEventListener('pointercancel', onFabPointerUp);
  window.removeEventListener('resize', onWindowResize);
  window.removeEventListener(RESET_FAB_EVENT, onResetFabPosition);
});

function onBackdropClick() {
  // Match the Esc semantics: backdrop collapses to docked, never closes.
  tutor.setDisplayMode('docked');
}
</script>

<template>
  <div :class="['tutor-host', { 'tutor-host--dim': isDimmed }]">
    <button
      v-if="!tutor.open"
      ref="fabRef"
      class="fab"
      :class="{ has_hint: unreadCount > 0, 'is-dragging': isDragging }"
      :style="dragStyle"
      :title="unreadCount > 0
        ? `Tutor — ${unreadCount} mensaje(s) nuevo(s) (arrastre para mover)`
        : 'Hablar con el tutor (arrastre para mover)'"
      :aria-label="unreadCount > 0
        ? `Abrir tutor, ${unreadCount} mensajes nuevos. Arrastre para mover`
        : 'Abrir tutor. Arrastre para mover'"
      @pointerdown="onFabPointerDown"
      @click="tutor.setOpen(true)"
    >
      <TutorBotIcon :size="22" :stroke-width="1.75" />
      <span v-if="unreadCount > 0" class="hint-badge" aria-hidden="true">
        <span class="hint-pulse" />
        <span class="hint-num">{{ unreadLabel }}</span>
      </span>
    </button>

    <!-- Docked mode: panel anchored to the corner with a slide transition. -->
    <transition name="slide">
      <div v-if="tutor.open && !isExpanded" class="panel-wrap docked">
        <div v-if="needsLogin" class="login-gate" role="dialog" aria-label="Inicie sesión">
          <button class="gate-close" @click="tutor.setOpen(false)" aria-label="Minimizar" title="Minimizar">
            <Minus :size="14" :stroke-width="2.5" />
          </button>
          <div class="gate-icon"><TutorBotIcon :size="32" :stroke-width="1.4" /></div>
          <h3 class="gate-title">MODARCHI protegido</h3>
          <p class="gate-text">
            Para conversar con MODARCHI sobre Sistemas Operativos debe
            iniciar sesión con su correo institucional.
          </p>
          <button class="gate-cta" @click="goToLogin">
            <LogIn :size="14" :stroke-width="2" />
            Iniciar sesión
          </button>
          <p class="gate-hint">El simulador sigue siendo de uso libre.</p>
        </div>
        <TutorPanel v-else @close="tutor.setOpen(false)" @cite="handleCite" />
      </div>
    </transition>

    <!-- Expanded mode: teleport to body so the modal escapes the
         fixed-positioned host and stays above every other layer. The backdrop
         is a sibling so a click on it can collapse without touching the
         panel. -->
    <Teleport to="body">
      <transition name="modal">
        <div
          v-if="tutor.open && isExpanded"
          class="tutor-modal-root"
          role="dialog"
          aria-modal="true"
          aria-label="Tutor expandido"
        >
          <div class="tutor-backdrop" @click="onBackdropClick" />
          <div class="panel-wrap expanded" @click.stop>
            <div v-if="needsLogin" class="login-gate expanded-gate" role="dialog" aria-label="Inicie sesión">
              <button class="gate-close" @click="tutor.setOpen(false)" aria-label="Minimizar" title="Minimizar">
                <Minus :size="16" :stroke-width="2.5" />
              </button>
              <div class="gate-icon"><TutorBotIcon :size="44" :stroke-width="1.4" /></div>
              <h3 class="gate-title">MODARCHI protegido</h3>
              <p class="gate-text">
                Para conversar con MODARCHI sobre Sistemas Operativos debe
                iniciar sesión con su correo institucional.
              </p>
              <button class="gate-cta" @click="goToLogin">
                <LogIn :size="14" :stroke-width="2" />
                Iniciar sesión
              </button>
              <p class="gate-hint">El simulador sigue siendo de uso libre.</p>
            </div>
            <TutorPanel v-else @close="tutor.setOpen(false)" @cite="handleCite" />
          </div>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<style scoped>
.tutor-host {
  position: fixed;
  /* Anchor for the FAB and the docked panel; both attach to the host's
     bottom-right corner via position: absolute. The FAB itself can be
     dragged out of this anchor temporarily via a CSS transform applied
     inline — see the drag logic in <script>. */
  right: 12px;
  bottom: 52px;
  /* Above the narrator overlay (--z-overlay = 500) so banners like the
     ModeFlip mode-switch card do not paint over the tutor panel. */
  z-index: var(--z-modal, 600);
  pointer-events: none;
  /* The host is a zero-size anchor; both the FAB and the panel attach to
     its bottom-right corner via position: absolute. A flex layout used to
     stack them but caused the FAB to jump to an off-center position
     during the panel leave-transition because it briefly shared a flex
     row with the leaving panel. */
}
.tutor-host > * {
  pointer-events: auto;
}

/* Mobile-only dim state: a foreground tool (drawer, picker, console,
   keyboard HW) is taking the bottom area. We hide the FAB outright so
   it doesn't float over the active tool's input affordances; the user
   can reach the tutor again as soon as the tool closes. */
.tutor-host--dim {
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition: opacity 180ms ease, visibility 0s linear 180ms;
}
.tutor-host--dim > * {
  pointer-events: none;
}

.fab {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 3.25rem;
  height: 3.25rem;
  border-radius: var(--radius-sm);
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.35), rgba(var(--accent-rgb), 0.05) 70%);
  border: 1px solid rgba(var(--accent-rgb), 0.5);
  color: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 0 18px rgba(var(--accent-rgb), 0.30);
  /* All three properties share the same curve and duration so transform,
     shadow and stroke colour land together. Compositor-backed transform
     and shadow feel snappier than the CPU-painted SVG stroke if timings
     diverge — keeping them identical hides the difference. */
  transition: transform 180ms ease-out,
              box-shadow 180ms ease-out,
              color 180ms ease-out;
  /* Grow upward from the bottom edge so hover doesn't shift the hit target
     and trigger a hover/unhover oscillation when the cursor approaches from
     below. */
  transform-origin: center bottom;
  animation: tutor-bounce 4.8s ease-in-out infinite;
}
.fab:hover {
  transform: scale(1.06);
  color: var(--text);
  box-shadow: 0 0 24px rgba(var(--accent-rgb), 0.45);
  animation: none;
}
.fab.is-dragging {
  cursor: grabbing;
  box-shadow: 0 0 28px rgba(var(--accent-rgb), 0.55);
}
.fab .tutor-bot-icon {
  transition: stroke 180ms ease-out;
}
.fab:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 3px;
}

.fab.has_hint {
  border-color: var(--accent);
  color: var(--text);
  animation: hint-pulse 1.4s ease-in-out infinite;
}

.hint-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 1.05rem;
  height: 1.05rem;
  padding: 0 0.28rem;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: var(--surface-0);
  font-family: var(--font-display, inherit);
  font-size: 0.62rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1.5px solid var(--surface-0);
  box-shadow: 0 0 10px rgba(var(--accent-rgb), 0.85);
}
.hint-num {
  position: relative;
  z-index: 1;
  line-height: 1;
}
.hint-pulse {
  position: absolute;
  inset: -2px;
  border-radius: var(--radius-sm);
  background: rgba(var(--accent-rgb), 0.55);
  animation: hint-ring 1.6s ease-out infinite;
  z-index: 0;
}

@keyframes hint-pulse {
  0%, 100% { box-shadow: 0 0 0 4px rgba(var(--accent-rgb), 0.18), 0 8px 24px rgba(0, 0, 0, 0.65), 0 0 22px rgba(var(--accent-rgb), 0.55); }
  50%      { box-shadow: 0 0 0 8px rgba(var(--accent-rgb), 0.05), 0 10px 28px rgba(0, 0, 0, 0.7),  0 0 34px rgba(var(--accent-rgb), 0.85); }
}
@keyframes hint-ring {
  0%   { transform: scale(0.85); opacity: 0.9; }
  100% { transform: scale(2);    opacity: 0;   }
}

.panel-wrap.docked {
  position: absolute;
  right: 0;
  bottom: 0;
  height: min(38rem, 82vh);
  width: min(24rem, 95vw);
}

.slide-enter-active,
.slide-leave-active {
  transition: transform 220ms cubic-bezier(0.2, 0.8, 0.2, 1), opacity 200ms ease;
}
.slide-enter-from,
.slide-leave-to {
  transform: translateY(12px) scale(0.97);
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .fab,
  .fab.has_hint { animation: none; }
  .hint-pulse { animation: none; }
}

.login-gate {
  position: relative;
  width: 100%;
  height: 100%;
  background: linear-gradient(180deg, var(--surface-1) 0%, var(--surface-0) 100%);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-family: inherit;
  box-shadow:
    0 20px 50px rgba(0, 0, 0, 0.75),
    var(--shadow-glow-amber);
  backdrop-filter: blur(10px);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  text-align: center;
  gap: 0.6rem;
}
.gate-close {
  position: absolute;
  top: 0.6rem;
  right: 0.6rem;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--accent);
  border-radius: var(--radius-sm);
  padding: 0.2rem 0.3rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.gate-close:hover {
  background: rgba(var(--accent-rgb), 0.12);
  border-color: var(--border-strong);
}
.gate-icon {
  color: var(--accent);
  opacity: 0.8;
  filter: drop-shadow(0 0 12px rgba(var(--accent-rgb), 0.35));
}
.gate-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 1rem;
  letter-spacing: 3px;
  color: var(--accent);
  text-transform: uppercase;
}
.gate-text {
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--text);
  max-width: 28rem;
}
.gate-cta {
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
.gate-cta:hover { background: rgba(var(--accent-rgb), 0.34); }
.gate-hint { margin: 0; font-size: 0.7rem; color: var(--text-muted); }

.expanded-gate {
  padding: 3rem 2rem;
  gap: 1rem;
}
</style>

<!-- Modal-mode styles live in a global block because the panel is teleported
     to <body>, where scoped attributes do not match. -->
<style>
.tutor-modal-root {
  position: fixed;
  inset: 0;
  /* Same rationale as .tutor-host: the modal must outrank narrator overlays
     so they never bleed through the dialog. */
  z-index: var(--z-modal, 600);
  display: flex;
  align-items: center;
  justify-content: center;
}
.tutor-backdrop {
  position: absolute;
  inset: 0;
  background: var(--surface-overlay-strong);
  backdrop-filter: blur(4px);
}
.panel-wrap.expanded {
  position: relative;
  z-index: 1;
  width: min(70vw, 64rem);
  height: min(80vh, 42rem);
  box-shadow:
    0 0 0 1px rgba(var(--accent-rgb), 0.3),
    0 24px 72px rgba(0, 0, 0, 0.75),
    0 0 80px rgba(var(--accent-rgb), 0.18);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
@media (max-width: 640px) {
  .panel-wrap.expanded {
    width: 100vw;
    height: 100vh;
    border-radius: 0;
  }
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 220ms ease;
}
.modal-enter-active .panel-wrap.expanded,
.modal-leave-active .panel-wrap.expanded {
  transition: transform 220ms cubic-bezier(0.2, 0.8, 0.2, 1), opacity 200ms ease;
}
.modal-enter-from,
.modal-leave-to { opacity: 0; }
.modal-enter-from .panel-wrap.expanded,
.modal-leave-to .panel-wrap.expanded {
  transform: scale(0.96);
  opacity: 0;
}
</style>
