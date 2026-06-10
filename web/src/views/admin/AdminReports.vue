<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  RotateCw,
  Inbox,
  ChevronLeft,
  ChevronRight,
  Bug,
  Lightbulb,
  MessageSquare,
} from '@lucide/vue';
import { useAdminStore, type ReportRow } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import { formatAgo } from './util';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const filters = reactive({ kind: '', status: 'open' });
const page = ref(1);
const pageSize = 50;
const loading = ref(false);
const selected = ref<ReportRow | null>(null);

const rows = computed<ReportRow[]>(() => admin.reportsPage?.rows ?? []);
const total = computed(() => admin.reportsPage?.total ?? 0);
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    await admin.loadReports(
      { kind: filters.kind || undefined, status: filters.status || undefined },
      page.value,
      pageSize,
    );
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.reports.errorLoad'));
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  page.value = 1;
  selected.value = null;
  void load();
}

function go(delta: number) {
  const next = page.value + delta;
  if (next < 1 || next > totalPages.value) return;
  page.value = next;
  void load();
}

async function transition(row: ReportRow, next: ReportRow['Status']) {
  try {
    await admin.updateReportStatus(row.ID, next);
    toast.ok(t('admin.reports.okStatus', { status: t(`admin.reports.status_${next}`) }));
    if (selected.value?.ID === row.ID) selected.value = { ...row, Status: next };
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.reports.errorStatus'));
  }
}

function kindIcon(k: string) {
  if (k === 'bug') return Bug;
  if (k === 'idea') return Lightbulb;
  return MessageSquare;
}

function prettyContext(json: string): string {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

onMounted(load);
</script>

<template>
  <Card :title="t('admin.reports.title')">
    <template #header>
      <Btn variant="ghost" size="sm" :loading="loading" @click="load">
        <RotateCw :size="13" :stroke-width="2" :class="{ 'icon--spin': loading }" />
        <span>{{ t('admin.reports.refresh') }}</span>
      </Btn>
    </template>

    <form class="filters" @submit.prevent="applyFilters">
      <label class="filter">
        <span>{{ t('admin.reports.filter_kind') }}</span>
        <select v-model="filters.kind">
          <option value="">{{ t('admin.reports.all') }}</option>
          <option value="feedback">{{ t('admin.reports.kind_feedback') }}</option>
          <option value="bug">{{ t('admin.reports.kind_bug') }}</option>
          <option value="idea">{{ t('admin.reports.kind_idea') }}</option>
        </select>
      </label>
      <label class="filter">
        <span>{{ t('admin.reports.filter_status') }}</span>
        <select v-model="filters.status">
          <option value="">{{ t('admin.reports.all') }}</option>
          <option value="open">{{ t('admin.reports.status_open') }}</option>
          <option value="reviewing">{{ t('admin.reports.status_reviewing') }}</option>
          <option value="resolved">{{ t('admin.reports.status_resolved') }}</option>
          <option value="dismissed">{{ t('admin.reports.status_dismissed') }}</option>
        </select>
      </label>
      <Btn type="submit" variant="primary" size="sm">{{ t('admin.reports.apply') }}</Btn>
    </form>

    <div v-if="rows.length === 0" class="empty">
      <Inbox :size="32" :stroke-width="1.5" />
      <p>{{ t('admin.reports.empty') }}</p>
    </div>

    <div v-else class="reports-grid">
      <ul class="reports-list">
        <li
          v-for="row in rows"
          :key="row.ID"
          :class="['reports-row', { 'reports-row--selected': selected?.ID === row.ID, ['reports-row--' + row.Status]: true, ['reports-row--' + row.Kind]: true }]"
          @click="selected = row"
        >
          <component :is="kindIcon(row.Kind)" :size="14" :stroke-width="2" class="reports-row__icon" />
          <div class="reports-row__main">
            <div class="reports-row__title">{{ row.Title }}</div>
            <div class="reports-row__meta">
              <span class="dim">{{ t('admin.reports.ago', { time: formatAgo(row.TS) }) }}</span>
              <span :class="['pill', 'pill--' + row.Status]">{{ t(`admin.reports.status_${row.Status}`) }}</span>
              <span :class="['pill', 'pill--' + row.Kind]">{{ t(`admin.reports.kind_${row.Kind}`) }}</span>
              <span v-if="!row.UserID" class="pill pill--anon">{{ t('admin.reports.anon') }}</span>
            </div>
          </div>
        </li>
      </ul>

      <aside v-if="selected" class="reports-detail">
        <h3>{{ selected.Title }}</h3>
        <div class="reports-detail__meta">
          <span class="dim">#{{ selected.ID }}</span>
          <span :class="['pill', 'pill--' + selected.Kind]">{{ t(`admin.reports.kind_${selected.Kind}`) }}</span>
          <span :class="['pill', 'pill--' + selected.Status]">{{ t(`admin.reports.status_${selected.Status}`) }}</span>
        </div>
        <p class="reports-detail__body">{{ selected.Body }}</p>

        <details class="reports-detail__context">
          <summary>{{ t('admin.reports.contextLabel') }}</summary>
          <pre>{{ prettyContext(selected.ContextJSON) }}</pre>
        </details>

        <div class="reports-detail__actions">
          <Btn
            v-if="selected.Status !== 'reviewing'"
            size="sm"
            variant="ghost"
            @click="transition(selected, 'reviewing')"
          >{{ t('admin.reports.actionReview') }}</Btn>
          <Btn
            v-if="selected.Status !== 'resolved'"
            size="sm"
            variant="primary"
            @click="transition(selected, 'resolved')"
          >{{ t('admin.reports.actionResolve') }}</Btn>
          <Btn
            v-if="selected.Status !== 'dismissed'"
            size="sm"
            variant="danger"
            @click="transition(selected, 'dismissed')"
          >{{ t('admin.reports.actionDismiss') }}</Btn>
          <Btn
            v-if="selected.Status !== 'open'"
            size="sm"
            variant="ghost"
            @click="transition(selected, 'open')"
          >{{ t('admin.reports.actionReopen') }}</Btn>
        </div>
      </aside>
      <aside v-else class="reports-detail reports-detail--empty">
        <p class="dim">{{ t('admin.reports.selectHint') }}</p>
      </aside>
    </div>

    <div v-if="total > pageSize" class="pager">
      <Btn variant="ghost" size="sm" :disabled="page <= 1" @click="go(-1)">
        <ChevronLeft :size="13" />
        <span>{{ t('admin.reports.prev') }}</span>
      </Btn>
      <span class="dim">{{ t('admin.reports.page_of', { page, total: totalPages, rows: total }) }}</span>
      <Btn variant="ghost" size="sm" :disabled="page >= totalPages" @click="go(1)">
        <span>{{ t('admin.reports.next') }}</span>
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
.filter select {
  font-family: inherit;
  font-size: 13px;
  padding: 6px 10px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  min-width: 140px;
}

.reports-grid {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) 1fr;
  gap: 16px;
}
@media (max-width: 900px) {
  .reports-grid { grid-template-columns: 1fr; }
}

