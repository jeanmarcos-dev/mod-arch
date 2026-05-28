<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Send, Mail, ShieldCheck, Eraser } from '@lucide/vue';
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

type TLSMode = 'starttls' | 'tls' | 'none';

interface SmtpForm {
  host: string;
  port: number;
  user: string;
  password: string;
  from_name: string;
  from_email: string;
  reply_to: string;
  subject_prefix: string;
  bcc: string;
  tls_mode: TLSMode;
  enabled: boolean;
}

const form = ref<SmtpForm>({
  host: '',
  port: 587,
  user: '',
  password: '',
  from_name: '',
  from_email: '',
  reply_to: '',
  subject_prefix: '',
  bcc: '',
  tls_mode: 'starttls',
  enabled: false,
});

// passwordSet mirrors the server's password_set flag so the input can show
// "(sin cambios)" as a placeholder instead of an empty field when there's
// already a stored secret. Editing the field replaces the password; leaving
// it untouched preserves the stored value.
const passwordSet = ref(false);
const passwordDirty = ref(false);
const loading = ref(false);
const saving = ref(false);
const testing = ref(false);

const testTo = ref('');
const testResult = ref<{ ok: boolean; message: string } | null>(null);

async function load() {
  loading.value = true;
  try {
    const v = await admin.fetchSmtpSettings();
    form.value = {
      host: v.host ?? '',
      port: v.port ?? 587,
      user: v.user ?? '',
      password: '',
      from_name: v.from_name ?? '',
      from_email: v.from_email ?? '',
      reply_to: v.reply_to ?? '',
      subject_prefix: v.subject_prefix ?? '',
      bcc: v.bcc ?? '',
      tls_mode: (v.tls_mode as TLSMode) ?? 'starttls',
      enabled: Boolean(v.enabled),
    };
    passwordSet.value = Boolean(v.password_set);
    passwordDirty.value = false;
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.smtp.errorLoad'));
  } finally {
    loading.value = false;
  }
}

onMounted(load);

const passwordPlaceholder = computed(() =>
  passwordSet.value ? t('admin.smtp.passwordNoChange') : t('admin.smtp.passwordPlaceholder'),
);

async function save() {
  saving.value = true;
  testResult.value = null;
  try {
    const payload: Record<string, unknown> = {
      host: form.value.host,
      port: Number(form.value.port) || 0,
      user: form.value.user,
      from_name: form.value.from_name,
      from_email: form.value.from_email,
      reply_to: form.value.reply_to,
      subject_prefix: form.value.subject_prefix,
      bcc: form.value.bcc,
      tls_mode: form.value.tls_mode,
      enabled: form.value.enabled,
    };
    // Only ship the password when the user actually touched the field. An
    // omitted password means "keep what is stored"; an empty one means
    // "clear it".
    if (passwordDirty.value) payload.password = form.value.password;

    const v = await admin.saveSmtpSettings(payload);
    passwordSet.value = Boolean(v.password_set);
    passwordDirty.value = false;
    form.value.password = '';
    toast.ok(t('admin.smtp.okSaved'));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.smtp.errorSave'));
  } finally {
    saving.value = false;
  }
}

async function clearPassword() {
  if (!confirm(t('admin.smtp.confirmClearPw'))) return;
  saving.value = true;
  try {
    await admin.saveSmtpSettings({ ...form.value, password: '' });
    passwordSet.value = false;
    passwordDirty.value = false;
    form.value.password = '';
    toast.ok(t('admin.smtp.okClearPw'));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  } finally {
    saving.value = false;
  }
}

async function sendTest() {
  if (!testTo.value.trim()) {
    testResult.value = { ok: false, message: t('admin.smtp.testNeedRecipient') };
    return;
  }
  testing.value = true;
  testResult.value = null;
  try {
    await admin.testSmtp(testTo.value.trim());
    testResult.value = { ok: true, message: t('admin.smtp.testOk') };
  } catch (e) {
    testResult.value = {
      ok: false,
      message: e instanceof Error ? e.message : t('admin.smtp.testError'),
    };
  } finally {
    testing.value = false;
  }
}
</script>

