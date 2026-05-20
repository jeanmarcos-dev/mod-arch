<script setup lang="ts">
import { useAdminStore } from '../../stores/admin';
import Card from '../../components/ui/Card.vue';
import EmptyState from '../../components/ui/EmptyState.vue';
import { formatTimestamp } from './util';

const admin = useAdminStore();
</script>

<template>
  <Card title="Registro de auditoría">
    <table v-if="admin.audit.length" class="grid">
      <thead>
        <tr>
          <th>Cuándo</th>
          <th>Actor</th>
          <th>Acción</th>
          <th>Objetivo</th>
          <th>Meta</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in admin.audit" :key="e.id">
          <td class="dim mono">{{ formatTimestamp(e.ts) }}</td>
          <td class="mono">{{ e.actor }}</td>
          <td><span class="action">{{ e.action }}</span></td>
          <td class="mono">{{ e.target || '—' }}</td>
          <td class="mono dim">{{ e.meta || '' }}</td>
        </tr>
      </tbody>
    </table>
    <EmptyState
      v-else
      icon="☰"
      title="El registro está vacío"
      message="Cada acción privilegiada (cerrar sesión, expulsar usuario, cambiar política de acceso, etc.) deja un rastro aquí."
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
  padding: 8px 12px;
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
