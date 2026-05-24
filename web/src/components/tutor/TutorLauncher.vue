<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { BotMessageSquare, LogIn, X } from '@lucide/vue';

import TutorPanel from './TutorPanel.vue';
import { useTutorStore, type TutorCitation } from '../../stores/tutor';
import { useCpuStore } from '../../stores/cpu';
import { useToastStore } from '../../stores/toast';
import { useAuthStore } from '../../stores/auth';

const tutor = useTutorStore();
const cpu = useCpuStore();
const toast = useToastStore();
const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

const needsLogin = computed(() => !auth.user);

function goToLogin() {
  // Preserve the current page so the user lands back here after login.
  router.push({ name: 'login', query: { next: route.fullPath } });
  tutor.setOpen(false);
}

const unreadHint = computed(() => {
  if (tutor.open) return 0;
  if (!tutor.lastMessage || tutor.lastMessage.role === 'student') return 0;
  return tutor.lastMessage.pending ? 0 : 1;
});

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
    case 'stallings':
    default: {
      const ref = citation.page ? `página ${citation.page}` : citation.id;
      toast.info(`Referencia: Stallings, ${ref}`);
    }
  }
}

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
});

function onBackdropClick() {
  // Match the Esc semantics: backdrop collapses to docked, never closes.
  tutor.setDisplayMode('docked');
}
</script>

<template>
  <div class="tutor-host">
    <button
      v-if="!tutor.open"
      class="fab"
      :class="{ has_hint: unreadHint > 0 }"
      title="Hablar con el tutor"
      aria-label="Abrir tutor"
      @click="tutor.setOpen(true)"
    >
      <span class="fab-glow" aria-hidden="true" />
      <BotMessageSquare :size="22" :stroke-width="1.75" />
      <span v-if="unreadHint > 0" class="hint-dot" aria-hidden="true" />
    </button>

    <!-- Docked mode: panel anchored to the corner with a slide transition. -->
    <transition name="slide">
      <div v-if="tutor.open && !isExpanded" class="panel-wrap docked">
        <div v-if="needsLogin" class="login-gate" role="dialog" aria-label="Inicie sesión">
          <button class="gate-close" @click="tutor.setOpen(false)" aria-label="Cerrar">
            <X :size="14" :stroke-width="2" />
          </button>
          <div class="gate-icon"><BotMessageSquare :size="32" :stroke-width="1.4" /></div>
          <h3 class="gate-title">Tutor protegido</h3>
          <p class="gate-text">
            Para conversar con el tutor de Sistemas Operativos debe iniciar
            sesión con su correo institucional.
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
              <button class="gate-close" @click="tutor.setOpen(false)" aria-label="Cerrar">
                <X :size="16" :stroke-width="2" />
              </button>
              <div class="gate-icon"><BotMessageSquare :size="44" :stroke-width="1.4" /></div>
              <h3 class="gate-title">Tutor protegido</h3>
              <p class="gate-text">
                Para conversar con el tutor de Sistemas Operativos debe
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
  /* Sit above the .credit overlay in Simulator.vue (bottom: 6px, ~28px tall),
     leaving room for a soft drop-shadow underneath. */
  right: 12px;
  bottom: 52px;
  z-index: 110;
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

.fab {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 3.25rem;
  height: 3.25rem;
  border-radius: 50%;
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.35), rgba(var(--accent-rgb), 0.12) 65%, rgba(0, 0, 0, 0.6) 100%);
  border: 1px solid rgba(var(--accent-rgb), 0.8);
  color: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  backdrop-filter: blur(8px);
  box-shadow:
    0 0 0 4px rgba(var(--accent-rgb), 0.05),
    0 8px 24px rgba(0, 0, 0, 0.65),
    0 0 18px rgba(var(--accent-rgb), 0.35);
  transition: transform 140ms ease, box-shadow 140ms ease, color 140ms ease;
  animation: idle-pulse 3.6s ease-in-out infinite;
}
.fab:hover {
  transform: translateY(-3px) scale(1.04);
  color: var(--text);
  box-shadow:
    0 0 0 4px rgba(var(--accent-rgb), 0.12),
    0 12px 28px rgba(0, 0, 0, 0.7),
    0 0 28px rgba(var(--accent-rgb), 0.55);
  animation: none;
}
.fab:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 3px;
}
.fab-glow {
  position: absolute;
  inset: -6px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(var(--accent-rgb), 0.5), transparent 70%);
  filter: blur(8px);
  z-index: -1;
  opacity: 0.7;
}

.hint-dot {
  position: absolute;
  top: 0.3rem;
  right: 0.3rem;
  width: 0.6rem;
  height: 0.6rem;
  border-radius: 50%;
  background: var(--green);
  box-shadow: 0 0 8px var(--green-glow);
  animation: blink 1.6s ease-in-out infinite;
}

@keyframes idle-pulse {
  0%, 100% { box-shadow: 0 0 0 4px rgba(var(--accent-rgb), 0.05), 0 8px 24px rgba(0, 0, 0, 0.65), 0 0 18px rgba(var(--accent-rgb), 0.35); }
  50%      { box-shadow: 0 0 0 6px rgba(var(--accent-rgb), 0.10), 0 10px 26px rgba(0, 0, 0, 0.65), 0 0 26px rgba(var(--accent-rgb), 0.55); }
}
@keyframes blink {
  0%, 100% { opacity: 1; }
  50%      { opacity: 0.45; }
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
  .fab { animation: none; }
  .hint-dot { animation: none; }
}

.login-gate {
  position: relative;
  width: 100%;
  height: 100%;
  background: linear-gradient(180deg, var(--surface-1) 0%, var(--surface-0) 100%);
  border: 1px solid var(--border-strong);
  border-radius: 0.75rem;
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
  border-radius: 0.3rem;
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
  border-radius: 0.35rem;
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
  z-index: 200;
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
  border-radius: 0.75rem;
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
