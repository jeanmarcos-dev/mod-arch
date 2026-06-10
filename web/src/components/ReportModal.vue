<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Bug, X, MessageSquare, Lightbulb, Send } from '@lucide/vue';
import VueTurnstile from 'vue-turnstile';
import Btn from './ui/Btn.vue';
import Field from './ui/Field.vue';
import { apiJSON } from '../lib/api';
import { useToastStore } from '../stores/toast';
import { useReportStore } from '../stores/report';
import { useAppConfigStore } from '../stores/appConfig';
import { useAuthStore } from '../stores/auth';

const { t } = useI18n();
const route = useRoute();
const toast = useToastStore();
const reports = useReportStore();
const appCfg = useAppConfigStore();
const auth = useAuthStore();

type Kind = 'feedback' | 'bug' | 'idea';

const kind = ref<Kind>('feedback');
const title = ref('');
const body = ref('');
const submitting = ref(false);
const turnstileToken = ref('');

// Turnstile only kicks in for anonymous submissions. Authenticated users
// already proved they were human at login + magic-link, and forcing a
// challenge on them would be friction without a security benefit.
const turnstileRequired = computed(
  () => Boolean(appCfg.turnstileSitekey) && !auth.user,
);

watch(
  () => reports.open,
  (isOpen) => {
    if (!isOpen) return;
    kind.value = 'feedback';
    title.value = '';
    body.value = '';
    submitting.value = false;
    turnstileToken.value = '';
  },
);

// Close on Escape while the modal is mounted. The listener is attached at
// component creation and removed in onBeforeUnmount so it cannot outlive the
// modal in a hot-reload scenario.
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && reports.open) {
    e.stopPropagation();
    reports.hide();
  }
}
window.addEventListener('keydown', onKey);
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

const canSubmit = computed(
  () =>
    !submitting.value &&
    title.value.trim().length > 0 &&
    body.value.trim().length > 0 &&
    title.value.length <= 120 &&
    body.value.length <= 4000 &&
    (!turnstileRequired.value || turnstileToken.value !== ''),
);

function buildContext(): Record<string, unknown> {
  return {
    route: route.fullPath,
    route_name: route.name ? String(route.name) : '',
    user_agent: navigator.userAgent,
    viewport: { w: window.innerWidth, h: window.innerHeight },
    locale: navigator.language || '',
    when: new Date().toISOString(),
  };
}

async function submit() {
  if (!canSubmit.value) return;
  submitting.value = true;
  try {
    await apiJSON('POST', '/api/reports', {
      kind: kind.value,
      title: title.value.trim(),
      body: body.value.trim(),
      context: buildContext(),
      turnstile_token: turnstileRequired.value ? turnstileToken.value : undefined,
    });
    toast.ok(t('report.thanks'));
    reports.hide();
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('report.errorSubmit'));
    // Tokens are single-use: if the verify failed or the network blew up,
    // the widget needs to mint a new one before we let the user retry.
    turnstileToken.value = '';
    submitting.value = false;
  }
}

function pick(k: Kind) {
  kind.value = k;
}
</script>

