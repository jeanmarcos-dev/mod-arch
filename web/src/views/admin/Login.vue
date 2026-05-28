<script setup lang="ts">
import { ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAdminStore } from '../../stores/admin';
import Btn from '../../components/ui/Btn.vue';
import Card from '../../components/ui/Card.vue';
import Field from '../../components/ui/Field.vue';

const { t } = useI18n();
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
        <h1>{{ t('admin.login.heading') }}</h1>
        <p class="admin-login__hint">{{ t('admin.login.hint') }}</p>
      </header>

      <form class="admin-login__form" @submit.prevent="submit">
        <Field :label="t('admin.login.userLabel')" required v-slot="{ id }">
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

        <Field :label="t('admin.login.passLabel')" required :error="admin.lastError ?? ''" v-slot="{ id }">
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
          {{ busy ? t('admin.login.submitBusy') : t('admin.login.submit') }}
        </Btn>
      </form>

      <footer class="admin-login__footer">
        <a href="/">{{ t('admin.login.backToSim') }}</a>
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
    radial-gradient(ellipse at center, rgba(var(--accent-rgb), 0.06), transparent 60%),
    var(--surface-0);
}

.admin-login__card {
  width: min(100%, 380px);
}

@media (max-width: 480px) {
  .admin-login { padding: var(--space-3); }
  .admin-login__header { margin-bottom: var(--space-3); }
}

.admin-login__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-bottom: var(--space-5);
}

.admin-login__header h1 {
  font-family: var(--font-display);
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
