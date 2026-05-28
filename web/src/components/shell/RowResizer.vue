<script setup lang="ts">
import { useI18n } from 'vue-i18n';

interface Props {
  active?: boolean;
}

defineProps<Props>();
const emit = defineEmits<{ (e: 'pointerdown', event: PointerEvent): void }>();

const { t } = useI18n();
</script>

<template>
  <div
    class="row-resizer"
    :class="{ 'row-resizer--active': active }"
    role="separator"
    aria-orientation="horizontal"
    :aria-label="t('panels.resizer.rowLabel')"
    @pointerdown="emit('pointerdown', $event)"
  >
    <span class="row-resizer__grip" aria-hidden="true" />
  </div>
</template>

<style scoped>
.row-resizer {
  /* flex: 0 0 4px; */
  padding: 4px 0;
  width: 100%;
  cursor: row-resize;
  position: relative;
  background: transparent;
  transition: background 0.15s ease;
  touch-action: none;
  user-select: none;
}

.row-resizer:hover,
.row-resizer--active {
  background: rgba(var(--accent-rgb), 0.18);
}

.row-resizer__grip {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 36px;
  height: 2px;
  transform: translate(-50%, -50%);
  background: var(--border-strong);
  border-radius: 1px;
  transition: background 0.15s ease;
  pointer-events: none;
}

.row-resizer:hover .row-resizer__grip,
.row-resizer--active .row-resizer__grip {
  background: var(--amber);
  box-shadow: 0 0 6px rgba(var(--accent-rgb), 0.4);
}

@media (max-width: 768px) {
  .row-resizer {
    display: none;
  }
}
</style>
