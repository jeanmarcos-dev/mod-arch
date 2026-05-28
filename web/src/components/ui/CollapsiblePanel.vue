<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDown, ChevronRight } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';

interface Props {
  title: string;
  collapsed: boolean;
  titleHint?: string;
  glossaryKey?: string;
}

const props = defineProps<Props>();
const emit = defineEmits<{ (e: 'update:collapsed', value: boolean): void }>();

const cpu = useCpuStore();
const { t } = useI18n();

const isOpen = computed(() => !props.collapsed);
const toggleLabel = computed(() =>
  props.collapsed ? t('panels.collapse.expand') : t('panels.collapse.collapse'),
);

function toggle() {
  emit('update:collapsed', !props.collapsed);
}

function openGlossary() {
  if (props.glossaryKey) cpu.openGlossary(props.glossaryKey);
}
</script>

<template>
  <section :class="['cpanel', { 'cpanel--collapsed': collapsed }]">
    <div class="cpanel__head">
      <div
        class="panel-title cpanel__title"
        :class="{ 'clickable-title': !!glossaryKey }"
        :title="titleHint"
        @click="openGlossary"
      >{{ title }}</div>
      <div class="cpanel__actions">
        <slot name="header-actions" />
        <span v-if="$slots['header-actions']" class="cpanel__actions-divider" aria-hidden="true" />
        <button
          type="button"
          class="cpanel__toggle"
          :title="toggleLabel"
          :aria-label="toggleLabel"
          :aria-expanded="isOpen"
          @click="toggle"
        >
          <ChevronDown v-if="isOpen" :size="14" :stroke-width="2" />
          <ChevronRight v-else :size="14" :stroke-width="2" />
        </button>
      </div>
    </div>
    <div v-show="isOpen" class="cpanel__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
/* Default to "claim a share of the parent's flex axis" so siblings in a flex
   column (e.g. the procesos / colas / IPC stack on the right) split the
   available height evenly. The grow weight comes from the --cpanel-weight
   custom property set by the parent on this root via inline style — that
   way we never compete against the parent's shorthand `flex` declaration
   when toggling collapsed, which is the only state that needs to override
   the sizing. */
.cpanel {
  display: flex;
  flex-direction: column;
  flex-grow: var(--cpanel-weight, 1);
  flex-shrink: 1;
  flex-basis: 0;
  min-height: 0;
}

.cpanel--collapsed {
  flex-grow: 0 !important;
  flex-shrink: 0 !important;
  flex-basis: auto !important;
}

.cpanel__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 8px;
  border-bottom: 1px dashed var(--border-strong);
  padding-bottom: 5px;
  margin-bottom: 8px;
  flex-shrink: 0;
}

.cpanel--collapsed .cpanel__head {
  margin-bottom: 0;
  border-bottom-style: solid;
}

/* The wrapped panel-title already provides its own underline at the global
   level; we suppress that here so the dashed line on .cpanel__head spans the
   whole header (mirrors the previous in-component proc-header pattern). */
.cpanel__head .panel-title {
  border-bottom: none;
  padding-bottom: 0;
  margin-bottom: 0;
}

.cpanel__title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cpanel__actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.cpanel__actions-divider {
  display: inline-block;
  width: 1px;
  height: 16px;
  background: var(--border);
}

.cpanel__toggle {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  box-sizing: border-box;
  width: 22px;
  height: 22px;
  min-height: 0;
  padding: 0;
  font-size: 0;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: color 0.18s ease, border-color 0.18s ease, background 0.18s ease;
}

.cpanel__toggle :deep(svg) {
  transition: opacity 0.18s ease;
}

.cpanel__toggle:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.cpanel__toggle:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: 1px;
}

.cpanel__body {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}
</style>
