<script setup lang="ts" generic="T extends Record<string, unknown>">
import { computed, ref, type Component } from 'vue';
import { ChevronUp, ChevronDown, Inbox } from '@lucide/vue';
import EmptyState from './EmptyState.vue';

export interface Column<R> {
  id: string;
  label: string;
  /** When set, used both for sorting and for default search. */
  accessor?: (row: R) => string | number | boolean;
  sortable?: boolean;
  align?: 'left' | 'right';
  width?: string;
}

const props = withDefaults(
  defineProps<{
    columns: Column<T>[];
    rows: T[];
    searchable?: boolean;
    /** Placeholder shown in the search box. */
    searchPlaceholder?: string;
    /** When set, sort starts on this column id. */
    initialSort?: string;
    /** When false, the empty state is rendered by the parent via the empty slot. */
    showEmpty?: boolean;
    emptyTitle?: string;
    emptyMessage?: string;
    emptyIcon?: Component;
    stickyHeader?: boolean;
  }>(),
  {
    searchable: true,
    searchPlaceholder: 'Buscar…',
    initialSort: '',
    showEmpty: true,
    emptyTitle: 'Sin datos',
    emptyMessage: '',
    stickyHeader: true,
  },
);

const resolvedEmptyIcon = computed<Component>(() => props.emptyIcon ?? Inbox);

const search = ref('');
const sortCol = ref<string>(props.initialSort);
const sortDir = ref<'asc' | 'desc'>('asc');

function toggleSort(col: Column<T>) {
  if (col.sortable === false) return;
  if (sortCol.value === col.id) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc';
  } else {
    sortCol.value = col.id;
    sortDir.value = 'asc';
  }
}

function valueOf(row: T, col: Column<T>): string | number | boolean {
  if (col.accessor) return col.accessor(row);
  const raw = row[col.id as keyof T];
  if (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean') return raw;
  return '';
}

const filtered = computed<T[]>(() => {
  let out = props.rows;
  const q = search.value.trim().toLowerCase();
  if (q) {
    out = out.filter((row) =>
      props.columns.some((col) => {
        const v = valueOf(row, col);
        return String(v).toLowerCase().includes(q);
      }),
    );
  }
  if (sortCol.value) {
    const col = props.columns.find((c) => c.id === sortCol.value);
    if (col) {
      const dir = sortDir.value === 'asc' ? 1 : -1;
      out = [...out].sort((a, b) => {
        const va = valueOf(a, col);
        const vb = valueOf(b, col);
        if (va < vb) return -1 * dir;
        if (va > vb) return 1 * dir;
        return 0;
      });
    }
  }
  return out;
});
</script>

<template>
  <div class="dtable">
    <div v-if="searchable" class="dtable__toolbar">
      <input
        v-model="search"
        type="search"
        class="dtable__search"
        :placeholder="searchPlaceholder"
        :aria-label="searchPlaceholder"
      />
      <span class="dtable__count">{{ filtered.length }} / {{ rows.length }}</span>
    </div>

    <div :class="['dtable__scroll', { 'dtable__scroll--sticky': stickyHeader }]">
      <table class="dtable__table">
        <thead>
          <tr>
            <th
              v-for="col in columns"
              :key="col.id"
              :class="[
                'dtable__th',
                col.align === 'right' && 'dtable__th--right',
                col.sortable !== false && 'dtable__th--sortable',
                sortCol === col.id && 'dtable__th--active',
              ]"
              :style="col.width ? { width: col.width } : undefined"
              @click="toggleSort(col)"
            >
              {{ col.label }}
              <span
                v-if="col.sortable !== false"
                class="dtable__sort"
                aria-hidden="true"
              >
                <component v-if="sortCol === col.id" :is="sortDir === 'asc' ? ChevronUp : ChevronDown" :size="12" :stroke-width="2" />
              </span>
            </th>
            <th v-if="$slots.actions" class="dtable__th dtable__th--right" />
          </tr>
        </thead>
        <tbody v-if="filtered.length">
          <tr v-for="(row, idx) in filtered" :key="(row.id as string) ?? idx">
            <td
              v-for="col in columns"
              :key="col.id"
              :class="['dtable__td', col.align === 'right' && 'dtable__td--right']"
            >
              <slot :name="`cell:${col.id}`" :row="row" :value="valueOf(row, col)">
                {{ valueOf(row, col) }}
              </slot>
            </td>
            <td v-if="$slots.actions" class="dtable__td dtable__td--right dtable__td--actions">
              <slot name="actions" :row="row" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <EmptyState
      v-if="!filtered.length && showEmpty"
      :icon="resolvedEmptyIcon"
      :title="search ? 'Sin coincidencias' : emptyTitle"
      :message="search ? `Ningún resultado para «${search}».` : emptyMessage"
    />
  </div>
</template>

<style scoped>
.dtable {
  display: flex;
  flex-direction: column;
  width: 100%;
  min-height: 0;
}

.dtable__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding-bottom: var(--space-3);
}

.dtable__search {
  flex: 1;
  background: var(--surface-overlay);
  border: 1px solid var(--border);
  color: var(--text);
  font-family: inherit;
  font-size: 12px;
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.dtable__search:focus {
  outline: none;
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}

.dtable__count {
  font-size: 10px;
  letter-spacing: 1.5px;
  color: var(--text-muted);
  text-transform: uppercase;
}

.dtable__scroll {
  overflow: auto;
  flex: 1;
  min-height: 0;
}

.dtable__scroll--sticky thead { position: sticky; top: 0; z-index: var(--z-sticky); }
.dtable__scroll--sticky thead .dtable__th { background: var(--surface-2); }

.dtable__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.dtable__th {
  padding: 9px 12px;
  text-align: left;
  font-size: 10px;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  text-transform: uppercase;
  font-weight: 500;
  border-bottom: 1px solid var(--border);
  user-select: none;
  white-space: nowrap;
}

.dtable__th--right { text-align: right; }

.dtable__th--sortable {
  cursor: pointer;
  transition: color var(--transition-fast);
}

.dtable__th--sortable:hover { color: var(--amber); }
.dtable__th--active { color: var(--amber); }

.dtable__sort {
  display: inline-block;
  width: 12px;
  text-align: center;
  color: var(--amber);
}

.dtable__td {
  padding: calc(8px * var(--row-density, 1)) 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  vertical-align: middle;
}

.dtable__td--right { text-align: right; }
.dtable__td--actions { white-space: nowrap; }

tbody tr:hover { background: rgba(var(--accent-rgb), 0.04); }

@media (max-width: 768px) {
  /* Force horizontal scroll so admin tables stay tabular instead of
     collapsing into an unreadable wrap. The first <th>/<td> sticks to the
     left edge as an anchor while the rest pans. */
  .dtable__scroll {
    -webkit-overflow-scrolling: touch;
  }
  .dtable__table {
    border-collapse: separate;
    border-spacing: 0;
    min-width: 600px;
  }
  .dtable__th,
  .dtable__td {
    border-bottom: 1px solid var(--border);
  }
  .dtable__th:first-child,
  .dtable__td:first-child {
    position: sticky;
    left: 0;
    z-index: 1;
    background: var(--surface-1);
    box-shadow: 2px 0 0 var(--border);
  }
  .dtable__scroll--sticky thead .dtable__th:first-child {
    z-index: calc(var(--z-sticky) + 1);
  }
}
</style>
