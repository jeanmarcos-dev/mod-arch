<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  useAdminStore,
  type AdminDefaults,
  type ScheduleAction,
  type ScheduleKind,
  type ScheduleRule,
  type ScheduleRuleInput,
} from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import { apiFetch } from '../../lib/api';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

// Target catalogs. Routes/panels mirror the static whitelist in
// internal/server/admin_ui_settings.go; demo names come from the live catalog.
const ROUTE_OPTIONS = ['overview', 'processes', 'memory', 'interrupts', 'concurrency', 'x86-real', 'glossary', 'preferences', 'profile'];
const PANEL_OPTIONS = ['cpu', 'code', 'events', 'memory', 'procs', 'ipc', 'console'];
const demoNames = ref<string[]>([]);

// A small curated zone list plus whatever is currently configured, so the admin
// can pick without typing while still seeing the active value.
const TZ_OPTIONS = ['UTC', 'America/Caracas', 'America/Bogota', 'America/Mexico_City', 'America/New_York', 'Europe/Madrid'];
const tzInput = ref('UTC');

const DEFAULTS_FALLBACK: AdminDefaults = {
  narratorsEnabled: true, mode: 'pedagogic', theme: 'amber', density: 'comfortable', speed: 2, narratorMinTickMs: 200,
};
const MODE_OPTIONS: AdminDefaults['mode'][] = ['pedagogic', 'compact', 'projector'];
const THEME_OPTIONS: AdminDefaults['theme'][] = ['amber', 'green', 'dark', 'accesible'];
const DENSITY_OPTIONS: AdminDefaults['density'][] = ['comfortable', 'compact'];

interface FormState {
  id: string | null;
  kind: ScheduleKind;
  target: string;
  action: ScheduleAction;
  startsLocal: string;
  endsLocal: string;
  enabled: boolean;
  defaults: AdminDefaults;
}

function blankForm(): FormState {
  return {
    id: null,
    kind: 'demo',
    target: '',
    action: 'hide',
    startsLocal: '',
    endsLocal: '',
    enabled: true,
    defaults: { ...DEFAULTS_FALLBACK },
  };
}

const form = reactive<FormState>(blankForm());

const targetOptions = computed<string[]>(() => {
  switch (form.kind) {
    case 'demo': return demoNames.value;
    case 'panel': return PANEL_OPTIONS;
    case 'route': return ROUTE_OPTIONS;
    default: return [];
  }
});

// Defaults rules always "apply"; the others choose show/hide.
function onKindChange() {
  if (form.kind === 'defaults') {
    form.action = 'apply';
    form.target = '';
  } else {
    if (form.action === 'apply') form.action = 'hide';
    form.target = targetOptions.value[0] ?? '';
  }
}

const editing = computed(() => form.id !== null);

function resetForm() {
  Object.assign(form, blankForm());
  form.target = targetOptions.value[0] ?? '';
}

function statusOf(rule: ScheduleRule): 'pending' | 'active' | 'expired' {
  const now = Date.now() / 1000;
  if (now < rule.starts_at) return 'pending';
  if (rule.ends_at != null && now >= rule.ends_at) return 'expired';
  return 'active';
}

function kindLabel(k: string): string {
  return t(`admin.schedule.kinds.${k}`);
}

function targetLabel(rule: ScheduleRule): string {
  if (rule.kind === 'defaults') return t('admin.schedule.defaultsTarget');
  return rule.target;
}

async function run(fn: () => Promise<void>, okMsg: string) {
  try {
    await fn();
    toast.ok(okMsg);
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  }
}

function buildInput(): ScheduleRuleInput {
  const target = form.kind === 'defaults' ? JSON.stringify(form.defaults) : form.target;
  return {
    kind: form.kind,
    target,
    action: form.action,
    starts_local: form.startsLocal,
    ends_local: form.endsLocal,
    timezone: tzInput.value,
    enabled: form.enabled,
  };
}

async function submit() {
  if (!form.startsLocal) {
    toast.err(t('admin.schedule.errStartRequired'));
    return;
  }
  if (form.kind !== 'defaults' && !form.target) {
    toast.err(t('admin.schedule.errTargetRequired'));
    return;
  }
  const input = buildInput();
  const id = form.id;
  await run(async () => {
    if (id) await admin.updateSchedule(id, input);
    else await admin.createSchedule(input);
  }, id ? t('admin.schedule.okUpdated') : t('admin.schedule.okCreated'));
  resetForm();
}

function editRule(rule: ScheduleRule) {
  form.id = rule.id;
  form.kind = rule.kind;
  form.action = rule.action;
  form.startsLocal = rule.starts_local;
  form.endsLocal = rule.ends_local;
  form.enabled = rule.enabled;
  if (rule.kind === 'defaults') {
    try {
      form.defaults = { ...DEFAULTS_FALLBACK, ...(JSON.parse(rule.target) as Partial<AdminDefaults>) };
    } catch {
      form.defaults = { ...DEFAULTS_FALLBACK };
    }
    form.target = '';
  } else {
    form.target = rule.target;
  }
  tzInput.value = rule.timezone;
}

