<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import LockWidget from './LockWidget.vue';
import Term from '../glossary/Term.vue';

// Node ids in the current deadlock circuit ('m1', 's2', ...). The parent
// view owns the single useLockGraph instance and feeds them down so this
// component never duplicates the graph subscription.
const props = defineProps<{
  cycleIds?: string[];
}>();

const { t } = useI18n();
const cpu = useCpuStore();

const cycleSet = computed(() => new Set(props.cycleIds ?? []));
const hasLocks = computed(() => cpu.mutexes.length > 0 || cpu.semaphores.length > 0);

function waitersText(waiters: number[]): string {
  return waiters.length > 0 ? waiters.map(pid => `P${pid}`).join(' → ') : '—';
}
</script>

<template>
  <div class="lb">
    <section class="lb__group">
      <h3><Term term="Mutex" mode="title">{{ t('sections.concurrency.mutexesTitle') }}</Term></h3>
      <p v-if="cpu.mutexes.length === 0" class="lb__empty">{{ t('sections.concurrency.mutexEmpty') }}</p>
      <div v-else class="lb__grid">
        <LockWidget
          v-for="m in cpu.mutexes"
          :key="`m${m.id}`"
          kind="mutex"
          :lock="m"
          :in-cycle="cycleSet.has(`m${m.id}`)"
        />
      </div>
    </section>

    <section class="lb__group">
      <h3><Term term="Semaphore" mode="title">{{ t('sections.concurrency.semsTitle') }}</Term></h3>
      <p v-if="cpu.semaphores.length === 0" class="lb__empty">{{ t('sections.concurrency.semsEmpty') }}</p>
      <div v-else class="lb__grid">
        <LockWidget
          v-for="s in cpu.semaphores"
          :key="`s${s.id}`"
          kind="sem"
          :lock="s"
          :in-cycle="cycleSet.has(`s${s.id}`)"
        />
      </div>
    </section>

    <details v-if="hasLocks" class="lb__details">
      <summary>{{ t('sections.concurrency.lockBay.viewAsTable') }}</summary>
      <table v-if="cpu.mutexes.length > 0" class="lb__table">
        <thead>
          <tr>
            <th>{{ t('sections.concurrency.colId') }}</th>
            <th>{{ t('sections.concurrency.colHolder') }}</th>
            <th>{{ t('sections.concurrency.colQueue') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in cpu.mutexes" :key="m.id">
            <td>M{{ m.id }}</td>
            <td>
              <code v-if="m.holder >= 0">P{{ m.holder }}</code>
              <span v-else class="lb__muted">{{ t('sections.concurrency.holderFree') }}</span>
            </td>
            <td>{{ waitersText(m.waiters) }}</td>
          </tr>
        </tbody>
      </table>
      <table v-if="cpu.semaphores.length > 0" class="lb__table">
        <thead>
          <tr>
            <th>{{ t('sections.concurrency.colId') }}</th>
            <th>{{ t('sections.concurrency.colCount') }}</th>
            <th>{{ t('sections.concurrency.colQueue') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in cpu.semaphores" :key="s.id">
            <td>S{{ s.id }}</td>
            <td><code>{{ s.count }}</code></td>
            <td>{{ waitersText(s.waiters) }}</td>
          </tr>
        </tbody>
      </table>
    </details>
  </div>
</template>

<style scoped>
.lb {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.lb__group h3 {
  font-size: 11px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
  margin: 0 0 var(--space-2);
  font-family: var(--font-display);
}

.lb__empty {
  font-size: 11px;
  color: var(--text-muted);
  margin: 0;
}

.lb__grid {
  display: grid;
  /* min(100%, 190px) lets a card shrink to the column width in narrow side
     panels instead of forcing a fixed 190px track that overflows the panel. */
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 190px), 1fr));
  gap: var(--space-2);
}

.lb__details summary {
  cursor: pointer;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-muted);
}
.lb__details summary:hover { color: var(--cyan); }

.lb__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  margin-top: var(--space-2);
}
.lb__table th {
  text-align: left;
  font-weight: normal;
  color: var(--text-muted);
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  padding: 4px 6px;
  border-bottom: 1px dashed var(--border);
}
.lb__table td {
  padding: 4px 6px;
  border-bottom: 1px dashed rgba(255, 255, 255, 0.04);
}
.lb__table code {
  background: rgba(var(--accent-rgb), 0.1);
  padding: 0 4px;
  border-radius: var(--radius-sm);
}
.lb__muted { color: var(--text-muted); }
</style>
