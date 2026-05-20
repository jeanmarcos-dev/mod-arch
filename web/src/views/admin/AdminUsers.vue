<script setup lang="ts">
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import EmptyState from '../../components/ui/EmptyState.vue';
import { formatTimestamp } from './util';

const admin = useAdminStore();
const toast = useToastStore();

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
    <table v-if="admin.users.length" class="grid">
      <thead>
        <tr>
          <th>Nombre</th>
          <th>Correo</th>
          <th>ID estudiante</th>
          <th>Último ingreso</th>
          <th>Sesiones activas</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="u in admin.users" :key="u.id">
          <td>{{ u.name }}</td>
          <td class="dim">{{ u.email || '—' }}</td>
          <td class="dim">{{ u.student_id || '—' }}</td>
          <td class="dim">{{ formatTimestamp(u.last_login_at) }}</td>
          <td>
            <span v-if="u.sessions.length" class="pill">{{ u.sessions.length }} activa(s)</span>
            <span v-else class="dim">—</span>
          </td>
          <td class="row-actions">
            <Btn
              variant="danger"
              size="sm"
              :disabled="!u.sessions.length"
              @click="kickUser(u.id, u.name)"
            >expulsar</Btn>
          </td>
        </tr>
      </tbody>
    </table>
    <EmptyState
      v-else
      icon="☖"
      title="Sin usuarios identificados"
      message="Cuando un alumno se identifique con su nombre y correo aparecerá aquí. Sólo se registran identificaciones explícitas; el modo anónimo no genera entradas."
    />
  </Card>
</template>

<style scoped>
.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.grid th, .grid td {
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

.dim { color: var(--text-dim); }

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
  background: rgba(255, 179, 71, 0.08);
}

.row-actions {
  display: flex;
  gap: var(--space-1);
  justify-content: flex-end;
}
</style>
