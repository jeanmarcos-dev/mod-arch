<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { ChevronDown, ChevronRight, Lock, Search } from '@lucide/vue';
import { useCpuStore, type ProgramInfo } from '../stores/cpu';
import { useAppConfigStore } from '../stores/appConfig';
import { useSimBarStore } from '../stores/simBar';
import { useIsStudent } from '../composables/useIsStudent';

const cpu = useCpuStore();
const route = useRoute();
const appCfg = useAppConfigStore();
const bar = useSimBarStore();
const isStudent = useIsStudent();
const { t, te } = useI18n();

const open = ref(false);
const root = ref<HTMLElement | null>(null);
const searchInput = ref<HTMLInputElement | null>(null);
const query = ref('');
const expanded = reactive<Record<string, boolean>>({});

const selectedProgram = computed(() =>
  cpu.programs.find(p => p.name === cpu.selectedProgram),
);

// Routes can constrain the picker to a single demo category by setting
// `meta.category`. Without it, every loaded demo is offered.
const routeCategory = computed(() =>
  typeof route.meta.category === 'string' ? (route.meta.category as string) : '',
);

const constrainedPrograms = computed(() => {
  const cat = routeCategory.value;
  if (!cat) return cpu.programs;
  return cpu.programs.filter(p => (p.category ?? '') === cat);
});

// normalize strips diacritics + lowercases for case-insensitive search.
function normalize(s: string): string {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

const normalizedQuery = computed(() => normalize(query.value.trim()));

function matchesQuery(p: ProgramInfo): boolean {
  const q = normalizedQuery.value;
  if (!q) return true;
  const haystack = [p.name, p.description, ...(p.topics ?? [])]
    .map(normalize)
    .join(' ');
  return haystack.includes(q);
}

const filteredPrograms = computed(() => constrainedPrograms.value.filter(matchesQuery));

// Section is a category header + its demos. Built from cpu.categories so the
// canonical order travels from the backend; demos without a registered slug
// fall under a single fallback section.
interface Section {
  slug: string;
  label: string;
  description: string;
  demos: ProgramInfo[];
}

const sections = computed<Section[]>(() => {
  const byCat = new Map<string, ProgramInfo[]>();
  for (const p of filteredPrograms.value) {
    const cat = p.category ?? '';
    if (!byCat.has(cat)) byCat.set(cat, []);
    byCat.get(cat)!.push(p);
  }
  const out: Section[] = [];
  for (const cm of cpu.categories) {
    const demos = byCat.get(cm.slug);
    if (demos && demos.length > 0) {
      out.push({ slug: cm.slug, label: cm.label, description: cm.description, demos });
      byCat.delete(cm.slug);
    }
  }
  // Anything not registered in cpu.categories lands here. Empty slug means
  // "Otros / sin categoría" — appears last so the curated order is preserved.
  for (const [cat, demos] of byCat) {
    if (demos.length === 0) continue;
    out.push({
      slug: cat,
      label: cat === '' ? t('catalog.sections.uncategorized') : cat,
      description: '',
      demos,
    });
  }
  return out;
});

const hasQuery = computed(() => normalizedQuery.value.length > 0);

// Section labels respect i18n when a key exists, otherwise fall back to the
// backend-provided label.
function sectionLabel(s: Section): string {
  const key = `catalog.categories.${s.slug}.label`;
  if (te(key)) return t(key);
  return s.label;
}

function difficultyLabel(d?: string): string {
  if (!d) return '';
  const key = `catalog.difficulty.${d}`;
  if (te(key)) return t(key);
  return d;
}

function isDemoLocked(name: string): boolean {
  if (!isStudent.value) return false;
  return appCfg.disabledDemos.has(name);
}

function isExpanded(slug: string): boolean {
  if (hasQuery.value) return true;
  if (slug in expanded) return expanded[slug];
  // Default: open only the section containing the currently selected demo.
  return selectedProgram.value?.category === slug;
}

function toggleSection(slug: string) {
  if (hasQuery.value) return; // sections are forced-open while searching
  expanded[slug] = !isExpanded(slug);
}

function tooltipFor(p: ProgramInfo): string {
  if (isDemoLocked(p.name)) return t('programs.locked.tooltip');
  const parts: string[] = [];
  if (p.topics && p.topics.length > 0) {
    parts.push(t('catalog.tooltip.topics', { list: p.topics.join(', ') }));
  }
  if (p.syscalls_used && p.syscalls_used.length > 0) {
    parts.push(t('catalog.tooltip.syscalls', { list: p.syscalls_used.join(', ') }));
  }
  return parts.join('\n');
}

function toggleOpen() {
  open.value = !open.value;
  if (open.value) {
    void nextTick(() => searchInput.value?.focus());
  } else {
    query.value = '';
  }
}

function pick(name: string) {
  if (isDemoLocked(name)) return;
  cpu.selectedProgram = name;
  open.value = false;
  query.value = '';
}

// Cierra al click fuera.
function onDocClick(e: MouseEvent) {
  if (!root.value) return;
  if (!root.value.contains(e.target as Node)) {
    open.value = false;
  }
}

// Atajo global: "/" enfoca búsqueda y abre dropdown. Solo si no estamos en un
// input ya, para no robar el slash a quien esté tipeando.
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    open.value = false;
    return;
  }
  if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey) {
    const target = e.target as HTMLElement | null;
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
      return;
    }
    e.preventDefault();
    open.value = true;
    void nextTick(() => searchInput.value?.focus());
  }
}

