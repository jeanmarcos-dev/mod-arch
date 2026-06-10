<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { ArrowLeft, BarChart3, Activity } from '@lucide/vue';
import { useAdminStore, type UserActivity } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import { formatAgo } from './util';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const admin = useAdminStore();
const toast = useToastStore();

const userId = computed(() => String(route.params.id ?? ''));
const activity = computed<UserActivity | null>(() => admin.userActivity[userId.value] ?? null);
const profile = computed(() => admin.userProfile[userId.value] ?? null);

async function load() {
  try {
    await Promise.all([admin.loadUserActivity(userId.value), admin.loadUserProfile(userId.value)]);
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.activity.errorLoad'));
  }
}

onMounted(load);
watch(userId, load);

const maxDay = computed(() => {
  const days = activity.value?.by_day ?? [];
  return days.reduce((m, d) => Math.max(m, d.ActiveSeconds), 0);
});

function durationHuman(seconds: number): string {
  if (seconds <= 0) return '0s';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${seconds}s`;
}

function avgMinutes(value: number | undefined): string {
  if (!value) return '0';
  return value.toFixed(1);
}

function flagEmoji(country: string): string {
  if (!country || country.length !== 2) return '';
  const base = 0x1f1e6;
  const a = country.toUpperCase().charCodeAt(0) - 'A'.charCodeAt(0);
  const b = country.toUpperCase().charCodeAt(1) - 'A'.charCodeAt(0);
  if (a < 0 || a > 25 || b < 0 || b > 25) return '';
  return String.fromCodePoint(base + a) + String.fromCodePoint(base + b);
}
</script>

<template>
  <Card :title="activity?.user_label ? t('admin.activity.titleFor', { name: activity.user_label }) : t('admin.activity.title')">
    <template #header>
      <Btn variant="ghost" size="sm" @click="router.push({ name: 'admin-users' })">
        <ArrowLeft :size="13" />
        <span>{{ t('admin.activity.back') }}</span>
      </Btn>
    </template>

    <div v-if="!activity" class="empty">
      <Activity :size="32" :stroke-width="1.5" />
      <p>{{ t('admin.activity.loading') }}</p>
    </div>

    <div v-else class="activity">
      <div v-if="profile" class="profile-card">
        <div v-if="profile.avatar_url" class="profile-card__avatar">
          <img :src="profile.avatar_url" :alt="profile.name" width="64" height="64" />
        </div>
        <div class="profile-card__body">
          <div class="profile-card__name">{{ profile.name }}</div>
          <div v-if="profile.email" class="dim mono">{{ profile.email }}</div>
          <div class="profile-card__meta">
            <span v-if="profile.cohort">{{ t('admin.activity.cohortLabel') }}: {{ profile.cohort }}</span>
            <span v-if="profile.institution">{{ profile.institution }}</span>
            <span v-if="profile.career">{{ profile.career }}</span>
            <span v-if="profile.preferred_language">{{ profile.preferred_language }}</span>
          </div>
          <div class="profile-card__links">
            <a v-if="profile.github" :href="`https://github.com/${profile.github}`" target="_blank" rel="noopener noreferrer">github.com/{{ profile.github }}</a>
            <a v-if="profile.linkedin" :href="`https://linkedin.com/in/${profile.linkedin}`" target="_blank" rel="noopener noreferrer">linkedin/{{ profile.linkedin }}</a>
          </div>
          <p v-if="profile.bio" class="profile-card__bio">{{ profile.bio }}</p>
        </div>
      </div>

      <div class="stat-grid">
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_total_sessions') }}</span>
          <span class="stat-value">{{ activity.total_sessions }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_total_time') }}</span>
          <span class="stat-value">{{ durationHuman(activity.total_active_seconds) }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_avg_session') }}</span>
          <span class="stat-value">{{ avgMinutes(activity.avg_session_minutes) }} min</span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_last_seen') }}</span>
          <span class="stat-value">
            <span v-if="activity.last_seen_at">{{ t('admin.activity.ago', { time: formatAgo(activity.last_seen_at) }) }}</span>
            <span v-else class="dim">—</span>
          </span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_total_commands') }}</span>
          <span class="stat-value">{{ activity.total_commands }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('admin.activity.stat_tutor_queries') }}</span>
          <span class="stat-value">{{ activity.total_tutor_queries }}</span>
        </div>
      </div>

      <div v-if="activity.by_day.length > 0" class="chart">
        <h3>
          <BarChart3 :size="14" />
          <span>{{ t('admin.activity.by_day') }}</span>
        </h3>
        <div class="bars">
          <div
            v-for="d in activity.by_day"
            :key="d.Day"
            class="bar"
            :style="{ height: maxDay > 0 ? `${(d.ActiveSeconds / maxDay) * 100}%` : '0%' }"
            :title="`${d.Day} — ${durationHuman(d.ActiveSeconds)} (${d.Sessions} ${t('admin.activity.sessionsShort')})`"
          />
        </div>
        <div class="bar-axis">
          <span v-if="activity.by_day.length > 0">{{ activity.by_day[0].Day }}</span>
          <span v-if="activity.by_day.length > 1">{{ activity.by_day[activity.by_day.length - 1].Day }}</span>
        </div>
      </div>

      <div v-if="activity.by_country.length > 0" class="country-list">
        <h3>{{ t('admin.activity.by_country') }}</h3>
        <ul>
          <li v-for="c in activity.by_country" :key="c.Country">
            <span>{{ flagEmoji(c.Country) }} {{ c.Country }}</span>
            <span class="dim">{{ c.Sessions }}</span>
          </li>
        </ul>
      </div>

      <div v-if="activity.total_sessions === 0" class="empty">
        <Activity :size="32" :stroke-width="1.5" />
        <p>{{ t('admin.activity.no_data') }}</p>
      </div>
    </div>
  </Card>
</template>

<style scoped>
.activity { display: flex; flex-direction: column; gap: 24px; }

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  background: rgba(var(--accent-rgb), 0.04);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.stat-label {
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.stat-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 18px;
  color: var(--text);
}

.chart h3 {
  display: flex;
  gap: 8px;
  align-items: center;
  margin: 0 0 8px;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.bars {
  display: flex;
  align-items: end;
  gap: 2px;
  height: 120px;
  padding: 8px;
  background: rgba(var(--accent-rgb), 0.04);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.bar {
  flex: 1;
  min-height: 2px;
  background: var(--green);
  opacity: 0.7;
}
.bar:hover { opacity: 1; }
.bar-axis {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: var(--text-dim);
  margin-top: 4px;
}

.country-list h3 {
  margin: 0 0 8px;
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
}
.country-list ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.country-list li {
  display: flex;
  justify-content: space-between;
  padding: 6px 12px;
  background: rgba(var(--accent-rgb), 0.04);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 13px;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 32px;
  color: var(--text-dim);
}
.dim { color: var(--text-dim); }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }

.profile-card {
  display: flex;
  gap: 16px;
  padding: 16px;
  background: rgba(var(--accent-rgb), 0.04);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.profile-card__avatar img {
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}
.profile-card__body { flex: 1; display: flex; flex-direction: column; gap: 4px; }
.profile-card__name { font-size: 14px; color: var(--text); letter-spacing: 0.5px; }
.profile-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 11px;
  color: var(--text-dim);
  margin-top: 4px;
}
.profile-card__links {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 11px;
  margin-top: 4px;
}
.profile-card__links a { color: var(--amber); text-decoration: none; }
.profile-card__links a:hover { text-shadow: var(--shadow-glow-amber); }
.profile-card__bio { margin: 8px 0 0; font-size: 12px; color: var(--text); line-height: 1.5; }
</style>
