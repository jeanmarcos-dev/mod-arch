<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const router = useRouter();

const name = ref('');
const email = ref('');
const studentId = ref('');
const busy = ref(false);

async function submit() {
  if (busy.value) return;
  busy.value = true;
  try {
    const ok = await auth.login({
      name: name.value.trim(),
      email: email.value.trim() || undefined,
      student_id: studentId.value.trim() || undefined,
    });
    if (ok && !auth.magicSent) {
      await router.replace('/');
    }
  } finally {
    busy.value = false;
  }
}

function continueAnonymous() {
  router.replace('/');
}
</script>

<template>
  <div class="login-page">
    <div class="card">
      <header>
        <h1>MODOS-CPU</h1>
        <p class="hint">Identifíquese antes de iniciar la sesión. Si proporciona su correo recibirá un enlace de acceso rápido para volver a entrar desde otro dispositivo.</p>
      </header>

      <form @submit.prevent="submit" v-if="!auth.magicSent">
        <label>
          <span>Nombre</span>
          <input
            v-model="name"
            autocomplete="name"
            autofocus
            required
            maxlength="32"
            :disabled="busy"
            placeholder="Cómo desea aparecer"
          />
        </label>

        <label>
          <span>Correo (opcional)</span>
          <input
            v-model="email"
            type="email"
            autocomplete="email"
            maxlength="120"
            :disabled="busy"
            placeholder="usted@correo.com"
          />
        </label>

        <label>
          <span>ID de estudiante (opcional)</span>
          <input
            v-model="studentId"
            autocomplete="off"
            maxlength="32"
            :disabled="busy"
            placeholder="UCV-12345"
          />
        </label>

        <p v-if="auth.lastError" class="error">{{ auth.lastError }}</p>

        <button type="submit" :disabled="busy || !name.trim()">
          {{ busy ? 'Verificando…' : 'Ingresar' }}
        </button>
      </form>

      <div v-else class="magic-info">
        <p class="ok">Le hemos enviado un enlace de acceso a <strong>{{ email.trim() }}</strong>.</p>
        <p class="hint">El enlace caduca en 15 minutos. Mientras tanto, su sesión actual ya está activa con el nombre <strong>{{ auth.user?.name }}</strong>.</p>
        <button type="button" @click="continueAnonymous">Continuar al simulador →</button>
      </div>

      <footer>
        <a href="#" @click.prevent="continueAnonymous">Continuar sin identificarse →</a>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 16px;
  background:
    radial-gradient(ellipse at center, rgba(255, 179, 71, 0.06), transparent 60%),
    var(--bg, #0a0a0a);
}

.card {
  width: min(100%, 440px);
  background: rgba(0, 0, 0, 0.55);
  border: 1px solid var(--border, #3a3a3a);
  box-shadow: 0 0 32px rgba(255, 179, 71, 0.05);
  padding: 28px 26px;
  font-family: 'JetBrains Mono', monospace;
}

header h1 {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 6px;
  color: var(--amber, #ffb347);
  margin: 0 0 6px;
  text-shadow: 0 0 10px rgba(255, 179, 71, 0.3);
}

.hint {
  color: var(--text-dim, #888);
  font-size: 12px;
  line-height: 1.5;
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

.magic-info {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.magic-info .ok {
  color: var(--green, #7fffaa);
  font-size: 13px;
  margin: 0;
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
