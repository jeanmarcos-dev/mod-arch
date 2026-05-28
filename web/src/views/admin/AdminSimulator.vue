<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import { apiFetch } from '../../lib/api';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';

interface DemoEntry {
  name: string;
  description: string;
  category?: string;
}

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

// Catalog whitelists. Keep in sync with the backend's
// internal/server/admin_ui_settings.go (uiValidRouteNames + uiValidPanelIDs).
const ROUTE_OPTIONS = [
  'overview',
  'processes',
  'memory',
  'interrupts',
  'concurrency',
  'x86-real',
  'glossary',
  'preferences',
  'profile',
];
const PANEL_OPTIONS = ['cpu', 'code', 'events', 'memory', 'procs', 'ipc', 'console'];

const demos = ref<DemoEntry[]>([]);
const demosLoading = ref(false);
const demoFilter = ref('');

const disabledRoutes = computed<Set<string>>(() => new Set(admin.settings?.disabled_routes ?? []));
const disabledPanels = computed<Set<string>>(() => new Set(admin.settings?.disabled_panels ?? []));
const disabledDemos = computed<Set<string>>(() => new Set(admin.settings?.disabled_demos ?? []));
const defaultRoute = computed<string>(() => admin.settings?.default_route ?? 'overview');

const availableDefaultRoutes = computed(() =>
  ROUTE_OPTIONS.filter((r) => !disabledRoutes.value.has(r)),
);

const filteredDemos = computed(() => {
  const q = demoFilter.value.trim().toLowerCase();
  if (!q) return demos.value;
  return demos.value.filter(
    (d) =>
      d.name.toLowerCase().includes(q) ||
      d.description.toLowerCase().includes(q) ||
      (d.category ?? '').toLowerCase().includes(q),
  );
});

async function run(fn: () => Promise<void>, okMsg: string) {
  try {
    await fn();
    toast.ok(okMsg);
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  }
}

async function loadDemos() {
  demosLoading.value = true;
  try {
    const r = await apiFetch('/admin/api/demos');
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    demos.value = (await r.json()) as DemoEntry[];
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.simulator.errorLoadDemos'));
  } finally {
    demosLoading.value = false;
  }
}

async function persistArray(key: string, value: string[], okMsg: string) {
  await run(() => admin.setSetting(key, JSON.stringify(value)), okMsg);
}

function toggleRoute(name: string) {
  const next = new Set(disabledRoutes.value);
  if (next.has(name)) next.delete(name);
  else next.add(name);
  void persistArray('ui.disabled_routes', Array.from(next), t('admin.simulator.routes.okToggled'));
}

