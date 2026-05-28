<script setup lang="ts">
import { watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Hand } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useViewerStore } from '../../stores/viewer';
import { useToastStore } from '../../stores/toast';

// HandRaiseBadge surfaces viewer hand-raises to the host. It hooks the
// cpu.handRaiseSignal counter (bumped by every 'hand_raise' WS event) and
// translates each tick into a toast + a persistent badge that the owner can
// click to acknowledge and reset.
const { t } = useI18n();
const cpu = useCpuStore();
const viewer = useViewerStore();
const toast = useToastStore();

watch(
  () => cpu.handRaiseSignal,
  (n, prev) => {
    if (n === prev) return;
    const label = cpu.lastHandRaiseLabel || t('shell.handRaise.fallbackSpectator');
    viewer.noteHandRaise(label);
    toast.push(t('shell.handRaise.toast', { label }), 'info');
  },
);

function onClick() {
  viewer.clearHandRaises();
}
</script>

<template>
  <button
    v-if="viewer.handRaiseCount > 0"
    type="button"
    class="hand-badge"
    :title="viewer.lastHandRaiseLabel ? t('shell.handRaise.titleSingle', { label: viewer.lastHandRaiseLabel }) : t('shell.handRaise.titleMany')"
    @click="onClick"
  >
    <Hand :size="14" :stroke-width="2" />
    <span class="hand-badge__count">{{ viewer.handRaiseCount }}</span>
  </button>
</template>

<style scoped>
.hand-badge {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.12);
  border: 1px solid var(--amber);
  color: var(--amber);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 10px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-glow-amber);
  animation: hand-badge-attention 1.6s ease-in-out infinite;
}
.hand-badge:hover {
  background: rgba(var(--accent-rgb), 0.22);
}
.hand-badge__count {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

@keyframes hand-badge-attention {
  0%, 100% { transform: rotate(0deg); }
  20% { transform: rotate(-8deg); }
  40% { transform: rotate(8deg); }
  60% { transform: rotate(-4deg); }
  80% { transform: rotate(4deg); }
}
</style>
