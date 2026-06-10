<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { LifeBuoy } from '@lucide/vue';
import { useReportStore } from '../stores/report';

const { t } = useI18n();
const reports = useReportStore();
</script>

<template>
  <button
    type="button"
    class="report-btn icon-btn"
    :title="t('report.launcherTitle')"
    :aria-label="t('report.launcherTitle')"
    @click="reports.show()"
  >
    <LifeBuoy :size="14" :stroke-width="2" />
  </button>
</template>

<style scoped>
.report-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  width: 28px;
  height: 28px;
  padding: 0;
  min-height: 0;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  transition: color var(--transition-fast), border-color var(--transition-fast), background var(--transition-fast);
}
.report-btn svg { display: block; }
.report-btn:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.06);
}
</style>