// Navegación con flechas dentro del dropdown abierto.
const visibleDemos = computed<ProgramInfo[]>(() => {
  const flat: ProgramInfo[] = [];
  for (const s of sections.value) {
    if (isExpanded(s.slug)) flat.push(...s.demos);
  }
  return flat;
});

function focusDemo(name: string) {
  const el = root.value?.querySelector(
    `[data-demo-row="${CSS.escape(name)}"]`,
  ) as HTMLElement | null;
  el?.focus();
}

function onItemKey(e: KeyboardEvent, name: string) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault();
    pick(name);
    return;
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    const list = visibleDemos.value;
    const idx = list.findIndex(p => p.name === name);
    if (idx < 0) return;
    if (e.key === 'ArrowUp' && idx === 0) {
      searchInput.value?.focus();
      return;
    }
    const nextIdx = e.key === 'ArrowDown' ? Math.min(idx + 1, list.length - 1) : Math.max(idx - 1, 0);
    const target = list[nextIdx];
    if (!target) return;
    focusDemo(target.name);
  }
}

// First non-locked visible demo. Used by Enter from the search input so we
// don't accidentally pick a demo the student isn't allowed to load.
function firstSelectable(): ProgramInfo | undefined {
  return visibleDemos.value.find(p => !isDemoLocked(p.name));
}

function onSearchKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    const first = visibleDemos.value[0];
    if (first) focusDemo(first.name);
    return;
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault();
    const list = visibleDemos.value;
    const last = list[list.length - 1];
    if (last) focusDemo(last.name);
    return;
  }
  if (e.key === 'Enter') {
    e.preventDefault();
    const target = firstSelectable();
    if (target) pick(target.name);
  }
}

watch(open, async (val) => {
  bar.setProgramPickerOpen(val);
  if (val) {
    await nextTick();
    searchInput.value?.focus();
  }
});

onMounted(() => {
  document.addEventListener('click', onDocClick);
  document.addEventListener('keydown', onKeydown);
  // Lazy-load categories when the picker mounts; cpu.connect() also kicks it
  // off but the picker may render before the WS is up (e.g. on /overview).
  void cpu.loadCategories();
});

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', onKeydown);
  bar.setProgramPickerOpen(false);
});
</script>

