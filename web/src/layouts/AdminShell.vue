<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Gauge, Monitor, Users, Mail, Lock, Send, ScrollText, ExternalLink, LogOut, ShieldUser, BotMessageSquare, SlidersHorizontal, ShieldCheck } from '@lucide/vue';
import { useAdminStore } from '../stores/admin';
import { useToastStore } from '../stores/toast';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import ThemeSwitcher from '../components/shell/ThemeSwitcher.vue';
import Toast from '../components/ui/Toast.vue';

const { t, locale } = useI18n();
const admin = useAdminStore();
const router = useRouter();
const route = useRoute();
const toast = useToastStore();

// Mirror AppShell: keep document.title in sync with the active admin route
// (titleKey lookup) and refresh when the locale toggles.
watch(
  [() => route.meta.titleKey, locale],
  ([titleKey]) => {
    const label = typeof titleKey === 'string' && titleKey ? t(titleKey) : '';
    const doc = label ? `${label} · MODARCH` : 'MODARCH';
    if (typeof document !== 'undefined') document.title = doc;
  },
  { immediate: true },
);

// Items moderators can reach: dashboard + read tables + the kick action lives
// inside /sesiones and /usuarios. Admin-only items (Invitaciones, Accesos)
// are filtered out so the sidebar reflects the same gating the API enforces.
const items = computed(() => {
  const grp = t('shell.groups.admin');
  const baseAll = [
    { to: '/admin', label: t('router.titles.adminDashboard'), icon: Gauge, group: grp },
    { to: '/admin/sesiones', label: t('router.titles.adminSessions'), icon: Monitor, group: grp },
    { to: '/admin/usuarios', label: t('router.titles.adminUsers'), icon: Users, group: grp },
    { to: '/admin/invitaciones', label: t('router.titles.adminInvites'), icon: Mail, group: grp },
    { to: '/admin/accesos', label: t('router.titles.adminAccess'), icon: Lock, group: grp },
    { to: '/admin/smtp', label: t('router.titles.adminSmtp'), icon: Send, group: grp },
    { to: '/admin/auditoria', label: t('router.titles.adminAudit'), icon: ScrollText, group: grp },
    { to: '/admin/tutor', label: t('router.titles.adminTutorMetrics'), icon: BotMessageSquare, group: grp },
    { to: '/admin/tutor/config', label: t('router.titles.adminTutorConfig'), icon: SlidersHorizontal, group: grp },
    { to: '/admin/seguridad', label: t('router.titles.adminSecurity'), icon: ShieldCheck, group: grp },
  ];
  if (admin.isModerator) {
    return baseAll.filter(item =>
      item.to !== '/admin/invitaciones' &&
      item.to !== '/admin/accesos' &&
      item.to !== '/admin/smtp' &&
      item.to !== '/admin/tutor/config' &&
      item.to !== '/admin/seguridad'
    );
  }
  return baseAll;
});

let refreshTimer: number | undefined;

async function refresh() {
  try {
    await admin.refresh();
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('shell.adminTopbar.refreshError'));
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
          <ThemeSwitcher />
          <RouterLink to="/" class="admin-shell__link" :title="t('shell.adminTopbar.simulatorHint')">
            <ExternalLink :size="14" :stroke-width="2" />
            <span>{{ t('shell.adminTopbar.simulator') }}</span>
          </RouterLink>
          <button
            type="button"
            class="admin-shell__logout"
            :title="t('shell.adminTopbar.logoutHint')"
            @click="logout"
          >
            <LogOut :size="14" :stroke-width="2" />
            <span>{{ t('shell.adminTopbar.logout') }}</span>
          </button>
        </template>
      </Topbar>
      <main class="admin-shell__content">
        <div v-if="admin.isModerator" class="admin-shell__mod-banner" role="status">
          <ShieldUser :size="14" :stroke-width="2" />
          <span>{{ t('shell.adminTopbar.modBanner') }}</span>
        </div>
        <!-- Plain RouterView without a Transition wrapper: the previous
             out-in transition could deadlock when a leaving view never
             emitted transitionend (Teleport modals + 5s polling were the
             usual triggers), leaving the admin panel stuck on a blank
             screen with no console error. -->
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

</style>
