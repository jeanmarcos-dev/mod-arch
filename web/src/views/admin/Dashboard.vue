<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAdminStore } from '../../stores/admin';

const admin = useAdminStore();
const router = useRouter();

const inviteLabel = ref('');
const toast = ref<{ message: string; tone: 'ok' | 'err' } | null>(null);
const busy = ref(false);
let timer: number | undefined;

function notify(message: string, tone: 'ok' | 'err' = 'ok') {
  toast.value = { message, tone };
  window.setTimeout(() => {
    if (toast.value?.message === message) toast.value = null;
  }, 2500);
}

async function load() {
  try {
    await admin.refresh();
  } catch (e) {
    notify(e instanceof Error ? e.message : 'Error al refrescar', 'err');
  }
}

async function killSession(id: string) {
  if (!confirm(`¿Está seguro de cerrar la sesión ${id.slice(0, 8)}…? Sus clientes se desconectarán.`)) return;
  await run(() => admin.killSession(id), 'Sesión cerrada');
}

async function setLabel(id: string, current: string) {
  const label = window.prompt('Nuevo alias para la sesión:', current);
  if (label === null) return;
  await run(() => admin.setLabel(id, label.trim()), 'Alias actualizado');
}

async function togglePublic(id: string, makePublic: boolean) {
  await run(() => admin.togglePublic(id, makePublic), makePublic ? 'Marcada pública' : 'Marcada privada');
}

async function createInvite() {
  const label = inviteLabel.value.trim();
  busy.value = true;
  try {
    const inv = await admin.createInvite(label);
    inviteLabel.value = '';
    try {
      await navigator.clipboard.writeText(inv.url);
      notify('Invitación creada y URL copiada al portapapeles');
    } catch {
      notify('Invitación creada: ' + inv.url);
    }
  } catch (e) {
    notify(e instanceof Error ? e.message : 'Error al crear', 'err');
  } finally {
    busy.value = false;
  }
}

async function copyInvite(url: string) {
  try {
    await navigator.clipboard.writeText(url);
    notify('URL copiada');
  } catch {
    notify('No se pudo copiar', 'err');
  }
}

async function revokeInvite(token: string) {
  if (!confirm('¿Revocar esta invitación? No podrá reutilizarse.')) return;
  await run(() => admin.revokeInvite(token), 'Invitación revocada');
}

async function run(fn: () => Promise<void>, okMsg: string) {
  try {
    await fn();
    notify(okMsg);
  } catch (e) {
    notify(e instanceof Error ? e.message : 'Error', 'err');
  }
}

async function logout() {
  await admin.logout();
  await router.replace({ name: 'admin-login' });
}

function formatAgo(unix: number): string {
  const d = Date.now() / 1000 - unix;
  if (d < 60) return Math.floor(d) + 's';
  if (d < 3600) return Math.floor(d / 60) + 'm';
  return Math.floor(d / 3600) + 'h';
}

onMounted(() => {
  load();
  timer = window.setInterval(load, 5000);
});

onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer);
});
</script>

<template>
  <div class="admin-dash">
    <header class="topbar">
      <div>
        <h1>ADMIN · MODOS-CPU</h1>
        <p class="hint">Conectado como <strong>{{ admin.user }}</strong></p>
      </div>
      <div class="actions">
        <a class="link" href="/" title="Ir al simulador">↗ simulador</a>
        <button class="ghost" @click="logout">Cerrar sesión</button>
      </div>
    </header>

    <section class="stats" v-if="admin.stats">
      <div class="stat">
        <span class="k">Públicas</span>
        <span class="v">{{ admin.stats.full }} / {{ admin.stats.max_public }}</span>
      </div>
      <div class="stat">
        <span class="k">Privilegiadas</span>
        <span class="v">{{ admin.stats.privileged }} / {{ admin.stats.max_privileged }}</span>
      </div>
      <div class="stat">
        <span class="k">Creadas (total)</span>
        <span class="v">{{ admin.stats.total_created }}</span>
      </div>
      <div class="stat">
        <span class="k">Pico simultáneo</span>
        <span class="v">{{ admin.stats.peak }}</span>
      </div>
    </section>

    <section class="panel">
      <header class="panel-head">
        <h2>Sesiones activas</h2>
        <button class="ghost" @click="load">↻ refrescar</button>
      </header>

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
              <span v-if="s.has_share" :class="['pill', s.public ? 'share' : 'private']">
                {{ s.public ? '◆ PÚBLICA' : '◆ PRIVADA' }}
              </span>
              <span v-else class="pill no-share">— sin share</span>
            </td>
            <td class="dim">hace {{ formatAgo(s.created_at) }}</td>
            <td class="dim">hace {{ formatAgo(s.last_active) }}</td>
            <td class="row-actions">
              <button @click="setLabel(s.id, s.label)">alias</button>
              <button v-if="s.has_share" @click="togglePublic(s.id, !s.public)">
                {{ s.public ? 'hacer privada' : 'hacer pública' }}
              </button>
              <button class="danger" @click="killSession(s.id)">cerrar</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty">No hay sesiones activas en este momento</p>
    </section>

    <section class="panel">
      <header class="panel-head">
        <h2>Invitaciones privilegiadas</h2>
      </header>

      <form class="invite-form" @submit.prevent="createInvite">
        <input
          v-model="inviteLabel"
          placeholder="Alias opcional para el invitado"
          maxlength="32"
          :disabled="busy"
        />
        <button type="submit" :disabled="busy">+ generar invitación</button>
      </form>

      <table v-if="admin.invites.length" class="grid">
        <thead>
          <tr>
            <th>Token</th>
            <th>Alias</th>
            <th>Creada</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="inv in admin.invites" :key="inv.token">
            <td class="mono">{{ inv.token.slice(0, 10) }}…</td>
            <td>
              <span v-if="inv.label">{{ inv.label }}</span>
              <span v-else class="dim">(sin alias)</span>
            </td>
            <td class="dim">hace {{ formatAgo(inv.created_at) }}</td>
            <td class="row-actions">
              <button @click="copyInvite(inv.url)">copiar URL</button>
              <button class="danger" @click="revokeInvite(inv.token)">revocar</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty">No hay invitaciones pendientes</p>
    </section>

    <transition name="toast">
      <div v-if="toast" :class="['toast', toast.tone]">{{ toast.message }}</div>
    </transition>
  </div>
