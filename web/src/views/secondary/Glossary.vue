<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Search } from '@lucide/vue';
import {
  GLOSSARY_ENTRIES,
  listCategories,
  filterEntries,
  groupByCategory,
  type GlossaryEntry,
} from '../../data/glossary';

const { t, locale } = useI18n();
const route = useRoute();

const search = ref('');
const expanded = ref<Set<string>>(new Set());

// All categories in declared order (matching the data file). The TOC renders
// in this order so the student walks through topics the way they would in
// class — modos → registros → flags → interrupciones → procesos → ...
const categories = computed<string[]>(() => listCategories());

const filtered = computed<GlossaryEntry[]>(() => filterEntries(search.value, locale.value));
const grouped = computed<Record<string, GlossaryEntry[]>>(() => groupByCategory(filtered.value));

// Toggle a single entry. Clicking the row expands the detail; clicking again
// collapses. Independent per-entry so the student can leave several open at
// once for comparison.
function toggle(term: string) {
  if (expanded.value.has(term)) expanded.value.delete(term);
  else expanded.value.add(term);
}

function expandAll() {
  expanded.value = new Set(filtered.value.map(e => e.term));
}

function collapseAll() {
  expanded.value = new Set();
}

// Category name → URL-safe anchor id so TOC links can jump.
function slug(s: string): string {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/(^-|-$)/g, '');
}

// Honor ?q=<term> on the URL so links from the simulator (e.g. a click on
// "EIP" that hits openGlossary) can deep-link to the page too.
onMounted(() => {
  const q = (route.query.q as string | undefined) ?? '';
  if (q) search.value = q;
  if (route.hash) {
    requestAnimationFrame(() => {
      const el = document.querySelector(route.hash);
      el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }
});

// Reset the expanded set on every new search so a fresh query starts fully
// collapsed; the student opens what matters.
watch(search, () => { expanded.value = new Set(); });

const totalEntries = GLOSSARY_ENTRIES.length;
</script>

<template>
  <div class="gloss-page">
    <header class="gloss-page__hero">
      <h2 class="gloss-page__title">{{ t('glossary.title') }}</h2>
      <p class="gloss-page__tagline">{{ t('glossary.tagline', { count: totalEntries }) }}</p>
      <div class="gloss-page__controls">
        <label class="gloss-page__search">
          <Search :size="14" :stroke-width="2" class="gloss-page__search-icon" aria-hidden="true" />
          <input
            v-model="search"
            type="search"
            name="glossary_search"
            :aria-label="t('glossary.searchAria')"
            :placeholder="t('glossary.searchPlaceholder')"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <div class="gloss-page__actions">
          <button type="button" class="gloss-page__btn" @click="expandAll">{{ t('glossary.expandAll') }}</button>
          <button type="button" class="gloss-page__btn" @click="collapseAll">{{ t('glossary.collapseAll') }}</button>
        </div>
      </div>
    </header>

    <div class="gloss-page__layout">
      <nav class="gloss-page__toc" :aria-label="t('glossary.tocAria')">
        <h3 class="gloss-page__toc-title">{{ t('glossary.tocTitle') }}</h3>
        <ul>
          <li v-for="cat in categories" :key="cat">
            <a :href="`#${slug(cat)}`">{{ t(`glossary.categories.${cat}`) }}</a>
            <span
              :class="['gloss-page__toc-count', { 'gloss-page__toc-count--zero': !grouped[cat] }]"
            >{{ grouped[cat]?.length ?? 0 }}</span>
          </li>
        </ul>
      </nav>

      <main class="gloss-page__main">
        <section
          v-for="cat in categories"
          v-show="grouped[cat]"
          :key="cat"
          :id="slug(cat)"
          class="gloss-page__section"
        >
          <h3 class="gloss-page__cat">{{ t(`glossary.categories.${cat}`) }}</h3>
          <div
            v-for="e in (grouped[cat] ?? [])"
            :key="e.term"
            class="gloss-entry"
            :class="{ 'gloss-entry--open': expanded.has(e.term) }"
            @click="toggle(e.term)"
          >
            <div class="gloss-entry__row">
              <span class="gloss-entry__term">{{ e.term }}</span>
              <span class="gloss-entry__short">{{ e.short }}</span>
              <span class="gloss-entry__arrow" aria-hidden="true">{{ expanded.has(e.term) ? '▾' : '▸' }}</span>
            </div>
            <div v-if="expanded.has(e.term)" class="gloss-entry__detail">
              {{ e.detail }}
            </div>
          </div>
        </section>

        <div v-if="filtered.length === 0" class="gloss-page__empty">
          {{ t('glossary.empty', { query: search }) }}
        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
.gloss-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: var(--space-5);
  gap: var(--space-4);
  overflow: hidden;
}

.gloss-page__hero {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.gloss-page__title {
  font-family: var(--font-display);
  font-size: 18px;
  letter-spacing: 5px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.gloss-page__tagline {
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
  max-width: 70ch;
}

.gloss-page__controls {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.gloss-page__search {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  width: 100%;
  max-width: 460px;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: 6px 12px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.gloss-page__search:focus-within {
  border-color: var(--amber);
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.3);
}

.gloss-page__search-icon {
  color: var(--amber);
  font-size: 14px;
}

.gloss-page__search input {
  flex: 1;
  background: transparent;
  border: 0;
  color: var(--text);
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  outline: none;
}

.gloss-page__actions {
  display: flex;
  gap: var(--space-2);
}

.gloss-page__btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  padding: 6px 10px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: color var(--transition-fast), background var(--transition-fast);
}

.gloss-page__btn:hover {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.06);
}

.gloss-page__layout {
  flex: 1;
  display: grid;
  grid-template-columns: 200px 1fr;
  gap: var(--space-4);
  min-height: 0;
}

@media (max-width: 768px) {
  .gloss-page__layout { grid-template-columns: 1fr; }
  .gloss-page__toc { display: none; }
}

.gloss-page__toc {
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--surface-1);
  padding: var(--space-3);
  height: fit-content;
  position: sticky;
  top: 0;
  max-height: calc(100vh - 220px);
  overflow-y: auto;
}

.gloss-page__toc-title {
  font-family: var(--font-display);
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-dim);
  text-transform: uppercase;
  margin-bottom: var(--space-2);
  font-weight: normal;
}

