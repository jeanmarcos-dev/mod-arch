<script setup lang="ts">
import { useAdminStore, type AuditRow } from '../../stores/admin';
import Card from '../../components/ui/Card.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import { formatTimestamp } from './util';

const admin = useAdminStore();

const columns: Column<AuditRow>[] = [
  { id: 'ts', label: 'Cuándo', accessor: (r) => formatTimestamp(r.ts), width: '170px' },
  { id: 'actor', label: 'Actor' },
  { id: 'action', label: 'Acción' },
  { id: 'target', label: 'Objetivo' },
  { id: 'meta', label: 'Meta' },
];
</script>

<template>
  <Card title="Registro de auditoría">
    <DataTable
      :columns="columns"
      :rows="admin.audit"
      initial-sort="ts"
      search-placeholder="Buscar por actor, acción u objetivo…"
      empty-icon="☰"
      empty-title="El registro está vacío"
      empty-message="Cada acción privilegiada (cerrar sesión, expulsar usuario, cambiar política de acceso, etc.) deja un rastro aquí."
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
