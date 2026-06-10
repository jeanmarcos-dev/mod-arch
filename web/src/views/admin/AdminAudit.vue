<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { ScrollText, RotateCw, ChevronLeft, ChevronRight } from '@lucide/vue';
import { useAdminStore, type AuditRow } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import { formatTimestamp } from './util';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const filters = reactive({ actor: '', action: '' });
const page = ref(1);
const pageSize = 50;
const loading = ref(false);

const rows = computed<AuditRow[]>(() => admin.auditPage?.rows ?? []);
const total = computed(() => admin.auditPage?.total ?? 0);
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    await admin.loadAuditPage(
      { actor: filters.actor || undefined, action: filters.action || undefined },
      page.value,
      pageSize,
    );
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.audit.errorLoad'));
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  page.value = 1;
  void load();
}

function resetFilters() {
  filters.actor = '';
  filters.action = '';
  page.value = 1;
  void load();
}

function go(delta: number) {
  const next = page.value + delta;
  if (next < 1 || next > totalPages.value) return;
  page.value = next;
  void load();
}

onMounted(load);
</script>

<template>
  <Card :title="t('admin.audit.title')">
    <template #header>
      <Btn variant="ghost" size="sm" :loading="loading" @click="load">
        <RotateCw :size="13" :stroke-width="2" :class="{ 'icon--spin': loading }" />
        <span>{{ t('admin.audit.refresh') }}</span>
      </Btn>
    </template>

    <form class="filters" @submit.prevent="applyFilters">
      <label class="filter">
        <span>{{ t('admin.audit.filter_actor') }}</span>
        <input v-model.trim="filters.actor" type="text" :placeholder="t('admin.audit.filter_actor_placeholder')" />
      </label>
      <label class="filter">
        <span>{{ t('admin.audit.filter_action') }}</span>
        <input v-model.trim="filters.action" type="text" :placeholder="t('admin.audit.filter_action_placeholder')" />
      </label>
      <Btn type="submit" variant="primary" size="sm">{{ t('admin.audit.apply') }}</Btn>
      <Btn type="button" variant="ghost" size="sm" @click="resetFilters">{{ t('admin.audit.reset') }}</Btn>
    </form>

    <div v-if="rows.length === 0" class="empty">
      <ScrollText :size="32" :stroke-width="1.5" />
      <p>{{ t('admin.audit.emptyMessage') }}</p>
    </div>

    <table v-else class="audit-table">
      <thead>
        <tr>
          <th>{{ t('admin.audit.colWhen') }}</th>
          <th>{{ t('admin.audit.colActor') }}</th>
          <th>{{ t('admin.audit.colAction') }}</th>
          <th>{{ t('admin.audit.colTarget') }}</th>
          <th>{{ t('admin.audit.colMeta') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.id">
          <td><span class="mono dim">{{ formatTimestamp(r.ts) }}</span></td>
          <td><span class="mono">{{ r.actor }}</span></td>
          <td><span class="action">{{ r.action }}</span></td>
          <td><span class="mono">{{ r.target || '—' }}</span></td>
          <td><span class="mono dim">{{ r.meta || '' }}</span></td>
        </tr>
      </tbody>
    </table>

    <div v-if="total > pageSize" class="pager">
      <Btn variant="ghost" size="sm" :disabled="page <= 1" @click="go(-1)">
        <ChevronLeft :size="13" />
        <span>{{ t('admin.audit.prev') }}</span>
      </Btn>
      <span class="dim">{{ t('admin.audit.page_of', { page, total: totalPages, rows: total }) }}</span>
      <Btn variant="ghost" size="sm" :disabled="page >= totalPages" @click="go(1)">
        <span>{{ t('admin.audit.next') }}</span>
        <ChevronRight :size="13" />
      </Btn>
    </div>
  </Card>
</template>

<style scoped>
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: end;
  margin-bottom: 16px;
  padding: 12px;
  background: rgba(var(--accent-rgb), 0.04);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.filter {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 11px;
  color: var(--text-dim);
  letter-spacing: 1px;
  text-transform: uppercase;
}
.filter input {
  font-family: inherit;
  font-size: 13px;
  padding: 6px 10px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  min-width: 160px;
}

.audit-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.audit-table th,
.audit-table td {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
}
.audit-table th {
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
  background: rgba(var(--accent-rgb), 0.04);
}

.action {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
}

.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  margin-top: 16px;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 32px;
  color: var(--text-dim);
}

.icon--spin { animation: spin 0.7s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