<template>
  <div class="picker" ref="root">
    <button class="trigger" :class="{ open }" type="button" @click="toggleOpen">
      <span class="sel-name">{{ selectedProgram?.name ?? t('catalog.empty') }}</span>
      <span v-if="selectedProgram?.procs && selectedProgram.procs.length > 1" class="sel-procs">
        {{ selectedProgram.procs.length }} proc
      </span>
      <ChevronDown :size="14" :stroke-width="2" class="arrow" />
    </button>

    <div v-if="open" class="dropdown" role="listbox">
      <div class="search-bar">
        <label class="search-field" @click="searchInput?.focus()">
          <Search :size="14" :stroke-width="2" class="search-icon" />
          <input
            ref="searchInput"
            v-model="query"
            type="text"
            class="search-input"
            :placeholder="t('catalog.search.placeholder')"
            :aria-label="t('catalog.search.placeholder')"
            @keydown="onSearchKey"
          />
          <span v-if="!query" class="search-hint">{{ t('catalog.search.hint') }}</span>
        </label>
      </div>

      <div v-if="sections.length === 0" class="empty">
        {{ t('catalog.search.noResults', { q: query }) }}
      </div>

      <div v-for="s in sections" :key="s.slug" class="section">
        <button
          type="button"
          class="section-header"
          :aria-expanded="isExpanded(s.slug)"
          @click="toggleSection(s.slug)"
        >
          <ChevronDown v-if="isExpanded(s.slug)" :size="12" :stroke-width="2" class="section-caret" />
          <ChevronRight v-else :size="12" :stroke-width="2" class="section-caret" />
          <span class="section-label">{{ sectionLabel(s) }}</span>
          <span class="section-count">({{ s.demos.length }})</span>
        </button>

        <div v-if="isExpanded(s.slug)" class="section-body">
          <div
            v-for="p in s.demos"
            :key="p.name"
            :data-demo-row="p.name"
            class="opt"
            :class="{
              selected: p.name === cpu.selectedProgram,
              locked: isDemoLocked(p.name),
            }"
            role="option"
            tabindex="0"
            :aria-disabled="isDemoLocked(p.name) ? 'true' : undefined"
            :aria-selected="p.name === cpu.selectedProgram"
            :title="tooltipFor(p)"
            @click="pick(p.name)"
            @keydown="onItemKey($event, p.name)"
          >
            <div class="opt-head">
              <span class="opt-name">{{ p.name }}</span>
              <span
                v-if="p.difficulty"
                class="badge"
                :class="`badge-${p.difficulty}`"
              >{{ difficultyLabel(p.difficulty) }}</span>
              <span v-if="p.procs && p.procs.length > 1" class="opt-procs">
                {{ p.procs.length }} proc
              </span>
              <Lock v-if="isDemoLocked(p.name)" class="opt-lock" :size="12" :stroke-width="2" />
            </div>
            <div class="opt-desc">{{ p.description }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.picker {
  position: relative;
  flex: 0 1 280px;
  min-width: 200px;
}

.trigger {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  box-sizing: border-box;
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.trigger:hover {
  background: rgba(var(--accent-rgb), 0.12);
  border-color: var(--amber);
}

.trigger.open {
  border-color: var(--amber);
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.3);
}

.sel-name {
  flex: 1;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sel-procs {
  font-size: 11px;
  color: var(--cyan);
  font-weight: 400;
  letter-spacing: 0;
}

.arrow {
  font-size: 12px;
  color: var(--amber-dim);
}

.dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  min-width: 380px;
  max-width: calc(100vw - var(--space-4));
  max-height: 60vh;
  overflow-y: auto;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-elevation-2, 0 8px 32px rgba(0, 0, 0, 0.6));
  z-index: 1000;
  display: flex;
  flex-direction: column;
}

.search-bar {
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-1);
  position: sticky;
  top: 0;
  z-index: 1;
}

