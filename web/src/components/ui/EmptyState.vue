<script setup lang="ts">
defineProps<{
  title: string;
  message?: string;
  icon?: string;
}>();
</script>

<template>
  <div class="empty">
    <div v-if="icon" class="empty__icon" aria-hidden="true">{{ icon }}</div>
    <h3 class="empty__title">{{ title }}</h3>
    <p v-if="message" class="empty__message">{{ message }}</p>
    <div v-if="$slots.default" class="empty__action"><slot /></div>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  padding: var(--space-6) var(--space-4);
  text-align: center;
  color: var(--text-dim);
}

.empty__icon {
  font-family: 'Major Mono Display', monospace;
  font-size: 32px;
  color: var(--amber);
  opacity: 0.7;
  letter-spacing: 4px;
}

.empty__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 14px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text);
  font-weight: normal;
}

.empty__message {
  font-size: 13px;
  color: var(--text-muted);
  max-width: 38ch;
}

.empty__action {
  margin-top: var(--space-2);
}
</style>