.gloss-page__toc ul {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0;
}

.gloss-page__toc li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.gloss-page__toc a {
  color: var(--text);
  text-decoration: none;
  font-size: 12px;
  padding: 4px 6px;
  border-radius: var(--radius-sm);
  flex: 1;
  transition: color var(--transition-fast), background var(--transition-fast);
}

.gloss-page__toc a:hover {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.06);
}

.gloss-page__toc-count {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
}

.gloss-page__toc-count--zero { opacity: 0.4; }

.gloss-page__main {
  overflow-y: auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding-right: var(--space-2);
}

.gloss-page__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  scroll-margin-top: var(--space-3);
}

.gloss-page__cat {
  font-family: var(--font-display);
  font-size: 13px;
  letter-spacing: 3px;
  color: var(--cyan);
  text-transform: uppercase;
  font-weight: normal;
  padding-bottom: var(--space-1);
  border-bottom: 1px dashed var(--border-strong);
}

.gloss-entry {
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.08);
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast);
}

.gloss-entry:hover { background: rgba(var(--accent-rgb), 0.04); }

.gloss-entry--open { background: rgba(var(--accent-rgb), 0.06); }

.gloss-entry__row {
  display: grid;
  grid-template-columns: minmax(160px, 200px) 1fr auto;
  gap: var(--space-3);
  align-items: baseline;
}

@media (max-width: 768px) {
  .gloss-entry__row {
    grid-template-columns: 1fr;
    gap: var(--space-1);
  }
}

.gloss-entry__term {
  color: var(--amber);
  font-weight: 700;
  font-size: 13px;
  font-family: 'JetBrains Mono', monospace;
}

.gloss-entry__short {
  color: var(--text);
  font-size: 13px;
  font-style: italic;
}

.gloss-entry__arrow {
  color: var(--amber-dim);
  font-size: 13px;
}

.gloss-entry__detail {
  margin-top: var(--space-2);
  padding: var(--space-3);
  background: rgba(0, 0, 0, 0.25);
  border-left: 2px solid var(--cyan);
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
}

.gloss-page__empty {
  color: var(--text-dim);
  font-style: italic;
  text-align: center;
  padding: var(--space-6);
}

@media print {
  .gloss-page__controls,
  .gloss-page__toc,
  .gloss-entry__arrow { display: none !important; }
  .gloss-page__layout { grid-template-columns: 1fr; }
  .gloss-entry { break-inside: avoid; }
  .gloss-entry__detail { display: block !important; }
}
</style>