<template>
  <Card :title="t('admin.smtp.title')">
    <p class="lead">{{ t('admin.smtp.lead') }}</p>

    <div class="form-grid">
      <Field :label="t('admin.smtp.host')" required>
        <template #default="{ id }">
          <input :id="id" v-model="form.host" type="text" placeholder="smtp.gmail.com" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.port')" required>
        <template #default="{ id }">
          <input :id="id" v-model.number="form.port" type="number" min="1" max="65535" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.tlsMode')" required>
        <template #default="{ id }">
          <select :id="id" v-model="form.tls_mode">
            <option value="starttls">{{ t('admin.smtp.tlsStarttls') }}</option>
            <option value="tls">{{ t('admin.smtp.tlsImplicit') }}</option>
            <option value="none">{{ t('admin.smtp.tlsNone') }}</option>
          </select>
        </template>
      </Field>

      <Field :label="t('admin.smtp.user')">
        <template #default="{ id }">
          <input :id="id" v-model="form.user" type="text" autocomplete="off" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.password')" class="span-2">
        <template #default="{ id }">
          <div class="pass-row">
            <input
              :id="id"
              v-model="form.password"
              type="password"
              autocomplete="new-password"
              :placeholder="passwordPlaceholder"
              @input="passwordDirty = true"
            />
            <Btn
              v-if="passwordSet"
              variant="ghost"
              size="sm"
              type="button"
              :title="t('admin.smtp.passwordClearHint')"
              :disabled="saving"
              @click="clearPassword"
            >
              <Eraser :size="12" :stroke-width="2" />
              <span>{{ t('admin.smtp.passwordClear') }}</span>
            </Btn>
          </div>
        </template>
      </Field>

      <Field :label="t('admin.smtp.fromName')">
        <template #default="{ id }">
          <input :id="id" v-model="form.from_name" type="text" maxlength="100" placeholder="MODARCH" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.fromEmail')" required>
        <template #default="{ id }">
          <input :id="id" v-model="form.from_email" type="email" placeholder="noreply@ejemplo.com" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.replyTo')">
        <template #default="{ id }">
          <input :id="id" v-model="form.reply_to" type="email" :placeholder="t('admin.smtp.optional')" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.subjectPrefix')">
        <template #default="{ id }">
          <input :id="id" v-model="form.subject_prefix" type="text" maxlength="64" placeholder="[MODARCH]" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.bcc')" class="span-2">
        <template #default="{ id }">
          <input :id="id" v-model="form.bcc" type="email" :placeholder="t('admin.smtp.bccPlaceholder')" />
        </template>
      </Field>

      <Field :label="t('admin.smtp.enabled')" class="span-2">
        <template #default>
          <label class="checkbox">
            <input v-model="form.enabled" type="checkbox" />
            <span>{{ t('admin.smtp.enabledHint') }}</span>
          </label>
        </template>
      </Field>
    </div>

    <template #footer>
      <div class="actions">
        <Btn variant="primary" :loading="saving" :disabled="loading" @click="save">
          <ShieldCheck :size="14" :stroke-width="2" />
          <span>{{ t('admin.smtp.save') }}</span>
        </Btn>
      </div>
    </template>
  </Card>

  <Card :title="t('admin.smtp.testTitle')">
    <p class="lead">{{ t('admin.smtp.testLead') }}</p>
    <div class="test-row">
      <Field :label="t('admin.smtp.testTo')" class="grow">
        <template #default="{ id }">
          <input :id="id" v-model="testTo" type="email" placeholder="usted@ejemplo.com" />
        </template>
      </Field>
      <Btn variant="primary" :loading="testing" :disabled="!form.enabled" @click="sendTest">
        <Send :size="14" :stroke-width="2" />
        <span>{{ t('admin.smtp.testButton') }}</span>
      </Btn>
    </div>
    <p v-if="testResult" :class="['result', testResult.ok ? 'result--ok' : 'result--err']" role="status">
      <Mail :size="14" :stroke-width="2" />
      <span>{{ testResult.message }}</span>
    </p>
    <p v-else-if="!form.enabled" class="note">{{ t('admin.smtp.testNote') }}</p>
  </Card>
</template>

<style scoped>
.lead {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.5;
  margin: 0 0 var(--space-3);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
}

.span-2 { grid-column: 1 / -1; }
.grow { flex: 1; }

.pass-row {
  display: flex;
  gap: var(--space-2);
}
.pass-row > input { flex: 1; }

.checkbox {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text);
  font-size: 13px;
  cursor: pointer;
}
.checkbox input { accent-color: var(--amber); }

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}

.test-row {
  display: flex;
  align-items: flex-end;
  gap: var(--space-3);
}

.result {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-3);
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  font-size: 12px;
}
.result--ok  { color: var(--green); border: 1px solid var(--green); background: rgba(127, 255, 170, 0.08); }
.result--err { color: var(--red-soft); border: 1px solid var(--red-soft); background: rgba(255, 122, 122, 0.08); }

.note {
  margin-top: var(--space-3);
  font-size: 12px;
  color: var(--text-muted);
  font-style: italic;
}

@media (max-width: 768px) {
  .form-grid { grid-template-columns: 1fr; }
  .span-2 { grid-column: auto; }
}
</style>
