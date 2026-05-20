<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import Btn from '../components/ui/Btn.vue';
import Card from '../components/ui/Card.vue';
import Field from '../components/ui/Field.vue';

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
    <Card class="login-page__card" :elevation="2">
      <header class="login-page__header">
        <h1>MODOS-CPU</h1>
        <p class="login-page__hint">
          Identifíquese antes de iniciar la sesión. Si proporciona su correo
          recibirá un enlace de acceso rápido para volver a entrar desde otro
          dispositivo.
        </p>
      </header>

      <form v-if="!auth.magicSent" class="login-page__form" @submit.prevent="submit">
        <Field label="Nombre" required v-slot="{ id }">
          <input
            :id="id"
            v-model="name"
            name="name"
            autocomplete="name"
            autofocus
            required
            maxlength="32"
            :disabled="busy"
            placeholder="Cómo desea aparecer"
          />
        </Field>

        <Field
          label="Correo (opcional)"
          help="Le enviaremos un enlace mágico para volver a entrar."
          v-slot="{ id }"
        >
          <input
            :id="id"
            v-model="email"
            name="email"
            type="email"
            autocomplete="email"
            maxlength="120"
            :disabled="busy"
            placeholder="usted@correo.com"
          />
        </Field>

        <Field label="ID de estudiante (opcional)" :error="auth.lastError ?? ''" v-slot="{ id }">
          <input
            :id="id"
            v-model="studentId"
            name="student_id"
            autocomplete="off"
            maxlength="32"
            :disabled="busy"
            placeholder="UCV-12345"
          />
        </Field>

        <Btn
          type="submit"
          :loading="busy"
          :disabled="!name.trim()"
          block
        >
          {{ busy ? 'Verificando…' : 'Ingresar' }}
        </Btn>
      </form>

      <div v-else class="login-page__magic">
        <p class="login-page__ok">
          Le hemos enviado un enlace de acceso a <strong>{{ email.trim() }}</strong>.
        </p>
        <p class="login-page__hint">
          El enlace caduca en 15 minutos. Mientras tanto, su sesión actual ya
          está activa con el nombre <strong>{{ auth.user?.name }}</strong>.
        </p>
        <Btn block @click="continueAnonymous">Continuar al simulador →</Btn>
      </div>

      <footer class="login-page__footer">
        <a href="#" @click.prevent="continueAnonymous">Continuar sin identificarse →</a>
      </footer>
    </Card>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: var(--space-4);
  background:
    radial-gradient(ellipse at center, rgba(255, 179, 71, 0.06), transparent 60%),
    var(--surface-0);
}

.login-page__card {
  width: min(100%, 440px);
}

.login-page__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-bottom: var(--space-5);
}

.login-page__header h1 {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 6px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.login-page__hint {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
}

.login-page__form,
.login-page__magic {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.login-page__ok {
  color: var(--green);
  font-size: 13px;
}

.login-page__footer {
  margin-top: var(--space-5);
  text-align: center;
  font-size: 11px;
}

.login-page__footer a {
  color: var(--text-dim);
  letter-spacing: 1px;
}
</style>
