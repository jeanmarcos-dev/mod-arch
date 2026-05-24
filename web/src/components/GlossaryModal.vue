<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { BookOpen, X, ChevronDown, ChevronRight } from '@lucide/vue';
import {
  GLOSSARY_ENTRIES,
  listCategories,
  filterEntries,
  groupByCategory,
} from '../data/glossary';

const props = defineProps<{ open: boolean; initialSearch?: string }>();
const emit = defineEmits<{ (e: 'close'): void }>();

const search = ref('');

// Prefilter when opened with a term (e.g. clicking "EIP" anywhere in the UI).
watch(() => [props.open, props.initialSearch], ([opening, term]) => {
  if (opening) {
    search.value = String(term ?? '');
  }
});

const categories = computed(() => listCategories());
const filtered = computed(() => filterEntries(search.value));
const grouped = computed(() => groupByCategory(filtered.value));

const expanded = ref<Set<string>>(new Set());
function toggle(term: string) {
  if (expanded.value.has(term)) expanded.value.delete(term);
  else expanded.value.add(term);
}

// Keep the import bound for downstream readers — the modal historically held
// the entries inline, now they live in data/glossary.ts.
void GLOSSARY_ENTRIES;
</script>

<template>
  <div v-if="open" class="backdrop" @click.self="emit('close')">
    <div class="modal">
      <div class="modal-head">
        <h2><BookOpen :size="16" :stroke-width="2" /> GLOSARIO</h2>
        <input
          v-model="search"
          name="glossary_search"
          aria-label="Buscar en el glosario"
          class="search"
          placeholder="buscar (IRQ, PL, PCB, scheduler...)"
          autofocus
        />
        <button class="close-btn" @click="emit('close')" aria-label="Cerrar"><X :size="16" :stroke-width="2" /></button>
      </div>

      <div class="modal-body">
        <div v-for="cat in categories" :key="cat">
          <template v-if="grouped[cat]">
            <h3 class="cat-title">{{ cat }}</h3>
            <div
              v-for="e in grouped[cat]"
              :key="e.term"
              class="entry"
              :class="{ expanded: expanded.has(e.term) }"
              @click="toggle(e.term)"
            >
              <div class="entry-row">
                <span class="entry-term">{{ e.term }}</span>
                <span class="entry-short">{{ e.short }}</span>
                <component :is="expanded.has(e.term) ? ChevronDown : ChevronRight" :size="14" :stroke-width="2" class="entry-arrow" />
              </div>
              <div v-if="expanded.has(e.term)" class="entry-detail">
                {{ e.detail }}
              </div>
            </div>
          </template>
        </div>
        <div v-if="filtered.length === 0" class="no-results">
          Sin resultados para "{{ search }}"
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  z-index: 5000;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(2px);
}

.modal {
  width: min(880px, 90vw);
  height: min(720px, 85vh);
  background: var(--bg-1);
  border: 2px solid var(--amber);
  box-shadow: 0 0 40px rgba(var(--accent-rgb), 0.25);
  display: flex;
  flex-direction: column;
}

.modal-head {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-strong);
  background: rgba(var(--accent-rgb), 0.06);
}

.modal-head h2 {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  color: var(--amber);
  letter-spacing: 4px;
  text-transform: uppercase;
  margin: 0;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.search {
  flex: 1;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border-strong);
  color: var(--text);
  padding: 8px 12px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  outline: none;
}

.search:focus {
  border-color: var(--amber);
  box-shadow: 0 0 6px rgba(var(--accent-rgb), 0.3);
}

.close-btn {
  background: transparent;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  width: 36px;
  height: 36px;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 18px;
}

.cat-title {
  font-family: 'Major Mono Display', monospace;
  font-size: 13px;
  color: var(--cyan);
  letter-spacing: 3px;
  text-transform: uppercase;
  margin-top: 16px;
  margin-bottom: 8px;
  padding-bottom: 4px;
  border-bottom: 1px dashed var(--border-strong);
}

.cat-title:first-of-type {
  margin-top: 0;
}

.entry {
  padding: 8px 10px;
  cursor: pointer;
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.08);
  transition: background 0.15s ease;
}

.entry:hover {
  background: rgba(var(--accent-rgb), 0.05);
}

.entry.expanded {
  background: rgba(var(--accent-rgb), 0.08);
}

.entry-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.entry-term {
  color: var(--amber);
  font-weight: 700;
  font-size: 13px;
  min-width: 180px;
  flex-shrink: 0;
}

.entry-short {
  color: var(--text);
  font-size: 13px;
  flex: 1;
  font-style: italic;
}

.entry-arrow {
  color: var(--amber-dim);
  font-size: 13px;
}

.entry-detail {
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(0, 0, 0, 0.3);
  border-left: 2px solid var(--cyan);
  color: var(--text);
  font-size: 13px;
  line-height: 1.55;
  font-style: normal;
}

.no-results {
  color: var(--text-muted);
  font-style: italic;
  text-align: center;
  padding: 32px;
}

@media (max-width: 768px) {
  .modal {
    width: 100vw;
    height: 100vh;
    max-width: none;
    max-height: none;
    border: none;
    border-top: 2px solid var(--amber);
  }
  .modal-head {
    padding: var(--space-2) var(--space-3);
    gap: var(--space-2);
  }
  .modal-head h2 { font-size: 14px; letter-spacing: 2px; }
}
</style>
