<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAdminStore } from '../stores/admin';
import { useToastStore } from '../stores/toast';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import Toast from '../components/ui/Toast.vue';

const admin = useAdminStore();
const router = useRouter();
const toast = useToastStore();

const items = [
  { to: '/admin', label: 'Resumen', icon: '◉', group: 'ADMINISTRACIÓN' },
  { to: '/admin/sesiones', label: 'Sesiones', icon: '▶', group: 'ADMINISTRACIÓN' },
  { to: '/admin/usuarios', label: 'Usuarios', icon: '☖', group: 'ADMINISTRACIÓN' },
  { to: '/admin/invitaciones', label: 'Invitaciones', icon: '✉', group: 'ADMINISTRACIÓN' },
  { to: '/admin/accesos', label: 'Accesos', icon: '⚿', group: 'ADMINISTRACIÓN' },
  { to: '/admin/auditoria', label: 'Auditoría', icon: '☰', group: 'ADMINISTRACIÓN' },
];

let refreshTimer: number | undefined;

async function refresh() {
  try {
    await admin.refresh();
  } catch (e) {
    toast.err(e instanceof Error ? e.message : 'Error al refrescar');
  }
}

async function logout() {
  await admin.logout();
  await router.replace({ name: 'admin-login' });
}

onMounted(() => {
  refresh();
  refreshTimer = window.setInterval(refresh, 5000);
});

onUnmounted(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer);
});
</script>

<template>
  <div class="admin-shell">
    <Sidebar :items="items" heading="ADMIN" />
    <div class="admin-shell__main">
      <Topbar>
        <template #actions>
          <RouterLink to="/" class="admin-shell__link" title="Volver al simulador">
            ↗ Simulador
          </RouterLink>
          <button
            type="button"
            class="admin-shell__logout"
            title="Cerrar sesión de admin"
            @click="logout"
          >
            ↩ Salir
          </button>
        </template>
      </Topbar>
      <main class="admin-shell__content">
        <RouterView />
      </main>
    </div>
    <Toast />
  </div>
</template>

<style scoped>
.admin-shell {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}

.admin-shell__main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.admin-shell__content {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.admin-shell__link,
.admin-shell__logout {
  font-family: inherit;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  padding: 6px 10px;
  cursor: pointer;
  text-decoration: none;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast);
}

.admin-shell__link:hover,
.admin-shell__logout:hover {
  background: rgba(255, 179, 71, 0.08);
  color: var(--amber);
}
</style>