async function removeRule(rule: ScheduleRule) {
  await run(() => admin.deleteSchedule(rule.id), t('admin.schedule.okDeleted'));
  if (form.id === rule.id) resetForm();
}

async function saveTZ() {
  await run(() => admin.setSetting('ui.schedule_tz', tzInput.value), t('admin.schedule.okTZ'));
}

async function loadDemoNames() {
  try {
    const r = await apiFetch('/admin/api/demos');
    if (!r.ok) return;
    const body = (await r.json()) as { demos?: { name: string }[] };
    demoNames.value = (body.demos ?? []).map((d) => d.name);
    if (form.kind === 'demo' && !form.target) form.target = demoNames.value[0] ?? '';
  } catch { /* non-fatal: the form just shows an empty demo list */ }
}

onMounted(async () => {
  await run(() => admin.loadSchedules(), '');
  tzInput.value = admin.scheduleTZ || 'UTC';
  await loadDemoNames();
});

const tzChoices = computed(() => {
  const set = new Set(TZ_OPTIONS);
  if (tzInput.value) set.add(tzInput.value);
  return Array.from(set);
});
</script>

<template>
  <Card :title="t('admin.schedule.title')">
    <p class="sched__intro">{{ t('admin.schedule.intro') }}</p>
  </Card>

  <Card :title="t('admin.schedule.tzHeading')">
    <p class="sched__hint">{{ t('admin.schedule.tzHint') }}</p>
    <div class="sched__tz-row">
      <select v-model="tzInput" class="sched__select">
        <option v-for="tz in tzChoices" :key="tz" :value="tz">{{ tz }}</option>
      </select>
      <Btn variant="ghost" @click="saveTZ">{{ t('admin.schedule.tzSave') }}</Btn>
    </div>
  </Card>

  <Card :title="editing ? t('admin.schedule.formEditHeading') : t('admin.schedule.formHeading')">
    <p class="sched__hint">{{ t('admin.schedule.formHint') }}</p>
    <div class="sched__form">
      <label class="sched__field">
        <span class="sched__label">{{ t('admin.schedule.fieldKind') }}</span>
        <select v-model="form.kind" class="sched__select" @change="onKindChange">
          <option value="demo">{{ kindLabel('demo') }}</option>
          <option value="panel">{{ kindLabel('panel') }}</option>
          <option value="route">{{ kindLabel('route') }}</option>
          <option value="defaults">{{ kindLabel('defaults') }}</option>
        </select>
      </label>

      <label v-if="form.kind !== 'defaults'" class="sched__field">
        <span class="sched__label">{{ t('admin.schedule.fieldTarget') }}</span>
        <select v-model="form.target" class="sched__select">
          <option v-for="opt in targetOptions" :key="opt" :value="opt">{{ opt }}</option>
        </select>
      </label>

      <label v-if="form.kind !== 'defaults'" class="sched__field">
        <span class="sched__label">{{ t('admin.schedule.fieldAction') }}</span>
        <select v-model="form.action" class="sched__select">
          <option value="hide">{{ t('admin.schedule.actions.hide') }}</option>
          <option value="show">{{ t('admin.schedule.actions.show') }}</option>
        </select>
      </label>

      <label class="sched__field">
        <span class="sched__label">{{ t('admin.schedule.fieldStart') }}</span>
        <input v-model="form.startsLocal" type="datetime-local" class="sched__select" />
      </label>

      <label class="sched__field">
        <span class="sched__label">{{ t('admin.schedule.fieldEnd') }}</span>
        <input v-model="form.endsLocal" type="datetime-local" class="sched__select" />
        <span class="sched__help">{{ t('admin.schedule.fieldEndHint') }}</span>
      </label>

      <label class="sched__field sched__field--check">
        <input v-model="form.enabled" type="checkbox" />
        <span class="sched__label">{{ t('admin.schedule.fieldEnabled') }}</span>
      </label>
    </div>

    <div v-if="form.kind === 'defaults'" class="sched__defaults">
      <p class="sched__hint">{{ t('admin.schedule.defaultsHint') }}</p>
      <div class="sched__defaults-grid">
        <label class="sched__field sched__field--check">
          <input v-model="form.defaults.narratorsEnabled" type="checkbox" />
          <span class="sched__label">{{ t('admin.simulator.defaults.narrators') }}</span>
        </label>
        <label class="sched__field">
          <span class="sched__label">{{ t('admin.simulator.defaults.mode') }}</span>
          <select v-model="form.defaults.mode" class="sched__select">
            <option v-for="m in MODE_OPTIONS" :key="m" :value="m">{{ t(`admin.simulator.defaults.modes.${m}`) }}</option>
          </select>
        </label>
        <label class="sched__field">
          <span class="sched__label">{{ t('admin.simulator.defaults.theme') }}</span>
          <select v-model="form.defaults.theme" class="sched__select">
            <option v-for="th in THEME_OPTIONS" :key="th" :value="th">{{ t(`admin.simulator.defaults.themes.${th}`) }}</option>
          </select>
        </label>
        <label class="sched__field">
          <span class="sched__label">{{ t('admin.simulator.defaults.density') }}</span>
          <select v-model="form.defaults.density" class="sched__select">
            <option v-for="d in DENSITY_OPTIONS" :key="d" :value="d">{{ t(`admin.simulator.defaults.densities.${d}`) }}</option>
          </select>
        </label>
        <label class="sched__field">
          <span class="sched__label">{{ t('admin.simulator.defaults.speed') }}</span>
          <input v-model.number="form.defaults.speed" type="number" min="1" max="60" class="sched__select" />
        </label>
        <label class="sched__field">
          <span class="sched__label">{{ t('admin.simulator.defaults.tick') }}</span>
          <input v-model.number="form.defaults.narratorMinTickMs" type="number" min="0" max="5000" step="50" class="sched__select" />
        </label>
      </div>
    </div>

    <div class="sched__actions">
      <Btn @click="submit">{{ editing ? t('admin.schedule.update') : t('admin.schedule.create') }}</Btn>
      <Btn v-if="editing" variant="ghost" @click="resetForm">{{ t('admin.schedule.cancel') }}</Btn>
    </div>
  </Card>

  <Card :title="t('admin.schedule.listHeading')">
    <div v-if="!admin.schedules.length" class="sched__empty">{{ t('admin.schedule.empty') }}</div>
    <table v-else class="sched__table">
      <thead>
        <tr>
          <th>{{ t('admin.schedule.colStatus') }}</th>
          <th>{{ t('admin.schedule.colKind') }}</th>
          <th>{{ t('admin.schedule.colTarget') }}</th>
          <th>{{ t('admin.schedule.colAction') }}</th>
          <th>{{ t('admin.schedule.colWindow') }}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="rule in admin.schedules" :key="rule.id" :class="{ 'sched__row--off': !rule.enabled }">
          <td>
            <span class="sched__status" :class="`sched__status--${statusOf(rule)}`">
              {{ t(`admin.schedule.status.${statusOf(rule)}`) }}
            </span>
            <span v-if="!rule.enabled" class="sched__status sched__status--off">{{ t('admin.schedule.status.disabled') }}</span>
          </td>
          <td>{{ kindLabel(rule.kind) }}</td>
          <td><code class="sched__id">{{ targetLabel(rule) }}</code></td>
          <td>{{ t(`admin.schedule.actions.${rule.action}`) }}</td>
          <td class="sched__window">
            <span>{{ rule.starts_local }}</span>
            <span class="sched__window-sep">→</span>
            <span>{{ rule.ends_local || t('admin.schedule.openEnded') }}</span>
            <code class="sched__tz">{{ rule.timezone }}</code>
          </td>
          <td class="sched__row-actions">
            <Btn variant="ghost" @click="editRule(rule)">{{ t('admin.schedule.edit') }}</Btn>
            <Btn variant="ghost" @click="removeRule(rule)">{{ t('admin.schedule.delete') }}</Btn>
          </td>
        </tr>
      </tbody>
    </table>
  </Card>
