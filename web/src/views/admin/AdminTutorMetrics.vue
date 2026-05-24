<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { apiFetch } from '../../lib/api';
import Card from '../../components/ui/Card.vue';

interface HourBucket { hour_start: string; count: number }
interface ErrorEntry { ts: string; session_id: string; error: string }
interface Summary {
  window: string;
  total_queries: number;
  unique_askers: number;
  cache_hits: number;
  cache_hit_rate: number;
  degraded_count: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  avg_tokens: number;
  tool_usage: Record<string, number>;
  by_hour: HourBucket[];
  recent_errors: ErrorEntry[];
}

const summary = ref<Summary | null>(null);
const hours = ref(24);
const loading = ref(false);
const errorMsg = ref('');

async function load() {
  loading.value = true;
  errorMsg.value = '';
  try {
    const r = await apiFetch(`/admin/api/tutor/metrics?hours=${hours.value}`);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    summary.value = await r.json();
  } catch (e) {
    errorMsg.value = String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(load);

// Sort tools by usage descending for the chart. The backend may serialize
// an empty map as null; guard against both cases here.
const sortedTools = computed(() => {
  const usage = summary.value?.tool_usage;
  if (!usage) return [];
  return Object.entries(usage).sort(([, a], [, b]) => b - a);
});

// Find the max bucket count for normalizing the bar chart heights.
const maxHourCount = computed(() => {
  const buckets = summary.value?.by_hour;
  if (!buckets || buckets.length === 0) return 1;
  return Math.max(1, ...buckets.map(h => h.count));
});

// Guarded views so a null/undefined slice from the backend never breaks the
// template. Empty array is the safe default.
const hourBuckets = computed(() => summary.value?.by_hour ?? []);
const recentErrors = computed(() => summary.value?.recent_errors ?? []);

function formatHour(iso: string): string {
  const d = new Date(iso);
  return `${d.getHours().toString().padStart(2, '0')}:00`;
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString('es-VE', { hour: '2-digit', minute: '2-digit' });
}

function pct(v: number): string {
  return `${(v * 100).toFixed(1)}%`;
}
</script>

<template>
  <div class="metrics">
    <header class="head">
      <h1>Tutor — métricas</h1>
      <div class="controls">
        <label>
          Ventana:
          <select v-model.number="hours" @change="load">
            <option :value="1">Última hora</option>
            <option :value="6">Últimas 6 h</option>
            <option :value="24">Últimas 24 h</option>
            <option :value="168">Últimos 7 días</option>
          </select>
        </label>
        <button class="refresh" @click="load" :disabled="loading">↻ Recargar</button>
      </div>
    </header>

    <p v-if="errorMsg" class="err">{{ errorMsg }}</p>

    <div v-if="summary" class="stats">
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Preguntas totales</span>
          <span class="stat__value">{{ summary.total_queries }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Sesiones únicas</span>
          <span class="stat__value">{{ summary.unique_askers }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Hit-rate de caché</span>
          <span class="stat__value">{{ pct(summary.cache_hit_rate) }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Respuestas degradadas</span>
          <span class="stat__value">{{ summary.degraded_count }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Latencia p50 / p95 / p99</span>
          <span class="stat__value small">
            {{ summary.latency_p50_ms }} / {{ summary.latency_p95_ms }} / {{ summary.latency_p99_ms }} ms
          </span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Tokens promedio</span>
          <span class="stat__value">{{ summary.avg_tokens.toFixed(0) }}</span>
        </div>
      </Card>
    </div>

    <Card v-if="summary" title="Volumen por hora">
      <div v-if="hourBuckets.length === 0" class="empty">Sin datos en la ventana seleccionada.</div>
      <div v-else class="bars">
        <div v-for="b in hourBuckets" :key="b.hour_start" class="bar-col">
          <div class="bar-track">
            <div
              class="bar-fill"
              :style="{ height: ((b.count / maxHourCount) * 100) + '%' }"
              :title="b.count + ' preguntas'"
            />
          </div>
          <span class="bar-label">{{ formatHour(b.hour_start) }}</span>
          <span class="bar-count">{{ b.count }}</span>
        </div>
      </div>
    </Card>

    <Card v-if="summary" title="Uso de herramientas">
      <div v-if="sortedTools.length === 0" class="empty">Ninguna tool invocada todavía.</div>
      <table v-else class="tools-table">
        <thead>
          <tr><th>Tool</th><th>Invocaciones</th></tr>
        </thead>
        <tbody>
          <tr v-for="[name, n] in sortedTools" :key="name">
            <td><code>{{ name }}</code></td>
            <td>{{ n }}</td>
          </tr>
        </tbody>
      </table>
    </Card>

    <Card v-if="summary && recentErrors.length" title="Errores recientes">
      <ul class="errors">
        <li v-for="(e, i) in recentErrors" :key="i">
          <span class="err-time">{{ formatTime(e.ts) }}</span>
          <span class="err-msg">{{ e.error }}</span>
          <span v-if="e.session_id" class="err-sess">{{ e.session_id.slice(0, 8) }}…</span>
        </li>
      </ul>
    </Card>
  </div>
</template>

<style scoped>
.metrics { display: flex; flex-direction: column; gap: var(--space-4); }
.head { display: flex; justify-content: space-between; align-items: center; gap: 1rem; }
.head h1 {
  font-family: var(--font-display);
  font-size: 18px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
  margin: 0;
}
.controls { display: flex; align-items: center; gap: 0.6rem; font-size: 13px; }
.controls select {
  background: var(--surface-overlay);
  border: 1px solid rgba(245, 158, 11, 0.4);
  color: var(--text);
  padding: 0.3rem 0.5rem;
  font-family: inherit;
}
.refresh {
  background: transparent;
  border: 1px solid rgba(245, 158, 11, 0.4);
  color: var(--amber);
  padding: 0.3rem 0.7rem;
  cursor: pointer;
  font-family: inherit;
}
.refresh:hover:not(:disabled) { background: rgba(245, 158, 11, 0.1); }
.refresh:disabled { opacity: 0.5; cursor: wait; }

.err { color: #fca5a5; font-size: 13px; }

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
}
.stat {
  display: flex;
  flex-direction: column;
  padding: 1rem;
  gap: 0.3rem;
}
.stat__label {
  font-size: 11px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.stat__value {
  font-size: 22px;
  font-family: 'JetBrains Mono', monospace;
  color: var(--amber);
}
.stat__value.small { font-size: 14px; }

.bars {
  display: flex;
  align-items: flex-end;
  gap: 0.3rem;
  height: 140px;
  padding: 0.5rem;
  overflow-x: auto;
}
.bar-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 36px;
  gap: 0.2rem;
}
.bar-track {
  flex: 1;
  width: 100%;
  background: rgba(245, 158, 11, 0.05);
  border-radius: 0.2rem;
  display: flex;
  align-items: flex-end;
}
.bar-fill {
  width: 100%;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.9), rgba(245, 158, 11, 0.3));
  border-radius: 0.2rem 0.2rem 0 0;
  min-height: 2px;
}
.bar-label { font-size: 10px; color: var(--text-dim); font-family: 'JetBrains Mono', monospace; }
.bar-count { font-size: 11px; color: var(--amber); font-family: 'JetBrains Mono', monospace; }

.tools-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.tools-table th, .tools-table td { text-align: left; padding: 0.4rem 0.6rem; border-bottom: 1px solid rgba(245, 158, 11, 0.1); }
.tools-table th { color: var(--text-dim); font-weight: 500; text-transform: uppercase; letter-spacing: 1px; font-size: 11px; }
.tools-table code { color: #c4b5fd; background: rgba(196, 181, 253, 0.08); padding: 0.1rem 0.35rem; border-radius: 0.2rem; }

.errors { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.4rem; }
.errors li {
  display: grid;
  grid-template-columns: 60px 1fr 90px;
  gap: 0.6rem;
  font-size: 12px;
  align-items: baseline;
  padding: 0.3rem 0;
  border-bottom: 1px solid rgba(239, 68, 68, 0.1);
}
.err-time { color: var(--text-dim); font-family: 'JetBrains Mono', monospace; }
.err-msg { color: #fca5a5; }
.err-sess { color: var(--text-dim); font-family: 'JetBrains Mono', monospace; font-size: 11px; }

.empty { color: var(--text-dim); font-size: 12px; padding: 0.5rem; }
</style>