.reports-list {
  list-style: none;
  padding: 0;
  margin: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  max-height: 65vh;
  overflow-y: auto;
}

.reports-row {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border);
  cursor: pointer;
  transition: background var(--transition-fast);
}
.reports-row:last-child { border-bottom: none; }
.reports-row:hover { background: rgba(var(--accent-rgb), 0.06); }
.reports-row--selected { background: rgba(var(--accent-rgb), 0.12); }

.reports-row__icon { color: var(--text-dim); margin-top: 2px; }
.reports-row--bug .reports-row__icon { color: var(--red, #ff4d4f); }
.reports-row--idea .reports-row__icon { color: var(--cyan, #6ad4ff); }

.reports-row__title { font-size: 13px; color: var(--text); }
.reports-row__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
  font-size: 10px;
}

.pill {
  padding: 1px 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.pill--open { color: var(--amber); border-color: currentColor; }
.pill--reviewing { color: var(--cyan, #6ad4ff); border-color: currentColor; }
.pill--resolved { color: var(--green); border-color: currentColor; }
.pill--dismissed { color: var(--text-dim); }
.pill--bug { color: var(--red, #ff4d4f); border-color: currentColor; }
.pill--feedback { color: var(--text); }
.pill--idea { color: var(--cyan, #6ad4ff); border-color: currentColor; }
.pill--anon { color: var(--text-dim); }

.reports-detail {
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 65vh;
  overflow-y: auto;
}
.reports-detail--empty {
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}
.reports-detail h3 { margin: 0; font-size: 15px; color: var(--text); }
.reports-detail__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 10px;
}
.reports-detail__body {
  margin: 0;
  font-size: 13px;
  color: var(--text);
  line-height: 1.6;
  white-space: pre-wrap;
}
.reports-detail__context {
  font-size: 11px;
  color: var(--text-dim);
}
.reports-detail__context pre {
  background: rgba(var(--accent-rgb), 0.05);
  padding: 8px;
  border-radius: var(--radius-sm);
  overflow-x: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  margin: 6px 0 0;
}
.reports-detail__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
}

.dim { color: var(--text-dim); }
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
