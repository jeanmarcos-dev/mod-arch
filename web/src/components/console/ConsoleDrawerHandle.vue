<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDown, FileText, Keyboard, Terminal, Zap } from '@lucide/vue';
import TutorBotIcon from '../tutor/TutorBotIcon.vue';

export type ConsoleDrawerTab = 'console' | 'modarchi';

interface Props {
  expanded: boolean;
  activeTab: ConsoleDrawerTab;
  unseenStdout: number;
  kbdReading: boolean;
  irqRecent: boolean;
  unreadModarchi: number;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  (e: 'toggle'): void;
  (e: 'select', tab: ConsoleDrawerTab): void;
}>();

const { t } = useI18n();

const isIdle = computed(() =>
  !props.unseenStdout && !props.kbdReading && !props.irqRecent && !props.unreadModarchi,
);
const toggleTitle = computed(() =>
  props.expanded ? t('panels.console.handle.collapseTitle') : t('panels.console.handle.expandTitle'),
);
const tutorBadgeLabel = computed(() =>
  props.unreadModarchi > 9 ? '9+' : String(props.unreadModarchi),
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
      <span
        v-if="unreadModarchi > 0"
        class="drawer-aside-handle__badge drawer-aside-handle__badge--tutor"
        :title="t('panels.console.handle.tutorTip')"
      >
        <TutorBotIcon :size="10" :stroke-width="2" />
        <span class="drawer-aside-handle__count">{{ tutorBadgeLabel }}</span>
      </span>
    </span>
  </div>

  <div v-else class="drawer-tabs">
    <div class="drawer-tabs__list" role="tablist">
      <button
        type="button"
        role="tab"
        class="drawer-tab"
        :class="{ 'is-active': activeTab === 'console' }"
        :aria-selected="activeTab === 'console'"
        :title="t('panels.console.tabs.consoleTitle')"
        @click="emit('select', 'console')"
      >
        <Terminal :size="13" :stroke-width="2" />
        <span>{{ t('panels.console.tabs.console') }}</span>
        <span
          v-if="activeTab !== 'console' && (unseenStdout > 0 || kbdReading || irqRecent)"
          class="drawer-tab__dot"
          :class="{ 'is-irq': irqRecent }"
          aria-hidden="true"
        />
      </button>
      <button
        type="button"
        role="tab"
        class="drawer-tab"
        :class="{ 'is-active': activeTab === 'modarchi' }"
        :aria-selected="activeTab === 'modarchi'"
        :title="t('panels.console.tabs.modarchiTitle')"
        @click="emit('select', 'modarchi')"
      >
        <TutorBotIcon :size="13" :stroke-width="2" />
        <span>{{ t('panels.console.tabs.modarchi') }}</span>
        <span
          v-if="activeTab !== 'modarchi' && unreadModarchi > 0"
          class="drawer-tab__badge"
          aria-hidden="true"
        >{{ tutorBadgeLabel }}</span>
      </button>
    </div>
    <button
      type="button"
      class="drawer-tabs__toggle"
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
  border: 1px solid var(--border);
  cursor: pointer;
  user-select: none;
  transition: background 120ms ease, border-color 120ms ease;
}

.drawer-aside-handle:hover {
  background: rgba(var(--accent-rgb), 0.08);
  border-color: var(--amber);
}

.drawer-aside-handle:focus-visible {
  background: rgba(var(--accent-rgb), 0.08);
  border-color: var(--amber);
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

.drawer-aside-handle__badge--tutor {
  color: var(--accent);
  border-color: var(--accent);
  background: color-mix(in oklab, var(--accent) 14%, transparent);
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

.drawer-tabs {
  display: flex;
  align-items: stretch;
  gap: 4px;
  background: var(--surface-overlay);
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  padding: 3px 6px;
  flex-shrink: 0;
  min-height: 30px;
}

.drawer-tabs__list {
  display: flex;
  gap: 4px;
  flex: 1 1 auto;
  min-width: 0;
}

.drawer-tab {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
  border: 1px solid var(--border);
  background: var(--panel-soft);
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
}

.drawer-tab:hover:not(.is-active) {
  color: var(--text);
  border-color: var(--border-strong);
  background: color-mix(in oklab, var(--accent) 6%, var(--panel-soft));
}

.drawer-tab.is-active {
  color: var(--amber);
  border-color: var(--amber);
  background: color-mix(in oklab, var(--accent) 14%, var(--panel-soft));
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.2);
}

.drawer-tab > svg { flex-shrink: 0; }
.drawer-tab > span {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.drawer-tab__dot {
  width: 6px;
  height: 6px;
  border-radius: 0;
  background: var(--cyan);
  flex-shrink: 0;
}

.drawer-tab__dot.is-irq {
  background: var(--yellow, var(--amber));
  animation: drawer-handle-irq-pulse 700ms ease-in-out infinite alternate;
}

.drawer-tab__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 14px;
  padding: 0 4px;
  height: 14px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0;
  color: var(--surface-0);
  background: var(--accent);
  flex-shrink: 0;
}

.drawer-tabs__toggle {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  box-sizing: border-box;
  width: 22px;
  height: 22px;
  align-self: center;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  transition: color 180ms ease, border-color 180ms ease, background 180ms ease;
}

.drawer-tabs__toggle:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.drawer-tabs__toggle:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}
</style>
