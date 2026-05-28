<script setup lang="ts">
import { useToastStore, type ToastItem } from '../../stores/toast';
const toast = useToastStore();

function runAction(t: ToastItem) {
  // Dismiss before running so a slow handler does not keep the snackbar on
  // screen while the action runs.
  toast.dismiss(t.id);
  t.action?.run();
}
</script>

<template>
  <Teleport to="body">
    <div class="toast-stack" aria-live="polite">
      <TransitionGroup name="toast">
        <div
          v-for="t in toast.items"
          :key="t.id"
          :class="['toast', `toast--${t.tone}`, { 'toast--has-action': !!t.action }]"
          role="status"
        >
          <span class="toast__msg">{{ t.message }}</span>
          <button
            v-if="t.action"
            type="button"
            class="toast__action"
            @click="runAction(t)"
          >{{ t.action.label }}</button>
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
.toast--has-action {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.toast__msg {
  flex: 1 1 auto;
  min-width: 0;
}
.toast__action {
  flex: 0 0 auto;
  background: transparent;
  border: 1px solid currentColor;
  color: inherit;
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 0.25rem 0.6rem;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--transition-base);
}
.toast__action:hover {
  background: rgba(255, 255, 255, 0.08);
}
.toast__action:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 2px;
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
