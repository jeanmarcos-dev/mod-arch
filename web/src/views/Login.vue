<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import VueTurnstile from 'vue-turnstile';
import { useAuthStore } from '../stores/auth';
import { useAppConfigStore } from '../stores/appConfig';
import Btn from '../components/ui/Btn.vue';
import Card from '../components/ui/Card.vue';
import Field from '../components/ui/Field.vue';

const { t } = useI18n();
const auth = useAuthStore();
const appCfg = useAppConfigStore();
const router = useRouter();

const name = ref('');
const email = ref('');
const studentId = ref('');
const busy = ref(false);
const turnstileToken = ref('');

onMounted(() => { appCfg.load(); });

const turnstileEnabled = computed(() => Boolean(appCfg.turnstileSitekey));
const canSubmit = computed(() => name.value.trim() !== '' && (!turnstileEnabled.value || turnstileToken.value !== ''));

// The post-submit "check your inbox" screen is shown only when SMTP is on
// and the server returned a pending state. With SMTP off the login is
// completed in-band and we redirect straight to the simulator.
const showSent = computed(() => auth.pendingEmail !== null);

async function submit() {
  if (busy.value) return;
  busy.value = true;
  try {
    const outcome = await auth.login({
      name: name.value.trim(),
      email: email.value.trim() || undefined,
      student_id: studentId.value.trim() || undefined,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined,
    });
    if (outcome === 'direct') {
      const next = (router.currentRoute.value.query.next as string) || '/';
      await router.replace(next);
    } else if (outcome === 'error') {
      // A bad token taints the widget; force a refresh on next attempt.
      turnstileToken.value = '';
    }
    // outcome === 'pending' falls through: the template flips to showSent
    // and the user waits for the magic link to land in their inbox.
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
        <h1>{{ t('topbar.brand') }}</h1>
        <p class="login-page__hint">
          {{ t('login.hint') }}
        </p>
      </header>

      <form v-if="!showSent" class="login-page__form" @submit.prevent="submit">
        <Field :label="t('login.fields.name')" required v-slot="{ id }">
          <input
            :id="id"
            v-model="name"
            name="name"
            autocomplete="name"
            autofocus
            required
            maxlength="32"
            :disabled="busy"
            :placeholder="t('login.fields.namePlaceholder')"
          />
        </Field>

        <Field
          :label="t('login.fields.email')"
          :help="t('login.fields.emailHelp')"
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
            :placeholder="t('login.fields.emailPlaceholder')"
          />
        </Field>

        <Field :label="t('login.fields.studentId')" :error="auth.lastError ?? ''" v-slot="{ id }">
          <input
            :id="id"
            v-model="studentId"
            name="student_id"
            autocomplete="off"
            maxlength="32"
            :disabled="busy"
            :placeholder="t('login.fields.studentIdPlaceholder')"
          />
        </Field>

        <div v-if="turnstileEnabled" class="login-page__turnstile">
          <VueTurnstile
            :site-key="appCfg.turnstileSitekey"
            v-model="turnstileToken"
            theme="dark"
          />
        </div>

        <Btn
          type="submit"
          :loading="busy"
          :disabled="!canSubmit"
          block
        >
          {{ busy ? t('login.submitBusy') : t('login.submit') }}
        </Btn>
      </form>

      <div v-else-if="showSent" class="login-page__magic">
        <i18n-t keypath="login.sent" tag="p" class="login-page__ok">
          <template #email><strong>{{ auth.pendingEmail }}</strong></template>
        </i18n-t>
        <p class="login-page__hint">
          {{ t('login.sentHint') }}
        </p>
        <p class="login-page__hint">
          {{ t('login.sentWait') }}
        </p>
        <Btn block variant="ghost" @click="continueAnonymous">{{ t('login.continueAsGuest') }}</Btn>
      </div>

      <footer class="login-page__footer">
        <a href="#" @click.prevent="continueAnonymous">{{ t('login.continueAnon') }}</a>
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
  font-family: var(--font-display);
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
