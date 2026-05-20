<script setup lang="ts">
import { Users } from '@lucide/vue';
import { useAdminStore, type UserRow } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import { formatTimestamp } from './util';

const admin = useAdminStore();
const toast = useToastStore();

const columns: Column<UserRow>[] = [
  { id: 'name', label: 'Nombre' },
  { id: 'email', label: 'Correo', accessor: (u) => u.email || '' },
  { id: 'student_id', label: 'ID estudiante', accessor: (u) => u.student_id || '' },
  { id: 'last_login_at', label: 'Último ingreso', accessor: (u) => u.last_login_at, width: '170px' },
  { id: 'sessions', label: 'Activas', accessor: (u) => u.sessions.length, align: 'right' },
];

async function kickUser(id: string, name: string) {
  if (!confirm(`¿Expulsar al usuario ${name}? Sus sesiones activas se cerrarán.`)) return;
  try {
    await admin.kickUser(id);
    toast.ok('Usuario expulsado');
  } catch (e) {
    toast.err(e instanceof Error ? e.message : 'Error');
  }
}
</script>

<template>
  <Card title="Usuarios identificados">
    <DataTable
      :columns="columns"
      :rows="admin.users"
      initial-sort="last_login_at"
      search-placeholder="Buscar por nombre, correo o ID estudiante…"
      :empty-icon="Users"
      empty-title="Sin usuarios identificados"
      empty-message="Cuando un alumno se identifique con su nombre y correo aparecerá aquí."
    >
      <template #[`cell:email`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:student_id`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:last_login_at`]="{ row }">
        <span class="dim mono">{{ formatTimestamp(row.last_login_at) }}</span>
      </template>
      <template #[`cell:sessions`]="{ row }">
        <span v-if="row.sessions.length" class="pill">{{ row.sessions.length }}</span>
        <span v-else class="dim">—</span>
      </template>
      <template #actions="{ row }">
        <Btn
          variant="danger"
          size="sm"
          :disabled="!row.sessions.length"
          @click="kickUser(row.id, row.name)"
        >expulsar</Btn>
      </template>
    </DataTable>
  </Card>
</template>

<style scoped>
.dim { color: var(--text-dim); }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
  background: rgba(255, 179, 71, 0.08);
}
</style>
