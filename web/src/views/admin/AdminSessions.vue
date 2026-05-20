<script setup lang="ts">
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import EmptyState from '../../components/ui/EmptyState.vue';
import { formatAgo } from './util';

const admin = useAdminStore();
const toast = useToastStore();

async function run(fn: () => Promise<void>, okMsg: string) {
  try { await fn(); toast.ok(okMsg); }
  catch (e) { toast.err(e instanceof Error ? e.message : 'Error'); }
}

async function killSession(id: string) {
  if (!confirm(`¿Cerrar la sesión ${id.slice(0, 8)}…? Sus clientes se desconectarán.`)) return;
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
      <Btn variant="ghost" size="sm" @click="admin.refresh()">↻ Refrescar</Btn>
    </template>

    <table v-if="admin.sessions.length" class="grid">
      <thead>
        <tr>
          <th>ID</th>
          <th>Modo</th>
          <th>Alias</th>
          <th>Compartir</th>
          <th>Creada</th>
          <th>Actividad</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="s in admin.sessions" :key="s.id">
          <td class="mono">{{ s.id.slice(0, 8) }}…</td>
          <td :class="'mode-' + s.mode">{{ s.mode.toUpperCase() }}</td>
          <td>
            <span v-if="s.label">{{ s.label }}</span>
            <span v-else class="dim">(sin alias)</span>
          </td>
          <td>
            <span v-if="s.has_share" :class="['pill', s.public ? 'pill--share' : 'pill--private']">
              {{ s.public ? '◆ PÚBLICA' : '◆ PRIVADA' }}
            </span>
            <span v-else class="dim">—</span>
          </td>
          <td class="dim">hace {{ formatAgo(s.created_at) }}</td>
          <td class="dim">hace {{ formatAgo(s.last_active) }}</td>
          <td class="row-actions">
            <Btn variant="ghost" size="sm" @click="setLabel(s.id, s.label)">alias</Btn>
            <Btn
              v-if="s.has_share"
              variant="ghost"
              size="sm"
              @click="togglePublic(s.id, !s.public)"
            >
              {{ s.public ? 'hacer privada' : 'hacer pública' }}
            </Btn>
            <Btn variant="danger" size="sm" @click="killSession(s.id)">cerrar</Btn>
          </td>
        </tr>
      </tbody>
    </table>
    <EmptyState
      v-else
      icon="◉"
      title="Sin sesiones activas"
      message="Cuando un alumno abra el simulador aparecerá aquí en tiempo real."
    />
  </Card>
</template>

<style scoped>
.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.grid th,
.grid td {
  padding: 9px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  text-align: left;
  vertical-align: middle;
}

.grid th {
  font-size: 10px;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  text-transform: uppercase;
  font-weight: 500;
}

.grid tbody tr:hover { background: rgba(255, 179, 71, 0.04); }

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
  background: rgba(255, 179, 71, 0.08);
}

.pill--private {
  color: var(--text-dim);
  border-color: var(--border);
}

.row-actions {
  display: flex;
  gap: var(--space-1);
  justify-content: flex-end;
  flex-wrap: wrap;
}
</style>
