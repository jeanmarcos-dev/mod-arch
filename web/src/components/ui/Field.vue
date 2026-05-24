<script setup lang="ts">
import { useId } from 'vue';

const props = withDefaults(
  defineProps<{
    label: string;
    help?: string;
    error?: string;
    required?: boolean;
    id?: string;
  }>(),
  { required: false },
);

const autoId = useId();
const fieldId = props.id || autoId;
</script>

<template>
  <div class="field">
    <label class="field__label" :for="fieldId">
      {{ label }}
      <span v-if="required" class="field__required" aria-hidden="true">*</span>
    </label>
    <slot :id="fieldId" />
    <span v-if="error" class="field__error" role="alert">{{ error }}</span>
    <span v-else-if="help" class="field__help">{{ help }}</span>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.field__label {
  font-size: 11px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-dim);
}

.field__required {
  color: var(--amber);
  margin-left: var(--space-1);
}

.field__help {
  font-size: 11px;
  color: var(--text-muted);
}

.field__error {
  font-size: 12px;
  color: var(--red-soft);
}

.field :deep(input),
.field :deep(textarea),
.field :deep(select) {
  font-family: inherit;
  font-size: 14px;
  background: var(--surface-overlay);
  color: var(--text);
  border: 1px solid var(--border);
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.field :deep(input:focus),
.field :deep(textarea:focus),
.field :deep(select:focus) {
  border-color: var(--amber);
  outline: none;
  box-shadow: var(--shadow-glow-amber);
}

.field :deep(input::placeholder),
.field :deep(textarea::placeholder) {
  color: var(--text-muted);
}

@media (max-width: 768px) {
  .field :deep(input),
  .field :deep(textarea),
  .field :deep(select) {
    min-height: 44px;
  }
}
</style>
