<script setup lang="ts">
import { useToastStore } from '../../stores/toast';
const toast = useToastStore();
</script>

<template>
  <Teleport to="body">
    <div class="toast-stack" aria-live="polite">
      <TransitionGroup name="toast">
        <div
          v-for="t in toast.items"
          :key="t.id"
          :class="['toast', `toast--${t.tone}`]"
          role="status"
        >
          {{ t.message }}
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-stack {
  position: fixed;
  bottom: var(--space-4);
  right: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  z-index: var(--z-toast);
  pointer-events: none;
  max-width: min(380px, 90vw);
}

.toast {
  padding: var(--space-3) var(--space-4);
  border: 1px solid;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  letter-spacing: 0.5px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  box-shadow: var(--shadow-elevation-2);
  pointer-events: auto;
}

.toast--ok {
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green), var(--shadow-elevation-2);
}

.toast--err {
  color: var(--red-soft);
  border-color: var(--red);
  box-shadow: var(--shadow-glow-red), var(--shadow-elevation-2);
}

.toast--info {
  color: var(--amber);
  border-color: var(--border-strong);
  box-shadow: var(--shadow-glow-amber), var(--shadow-elevation-2);
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(40px);
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity var(--transition-base), transform var(--transition-base);
}
</style>
