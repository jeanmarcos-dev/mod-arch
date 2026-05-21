<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { Share2, Square, UserRound, LogIn, LogOut } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useAuthStore } from '../../stores/auth';
import ShareModal from '../ShareModal.vue';

const cpu = useCpuStore();
const auth = useAuthStore();
const router = useRouter();

const canShare = computed(() => (cpu.sessionInfo?.role ?? 'none') === 'owner');
const isShared = computed(() => Boolean(cpu.sessionInfo?.view_token));
const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));
const viewer = computed(() => cpu.isViewer());

const aliasInput = ref(cpu.sessionInfo.label);
watch(() => cpu.sessionInfo.label, (lab) => { aliasInput.value = lab; });
function commitAlias() {
  const v = aliasInput.value.trim().slice(0, 32);
  if (v !== cpu.sessionInfo.label) cpu.setAlias(v);
}

const shareOpen = ref(false);
function openShare() { shareOpen.value = true; }
async function goLogin() { await router.push('/login'); }
async function doLogout() { await auth.logout(); }
</script>

<template>
  <header class="mini-bar" role="banner" data-pizarra-anchor="topbar">
    <span class="mini-bar__brand">MODOS-CPU</span>

    <div class="mini-bar__spacer"></div>

    <div
      v-if="loggedIn"
      class="mini-bar__user"
      :title="cpu.sessionInfo.user_email ? `Conectado como ${cpu.sessionInfo.user_name} · ${cpu.sessionInfo.user_email}` : `Conectado como ${cpu.sessionInfo.user_name}`"
    >
      <UserRound :size="14" :stroke-width="2" class="mini-bar__user-icon" />
      <span class="mini-bar__user-name">{{ cpu.sessionInfo.user_name }}</span>
      <button type="button" class="mini-bar__ghost-btn" title="Cerrar sesión" @click="doLogout">
        <LogOut :size="14" :stroke-width="2" />
      </button>
    </div>
    <label v-else-if="!viewer" class="mini-bar__alias">
      <UserRound :size="14" :stroke-width="2" class="mini-bar__user-icon" />
      <input
        v-model="aliasInput"
        class="mini-bar__alias-input"
        type="text"
        maxlength="32"
        placeholder="su alias…"
        autocomplete="off"
        aria-label="Alias visible para los espectadores"
        @blur="commitAlias"
        @keydown.enter="commitAlias"
      />
      <button type="button" class="mini-bar__ghost-btn" title="Iniciar sesión con correo" @click="goLogin">
        <LogIn :size="14" :stroke-width="2" />
      </button>
    </label>

    <button
      v-if="canShare"
      type="button"
      :class="['mini-bar__share', { 'mini-bar__share--on': isShared }]"
      :title="isShared ? 'Sesión compartida — clic para gestionarla' : 'Generar un enlace para que otros vean la sesión en modo lectura'"
      @click="openShare"
    >
      <component :is="isShared ? Square : Share2" :size="14" :stroke-width="2" />
      <span>{{ isShared ? 'Compartiendo' : 'Compartir' }}</span>
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
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 12px;
}

.mini-bar__spacer { flex: 1; }

.mini-bar__user,
.mini-bar__alias {
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
.mini-bar__alias-input {
  background: transparent;
  border: none;
  color: var(--text);
  font-family: inherit;
  font-size: 11px;
  width: 120px;
  outline: none;
}
.mini-bar__alias-input::placeholder { color: var(--text-muted); }

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

@media (max-width: 700px) {
  .mini-bar__brand { display: none; }
}
</style>
