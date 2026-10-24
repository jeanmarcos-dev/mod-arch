<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAdminStore, type AdminDefaults } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import { apiFetch } from '../../lib/api';
import type { CategoryMeta, Difficulty } from '../../stores/cpu';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';

interface DemoEntry {
  name: string;
  description: string;
  category?: string;
  difficulty?: Difficulty;
  topics?: string[];
  syscalls_used?: string[];
}

interface AdminCatalog {
  categories: CategoryMeta[];
  demos: DemoEntry[];
}

const { t, te } = useI18n();
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
const categories = ref<CategoryMeta[]>([]);
const demosLoading = ref(false);
const demoFilter = ref('');
const filterCategory = ref<string>('');
const filterDifficulty = ref<Difficulty | ''>('');
const expanded = reactive<Record<string, boolean>>({});

const disabledRoutes = computed<Set<string>>(() => new Set(admin.settings?.disabled_routes ?? []));
const disabledPanels = computed<Set<string>>(() => new Set(admin.settings?.disabled_panels ?? []));
const disabledDemos = computed<Set<string>>(() => new Set(admin.settings?.disabled_demos ?? []));
const defaultRoute = computed<string>(() => admin.settings?.default_route ?? 'overview');

const availableDefaultRoutes = computed(() =>
  ROUTE_OPTIONS.filter((r) => !disabledRoutes.value.has(r)),
);

