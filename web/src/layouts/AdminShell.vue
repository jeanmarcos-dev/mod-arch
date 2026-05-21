<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { Gauge, Monitor, Users, Mail, Lock, ScrollText, ExternalLink, LogOut, ShieldUser } from '@lucide/vue';
import { useAdminStore } from '../stores/admin';
import { useToastStore } from '../stores/toast';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import Toast from '../components/ui/Toast.vue';

const admin = useAdminStore();
const router = useRouter();
const toast = useToastStore();

// Items moderators can reach: dashboard + read tables + the kick action lives
// inside /sesiones and /usuarios. Admin-only items (Invitaciones, Accesos)
// are filtered out so the sidebar reflects the same gating the API enforces.
const items = computed(() => {
  const baseAll = [
    { to: '/admin', label: 'Resumen', icon: Gauge, group: 'ADMINISTRACIÓN' },
    { to: '/admin/sesiones', label: 'Sesiones', icon: Monitor, group: 'ADMINISTRACIÓN' },
    { to: '/admin/usuarios', label: 'Usuarios', icon: Users, group: 'ADMINISTRACIÓN' },
    { to: '/admin/invitaciones', label: 'Invitaciones', icon: Mail, group: 'ADMINISTRACIÓN' },
    { to: '/admin/accesos', label: 'Accesos', icon: Lock, group: 'ADMINISTRACIÓN' },
    { to: '/admin/auditoria', label: 'Auditoría', icon: ScrollText, group: 'ADMINISTRACIÓN' },
  ];
  if (admin.isModerator) {
    return baseAll.filter(item =>
      item.to !== '/admin/invitaciones' &&
      item.to !== '/admin/accesos'
    );
  }
  return baseAll;
});

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
    <Sidebar :items="items" />
    <div class="admin-shell__main">
      <Topbar>
        <template #actions>
          <RouterLink to="/" class="admin-shell__link" title="Volver al simulador">
            <ExternalLink :size="14" :stroke-width="2" />
            <span>Simulador</span>
          </RouterLink>
          <button
            type="button"
            class="admin-shell__logout"
            title="Cerrar sesión de admin"
            @click="logout"
          >
            <LogOut :size="14" :stroke-width="2" />
            <span>Salir</span>
          </button>
        </template>
      </Topbar>
      <main class="admin-shell__content">
        <div v-if="admin.isModerator" class="admin-shell__mod-banner" role="status">
          <ShieldUser :size="14" :stroke-width="2" />
          <span>Sesión de moderación — algunas acciones de gobierno quedan reservadas al administrador.</span>
        </div>
        <RouterView v-slot="{ Component }">
          <Transition name="route" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
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
  padding: 0 10px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  text-decoration: none;
  border-radius: var(--radius-sm);
  box-sizing: border-box;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.admin-shell__link:hover,
.admin-shell__logout:hover {
  background: rgba(var(--accent-rgb), 0.08);
  color: var(--amber);
}

.admin-shell__mod-banner {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  align-self: flex-start;
  padding: 6px 12px;
  background: rgba(106, 212, 255, 0.08);
  border: 1px solid var(--cyan);
  color: var(--cyan);
  border-radius: var(--radius-sm);
  font-size: 11px;
  letter-spacing: 0.5px;
}

.route-enter-from { opacity: 0; transform: translateY(8px); }
.route-leave-to   { opacity: 0; transform: translateY(-6px); }
.route-enter-active,
.route-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
</style>
