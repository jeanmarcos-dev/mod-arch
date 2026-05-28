<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Share2, Square, UserRound, LogOut } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useAuthStore } from '../../stores/auth';
import ShareModal from '../ShareModal.vue';

const cpu = useCpuStore();
const auth = useAuthStore();
const router = useRouter();
const route = useRoute();
const { t } = useI18n();

const canShare = computed(() => (cpu.sessionInfo?.role ?? 'none') === 'owner');
const isShared = computed(() => Boolean(cpu.sessionInfo?.view_token));
const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));
const viewer = computed(() => cpu.isViewer());
// Identifying yourself is a student/owner action — inside /admin the operator
// is already authenticated through the admin cookie and a magic-link login
// adds nothing, so we hide the button on those routes.
const onAdminRoute = computed(() => route.path === '/admin' || route.path.startsWith('/admin/'));

const shareOpen = ref(false);
function openShare() { shareOpen.value = true; }
async function goLogin() { await router.push('/login'); }
async function doLogout() { await auth.logout(); }
</script>

<template>
  <header class="mini-bar" role="banner" data-pizarra-anchor="topbar">
    <span class="mini-bar__brand">{{ t('topbar.brand') }}</span>

    <div class="mini-bar__spacer"></div>

    <div
      v-if="loggedIn"
      class="mini-bar__user"
      :title="cpu.sessionInfo.user_email
        ? t('topbar.connectedAsWithEmail', { name: cpu.sessionInfo.user_name, email: cpu.sessionInfo.user_email })
        : t('topbar.connectedAs', { name: cpu.sessionInfo.user_name })"
    >
      <UserRound :size="14" :stroke-width="2" class="mini-bar__user-icon" />
      <span class="mini-bar__user-name">{{ cpu.sessionInfo.user_name }}</span>
      <button type="button" class="mini-bar__ghost-btn" :title="t('topbar.logout')" @click="doLogout">
        <LogOut :size="14" :stroke-width="2" />
      </button>
    </div>
    <button
      v-else-if="!viewer && !onAdminRoute"
      type="button"
      class="mini-bar__login"
      :title="t('topbar.identifyHint')"
      @click="goLogin"
    >
      <UserRound :size="14" :stroke-width="2" />
      <span>{{ t('topbar.identify') }}</span>
    </button>

    <button
      v-if="canShare"
      type="button"
      :class="['mini-bar__share', { 'mini-bar__share--on': isShared }]"
      :title="isShared ? t('topbar.shareManageHint') : t('topbar.shareHint')"
      @click="openShare"
    >
      <component :is="isShared ? Square : Share2" :size="14" :stroke-width="2" />
      <span>{{ isShared ? t('topbar.sharing') : t('topbar.share') }}</span>
    </button>

    <slot name="actions" />

    <ShareModal :open="shareOpen" @close="shareOpen = false" />
  </header>
</template>

<style scoped>
.mini-bar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  height: 44px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.mini-bar__brand {
  font-family: var(--font-display);
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 12px;
}

.mini-bar__spacer { flex: 1; }

.mini-bar__user {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 6px 0 8px;
  height: 28px;
  color: var(--text);
}
.mini-bar__user-icon { color: var(--amber); }
.mini-bar__user-name { font-size: 11px; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mini-bar__ghost-btn {
  background: transparent;
  border: none;
  color: var(--text-dim);
  cursor: pointer;
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  padding: 0;
}
.mini-bar__ghost-btn:hover { background: rgba(var(--accent-rgb), 0.10); color: var(--amber); }

/* "Identificarse" entry point — matches the share button so the topbar reads
   as a row of equally-weighted actions. */
.mini-bar__login {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
  white-space: nowrap;
}
.mini-bar__login:hover {
  background: rgba(var(--accent-rgb), 0.12);
  color: var(--amber);
  border-color: var(--border-strong);
}

.mini-bar__share {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
}
.mini-bar__share:hover { background: rgba(var(--accent-rgb), 0.12); color: var(--amber); border-color: var(--border-strong); }
.mini-bar__share--on {
  background: rgba(127, 255, 170, 0.10);
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green);
}

@media (max-width: 768px) {
  .mini-bar {
    flex-wrap: wrap;
    height: auto;
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    gap: var(--space-2);
  }
  .mini-bar__spacer { display: none; }
  .mini-bar__login,
  .mini-bar__user { margin-left: auto; }
}

@media (max-width: 700px) {
  .mini-bar__brand { display: none; }
}

@media (max-width: 480px) {
  /* Leave room for the floating hamburger button at the top-left. */
  .mini-bar { padding-left: 52px; }
  .mini-bar__share span { display: none; }
  .mini-bar__share { padding: 0 8px; }
}
</style>