.search-field {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  cursor: text;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.search-field:hover {
  border-color: var(--amber-dim);
}

.search-field:focus-within {
  border-color: var(--amber);
  box-shadow: 0 0 0 1px rgba(var(--accent-rgb), 0.25);
}

.search-icon {
  color: var(--text-muted);
  flex-shrink: 0;
  transition: color var(--transition-fast);
}

.search-field:focus-within .search-icon {
  color: var(--amber);
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  box-shadow: none;
  color: var(--text);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  letter-spacing: 0;
  padding: 2px 0;
  min-width: 0;
}

.search-input::placeholder {
  color: var(--text-muted);
  opacity: 0.7;
}

.search-hint {
  font-size: 10px;
  color: var(--text-muted);
  opacity: 0.7;
  font-family: 'JetBrains Mono', monospace;
  flex-shrink: 0;
  border: 1px solid var(--border);
  padding: 0 5px;
  border-radius: 3px;
  line-height: 1.4;
}

.empty {
  padding: 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
  font-style: italic;
}

.section + .section {
  border-top: 1px solid var(--border);
}

.section-header {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  background: rgba(var(--accent-rgb), 0.03);
  border: none;
  padding: 8px 14px;
  cursor: pointer;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--amber-dim);
  text-align: left;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.section-header:hover {
  background: rgba(var(--accent-rgb), 0.08);
  color: var(--amber);
}

.section-caret {
  color: var(--text-muted);
  flex-shrink: 0;
}

.section-label {
  flex: 1;
}

.section-count {
  font-size: 10px;
  color: var(--text-muted);
  font-weight: 400;
  letter-spacing: 0;
}

.section-body {
  display: flex;
  flex-direction: column;
}

.opt {
  /* role="option" on a <div> rather than <button>: native <button> ignores
     content-driven height when inner text wraps, causing items to overlap. */
  display: block;
  width: 100%;
  text-align: left;
  background: transparent;
  border-bottom: 1px solid var(--border);
  padding: 10px 14px 10px 28px;
  cursor: pointer;
  font-family: 'JetBrains Mono', monospace;
  transition: background var(--transition-fast);
  text-transform: none;
  letter-spacing: 0;
  color: var(--text);
}

.opt:last-child {
  border-bottom: none;
}

.opt:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -1px;
}

.opt:hover {
  background: rgba(var(--accent-rgb), 0.08);
}

.opt.selected {
  background: rgba(var(--accent-rgb), 0.15);
}

.opt.selected .opt-name {
  color: var(--green);
}

.opt.locked {
  opacity: 0.55;
  cursor: not-allowed;
}

.opt.locked:hover {
  background: transparent;
}

.opt-lock {
  margin-left: auto;
  color: var(--text-muted);
  opacity: 0.85;
}

.opt-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  flex-wrap: wrap;
}

.opt-name {
  color: var(--amber);
  font-weight: 700;
  font-size: 12px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.badge {
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 1px 6px;
  border-radius: 2px;
  border: 1px solid currentColor;
}

.badge-basic {
  color: var(--green);
  background: rgba(106, 255, 158, 0.08);
}

.badge-intermediate {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
}

.badge-advanced {
  color: var(--red);
  background: rgba(255, 106, 106, 0.08);
}

.opt-procs {
  font-size: 10px;
  color: var(--cyan);
  background: rgba(106, 212, 255, 0.08);
  border: 1px solid var(--cyan);
  padding: 1px 6px;
}

.opt-desc {
  color: var(--text-dim);
  font-size: 11px;
  line-height: 1.45;
  font-style: italic;
  white-space: normal;
  word-wrap: break-word;
}

@media (max-width: 768px) {
  .picker {
    flex: 1 1 100%;
    min-width: 100%;
  }

  .trigger {
    min-height: 40px;
    font-size: 13px;
    padding: 6px 12px;
  }

  .dropdown {
    min-width: 100%;
    max-width: 100vw;
  }

  .search-field { padding: 8px 12px; }
  .search-input { min-height: 28px; font-size: 14px; }

  .opt {
    padding: 12px 14px 12px 28px;
  }

  .opt-desc {
    font-size: 13px;
    line-height: 1.55;
  }
}

@media (max-width: 480px) {
  /* Anchor the dropdown to the viewport, not the trigger, so it never gets
     clipped by the SimBar wrap. max-height leaves room for the on-screen
     keyboard on phones. */
  .dropdown {
    position: fixed;
    top: auto;
    left: var(--space-2);
    right: var(--space-2);
    min-width: 0;
    max-width: none;
    max-height: 70vh;
  }
  .opt { padding: 12px 12px 12px 24px; }
  .opt-desc { font-size: 12px; line-height: 1.45; }
}
</style>
