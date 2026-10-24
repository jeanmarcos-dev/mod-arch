<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { LifeBuoy } from '@lucide/vue';
import { useReportStore } from '../stores/report';

const props = withDefaults(defineProps<{ compact?: boolean; variant?: 'default' | 'bar' }>(), {
  compact: false,
  variant: 'default',
});

const { t } = useI18n();
const reports = useReportStore();
</script>

<template>
  <button
    type="button"
    class="report-btn icon-btn"
    :class="{ 'report-btn--compact': props.compact, 'report-btn--bar': props.variant === 'bar' }"
    :title="t('report.launcherTitle')"
    :aria-label="props.compact ? t('report.launcher') : undefined"
    @click="reports.show()"
  >
    <LifeBuoy :size="14" :stroke-width="2" />
    <span
      class="report-btn__label"
      :class="{ 'report-btn__label--sr': props.compact }"
      data-testid="report-btn-label"
    >{{ t('report.launcher') }}</span>
  </button>
</template>

<style scoped>
.report-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  gap: var(--space-2);
  height: 28px;
  padding: 0 var(--space-3);
  min-height: 0;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  transition: color var(--transition-fast), border-color var(--transition-fast), background var(--transition-fast);
}
.report-btn svg { display: block; flex-shrink: 0; }
.report-btn__label { white-space: nowrap; }
.report-btn--bar {
  height: 32px;
  padding: 0 12px;
  gap: 6px;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  line-height: 1;
  background: rgba(var(--accent-rgb), 0.05);
}
.report-btn--bar .report-btn__label { line-height: 1; }
@media (max-width: 768px) {
  .report-btn--bar { height: 44px; }
}
.report-btn--compact {
  width: 28px;
  padding: 0;
  gap: 0;
}
.report-btn__label--sr {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}
@media (max-width: 480px) {
  .report-btn { width: 28px; padding: 0; gap: 0; }
  .report-btn--bar { width: auto; min-width: 44px; padding: 0 10px; }
  .report-btn__label {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
}
.report-btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.06);
}
</style>
