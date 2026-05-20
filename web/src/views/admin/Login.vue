<script setup lang="ts">
import { ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAdminStore } from '../../stores/admin';
import Btn from '../../components/ui/Btn.vue';
import Card from '../../components/ui/Card.vue';
import Field from '../../components/ui/Field.vue';

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
    <Card class="admin-login__card" :elevation="2">
      <header class="admin-login__header">
        <h1>ADMIN · MODOS-CPU</h1>
        <p class="admin-login__hint">Acceso restringido al panel de administración.</p>
      </header>

      <form class="admin-login__form" @submit.prevent="submit">
        <Field label="Usuario" required v-slot="{ id }">
          <input
            :id="id"
            v-model="user"
            name="user"
            autocomplete="username"
            autofocus
            required
            :disabled="busy"
          />
        </Field>

        <Field label="Contraseña" required :error="admin.lastError ?? ''" v-slot="{ id }">
          <input
            :id="id"
            v-model="pass"
            name="pass"
            type="password"
            autocomplete="current-password"
            required
            :disabled="busy"
          />
        </Field>

        <Btn
          type="submit"
          :loading="busy"
          :disabled="!user || !pass"
          block
        >
          {{ busy ? 'Verificando…' : 'Ingresar' }}
        </Btn>
      </form>

      <footer class="admin-login__footer">
        <a href="/">← Volver al simulador</a>
      </footer>
    </Card>
  </div>
</template>

<style scoped>
.admin-login {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: var(--space-4);
  background:
    radial-gradient(ellipse at center, rgba(255, 179, 71, 0.06), transparent 60%),
    var(--surface-0);
}

.admin-login__card {
  width: min(100%, 380px);
}

.admin-login__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-bottom: var(--space-5);
}

.admin-login__header h1 {
  font-family: 'Major Mono Display', monospace;
  font-size: 16px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.admin-login__hint {
  color: var(--text-dim);
  font-size: 12px;
}

.admin-login__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.admin-login__footer {
  margin-top: var(--space-5);
  text-align: center;
  font-size: 11px;
}

.admin-login__footer a {
  color: var(--text-dim);
  letter-spacing: 1px;
}
</style>