function normalize(s: string): string {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

const filteredDemos = computed(() => {
  const q = normalize(demoFilter.value.trim());
  return demos.value.filter((d) => {
    if (filterCategory.value && (d.category ?? '') !== filterCategory.value) return false;
    if (filterDifficulty.value && (d.difficulty ?? 'basic') !== filterDifficulty.value) return false;
    if (!q) return true;
    const haystack = [
      d.name,
      d.description,
      d.category ?? '',
      ...(d.topics ?? []),
      ...(d.syscalls_used ?? []),
    ]
      .map(normalize)
      .join(' ');
    return haystack.includes(q);
  });
});

// Section bundles a category header with the demos that survived the filters.
// Built once per filter change so each header row can compute counts cheaply.
interface Section {
  slug: string;
  label: string;
  description: string;
  demos: DemoEntry[];
  activeCount: number;
}

const sections = computed<Section[]>(() => {
  const byCat = new Map<string, DemoEntry[]>();
  for (const d of filteredDemos.value) {
    const cat = d.category ?? '';
    if (!byCat.has(cat)) byCat.set(cat, []);
    byCat.get(cat)!.push(d);
  }
  const out: Section[] = [];
  for (const cm of categories.value) {
    const list = byCat.get(cm.slug);
    if (!list || list.length === 0) continue;
    out.push({
      slug: cm.slug,
      label: cm.label,
      description: cm.description,
      demos: list,
      activeCount: list.filter((d) => !disabledDemos.value.has(d.name)).length,
    });
    byCat.delete(cm.slug);
  }
  for (const [cat, list] of byCat) {
    if (list.length === 0) continue;
    out.push({
      slug: cat,
      label: cat === '' ? t('admin.simulator.demos.uncategorized') : cat,
      description: '',
      demos: list,
      activeCount: list.filter((d) => !disabledDemos.value.has(d.name)).length,
    });
  }
  return out;
});

function sectionLabel(s: Section): string {
  const key = `catalog.categories.${s.slug}.label`;
  return te(key) ? t(key) : s.label;
}

function sectionDescription(s: Section): string {
  const key = `catalog.categories.${s.slug}.description`;
  return te(key) ? t(key) : s.description;
}

function difficultyLabel(d?: string): string {
  const key = `catalog.difficulty.${d ?? 'basic'}`;
  return te(key) ? t(key) : (d ?? '');
}

function isExpanded(slug: string): boolean {
  if (slug in expanded) return expanded[slug];
  // Default: collapsed unless a filter is narrowing the view.
  return Boolean(demoFilter.value || filterCategory.value || filterDifficulty.value);
}

function toggleSection(slug: string) {
  expanded[slug] = !isExpanded(slug);
}

// Header checkbox state. Tri-state because a category can be fully enabled,
// fully disabled, or somewhere in between; the browser does not expose
// "indeterminate" through the bound `:checked`, so we set it via ref hook.
function sectionAllChecked(s: Section): boolean {
  return s.activeCount === s.demos.length;
}
function sectionAnyChecked(s: Section): boolean {
  return s.activeCount > 0;
}
function setIndeterminate(el: Element | null, s: Section) {
  if (!(el instanceof HTMLInputElement)) return;
  el.indeterminate = s.activeCount > 0 && s.activeCount < s.demos.length;
}

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
    const body = (await r.json()) as AdminCatalog;
    demos.value = body.demos ?? [];
    categories.value = body.categories ?? [];
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

// Tri-state category toggle: if any demo is enabled, the click disables ALL
// of them; otherwise it enables them. This matches the visual hint of
// `indeterminate → false` rather than the reverse, which we found less
// surprising in admin testing.
function toggleCategory(s: Section) {
  const next = new Set(disabledDemos.value);
  if (s.activeCount > 0) {
    for (const d of s.demos) next.add(d.name);
    void persistArray(
      'ui.disabled_demos',
      Array.from(next),
      t('admin.simulator.demos.okCategoryDisabled', { label: sectionLabel(s) }),
    );
  } else {
    for (const d of s.demos) next.delete(d.name);
    void persistArray(
      'ui.disabled_demos',
      Array.from(next),
      t('admin.simulator.demos.okCategoryEnabled', { label: sectionLabel(s) }),
    );
  }
}

async function setDefaultRoute(name: string) {
  await run(() => admin.setSetting('ui.default_route', name), t('admin.simulator.default.okUpdated'));
}

// Default experience: the baseline narrators / mode / theme / density / speed a
// fresh student inherits. Applied client-side per-field and sticky, so it only
// touches options the student has not changed themselves.
const DEFAULTS_FALLBACK: AdminDefaults = {
  narratorsEnabled: true,
  mode: 'pedagogic',
  theme: 'amber',
  density: 'comfortable',
  speed: 2,
  narratorMinTickMs: 200,
};
const MODE_OPTIONS: AdminDefaults['mode'][] = ['pedagogic', 'compact', 'projector'];
const THEME_OPTIONS: AdminDefaults['theme'][] = ['amber', 'green', 'dark', 'accesible'];
const DENSITY_OPTIONS: AdminDefaults['density'][] = ['comfortable', 'compact'];

const defaults = computed<AdminDefaults>(() => admin.settings?.defaults ?? DEFAULTS_FALLBACK);

async function patchDefaults(patch: Partial<AdminDefaults>) {
  const next: AdminDefaults = { ...defaults.value, ...patch };
  await run(() => admin.setSetting('ui.defaults', JSON.stringify(next)), t('admin.simulator.defaults.okUpdated'));
}

function onDefaultSpeed(e: Event) {
  const v = Number((e.target as HTMLInputElement).value);
  if (Number.isFinite(v)) void patchDefaults({ speed: Math.max(1, Math.min(60, Math.round(v))) });
}

function onDefaultTick(e: Event) {
  const v = Number((e.target as HTMLInputElement).value);
  if (Number.isFinite(v)) void patchDefaults({ narratorMinTickMs: Math.max(0, Math.min(5000, Math.round(v))) });
}

function routeLabel(name: string): string {
  const key = `router.titles.${name === 'x86-real' ? 'x86' : name}`;
  return t(key);
}

function panelLabel(id: string): string {
  return t(`admin.simulator.panels.labels.${id}`);
}

const difficultyOptions: Difficulty[] = ['basic', 'intermediate', 'advanced'];

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

  <Card :title="t('admin.simulator.defaults.heading')">
    <p class="sim-admin__hint">{{ t('admin.simulator.defaults.hint') }}</p>
    <ul class="sim-admin__grid">
      <li class="sim-admin__row">
        <label class="sim-admin__check">
          <input
            type="checkbox"
            :checked="defaults.narratorsEnabled"
            @change="patchDefaults({ narratorsEnabled: ($event.target as HTMLInputElement).checked })"
          />
          <span class="sim-admin__name">{{ t('admin.simulator.defaults.narrators') }}</span>
        </label>
      </li>
    </ul>
    <div class="sim-admin__defaults-grid">
      <label class="sim-admin__field">
        <span class="sim-admin__field-label">{{ t('admin.simulator.defaults.mode') }}</span>
        <select
          class="sim-admin__select sim-admin__select--narrow"
          :value="defaults.mode"
          @change="patchDefaults({ mode: ($event.target as HTMLSelectElement).value as AdminDefaults['mode'] })"
        >
          <option v-for="m in MODE_OPTIONS" :key="m" :value="m">{{ t(`admin.simulator.defaults.modes.${m}`) }}</option>
        </select>
      </label>
      <label class="sim-admin__field">
        <span class="sim-admin__field-label">{{ t('admin.simulator.defaults.theme') }}</span>
        <select
          class="sim-admin__select sim-admin__select--narrow"
          :value="defaults.theme"
          @change="patchDefaults({ theme: ($event.target as HTMLSelectElement).value as AdminDefaults['theme'] })"
        >
          <option v-for="th in THEME_OPTIONS" :key="th" :value="th">{{ t(`admin.simulator.defaults.themes.${th}`) }}</option>
        </select>
      </label>
      <label class="sim-admin__field">
        <span class="sim-admin__field-label">{{ t('admin.simulator.defaults.density') }}</span>
        <select
          class="sim-admin__select sim-admin__select--narrow"
          :value="defaults.density"
          @change="patchDefaults({ density: ($event.target as HTMLSelectElement).value as AdminDefaults['density'] })"
        >
          <option v-for="d in DENSITY_OPTIONS" :key="d" :value="d">{{ t(`admin.simulator.defaults.densities.${d}`) }}</option>
        </select>
      </label>
      <label class="sim-admin__field">
        <span class="sim-admin__field-label">{{ t('admin.simulator.defaults.speed') }}</span>
        <input
          type="number"
          min="1"
          max="60"
          class="sim-admin__num"
          :value="defaults.speed"
          @change="onDefaultSpeed"
        />
        <span class="sim-admin__field-help">{{ t('admin.simulator.defaults.speedHint') }}</span>
      </label>
      <label class="sim-admin__field">
        <span class="sim-admin__field-label">{{ t('admin.simulator.defaults.tick') }}</span>
        <input
          type="number"
          min="0"
          max="5000"
          step="50"
          class="sim-admin__num"
          :value="defaults.narratorMinTickMs"
          @change="onDefaultTick"
        />
        <span class="sim-admin__field-help">{{ t('admin.simulator.defaults.tickHint') }}</span>
      </label>
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
      <select v-model="filterCategory" class="sim-admin__select sim-admin__select--narrow">
        <option value="">{{ t('admin.simulator.demos.filterAllCategories') }}</option>
        <option v-for="cm in categories" :key="cm.slug" :value="cm.slug">
          {{ te(`catalog.categories.${cm.slug}.label`) ? t(`catalog.categories.${cm.slug}.label`) : cm.label }}
        </option>
      </select>
      <select v-model="filterDifficulty" class="sim-admin__select sim-admin__select--narrow">
        <option value="">{{ t('admin.simulator.demos.filterAllDifficulties') }}</option>
        <option v-for="d in difficultyOptions" :key="d" :value="d">{{ difficultyLabel(d) }}</option>
      </select>
      <Btn variant="ghost" @click="loadDemos" :disabled="demosLoading">
        {{ demosLoading ? t('admin.simulator.demos.loading') : t('admin.simulator.demos.reload') }}
      </Btn>
    </div>

    <div v-if="!sections.length" class="sim-admin__empty">
      {{ t('admin.simulator.demos.empty') }}
    </div>

    <div v-for="s in sections" :key="s.slug" class="sim-admin__section">
      <div class="sim-admin__section-header">
        <input
          type="checkbox"
          class="sim-admin__master"
          :checked="sectionAllChecked(s)"
          :ref="(el) => setIndeterminate(el as Element | null, s)"
          :title="sectionAnyChecked(s)
            ? t('admin.simulator.demos.tooltipDisableAll', { label: sectionLabel(s) })
            : t('admin.simulator.demos.tooltipEnableAll', { label: sectionLabel(s) })"
          @change="toggleCategory(s)"
        />
        <button
          type="button"
          class="sim-admin__section-trigger"
          :aria-expanded="isExpanded(s.slug)"
          @click="toggleSection(s.slug)"
        >
          <span class="sim-admin__section-caret">{{ isExpanded(s.slug) ? '▾' : '▸' }}</span>
          <span class="sim-admin__section-label">{{ sectionLabel(s) }}</span>
          <span class="sim-admin__section-count">
            {{ t('admin.simulator.demos.categoryStatus', { active: s.activeCount, total: s.demos.length }) }}
          </span>
        </button>
        <span class="sim-admin__section-slug">{{ s.slug }}</span>
      </div>
      <p v-if="isExpanded(s.slug) && sectionDescription(s)" class="sim-admin__section-desc">
        {{ sectionDescription(s) }}
      </p>

      <table v-if="isExpanded(s.slug)" class="sim-admin__table">
        <thead>
          <tr>
            <th></th>
            <th>{{ t('admin.simulator.demos.colName') }}</th>
            <th>{{ t('admin.simulator.demos.colDifficulty') }}</th>
            <th>{{ t('admin.simulator.demos.colDescription') }}</th>
            <th>{{ t('admin.simulator.demos.colTopics') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="d in s.demos" :key="d.name">
            <td class="sim-admin__cell-toggle">
              <input
                type="checkbox"
                :checked="!disabledDemos.has(d.name)"
                @change="toggleDemo(d.name)"
              />
            </td>
            <td><code class="sim-admin__id">{{ d.name }}</code></td>
            <td>
              <span
                v-if="d.difficulty"
                class="sim-admin__badge"
                :class="`sim-admin__badge--${d.difficulty}`"
              >{{ difficultyLabel(d.difficulty) }}</span>
            </td>
            <td class="sim-admin__cell-desc">{{ d.description }}</td>
            <td class="sim-admin__cell-chips">
              <span v-for="topic in d.topics ?? []" :key="topic" class="sim-admin__chip">{{ topic }}</span>
              <span
                v-for="sc in d.syscalls_used ?? []"
                :key="`sc-${sc}`"
                class="sim-admin__chip sim-admin__chip--syscall"
                :title="t('admin.simulator.demos.chipSyscall')"
              >{{ sc }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
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

.sim-admin__select--narrow {
  min-width: 180px;
}

.sim-admin__defaults-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-3);
  margin-top: var(--space-3);
}

.sim-admin__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sim-admin__field-label {
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
}

.sim-admin__field-help {
  font-size: 10px;
  color: var(--text-muted);
}

.sim-admin__num {
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

.sim-admin__demo-controls {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  flex-wrap: wrap;
}

.sim-admin__search {
  flex: 1;
  min-width: 200px;
  font-family: inherit;
  font-size: 12px;
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
}

.sim-admin__section {
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  margin-bottom: var(--space-2);
  background: rgba(var(--accent-rgb), 0.03);
}

.sim-admin__section-header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 10px 12px;
}

.sim-admin__master {
  width: 18px;
  height: 18px;
  accent-color: var(--amber);
  flex-shrink: 0;
  cursor: pointer;
}

.sim-admin__section-trigger {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex: 1;
  background: transparent;
  border: none;
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
  padding: 0;
  text-align: left;
}

.sim-admin__section-trigger:hover {
  color: var(--amber);
  filter: brightness(1.2);
}

.sim-admin__section-caret {
  color: var(--text-muted);
  font-size: 10px;
  width: 12px;
  flex-shrink: 0;
}

.sim-admin__section-label {
  flex: 1;
}

.sim-admin__section-count {
  font-size: 11px;
  color: var(--text-dim);
  font-weight: 400;
  letter-spacing: 0;
  text-transform: none;
}

.sim-admin__section-slug {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-muted);
  opacity: 0.7;
}

.sim-admin__section-desc {
  font-size: 11px;
  color: var(--text-dim);
  font-style: italic;
  padding: 0 12px 8px 42px;
  margin: 0;
  line-height: 1.5;
}

.sim-admin__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.sim-admin__table th,
.sim-admin__table td {
  text-align: left;
  padding: 6px 12px;
  border-top: 1px solid var(--border);
  vertical-align: top;
}

.sim-admin__table th {
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: normal;
  border-top: 1px solid var(--border-strong);
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
  max-width: 380px;
}

.sim-admin__cell-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: 280px;
}

.sim-admin__chip {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  letter-spacing: 0.5px;
  color: var(--text-dim);
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid var(--border);
  padding: 1px 5px;
  border-radius: 2px;
}

.sim-admin__chip--syscall {
  color: var(--cyan);
  background: rgba(106, 212, 255, 0.08);
  border-color: var(--cyan);
}

.sim-admin__badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 1px 6px;
  border-radius: 2px;
  border: 1px solid currentColor;
  display: inline-block;
}

.sim-admin__badge--basic {
  color: var(--green);
  background: rgba(106, 255, 158, 0.08);
}

.sim-admin__badge--intermediate {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.sim-admin__badge--advanced {
  color: var(--red);
  background: rgba(255, 106, 106, 0.08);
}

.sim-admin__empty {
  text-align: center;
  color: var(--text-muted);
  font-style: italic;
  padding: var(--space-4);
}
</style>
