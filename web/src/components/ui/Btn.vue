<script setup lang="ts">
type Variant = 'primary' | 'ghost' | 'danger';
type Size = 'sm' | 'md';

withDefaults(
  defineProps<{
    variant?: Variant;
    size?: Size;
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    loading?: boolean;
    block?: boolean;
  }>(),
  {
    variant: 'primary',
    size: 'md',
    type: 'button',
    disabled: false,
    loading: false,
    block: false,
  },
);

defineEmits<{ (e: 'click', ev: MouseEvent): void }>();
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="['btn', `btn--${variant}`, `btn--${size}`, { 'btn--block': block, 'btn--loading': loading }]"
    @click="(ev) => $emit('click', ev)"
  >
    <span v-if="loading" class="btn__spinner" aria-hidden="true">▮</span>
    <span class="btn__label"><slot /></span>
  </button>
</template>

<style scoped>
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  font-family: inherit;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
  transition:
    background var(--transition-fast),
    box-shadow var(--transition-fast),
    transform var(--transition-fast),
    border-color var(--transition-fast);
  border-radius: var(--radius-sm);
  user-select: none;
}

.btn--sm { padding: 4px 10px; font-size: 12px; min-height: 28px; }
.btn--md { padding: 6px 14px; font-size: 14px; min-height: 32px; }
.btn--block { width: 100%; }

.btn--primary {
  background: rgba(var(--accent-rgb), 0.08);
  border: 1px solid var(--border-strong);
  color: var(--amber);
}
.btn--primary:hover:not(:disabled) {
  background: rgba(var(--accent-rgb), 0.18);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
  transform: translateY(-1px);
}

.btn--ghost {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
}
.btn--ghost:hover:not(:disabled) {
  background: rgba(var(--accent-rgb), 0.06);
  border-color: var(--border-strong);
  color: var(--amber);
}

.btn--danger {
  background: rgba(255, 58, 58, 0.08);
  border: 1px solid rgba(255, 58, 58, 0.45);
  color: var(--red-soft);
}
.btn--danger:hover:not(:disabled) {
  background: rgba(255, 58, 58, 0.18);
  border-color: var(--red);
  box-shadow: var(--shadow-glow-red);
  transform: translateY(-1px);
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none;
  box-shadow: none;
}

.btn__label {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}
.btn--loading .btn__label { opacity: 0.6; }
.btn__spinner {
  animation: btn-blink 0.9s steps(2, jump-none) infinite;
}
@keyframes btn-blink { 50% { opacity: 0.2; } }

@media (max-width: 768px) {
  .btn--sm { min-height: 44px; padding: 8px 12px; }
  .btn--md { min-height: 44px; padding: 10px 16px; }
}
</style>
