<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { apiFetch } from '../../lib/api';
import { useAuthStore } from '../../stores/auth';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import EmptyState from '../../components/ui/EmptyState.vue';

interface MySession {
  id: string;
  mode: string;
  label: string;
  public: boolean;
  view_token?: string;
  created_at: number;
  last_active: number;
}

const auth = useAuthStore();
const toast = useToastStore();
const sessions = ref<MySession[]>([]);
const loading = ref(false);
const errMsg = ref<string | null>(null);

async function load() {
  loading.value = true;
  errMsg.value = null;
  try {
    const r = await apiFetch('/api/auth/sessions');
    if (r.status === 401) {
      errMsg.value = 'Identifíquese para ver sus sesiones.';
      sessions.value = [];
      return;
    }
    if (!r.ok) throw new Error('HTTP ' + r.status);
    sessions.value = (await r.json()) as MySession[];
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Error al cargar';
  } finally {
    loading.value = false;
  }
}

function timeAgo(unix: number): string {
  const d = Date.now() / 1000 - unix;
  if (d < 60) return Math.floor(d) + 's';
  if (d < 3600) return Math.floor(d / 60) + 'm';
  if (d < 86400) return Math.floor(d / 3600) + 'h';
  return Math.floor(d / 86400) + 'd';
}

async function copyViewURL(token: string) {
  const url = `${location.origin}/v/${token}`;
  try {
    await navigator.clipboard.writeText(url);
    toast.ok('URL del visor copiada');
  } catch {
    toast.err('No se pudo copiar al portapapeles');
  }
}

onMounted(load);
</script>

<template>
  <div class="my-sessions">
    <Card title="Mis sesiones" :elevation="1" class="my-sessions__card">
      <template #header>
        <Btn variant="ghost" size="sm" :loading="loading" @click="load">↻ Refrescar</Btn>
      </template>

      <div v-if="errMsg" class="my-sessions__err">{{ errMsg }}</div>

      <table v-else-if="sessions.length" class="grid">
        <thead>
          <tr>
            <th>ID</th>
            <th>Modo</th>
            <th>Alias</th>
            <th>Compartida</th>
            <th>Creada</th>
            <th>Actividad</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in sessions" :key="s.id">
            <td class="mono">{{ s.id.slice(0, 10) }}…</td>
            <td :class="'mode-' + s.mode">{{ s.mode.toUpperCase() }}</td>
            <td>
              <span v-if="s.label">{{ s.label }}</span>
              <span v-else class="dim">(sin alias)</span>
            </td>
            <td>
              <span v-if="s.view_token" class="pill">◆ pública</span>
              <span v-else class="dim">—</span>
            </td>
            <td class="dim">hace {{ timeAgo(s.created_at) }}</td>
            <td class="dim">hace {{ timeAgo(s.last_active) }}</td>
            <td class="row-actions">
              <Btn
                v-if="s.view_token"
                variant="ghost"
                size="sm"
                @click="copyViewURL(s.view_token)"
              >URL visor</Btn>
            </td>
          </tr>
        </tbody>
      </table>

      <EmptyState
        v-else-if="!auth.user"
        icon="◐"
        title="No ha iniciado sesión"
        message="Identifíquese para que sus sesiones queden asociadas a su nombre y pueda recuperarlas más adelante."
      >
        <RouterLink to="/login" class="my-sessions__cta">→ Identificarse</RouterLink>
      </EmptyState>

      <EmptyState
        v-else
        icon="⎙"
        title="Aún no tiene sesiones"
        message="Al abrir el simulador y permanecer un instante quedará registrada su sesión. Sólo aparecen las creadas mientras estaba identificado."
      />
    </Card>
  </div>
</template>

<style scoped>
.my-sessions {
  flex: 1;
  padding: var(--space-5);
  overflow-y: auto;
}

.my-sessions__card {
  max-width: 960px;
  margin: 0 auto;
}

.my-sessions__err {
  color: var(--red-soft);
  font-size: 13px;
  padding: var(--space-3) 0;
}

.my-sessions__cta {
  color: var(--amber);
  text-decoration: underline;
  text-decoration-style: dashed;
}

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

.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }

.mode-full { color: var(--green); letter-spacing: 1px; }
.mode-privileged { color: var(--yellow); letter-spacing: 1px; }

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
