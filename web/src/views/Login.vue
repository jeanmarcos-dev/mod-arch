<script setup lang="ts">
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import { Copy, Check } from '@lucide/vue';
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
const copied = ref(false);

// Two terminal states after submit:
//  - hasFallbackLink: SMTP off/failed — show the URL inline so the student
//    can open it directly. No automatic redirect.
//  - magicSent:       SMTP delivered the email — show "check your inbox".
const hasFallbackLink = computed(() => Boolean(auth.magicLink));
const showSent = computed(() => auth.magicSent && !hasFallbackLink.value);

async function submit() {
  if (busy.value) return;
  busy.value = true;
  try {
    const ok = await auth.login({
      name: name.value.trim(),
      email: email.value.trim() || undefined,
      student_id: studentId.value.trim() || undefined,
    });
    if (ok && !auth.magicSent && !hasFallbackLink.value) {
      await router.replace('/');
    }
  } finally {
    busy.value = false;
  }
}

function continueAnonymous() {
  router.replace('/');
}

async function copyLink() {
  if (!auth.magicLink) return;
  try {
    await navigator.clipboard.writeText(auth.magicLink);
    copied.value = true;
    setTimeout(() => { copied.value = false; }, 1500);
  } catch {
    // Older browsers fall through to the textarea select trick.
    const t = document.createElement('textarea');
    t.value = auth.magicLink;
    document.body.appendChild(t);
    t.select();
    try { document.execCommand('copy'); copied.value = true; } catch { /* ignore */ }
    document.body.removeChild(t);
    setTimeout(() => { copied.value = false; }, 1500);
  }
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

      <form v-if="!auth.magicSent && !hasFallbackLink" class="login-page__form" @submit.prevent="submit">
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

      <div v-else-if="showSent" class="login-page__magic">
        <p class="login-page__ok">
          Le hemos enviado un enlace de acceso a <strong>{{ email.trim() }}</strong>.
        </p>
        <p class="login-page__hint">
          El enlace caduca en 15 minutos. Mientras tanto, su sesión actual ya
          está activa con el nombre <strong>{{ auth.user?.name }}</strong>.
        </p>
        <Btn block @click="continueAnonymous">Continuar al simulador →</Btn>
      </div>

      <div v-else class="login-page__magic">
        <p class="login-page__warn">
          El envío por correo no está disponible en este momento. Use el
          siguiente enlace para entrar (vence en 15 minutos):
        </p>
        <div class="login-page__link-row">
          <input class="login-page__link-input" readonly :value="auth.magicLink ?? ''" @click="($event.target as HTMLInputElement).select()" />
          <button class="login-page__copy" type="button" :title="copied ? 'Copiado' : 'Copiar enlace'" @click="copyLink">
            <component :is="copied ? Check : Copy" :size="14" :stroke-width="2" />
            <span>{{ copied ? 'COPIADO' : 'COPIAR' }}</span>
          </button>
        </div>
        <p class="login-page__hint">
          Su sesión actual ya está activa con el nombre <strong>{{ auth.user?.name }}</strong>.
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
    radial-gradient(ellipse at center, rgba(var(--accent-rgb), 0.06), transparent 60%),
    var(--surface-0);
}

.login-page__card {
  width: min(100%, 440px);
}

@media (max-width: 480px) {
  .login-page { padding: var(--space-3); }
  .login-page__header { margin-bottom: var(--space-3); }
  .login-page__header h1 { font-size: 15px; letter-spacing: 4px; }
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

.login-page__warn {
  color: var(--amber);
  font-size: 13px;
  line-height: 1.5;
}

.login-page__link-row {
  display: flex;
  gap: 6px;
}

.login-page__link-input {
  flex: 1;
  min-width: 0;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border);
  color: var(--cyan);
  padding: 8px 10px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  border-radius: var(--radius-sm);
}
.login-page__link-input:focus {
  outline: none;
  border-color: var(--cyan);
}

.login-page__copy {
  background: rgba(106, 212, 255, 0.10);
  border: 1px solid var(--cyan);
  color: var(--cyan);
  padding: 8px 12px;
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.login-page__copy:hover { background: rgba(106, 212, 255, 0.22); }

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