</template>

<style scoped>
.admin-dash {
  min-height: 100vh;
  padding: 18px 22px 28px;
  font-family: 'JetBrains Mono', monospace;
  color: var(--text, #e0e0e0);
  background:
    radial-gradient(ellipse at top, rgba(255, 179, 71, 0.04), transparent 60%),
    var(--bg, #0a0a0a);
}

.topbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid var(--border, #3a3a3a);
  padding-bottom: 14px;
  margin-bottom: 18px;
}

.topbar h1 {
  font-family: 'Major Mono Display', monospace;
  font-size: 15px;
  letter-spacing: 4px;
  color: var(--amber, #ffb347);
  margin: 0;
  text-shadow: 0 0 8px rgba(255, 179, 71, 0.3);
}

.hint {
  font-size: 11px;
  color: var(--text-dim, #888);
  margin: 4px 0 0;
}

.hint strong {
  color: var(--amber, #ffb347);
  letter-spacing: 1px;
}

.actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.link {
  font-size: 12px;
  color: var(--text-dim, #888);
  text-decoration: none;
  letter-spacing: 1px;
}

.link:hover {
  color: var(--amber, #ffb347);
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 12px;
  margin-bottom: 22px;
}

.stat {
  border: 1px solid var(--border, #3a3a3a);
  background: rgba(0, 0, 0, 0.4);
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat .k {
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim, #888);
}

.stat .v {
  font-size: 18px;
  color: var(--amber, #ffb347);
  letter-spacing: 1px;
}

.panel {
  border: 1px solid var(--border, #3a3a3a);
  background: rgba(0, 0, 0, 0.35);
  margin-bottom: 22px;
}

.panel-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border, #3a3a3a);
}

.panel-head h2 {
  font-size: 12px;
  letter-spacing: 2px;
  margin: 0;
  color: var(--text, #e0e0e0);
  text-transform: uppercase;
}

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
  color: var(--text-dim, #888);
  text-transform: uppercase;
  font-weight: 500;
}

.grid tbody tr:hover {
  background: rgba(255, 179, 71, 0.04);
}

.mono {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.dim {
  color: var(--text-dim, #888);
}

.mode-full {
  color: var(--green, #7fffaa);
  letter-spacing: 1px;
}

.mode-privileged {
  color: var(--yellow, #ffd54f);
  letter-spacing: 1px;
}

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid;
  letter-spacing: 1px;
}

.pill.share {
  color: var(--amber, #ffb347);
  border-color: var(--amber, #ffb347);
  background: rgba(255, 179, 71, 0.08);
}

.pill.private {
  color: var(--text-dim, #888);
  border-color: var(--border, #3a3a3a);
}

.pill.no-share {
  color: var(--text-dim, #888);
  border-color: transparent;
}

.row-actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
  flex-wrap: wrap;
}

button {
  font-family: 'JetBrains Mono', monospace;
  background: rgba(255, 179, 71, 0.08);
  color: var(--amber, #ffb347);
  border: 1px solid var(--amber, #ffb347);
  padding: 5px 9px;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
  transition: background 0.15s ease, box-shadow 0.15s ease;
}

button:hover:not(:disabled) {
  background: rgba(255, 179, 71, 0.16);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

button.ghost {
  background: transparent;
  border-color: var(--border, #3a3a3a);
  color: var(--text-dim, #888);
}

button.ghost:hover {
  border-color: var(--amber, #ffb347);
  color: var(--amber, #ffb347);
}

button.danger {
  background: rgba(255, 122, 122, 0.08);
  color: var(--red, #ff7a7a);
  border-color: var(--red, #ff7a7a);
}

button.danger:hover:not(:disabled) {
  background: rgba(255, 122, 122, 0.18);
}

.invite-form {
  display: flex;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.invite-form input {
  flex: 1;
  background: rgba(0, 0, 0, 0.5);
  border: 1px solid var(--border, #3a3a3a);
  color: var(--text, #e0e0e0);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 6px 10px;
  outline: none;
}

.invite-form input:focus {
  border-color: var(--amber, #ffb347);
  box-shadow: 0 0 6px rgba(255, 179, 71, 0.2);
}

.empty {
  padding: 20px;
  text-align: center;
  color: var(--text-dim, #888);
  font-size: 12px;
  letter-spacing: 1px;
}

.toast {
  position: fixed;
  bottom: 20px;
  right: 20px;
  padding: 10px 16px;
  border: 1px solid;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  letter-spacing: 1px;
  z-index: 1000;
}

.toast.ok {
  color: var(--green, #7fffaa);
  border-color: var(--green, #7fffaa);
  background: rgba(127, 255, 170, 0.1);
}

.toast.err {
  color: var(--red, #ff7a7a);
  border-color: var(--red, #ff7a7a);
  background: rgba(255, 122, 122, 0.12);
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
</style>