<template>
  <Teleport to="body">
    <div v-if="reports.open" class="report-overlay" @click.self="reports.hide()">
      <div class="report-modal" role="dialog" aria-modal="true" :aria-label="t('report.modalTitle')">
        <header class="report-modal__head">
          <span class="report-modal__title">
            <MessageSquare :size="14" :stroke-width="2" />
            <span>{{ t('report.modalTitle').toUpperCase() }}</span>
          </span>
          <button
            type="button"
            class="report-modal__close"
            @click="reports.hide()"
            :aria-label="t('report.close')"
            :title="t('report.close')"
          >
            <X :size="16" :stroke-width="2" />
          </button>
        </header>

        <form class="report-modal__body" @submit.prevent="submit">
          <div class="kind-picker" role="radiogroup" :aria-label="t('report.kindLabel')">
            <button
              type="button"
              role="radio"
              :aria-checked="kind === 'feedback'"
              :class="['kind', { 'kind--on': kind === 'feedback' }]"
              @click="pick('feedback')"
            >
              <MessageSquare :size="14" :stroke-width="2" />
              <span>{{ t('report.kindFeedback') }}</span>
            </button>
            <button
              type="button"
              role="radio"
              :aria-checked="kind === 'bug'"
              :class="['kind', { 'kind--on': kind === 'bug' }]"
              @click="pick('bug')"
            >
              <Bug :size="14" :stroke-width="2" />
              <span>{{ t('report.kindBug') }}</span>
            </button>
            <button
              type="button"
              role="radio"
              :aria-checked="kind === 'idea'"
              :class="['kind', { 'kind--on': kind === 'idea' }]"
              @click="pick('idea')"
            >
              <Lightbulb :size="14" :stroke-width="2" />
              <span>{{ t('report.kindIdea') }}</span>
            </button>
          </div>

          <Field :label="t('report.titleField')" required>
            <template #default="{ id }">
              <input
                :id="id"
                v-model="title"
                type="text"
                maxlength="120"
                :placeholder="t('report.titlePlaceholder')"
                required
              />
            </template>
          </Field>

          <Field :label="t('report.bodyField')" :help="t('report.bodyHelp')" required>
            <template #default="{ id }">
              <textarea
                :id="id"
                v-model="body"
                maxlength="4000"
                rows="6"
                :placeholder="t('report.bodyPlaceholder')"
                required
                class="report-body"
              />
            </template>
          </Field>

          <p class="report-modal__hint">{{ t('report.hintContext') }}</p>

          <div v-if="turnstileRequired" class="report-modal__turnstile">
            <VueTurnstile
              :site-key="appCfg.turnstileSitekey"
              v-model="turnstileToken"
              theme="dark"
            />
          </div>

          <footer class="report-modal__actions">
            <Btn variant="ghost" type="button" @click="reports.hide()">{{ t('report.cancel') }}</Btn>
            <Btn variant="primary" type="submit" :disabled="!canSubmit" :loading="submitting">
              <Send :size="13" :stroke-width="2" />
              <span>{{ t('report.submit') }}</span>
            </Btn>
          </footer>
        </form>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.report-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  backdrop-filter: blur(2px);
}

.report-modal {
  background: var(--bg-1);
  border: 1px solid var(--border-strong);
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.8), 0 0 24px rgba(var(--accent-rgb), 0.15);
  width: 100%;
  max-width: 520px;
}

.report-modal__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
  background: rgba(var(--accent-rgb), 0.05);
}
.report-modal__title {
  font-family: var(--font-display);
  font-size: 13px;
  letter-spacing: 3px;
  color: var(--amber);
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.report-modal__close {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  width: 28px;
  height: 28px;
  padding: 0;
  min-height: 0;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.report-modal__close svg { display: block; }
.report-modal__close:hover {
  color: var(--red-soft);
  border-color: var(--red-soft);
}

.report-modal__body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.kind-picker {
  display: flex;
  gap: 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.kind {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 10px;
  background: transparent;
  border: none;
  border-right: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
}
.kind:last-child { border-right: none; }
.kind:hover {
  background: rgba(var(--accent-rgb), 0.06);
  color: var(--text);
}
.kind--on {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  box-shadow: inset 0 0 8px rgba(var(--accent-rgb), 0.25);
}

.report-body {
  font-family: inherit;
  font-size: 13px;
  padding: 8px 10px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  resize: vertical;
  width: 100%;
  box-sizing: border-box;
}
.report-body:focus {
  outline: none;
  border-color: var(--amber);
}

.report-modal__hint {
  font-size: 11px;
  color: var(--text-dim);
  margin: 0;
}

.report-modal__turnstile {
  display: flex;
  justify-content: center;
}

.report-modal__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