</template>

<style scoped>
.sched__intro,
.sched__hint {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.6;
  margin: 0 0 var(--space-3) 0;
}

.sched__tz-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.sched__form,
.sched__defaults-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-3);
}

.sched__defaults {
  margin-top: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--border);
}

.sched__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sched__field--check {
  flex-direction: row;
  align-items: center;
  gap: var(--space-2);
}

.sched__label {
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
}

.sched__help {
  font-size: 10px;
  color: var(--text-muted);
}

.sched__select {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
  width: 100%;
  box-sizing: border-box;
}

.sched__actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

.sched__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.sched__table th,
.sched__table td {
  text-align: left;
  padding: 8px 12px;
  border-top: 1px solid var(--border);
  vertical-align: top;
}

.sched__table th {
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: normal;
  border-top: 1px solid var(--border-strong);
}

.sched__row--off { opacity: 0.55; }

.sched__status {
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 1px 6px;
  border-radius: 2px;
  border: 1px solid currentColor;
  display: inline-block;
  margin-right: 4px;
}

.sched__status--active { color: var(--green); background: rgba(106, 255, 158, 0.08); }
.sched__status--pending { color: var(--amber); background: rgba(var(--accent-rgb), 0.08); }
.sched__status--expired { color: var(--text-muted); }
.sched__status--off { color: var(--text-muted); }

.sched__id,
.sched__tz {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-muted);
  background: rgba(var(--accent-rgb), 0.06);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
}

.sched__window {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.sched__window-sep { color: var(--text-muted); }

.sched__row-actions {
  display: flex;
  gap: var(--space-2);
}

.sched__empty {
  text-align: center;
  color: var(--text-muted);
  font-style: italic;
  padding: var(--space-4);
}
</style>
