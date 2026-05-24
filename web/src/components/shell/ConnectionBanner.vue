<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import { X } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useConnectionBanner } from '../../composables/useConnectionBanner';

const cpu = useCpuStore();
const { phase, visible, dismiss } = useConnectionBanner();

// Refresh the "next retry in Ns" label every 500ms so the countdown ticks
// down smoothly while the user waits. Only runs while the banner is mounted.
const now = ref(Date.now());
const tickHandle = window.setInterval(() => { now.value = Date.now(); }, 500);
onBeforeUnmount(() => window.clearInterval(tickHandle));

const secondsUntilRetry = computed(() => {
  if (cpu.nextRetryAt === null) return null;
  const ms = cpu.nextRetryAt - now.value;
  return Math.max(0, Math.ceil(ms / 1000));
});

const title = computed(() =>
  phase.value === 'connecting'
    ? 'Conectando con el simulador'
    : 'Conexión perdida con el simulador',
);

const statusLine = computed(() => {
  if (secondsUntilRetry.value === null) {
    if (cpu.reconnectAttempts > 0) return 'Reintentando conexión…';
    return phase.value === 'connecting'
      ? 'Estableciendo enlace…'
      : 'Sin conexión con el servidor.';
  }
  if (secondsUntilRetry.value === 0) return 'Reintentando ahora…';
  return `Próximo intento en ${secondsUntilRetry.value}s.`;
});

const attemptsLine = computed(() => {
  if (cpu.reconnectAttempts === 0) return '';
  return ` · intento #${cpu.reconnectAttempts}`;
});
</script>

<template>
  <transition name="conn-banner-fade">
    <div
      v-if="visible"
      class="conn-banner"
      :class="{ 'conn-banner--connecting': phase === 'connecting' }"
      role="alert"
      aria-live="polite"
    >
      <span class="conn-banner__dot" aria-hidden="true" />
      <span class="conn-banner__title">{{ title }}</span>
      <span class="conn-banner__detail">{{ statusLine }}{{ attemptsLine }}</span>
      <button
        v-if="phase === 'lost'"
        type="button"
        class="conn-banner__btn"
        @click="cpu.reconnectNow()"
      >Reconectar ahora</button>
      <button
        type="button"
        class="conn-banner__close"
        title="Ocultar este aviso hasta el próximo cambio de estado"
        aria-label="Cerrar aviso de conexión"
        @click="dismiss()"
      >
        <X :size="14" :stroke-width="2" />
      </button>
    </div>
  </transition>
</template>

<style scoped>
.conn-banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 2000;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 8px var(--space-5);
  background: rgba(40, 8, 8, 0.96);
  border-bottom: 1px solid var(--red-soft, #ff7a7a);
  color: #ffd6d6;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  box-shadow: 0 2px 12px rgba(255, 122, 122, 0.18);
}

.conn-banner--connecting {
  background: rgba(8, 24, 32, 0.96);
  border-bottom-color: var(--cyan, #6ad4ff);
  color: #d6eaff;
  box-shadow: 0 2px 12px rgba(106, 212, 255, 0.18);
}

.conn-banner__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--red-soft, #ff7a7a);
  box-shadow: 0 0 8px var(--red-soft, #ff7a7a);
  animation: conn-banner-pulse 1.1s ease-in-out infinite;
}

.conn-banner--connecting .conn-banner__dot {
  background: var(--cyan, #6ad4ff);
  box-shadow: 0 0 8px var(--cyan, #6ad4ff);
}

.conn-banner__title {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 2px;
  text-transform: uppercase;
  font-size: 11px;
  color: var(--red-soft, #ff7a7a);
}

.conn-banner--connecting .conn-banner__title {
  color: var(--cyan, #6ad4ff);
}

.conn-banner__detail {
  flex: 1;
  color: #ffd6d6;
  font-size: 11.5px;
  letter-spacing: 0.5px;
}

.conn-banner--connecting .conn-banner__detail {
  color: #d6eaff;
}

.conn-banner__btn {
  background: transparent;
  border: 1px solid var(--red-soft, #ff7a7a);
  color: var(--red-soft, #ff7a7a);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 4px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
  min-height: unset;
}

.conn-banner__btn:hover {
  background: var(--red-soft, #ff7a7a);
  color: #1a0606;
}

.conn-banner__btn:focus-visible {
  outline: 2px solid var(--red-soft, #ff7a7a);
  outline-offset: 2px;
}

.conn-banner__close {
  background: transparent;
  border: 1px solid transparent;
  color: var(--red-soft, #ff7a7a);
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 0;
  flex-shrink: 0;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}

.conn-banner__close:hover {
  background: rgba(255, 58, 58, 0.18);
  border-color: var(--red-soft, #ff7a7a);
}

.conn-banner--connecting .conn-banner__close {
  color: var(--cyan, #6ad4ff);
}

.conn-banner--connecting .conn-banner__close:hover {
  background: rgba(106, 212, 255, 0.18);
  border-color: var(--cyan, #6ad4ff);
}

@media (max-width: 768px) {
  .conn-banner {
    padding: 4px var(--space-3);
    gap: var(--space-2);
    font-size: 11px;
  }
  .conn-banner__title { display: none; }
  .conn-banner__detail { font-size: 10.5px; }
  .conn-banner__btn {
    font-size: 10px;
    padding: 2px 8px;
    letter-spacing: 0.5px;
  }
}

@keyframes conn-banner-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50%      { opacity: 0.5; transform: scale(0.7); }
}

.conn-banner-fade-enter-active,
.conn-banner-fade-leave-active {
  transition: transform var(--transition-base), opacity var(--transition-base);
}

.conn-banner-fade-enter-from,
.conn-banner-fade-leave-to {
  transform: translateY(-100%);
  opacity: 0;
}
</style>
