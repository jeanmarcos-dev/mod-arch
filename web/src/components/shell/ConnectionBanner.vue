<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();

// Refresh the "next retry in Ns" label every 500ms so the countdown ticks
// down smoothly while the user waits. Only runs while the banner is mounted.
const now = ref(Date.now());
const tickHandle = window.setInterval(() => { now.value = Date.now(); }, 500);
onBeforeUnmount(() => window.clearInterval(tickHandle));

const visible = computed(() => !cpu.connected);

const secondsUntilRetry = computed(() => {
  if (cpu.nextRetryAt === null) return null;
  const ms = cpu.nextRetryAt - now.value;
  return Math.max(0, Math.ceil(ms / 1000));
});

const statusLine = computed(() => {
  if (secondsUntilRetry.value === null) {
    return cpu.reconnectAttempts > 0
      ? 'Reintentando conexión…'
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
    <div v-if="visible" class="conn-banner" role="alert" aria-live="polite">
      <span class="conn-banner__dot" aria-hidden="true" />
      <span class="conn-banner__title">Conexión perdida con el simulador</span>
      <span class="conn-banner__detail">{{ statusLine }}{{ attemptsLine }}</span>
      <button
        type="button"
        class="conn-banner__btn"
        @click="cpu.reconnectNow()"
      >Reconectar ahora</button>
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

.conn-banner__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--red-soft, #ff7a7a);
  box-shadow: 0 0 8px var(--red-soft, #ff7a7a);
  animation: conn-banner-pulse 1.1s ease-in-out infinite;
}

.conn-banner__title {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 2px;
  text-transform: uppercase;
  font-size: 11px;
  color: var(--red-soft, #ff7a7a);
}

.conn-banner__detail {
  flex: 1;
  color: #ffd6d6;
  font-size: 11.5px;
  letter-spacing: 0.5px;
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
