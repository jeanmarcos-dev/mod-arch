<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ScrollText } from '@lucide/vue';
import { useAdminStore, type AuditRow } from '../../stores/admin';
import Card from '../../components/ui/Card.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import { formatTimestamp } from './util';

const { t } = useI18n();
const admin = useAdminStore();

const columns = computed<Column<AuditRow>[]>(() => [
  { id: 'ts', label: t('admin.audit.colWhen'), accessor: (r) => formatTimestamp(r.ts), width: '170px' },
  { id: 'actor', label: t('admin.audit.colActor') },
  { id: 'action', label: t('admin.audit.colAction') },
  { id: 'target', label: t('admin.audit.colTarget') },
  { id: 'meta', label: t('admin.audit.colMeta') },
]);
</script>

<template>
  <Card :title="t('admin.audit.title')">
    <DataTable
      :columns="columns"
      :rows="admin.audit"
      initial-sort="ts"
      :search-placeholder="t('admin.audit.searchPlaceholder')"
      :empty-icon="ScrollText"
      :empty-title="t('admin.audit.emptyTitle')"
      :empty-message="t('admin.audit.emptyMessage')"
    >
      <template #[`cell:ts`]="{ value }">
        <span class="mono dim">{{ value }}</span>
      </template>
      <template #[`cell:actor`]="{ row }">
        <span class="mono">{{ row.actor }}</span>
      </template>
      <template #[`cell:action`]="{ row }">
        <span class="action">{{ row.action }}</span>
      </template>
      <template #[`cell:target`]="{ row }">
        <span class="mono">{{ row.target || '—' }}</span>
      </template>
      <template #[`cell:meta`]="{ row }">
        <span class="mono dim">{{ row.meta || '' }}</span>
      </template>
    </DataTable>
  </Card>
</template>

<style scoped>
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }

.action {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
}
</style>
