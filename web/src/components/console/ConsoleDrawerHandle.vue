<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDown, FileText, Keyboard, Zap } from '@lucide/vue';

interface Props {
  expanded: boolean;
  unseenStdout: number;
  kbdReading: boolean;
  irqRecent: boolean;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  (e: 'toggle'): void;
}>();

const { t } = useI18n();

const isIdle = computed(() =>
  !props.unseenStdout && !props.kbdReading && !props.irqRecent,
);
const toggleTitle = computed(() =>
  props.expanded ? t('panels.console.handle.collapseTitle') : t('panels.console.handle.expandTitle'),
);
</script>

<template>
  <div
    v-if="!expanded"
    class="drawer-aside-handle"
    role="button"
    tabindex="0"
    :title="toggleTitle"
    :aria-label="toggleTitle"
    @click="emit('toggle')"
    @keydown.enter.prevent="emit('toggle')"
    @keydown.space.prevent="emit('toggle')"
  >
    <span class="drawer-aside-handle__title">
      {{ t('panels.console.handle.label') }}<span
        v-if="isIdle"
        class="drawer-aside-handle__idle"
      > · {{ t('panels.console.handle.idle') }}</span>
    </span>
    <span v-if="!isIdle" class="drawer-aside-handle__badges">
      <span
        v-if="unseenStdout > 0"
        class="drawer-aside-handle__badge drawer-aside-handle__badge--stdout"
        :title="t('panels.console.handle.stdoutTip')"
      >
        <FileText :size="10" aria-hidden="true" />
        <span class="drawer-aside-handle__count">{{ unseenStdout }}</span>
      </span>
      <span
        v-if="kbdReading"
        class="drawer-aside-handle__badge drawer-aside-handle__badge--kbd"
        :title="t('panels.console.handle.kbdTip')"
      >
        <Keyboard :size="10" aria-hidden="true" />
      </span>
      <span
        v-if="irqRecent"
        class="drawer-aside-handle__badge drawer-aside-handle__badge--irq is-fired"
        :title="t('panels.console.handle.irqTip')"
      >
        <Zap :size="10" aria-hidden="true" />
      </span>
    </span>
  </div>

  <div v-else class="drawer-header">
    <span class="drawer-header__title">{{ t('panels.console.handle.label') }}</span>
    <button
      type="button"
      class="drawer-header__toggle"
      :title="toggleTitle"
      :aria-label="toggleTitle"
      :aria-expanded="expanded"
      @click="emit('toggle')"
    >
      <ChevronDown :size="14" :stroke-width="2" />
    </button>
  </div>
</template>

<style scoped>
.drawer-aside-handle {
  flex: 0 0 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: var(--surface-overlay);
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  cursor: pointer;
  user-select: none;
  transition: background 120ms ease, border-color 120ms ease;
}

.drawer-aside-handle:hover {
  background: rgba(var(--accent-rgb), 0.08);
  border-top-color: var(--amber);
  border-bottom-color: var(--amber);
}

.drawer-aside-handle:focus-visible {
  background: rgba(var(--accent-rgb), 0.08);
  border-top-color: var(--amber);
  border-bottom-color: var(--amber);
  outline: 2px solid var(--amber);
  outline-offset: -2px;
}

.drawer-aside-handle__title {
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
}

.drawer-aside-handle__idle {
  font-style: italic;
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-muted);
}

.drawer-aside-handle__badges {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.drawer-aside-handle__badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 1px 4px;
  border: 1px solid var(--terminal-border);
  border-radius: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  line-height: 1;
}

.drawer-aside-handle__badge--stdout {
  color: var(--cyan);
  border-color: var(--cyan);
  background: color-mix(in oklab, var(--cyan) 12%, transparent);
}

.drawer-aside-handle__badge--kbd {
  color: var(--amber);
  border-color: var(--amber);
  background: color-mix(in oklab, var(--amber) 12%, transparent);
}

.drawer-aside-handle__badge--irq {
  color: var(--yellow, var(--amber));
  border-color: var(--yellow, var(--amber));
  background: color-mix(in oklab, var(--yellow, var(--amber)) 12%, transparent);
}

.drawer-aside-handle__badge--irq.is-fired {
  box-shadow: 0 0 6px color-mix(in oklab, var(--yellow, var(--amber)) 55%, transparent);
  animation: drawer-handle-irq-pulse 700ms ease-in-out infinite alternate;
}

@keyframes drawer-handle-irq-pulse {
  from { opacity: 1; }
  to   { opacity: 0.55; }
}

.drawer-aside-handle__count {
  font-weight: 600;
}

.drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  background: var(--surface-overlay);
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  padding: 4px 8px;
  flex-shrink: 0;
  min-height: 28px;
}

.drawer-header__title {
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
}

.drawer-header__toggle {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  box-sizing: border-box;
  width: 22px;
  height: 22px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  transition: color 180ms ease, border-color 180ms ease, background 180ms ease;
}

.drawer-header__toggle:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.drawer-header__toggle:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}
</style>
