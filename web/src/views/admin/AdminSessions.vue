<script setup lang="ts">
import { ref } from 'vue';
import { RotateCw, Monitor } from '@lucide/vue';
import { useAdminStore, type SessionRow } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import { formatAgo } from './util';

const admin = useAdminStore();
const toast = useToastStore();

const refreshing = ref(false);

async function doRefresh() {
  if (refreshing.value) return;
  refreshing.value = true;
  try {
    await admin.refresh();
  } catch (e) {
    toast.err(e instanceof Error ? e.message : 'Error al refrescar');
  } finally {
    setTimeout(() => { refreshing.value = false; }, 350);
  }
}

const columns: Column<SessionRow>[] = [
  { id: 'id', label: 'ID' },
  { id: 'mode', label: 'Modo' },
  { id: 'label', label: 'Alias' },
  { id: 'public', label: 'Compartir', accessor: (s) => (s.has_share ? (s.public ? 'pública' : 'privada') : '') },
  { id: 'created_at', label: 'Creada', accessor: (s) => s.created_at, width: '110px' },
  { id: 'last_active', label: 'Actividad', accessor: (s) => s.last_active, width: '110px' },
];

async function run(fn: () => Promise<void>, okMsg: string) {
  try { await fn(); toast.ok(okMsg); }
  catch (e) { toast.err(e instanceof Error ? e.message : 'Error'); }
}

async function killSession(id: string) {
  if (!confirm(`¿Cerrar la sesión ${id.slice(0, 8)}…?`)) return;
  await run(() => admin.killSession(id), 'Sesión cerrada');
}

async function setLabel(id: string, current: string) {
  const label = window.prompt('Nuevo alias para la sesión:', current);
  if (label === null) return;
  await run(() => admin.setLabel(id, label.trim()), 'Alias actualizado');
}

async function togglePublic(id: string, makePublic: boolean) {
  await run(
    () => admin.togglePublic(id, makePublic),
    makePublic ? 'Marcada pública' : 'Marcada privada',
  );
}
</script>

<template>
  <Card title="Sesiones activas">
    <template #header>
      <Btn variant="ghost" size="sm" :loading="refreshing" @click="doRefresh">
        <RotateCw :size="13" :stroke-width="2" :class="{ 'icon--spin': refreshing }" />
        <span>Refrescar</span>
      </Btn>
    </template>

    <DataTable
      :columns="columns"
      :rows="admin.sessions"
      initial-sort="last_active"
      search-placeholder="Buscar por id, alias o modo…"
      :empty-icon="Monitor"
      empty-title="Sin sesiones activas"
      empty-message="Cuando un alumno abra el simulador aparecerá aquí en tiempo real."
    >
      <template #[`cell:id`]="{ row }">
        <span class="mono">{{ row.id.slice(0, 8) }}…</span>
      </template>
      <template #[`cell:mode`]="{ row }">
        <span :class="'mode-' + row.mode">{{ row.mode.toUpperCase() }}</span>
      </template>
      <template #[`cell:label`]="{ row }">
        <span v-if="row.label">{{ row.label }}</span>
        <span v-else class="dim">(sin alias)</span>
      </template>
      <template #[`cell:public`]="{ row }">
        <span v-if="row.has_share" :class="['pill', row.public ? 'pill--share' : 'pill--private']">
          {{ row.public ? '◆ pública' : '◆ privada' }}
        </span>
        <span v-else class="dim">—</span>
      </template>
      <template #[`cell:created_at`]="{ row }">
        <span class="dim">hace {{ formatAgo(row.created_at) }}</span>
      </template>
      <template #[`cell:last_active`]="{ row }">
        <span class="dim">hace {{ formatAgo(row.last_active) }}</span>
      </template>
      <template #actions="{ row }">
        <Btn variant="ghost" size="sm" @click="setLabel(row.id, row.label)">alias</Btn>
        <Btn
          v-if="row.has_share"
          variant="ghost"
          size="sm"
          @click="togglePublic(row.id, !row.public)"
        >
          {{ row.public ? '→ privada' : '→ pública' }}
        </Btn>
        <Btn variant="danger" size="sm" @click="killSession(row.id)">cerrar</Btn>
      </template>
    </DataTable>
  </Card>
</template>

<style scoped>
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }

.mode-full { color: var(--green); letter-spacing: 1px; }
.mode-privileged { color: var(--yellow); letter-spacing: 1px; }

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid;
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
}

.pill--share {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.pill--private {
  color: var(--text-dim);
  border-color: var(--border);
}

.icon--spin {
  animation: spin 0.7s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
