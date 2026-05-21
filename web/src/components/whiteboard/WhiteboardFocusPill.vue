<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { Crosshair } from '@lucide/vue';
import { useWhiteboardStore } from '../../stores/whiteboard';
import { useCpuStore } from '../../stores/cpu';

// WhiteboardFocusPill renders a discreet "Está mirando: <panel>" badge in
// the bottom-right of the viewer while the host's cursor is on a known
// anchor. The pill makes the cursor's context legible — a moving dot on
// screen does not mean much by itself, but "Está mirando: Memoria" cues the
// audience to look at the right area before they spot the cursor.
//
// shell-content (the fallback anchor used when the cursor is over a
// non-tagged area) gets no pill: we only surface the badge when it adds
// signal.

const ANCHOR_LABELS: Record<string, string> = {
  'cpu-panel': 'Panel del CPU',
  'code-panel': 'Código fuente',
  'events-panel': 'Bitácora de eventos',
  'memory-panel': 'Memoria',
  'console-panel': 'Consola',
  'procs-panel': 'Procesos',
  'sim-bar': 'Barra de control',
  'sim-status-row': 'Estado del CPU',
  'sidebar': 'Barra lateral',
  'topbar': 'Barra superior',
  'viewer-topbar': 'Barra superior',
  'view-overview': 'Vista general',
  'view-procesos': 'Sección Procesos',
  'view-memoria': 'Sección Memoria',
  'view-interrupciones': 'Sección Interrupciones',
  'view-concurrencia': 'Sección Concurrencia',
};

const cpu = useCpuStore();
const whiteboard = useWhiteboardStore();
const route = useRoute();

const label = computed<string | null>(() => {
  const p = whiteboard.ownerPointer;
  if (!p) return null;
  if (p.route !== route.path) return null;
  if (cpu.sessionInfo.role === 'owner') return null;
  return ANCHOR_LABELS[p.anchor] ?? null;
});
</script>

<template>
  <Teleport to="body">
    <Transition name="fp">
      <div v-if="label" class="wb-focus-pill" aria-live="polite">
        <Crosshair :size="12" :stroke-width="2" class="wb-focus-pill__icon" />
        <span class="wb-focus-pill__caption">El anfitrión está mirando</span>
        <span class="wb-focus-pill__label">{{ label }}</span>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.wb-focus-pill {
  position: fixed;
  right: 16px;
  bottom: 16px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background: var(--surface-2);
  border: 1px solid var(--accent);
  border-radius: var(--radius-md);
  box-shadow: 0 0 12px rgba(var(--accent-rgb), 0.35);
  font-family: var(--font-body);
  font-size: 11px;
  letter-spacing: 1px;
  color: var(--text);
  pointer-events: none;
  z-index: var(--z-overlay);
  text-transform: uppercase;
}

.wb-focus-pill__icon { color: var(--accent); }

.wb-focus-pill__caption {
  color: var(--text-muted);
  font-size: 9px;
  letter-spacing: 1.5px;
}

.wb-focus-pill__label {
  color: var(--accent);
  font-weight: 600;
}

.fp-enter-active,
.fp-leave-active {
  transition: opacity 200ms ease, transform 200ms ease;
}
.fp-enter-from,
.fp-leave-to {
  opacity: 0;
  transform: translateY(6px);
}

@media (prefers-reduced-motion: reduce) {
  .fp-enter-active, .fp-leave-active { transition: none; }
}
</style>
