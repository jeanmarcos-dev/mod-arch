<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const newViewerPassword = ref('');

async function run(fn: () => Promise<void>, okMsg: string) {
  try { await fn(); toast.ok(okMsg); }
  catch (e) { toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric')); }
}

async function setAccessMode(mode: 'open' | 'invite_only' | 'closed') {
  await run(() => admin.setSetting('access_mode', mode), t('admin.access.okModeUpdated'));
}

async function toggleViewerPasswordRequired(required: boolean) {
  await run(
    () => admin.setSetting('viewer_password_required', String(required)),
    required ? t('admin.access.okPwRequired') : t('admin.access.okPwNotRequired'),
  );
}

async function setViewerPassword() {
  const v = newViewerPassword.value.trim();
  if (!v) return;
  await run(() => admin.setSetting('viewer_password', v), t('admin.access.okPwUpdated'));
  newViewerPassword.value = '';
}

async function clearViewerPassword() {
  if (!confirm(t('admin.access.confirmClear'))) return;
  await run(() => admin.setSetting('viewer_password', ''), t('admin.access.okPwCleared'));
}
</script>

<template>
  <Card v-if="admin.settings" :title="t('admin.access.title')">
    <div class="access-grid">
      <section class="access-block">
        <h3 class="access-block__label">{{ t('admin.access.modeLabel') }}</h3>
        <div class="seg" role="radiogroup" :aria-label="t('admin.access.modeAriaLabel')">
          <button
            role="radio"
            :aria-checked="admin.settings.access_mode === 'open'"
            :class="{ 'seg__btn': true, 'seg__btn--on': admin.settings.access_mode === 'open' }"
            @click="setAccessMode('open')"
          >{{ t('admin.access.modeOpen') }}</button>
          <button
            role="radio"
            :aria-checked="admin.settings.access_mode === 'invite_only'"
            :class="{ 'seg__btn': true, 'seg__btn--on': admin.settings.access_mode === 'invite_only' }"
            @click="setAccessMode('invite_only')"
          >{{ t('admin.access.modeInvite') }}</button>
          <button
            role="radio"
            :aria-checked="admin.settings.access_mode === 'closed'"
            :class="{ 'seg__btn': true, 'seg__btn--on': admin.settings.access_mode === 'closed' }"
            @click="setAccessMode('closed')"
          >{{ t('admin.access.modeClosed') }}</button>
        </div>
        <p class="access-block__hint">
          <template v-if="admin.settings.access_mode === 'open'">{{ t('admin.access.hintOpen') }}</template>
          <template v-else-if="admin.settings.access_mode === 'invite_only'">{{ t('admin.access.hintInvite') }}</template>
          <template v-else>{{ t('admin.access.hintClosed') }}</template>
        </p>
      </section>

      <section class="access-block">
        <h3 class="access-block__label">{{ t('admin.access.viewerPwTitle') }}</h3>
        <label class="access-check">
          <input
            type="checkbox"
            :checked="admin.settings.viewer_password_required"
            :disabled="!admin.settings.viewer_password_set"
            @change="toggleViewerPasswordRequired(($event.target as HTMLInputElement).checked)"
          />
          <span>{{ t('admin.access.viewerPwCheck') }} <code>/v/&lt;token&gt;</code></span>
        </label>
        <form class="access-pw" @submit.prevent="setViewerPassword">
          <Field :label="t('admin.access.viewerPwNewLabel')" v-slot="{ id }">
            <input
              :id="id"
              v-model="newViewerPassword"
              name="viewer_password"
              type="password"
              :placeholder="t('admin.access.viewerPwPlaceholder')"
              minlength="4"
              maxlength="64"
              autocomplete="new-password"
            />
          </Field>
          <div class="access-pw__actions">
            <Btn type="submit" :disabled="newViewerPassword.trim().length < 4">
              {{ admin.settings.viewer_password_set ? t('admin.access.viewerPwChange') : t('admin.access.viewerPwSet') }}
            </Btn>
            <Btn
              v-if="admin.settings.viewer_password_set"
              variant="danger"
              @click="clearViewerPassword"
            >{{ t('admin.access.viewerPwClear') }}</Btn>
          </div>
        </form>
        <p class="access-block__hint">
          {{ t('admin.access.viewerPwHint') }}
        </p>
      </section>
    </div>
  </Card>
</template>

<style scoped>
.access-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-5);
}

@media (max-width: 800px) {
  .access-grid { grid-template-columns: 1fr; }
}

.access-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.access-block__label {
  font-size: 11px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
  font-weight: normal;
}

.access-block__hint {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.access-check {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: 13px;
  color: var(--text);
}

.access-check input {
  width: 16px;
  height: 16px;
  accent-color: var(--amber);
}

.access-check code {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--amber);
  padding: 1px 6px;
  font-size: 11px;
  border-radius: var(--radius-sm);
}

.seg {
  display: flex;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.seg__btn {
  flex: 1;
  background: transparent;
  border: none;
  border-right: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 8px 10px;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.seg__btn:last-child { border-right: none; }

.seg__btn:hover {
  background: rgba(var(--accent-rgb), 0.06);
  color: var(--amber);
}

.seg__btn--on {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  box-shadow: inset 0 0 8px rgba(var(--accent-rgb), 0.25);
}

.access-pw {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.access-pw__actions {
  display: flex;
  gap: var(--space-2);
}
</style>
