<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { RotateCw, Clock, ChevronLeft, ChevronRight } from '@lucide/vue';
import { useAdminStore, type SessionHistoryRow } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import { formatAgo } from './util';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const filters = reactive({
  user_id: '',
  end_reason: '',
  country: '',
});
const page = ref(1);
const pageSize = 50;
const loading = ref(false);

const rows = computed<SessionHistoryRow[]>(() => admin.sessionHistory?.rows ?? []);
const total = computed(() => admin.sessionHistory?.total ?? 0);
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    await admin.loadSessionHistory(
      {
        user_id: filters.user_id || undefined,
        end_reason: filters.end_reason || undefined,
        country: filters.country || undefined,
      },
      page.value,
      pageSize,
    );
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.history.errorLoad'));
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  page.value = 1;
  void load();
}

function resetFilters() {
  filters.user_id = '';
  filters.end_reason = '';
  filters.country = '';
  page.value = 1;
  void load();
}

function go(delta: number) {
  const next = page.value + delta;
  if (next < 1 || next > totalPages.value) return;
  page.value = next;
  void load();
}

function reasonLabel(reason: string): string {
  const key = `admin.history.reason_${reason}`;
  const fallback = reason;
  const translated = t(key);
  return translated === key ? fallback : translated;
}

function durationHuman(seconds: number): string {
  if (seconds <= 0) return '0s';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function flagEmoji(country?: string): string {
  if (!country || country.length !== 2) return '';
  const base = 0x1f1e6;
  const a = country.toUpperCase().charCodeAt(0) - 'A'.charCodeAt(0);
  const b = country.toUpperCase().charCodeAt(1) - 'A'.charCodeAt(0);
  if (a < 0 || a > 25 || b < 0 || b > 25) return '';
  return String.fromCodePoint(base + a) + String.fromCodePoint(base + b);
}

onMounted(load);
</script>

<template>
  <Card :title="t('admin.history.title')">
    <template #header>
      <Btn variant="ghost" size="sm" :loading="loading" @click="load">
        <RotateCw :size="13" :stroke-width="2" :class="{ 'icon--spin': loading }" />
        <span>{{ t('admin.history.refresh') }}</span>
      </Btn>
    </template>

    <form class="filters" @submit.prevent="applyFilters">
      <label class="filter">
        <span>{{ t('admin.history.filter_user') }}</span>
        <input v-model.trim="filters.user_id" type="text" :placeholder="t('admin.history.filter_user_placeholder')" />
      </label>
      <label class="filter">
        <span>{{ t('admin.history.filter_reason') }}</span>
        <select v-model="filters.end_reason">
          <option value="">{{ t('admin.history.all') }}</option>
          <option value="idle">{{ t('admin.history.reason_idle') }}</option>
          <option value="admin_kill">{{ t('admin.history.reason_admin_kill') }}</option>
          <option value="replaced">{{ t('admin.history.reason_replaced') }}</option>
          <option value="shutdown">{{ t('admin.history.reason_shutdown') }}</option>
          <option value="user_logout">{{ t('admin.history.reason_user_logout') }}</option>
        </select>
      </label>
      <label class="filter">
        <span>{{ t('admin.history.filter_country') }}</span>
        <input v-model.trim="filters.country" type="text" maxlength="2" placeholder="VE" />
      </label>
      <Btn type="submit" variant="primary" size="sm">{{ t('admin.history.apply') }}</Btn>
      <Btn type="button" variant="ghost" size="sm" @click="resetFilters">{{ t('admin.history.reset') }}</Btn>
    </form>

    <div v-if="rows.length === 0" class="empty">
      <Clock :size="32" :stroke-width="1.5" />
      <p>{{ t('admin.history.empty') }}</p>
    </div>

    <table v-else class="history-table">
      <thead>
        <tr>
          <th>{{ t('admin.history.col_ended') }}</th>
          <th>{{ t('admin.history.col_duration') }}</th>
          <th>{{ t('admin.history.col_user') }}</th>
          <th>{{ t('admin.history.col_mode') }}</th>
          <th>{{ t('admin.history.col_reason') }}</th>
          <th>{{ t('admin.history.col_country') }}</th>
          <th>{{ t('admin.history.col_ua') }}</th>
          <th class="num">{{ t('admin.history.col_commands') }}</th>
          <th class="num">{{ t('admin.history.col_tutor') }}</th>
          <th>{{ t('admin.history.col_ip') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.id">
          <td><span class="dim">{{ t('admin.history.ago', { time: formatAgo(row.ended_at) }) }}</span></td>
          <td>{{ durationHuman(row.duration_seconds) }}</td>
          <td>
            <span v-if="row.user_label">{{ row.user_label }}</span>
            <span v-else class="dim">{{ t('admin.history.anon') }}</span>
          </td>
          <td><span :class="'mode-' + row.mode">{{ row.mode.toUpperCase() }}</span></td>
          <td><span :class="['pill', 'pill--' + row.end_reason]">{{ reasonLabel(row.end_reason) }}</span></td>
          <td>
            <span v-if="row.geo_country">{{ flagEmoji(row.geo_country) }} {{ row.geo_country }}</span>
            <span v-else class="dim">—</span>
          </td>
          <td><span class="mono">{{ row.ua_short || '—' }}</span></td>
          <td class="num">{{ row.total_commands }}</td>
          <td class="num">{{ row.total_tutor_queries }}</td>
          <td>
            <span v-if="row.client_ip" class="mono" :title="t('admin.history.ip_revealed')">{{ row.client_ip }}</span>
            <span v-else-if="row.client_ip_hash" class="mono dim" :title="t('admin.history.ip_hash_tip')">
              {{ row.client_ip_hash.slice(0, 8) }}…
            </span>
            <span v-else class="dim">—</span>
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="total > pageSize" class="pager">
      <Btn variant="ghost" size="sm" :disabled="page <= 1" @click="go(-1)">
        <ChevronLeft :size="13" />
        <span>{{ t('admin.history.prev') }}</span>
      </Btn>
      <span class="dim">
        {{ t('admin.history.page_of', { page, total: totalPages, rows: total }) }}
      </span>
      <Btn variant="ghost" size="sm" :disabled="page >= totalPages" @click="go(1)">
        <span>{{ t('admin.history.next') }}</span>
        <ChevronRight :size="13" />
      </Btn>
    </div>
  </Card>
</template>

<style scoped>
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
.filter input,
.filter select {
  font-family: inherit;
  font-size: 13px;
  padding: 6px 10px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  min-width: 120px;
}

.history-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.history-table th,
.history-table td {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
}
.history-table th {
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
  background: rgba(var(--accent-rgb), 0.04);
}
.history-table td.num,
.history-table th.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }
.mode-full { color: var(--green); letter-spacing: 1px; }
.mode-privileged { color: var(--yellow); letter-spacing: 1px; }

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid var(--border);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
  color: var(--text-dim);
}
.pill--idle { color: var(--text-dim); }
.pill--admin_kill { color: var(--red, #ff4d4f); border-color: currentColor; }
.pill--replaced { color: var(--amber); border-color: currentColor; }
.pill--shutdown { color: var(--blue, #4d94ff); border-color: currentColor; }
.pill--user_logout { color: var(--green); border-color: currentColor; }

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
