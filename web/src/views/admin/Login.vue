<script setup lang="ts">
import { ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAdminStore } from '../../stores/admin';

const admin = useAdminStore();
const router = useRouter();
const route = useRoute();

const user = ref('');
const pass = ref('');
const busy = ref(false);

async function submit() {
  if (busy.value) return;
  busy.value = true;
  try {
    const ok = await admin.login({ user: user.value, pass: pass.value });
    if (ok) {
      const next = typeof route.query.next === 'string' ? route.query.next : '/admin';
      await router.replace(next);
    }
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="admin-login">
    <div class="card">
      <header>
        <h1>ADMIN · MODOS-CPU</h1>
        <p class="hint">Acceso restringido al panel de administración</p>
      </header>

      <form @submit.prevent="submit">
        <label>
          <span>Usuario</span>
          <input
            v-model="user"
            autocomplete="username"
            autofocus
            required
            :disabled="busy"
          />
        </label>
        <label>
          <span>Contraseña</span>
          <input
            v-model="pass"
            type="password"
            autocomplete="current-password"
            required
            :disabled="busy"
          />
        </label>

        <p v-if="admin.lastError" class="error">{{ admin.lastError }}</p>

        <button type="submit" :disabled="busy || !user || !pass">
          {{ busy ? 'Verificando…' : 'Ingresar' }}
        </button>
      </form>

      <footer>
        <a href="/">← Volver al simulador</a>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.admin-login {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 16px;
  background:
    radial-gradient(ellipse at center, rgba(255, 179, 71, 0.06), transparent 60%),
    var(--bg, #0a0a0a);
}

.card {
  width: min(100%, 380px);
  background: rgba(0, 0, 0, 0.55);
  border: 1px solid var(--border, #3a3a3a);
  box-shadow: 0 0 32px rgba(255, 179, 71, 0.05);
  padding: 28px 26px;
  font-family: 'JetBrains Mono', monospace;
}

header h1 {
  font-family: 'Major Mono Display', monospace;
  font-size: 16px;
  letter-spacing: 4px;
  color: var(--amber, #ffb347);
  margin: 0 0 6px;
  text-shadow: 0 0 8px rgba(255, 179, 71, 0.3);
}

.hint {
  color: var(--text-dim, #888);
  font-size: 12px;
  margin: 0 0 22px;
}

form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 11px;
  letter-spacing: 1px;
  color: var(--text-dim, #888);
  text-transform: uppercase;
}

input {
  font-family: 'JetBrains Mono', monospace;
  background: rgba(0, 0, 0, 0.55);
  border: 1px solid var(--border, #3a3a3a);
  color: var(--text, #e0e0e0);
  padding: 9px 10px;
  font-size: 14px;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

input:focus {
  border-color: var(--amber, #ffb347);
  box-shadow: 0 0 6px rgba(255, 179, 71, 0.25);
}

input:disabled {
  opacity: 0.6;
}

button {
  margin-top: 6px;
  font-family: 'JetBrains Mono', monospace;
  background: rgba(255, 179, 71, 0.12);
  border: 1px solid var(--amber, #ffb347);
  color: var(--amber, #ffb347);
  padding: 10px 12px;
  font-size: 13px;
  letter-spacing: 2px;
  text-transform: uppercase;
  cursor: pointer;
  transition: background 0.15s ease, box-shadow 0.15s ease;
}

button:hover:not(:disabled) {
  background: rgba(255, 179, 71, 0.2);
  box-shadow: 0 0 10px rgba(255, 179, 71, 0.3);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  margin: 0;
  color: var(--red, #ff7a7a);
  font-size: 12px;
  border-left: 2px solid var(--red, #ff7a7a);
  padding-left: 8px;
}

footer {
  margin-top: 22px;
  text-align: center;
  font-size: 11px;
}

footer a {
  color: var(--text-dim, #888);
  text-decoration: none;
  letter-spacing: 1px;
}

footer a:hover {
  color: var(--amber, #ffb347);
}
</style>
