<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  Activity,
  Globe,
  Monitor,
  Clock as ClockIcon,
  BotMessageSquare,
  Mail,
  Lock,
  ScrollText,
} from '@lucide/vue';
import { useAdminStore, type SessionHistoryRow } from '../../stores/admin';
import Card from '../../components/ui/Card.vue';
import { formatAgo } from './util';

const { t } = useI18n();
const admin = useAdminStore();

const recent = ref<SessionHistoryRow[]>([]);
const topCountries = ref<Array<{ code: string; count: number }>>([]);

async function loadWidgets() {
  try {
    const page = await admin.loadSessionHistory({}, 1, 25);
    recent.value = page.rows.slice(0, 5);
    const counts = new Map<string, number>();
    for (const row of page.rows) {
      const c = row.geo_country || '';
      if (!c) continue;
      counts.set(c, (counts.get(c) ?? 0) + 1);
    }
    topCountries.value = [...counts.entries()]
      .map(([code, count]) => ({ code, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 5);
  } catch {
    // Best-effort widget: a fresh DB with no history yet shows zero state.
  }
}

const totalRecent = computed(() => admin.sessionHistory?.total ?? 0);

function durationHuman(seconds: number): string {
  if (seconds <= 0) return '0s';
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function flagEmoji(country: string): string {
  if (!country || country.length !== 2) return '';
  const base = 0x1f1e6;
  const a = country.toUpperCase().charCodeAt(0) - 'A'.charCodeAt(0);
  const b = country.toUpperCase().charCodeAt(1) - 'A'.charCodeAt(0);
  if (a < 0 || a > 25 || b < 0 || b > 25) return '';
  return String.fromCodePoint(base + a) + String.fromCodePoint(base + b);
}

onMounted(loadWidgets);
</script>

<template>
  <div class="overview">
    <h1 class="overview__title">{{ t('admin.overview.title') }}</h1>
    <p class="overview__subtitle">
      {{ t('admin.overview.connectedAs') }} <strong>{{ admin.user }}</strong>
    </p>

    <div v-if="admin.stats" class="overview__stats">
      <Card v-if="admin.metrics" :surface="2" :padded="false">
        <div class="stat">
          <Globe :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statWsConnections') }}</span>
          <span class="stat__value">{{ admin.metrics.active_ws_connections }}</span>
        </div>
      </Card>
      <Card v-if="admin.metrics" :surface="2" :padded="false">
        <div class="stat">
          <BotMessageSquare :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statLlmLatency') }}</span>
          <span class="stat__value">{{ Math.round(admin.metrics.avg_llm_latency_ms) }}</span>
        </div>
      </Card>

      <Card :surface="2" :padded="false">
        <div class="stat">
          <Monitor :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statPublic') }}</span>
          <span class="stat__value">{{ admin.stats.full }} / {{ admin.stats.max_public }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <Activity :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statPrivileged') }}</span>
          <span class="stat__value">{{ admin.stats.privileged }} / {{ admin.stats.max_privileged }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <ClockIcon :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statTotalCreated') }}</span>
          <span class="stat__value">{{ admin.stats.total_created }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <Activity :size="14" :stroke-width="2" class="stat__icon" />
          <span class="stat__label">{{ t('admin.overview.statPeak') }}</span>
          <span class="stat__value">{{ admin.stats.peak }}</span>
        </div>
      </Card>
    </div>

    <div class="overview__widgets">
      <Card :title="t('admin.overview.recentTitle')" :surface="2">
        <ul v-if="recent.length > 0" class="recent">
          <li v-for="r in recent" :key="r.id" class="recent__row">
            <span class="recent__when">{{ t('admin.overview.ago', { time: formatAgo(r.ended_at) }) }}</span>
            <span class="recent__who">
              <span v-if="r.user_label">{{ r.user_label }}</span>
              <span v-else class="dim">{{ t('admin.overview.anon') }}</span>
            </span>
            <span class="recent__duration">{{ durationHuman(r.duration_seconds) }}</span>
            <span :class="['recent__reason', 'pill--' + r.end_reason]">{{ r.end_reason }}</span>
            <span v-if="r.geo_country" class="recent__flag">{{ flagEmoji(r.geo_country) }}</span>
          </li>
        </ul>
        <div v-else class="empty">
          <ClockIcon :size="24" :stroke-width="1.5" />
          <p>{{ t('admin.overview.recentEmpty') }}</p>
        </div>
        <RouterLink to="/admin/historial" class="widget-link">
          {{ t('admin.overview.viewAllHistory', { total: totalRecent }) }} →
        </RouterLink>
      </Card>

      <Card :title="t('admin.overview.countriesTitle')" :surface="2">
        <ul v-if="topCountries.length > 0" class="countries">
          <li v-for="c in topCountries" :key="c.code">
            <span>{{ flagEmoji(c.code) }} {{ c.code }}</span>
            <span class="bar-wrap">
              <span
                class="bar-fill"
                :style="{ width: topCountries[0] ? `${(c.count / topCountries[0].count) * 100}%` : '0%' }"
              />
            </span>
            <span class="dim">{{ c.count }}</span>
          </li>
        </ul>
        <div v-else class="empty">
          <Globe :size="24" :stroke-width="1.5" />
          <p>{{ t('admin.overview.countriesEmpty') }}</p>
        </div>
      </Card>

      <Card :title="t('admin.overview.shortcutsTitle')" :surface="2">
        <ul class="overview__shortcuts">
          <li><RouterLink to="/admin/sesiones"><Monitor :size="13" /> {{ t('admin.overview.shortcutSessions') }}</RouterLink></li>
          <li><RouterLink to="/admin/historial"><ClockIcon :size="13" /> {{ t('admin.overview.shortcutHistory') }}</RouterLink></li>
          <li><RouterLink to="/admin/usuarios"><Activity :size="13" /> {{ t('admin.overview.shortcutUsers') }}</RouterLink></li>
          <li v-if="admin.canManageInvites"><RouterLink to="/admin/invitaciones"><Mail :size="13" /> {{ t('admin.overview.shortcutInvites') }}</RouterLink></li>
          <li v-if="admin.canManageSettings"><RouterLink to="/admin/accesos"><Lock :size="13" /> {{ t('admin.overview.shortcutAccess') }}</RouterLink></li>
          <li><RouterLink to="/admin/auditoria"><ScrollText :size="13" /> {{ t('admin.overview.shortcutAudit') }}</RouterLink></li>
          <li><RouterLink to="/admin/tutor"><BotMessageSquare :size="13" /> {{ t('admin.overview.shortcutTutor') }}</RouterLink></li>
        </ul>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.overview__title {
  font-family: var(--font-display);
  font-size: 18px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.overview__subtitle {
  font-size: 12px;
  color: var(--text-dim);
  margin-top: calc(-1 * var(--space-3));
}

.overview__subtitle strong {
  color: var(--amber);
  letter-spacing: 1px;
}

.overview__stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
}

.stat {
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-rows: auto auto;
  align-items: center;
  column-gap: var(--space-2);
  row-gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
}

.stat__icon { grid-row: 1 / span 2; color: var(--text-dim); }
.stat__label {
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.stat__value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 22px;
  color: var(--amber);
  letter-spacing: 1px;
  text-shadow: var(--shadow-glow-amber);
}

.overview__widgets {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-4);
}

.recent {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.recent__row {
  display: grid;
  grid-template-columns: auto 1fr auto auto auto;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
}
.recent__row:last-child { border-bottom: none; }
.recent__when, .recent__duration { font-size: 11px; color: var(--text-dim); }
.recent__reason {
  font-size: 9px;
  padding: 1px 6px;
  letter-spacing: 1px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  text-transform: uppercase;
}
.pill--idle { color: var(--text-dim); }
.pill--admin_kill { color: var(--red, #ff4d4f); border-color: currentColor; }
.pill--replaced { color: var(--amber); border-color: currentColor; }
.pill--shutdown { color: var(--blue, #4d94ff); border-color: currentColor; }
.pill--user_logout { color: var(--green); border-color: currentColor; }
.dim { color: var(--text-dim); }

.countries {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.countries li {
  display: grid;
  grid-template-columns: 60px 1fr auto;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.bar-wrap {
  height: 6px;
  background: rgba(var(--accent-rgb), 0.08);
  border-radius: 3px;
  overflow: hidden;
}
.bar-fill {
  display: block;
  height: 100%;
  background: var(--green);
  opacity: 0.7;
}

.widget-link {
  display: inline-block;
  margin-top: 8px;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--amber);
  text-decoration: none;
}
.widget-link:hover { text-shadow: var(--shadow-glow-amber); }

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 16px;
  color: var(--text-dim);
  font-size: 12px;
}

.overview__shortcuts {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  padding: 0;
  margin: 0;
}
.overview__shortcuts a {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text);
  padding: 6px 0;
  transition: color var(--transition-fast), text-shadow var(--transition-fast);
  text-decoration: none;
}
.overview__shortcuts a:hover {
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}
</style>