function togglePanel(id: string) {
  const next = new Set(disabledPanels.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  void persistArray('ui.disabled_panels', Array.from(next), t('admin.simulator.panels.okToggled'));
}

function toggleDemo(name: string) {
  const next = new Set(disabledDemos.value);
  if (next.has(name)) next.delete(name);
  else next.add(name);
  void persistArray('ui.disabled_demos', Array.from(next), t('admin.simulator.demos.okToggled'));
}

async function setDefaultRoute(name: string) {
  await run(() => admin.setSetting('ui.default_route', name), t('admin.simulator.default.okUpdated'));
}

function routeLabel(name: string): string {
  const key = `router.titles.${name === 'x86-real' ? 'x86' : name}`;
  return t(key);
}

function panelLabel(id: string): string {
  return t(`admin.simulator.panels.labels.${id}`);
}

onMounted(() => {
  loadDemos();
});
</script>

<template>
  <Card :title="t('admin.simulator.title')">
    <p class="sim-admin__intro">{{ t('admin.simulator.intro') }}</p>
  </Card>

  <Card :title="t('admin.simulator.routes.heading')">
    <p class="sim-admin__hint">{{ t('admin.simulator.routes.hint') }}</p>
    <ul class="sim-admin__grid">
      <li v-for="r in ROUTE_OPTIONS" :key="r" class="sim-admin__row">
        <label class="sim-admin__check">
          <input
            type="checkbox"
            :checked="!disabledRoutes.has(r)"
            :disabled="r === defaultRoute"
            @change="toggleRoute(r)"
          />
          <span class="sim-admin__name">{{ routeLabel(r) }}</span>
          <code class="sim-admin__id">{{ r }}</code>
        </label>
        <span v-if="r === defaultRoute" class="sim-admin__tag">{{ t('admin.simulator.default.tag') }}</span>
      </li>
    </ul>
  </Card>

  <Card :title="t('admin.simulator.panels.heading')">
    <p class="sim-admin__hint">{{ t('admin.simulator.panels.hint') }}</p>
    <ul class="sim-admin__grid">
      <li v-for="p in PANEL_OPTIONS" :key="p" class="sim-admin__row">
        <label class="sim-admin__check">
          <input
            type="checkbox"
            :checked="!disabledPanels.has(p)"
            @change="togglePanel(p)"
          />
          <span class="sim-admin__name">{{ panelLabel(p) }}</span>
          <code class="sim-admin__id">{{ p }}</code>
        </label>
      </li>
    </ul>
  </Card>

  <Card :title="t('admin.simulator.default.heading')">
    <p class="sim-admin__hint">{{ t('admin.simulator.default.hint') }}</p>
    <div class="sim-admin__default">
      <select
        class="sim-admin__select"
        :value="defaultRoute"
        @change="setDefaultRoute(($event.target as HTMLSelectElement).value)"
      >
        <option
          v-for="r in availableDefaultRoutes"
          :key="r"
          :value="r"
        >{{ routeLabel(r) }} ({{ r }})</option>
      </select>
    </div>
  </Card>

  <Card :title="t('admin.simulator.demos.heading')">
    <p class="sim-admin__hint">{{ t('admin.simulator.demos.hint') }}</p>
    <div class="sim-admin__demo-controls">
      <input
        v-model="demoFilter"
        type="search"
        class="sim-admin__search"
        :placeholder="t('admin.simulator.demos.searchPlaceholder')"
      />
      <Btn variant="ghost" @click="loadDemos" :disabled="demosLoading">
        {{ demosLoading ? t('admin.simulator.demos.loading') : t('admin.simulator.demos.reload') }}
      </Btn>
    </div>
    <table class="sim-admin__table">
      <thead>
        <tr>
          <th></th>
          <th>{{ t('admin.simulator.demos.colName') }}</th>
          <th>{{ t('admin.simulator.demos.colDescription') }}</th>
          <th>{{ t('admin.simulator.demos.colCategory') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in filteredDemos" :key="d.name">
          <td class="sim-admin__cell-toggle">
            <input
              type="checkbox"
              :checked="!disabledDemos.has(d.name)"
              @change="toggleDemo(d.name)"
            />
          </td>
          <td><code class="sim-admin__id">{{ d.name }}</code></td>
          <td class="sim-admin__cell-desc">{{ d.description }}</td>
          <td>{{ d.category || '—' }}</td>
        </tr>
        <tr v-if="!filteredDemos.length">
          <td colspan="4" class="sim-admin__empty">{{ t('admin.simulator.demos.empty') }}</td>
        </tr>
      </tbody>
    </table>
  </Card>
</template>

<style scoped>
.sim-admin__intro {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.6;
  margin: 0;
}

.sim-admin__hint {
  font-size: 12px;
  color: var(--text-dim);
  margin: 0 0 var(--space-3) 0;
  line-height: 1.5;
}

.sim-admin__grid {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: var(--space-2);
}

.sim-admin__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(var(--accent-rgb), 0.03);
}

.sim-admin__check {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  flex: 1;
  min-width: 0;
}

.sim-admin__check input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--amber);
  flex-shrink: 0;
}

.sim-admin__name {
  font-size: 13px;
  color: var(--text);
}

.sim-admin__id {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-muted);
  background: rgba(var(--accent-rgb), 0.06);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
}

.sim-admin__tag {
  font-size: 9px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--cyan);
  border: 1px solid var(--cyan);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
}

.sim-admin__default {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.sim-admin__select {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
  min-width: 280px;
}

.sim-admin__demo-controls {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
}

.sim-admin__search {
  flex: 1;
  font-family: inherit;
  font-size: 12px;
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
}

.sim-admin__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.sim-admin__table th,
.sim-admin__table td {
  text-align: left;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
}

.sim-admin__table th {
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: normal;
  border-bottom-color: var(--border-strong);
}

.sim-admin__cell-toggle {
  width: 40px;
}

.sim-admin__cell-toggle input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--amber);
}

.sim-admin__cell-desc {
  color: var(--text-dim);
}

.sim-admin__empty {
  text-align: center;
  color: var(--text-muted);
  font-style: italic;
  padding: var(--space-4);
}
</style>
