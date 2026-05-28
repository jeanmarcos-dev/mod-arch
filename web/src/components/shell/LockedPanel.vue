<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { Lock } from '@lucide/vue';

defineProps<{
  /** Optional label shown above the body. Falls back to the generic copy. */
  title?: string;
}>();

const { t } = useI18n();
</script>

<template>
  <div class="locked-panel" role="region" :aria-label="title ?? t('simulator.lockedPanel.title')">
    <span class="locked-panel__icon" aria-hidden="true">
      <Lock :size="22" :stroke-width="1.8" />
    </span>
    <span class="locked-panel__title">{{ title ?? t('simulator.lockedPanel.title') }}</span>
    <span class="locked-panel__body">{{ t('simulator.lockedPanel.body') }}</span>
  </div>
</template>

<style scoped>
.locked-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  width: 100%;
  height: 100%;
  padding: var(--space-4);
  background:
    repeating-linear-gradient(
      135deg,
      rgba(var(--accent-rgb), 0.04) 0,
      rgba(var(--accent-rgb), 0.04) 6px,
      transparent 6px,
      transparent 12px
    ),
    var(--surface-1);
  border: 1px dashed var(--border);
  color: var(--text-muted);
  text-align: center;
  user-select: none;
}

.locked-panel__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--text-dim);
  opacity: 0.7;
}

.locked-panel__title {
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
}

.locked-panel__body {
  font-size: 11px;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  max-width: 280px;
  line-height: 1.5;
}
</style>
