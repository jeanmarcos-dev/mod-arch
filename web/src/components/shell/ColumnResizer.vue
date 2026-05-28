<script setup lang="ts">
import { useI18n } from 'vue-i18n';

interface Props {
  side: 'left' | 'right';
  active?: boolean;
}

defineProps<Props>();
const emit = defineEmits<{ (e: 'pointerdown', event: PointerEvent): void }>();

const { t } = useI18n();
</script>

<template>
  <div
    class="col-resizer"
    :class="[`col-resizer--${side}`, { 'col-resizer--active': active }]"
    role="separator"
    aria-orientation="vertical"
    :aria-label="t('panels.resizer.label')"
    @pointerdown="emit('pointerdown', $event)"
  >
    <span class="col-resizer__grip" aria-hidden="true" />
  </div>
</template>

<style scoped>
.col-resizer {
  padding: 0 4px;
  align-self: stretch;
  cursor: col-resize;
  position: relative;
  background: transparent;
  transition: background 0.15s ease;
  touch-action: none;
  user-select: none;
}

.col-resizer:hover,
.col-resizer--active {
  background: rgba(var(--accent-rgb), 0.18);
}

.col-resizer__grip {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 2px;
  height: 36px;
  transform: translate(-50%, -50%);
  background: var(--border-strong);
  border-radius: 1px;
  transition: background 0.15s ease;
  pointer-events: none;
}

.col-resizer:hover .col-resizer__grip,
.col-resizer--active .col-resizer__grip {
  background: var(--amber);
  box-shadow: 0 0 6px rgba(var(--accent-rgb), 0.4);
}

@media (max-width: 768px) {
  .col-resizer {
    display: none;
  }
}
</style>
