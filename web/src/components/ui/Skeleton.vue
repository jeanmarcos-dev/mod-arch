<script setup lang="ts">
import { computed } from 'vue';

withDefaults(
  defineProps<{
    /** Number of placeholder rows to render. */
    rows?: number;
    /** Width of each row as a CSS length. */
    width?: string;
    /** Per-row gap as a CSS length (uses the spacing scale by default). */
    gap?: string;
  }>(),
  { rows: 3, width: '100%', gap: 'var(--space-2)' },
);

// Pre-computed array so v-for has stable keys without re-rendering.
const _ROW_INDEXES = computed(() => Array.from({ length: 16 }, (_, i) => i));
</script>

<template>
  <div class="skeleton" :style="{ gap }" aria-hidden="true">
    <div
      v-for="i in rows"
      :key="i"
      class="skeleton__row"
      :style="{ width }"
    >
      <span
        v-for="j in _ROW_INDEXES"
        :key="j"
        class="skeleton__char"
        :style="{ animationDelay: `${(i * 13 + j) * 60}ms` }"
      />
    </div>
  </div>
</template>

<style scoped>
.skeleton {
  display: flex;
  flex-direction: column;
  width: 100%;
}

.skeleton__row {
  display: flex;
  gap: 3px;
  height: 14px;
  align-items: center;
}

.skeleton__char {
  flex: 1;
  height: 10px;
  background: rgba(var(--accent-rgb), 0.08);
  border-radius: 1px;
  animation: skel-blink 1.4s infinite ease-in-out;
}

@keyframes skel-blink {
  0%, 100% { opacity: 0.25; transform: scaleY(0.6); }
  50% { opacity: 1; transform: scaleY(1); }
}

@media (prefers-reduced-motion: reduce) {
  .skeleton__char { animation: none; opacity: 0.4; }
}
</style>
