<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Maximize, Minimize, Presentation, Radio, ThumbsDown, ThumbsUp, WifiOff } from '@lucide/vue';
import { LIVE_WINDOWS_MINUTES, useAdminStore } from '../../stores/admin';
import { useLiveDashboard } from '../../composables/useLiveDashboard';
import {
  flagEmoji,
  formatClock,
  formatLatency,
  formatShortDuration,
  sectionTitleKey,
  stripCitationBrackets,
} from '../../lib/liveFormat';
import LiveBars from '../../components/live/LiveBars.vue';
import LiveSparkline from '../../components/live/LiveSparkline.vue';

const { t, te } = useI18n();
const admin = useAdminStore();
const { presentation, windowMinutes, lastOkAt, stale, loading } = useLiveDashboard();

const root = ref<HTMLElement | null>(null);
const fullscreen = ref(false);

const live = computed(() => admin.live);
const emptyLabel = computed(() => t('admin.live.empty'));

const windowOptions = LIVE_WINDOWS_MINUTES.map((minutes) => ({
  minutes,
  key: `admin.live.controls.w${minutes}`,
}));

function sectionLabel(path: string): string {
  const key = sectionTitleKey(path);
  return key && te(key) ? t(key) : path;
}

const pageViewItems = computed(() =>
  (live.value?.window.page_views ?? []).map((p) => ({ name: p.name, count: p.views })),
);
const averageByScreen = computed(() => {
  const map = new Map<string, number>();
  for (const p of live.value?.window.page_views ?? []) map.set(p.name, p.avg_ms);
  return map;
});
const demoWindowItems = computed(() =>
  (live.value?.window.demos ?? []).map((d) => ({ name: d.name, count: d.loads + d.runs })),
);
const demoWindowDetail = computed(() => {
  const map = new Map<string, string>();
  for (const d of live.value?.window.demos ?? []) {
    map.set(d.name, t('admin.live.window.loadsRuns', { loads: d.loads, runs: d.runs }));
  }
  return map;
});
const countryItems = computed(() =>
  (live.value?.countries ?? []).map((c) => ({ name: c.code, count: c.count })),
);
const topicItems = computed(() =>
  (live.value?.tutor.top_topics ?? []).map((c) => ({ name: c.label, count: c.count })),
);

function screenLabel(path: string): string {
  const avg = averageByScreen.value.get(path);
  const base = sectionLabel(path);
  return avg ? `${base} · ${t('admin.live.window.average', { time: formatShortDuration(avg) })}` : base;
}

function demoLabel(name: string): string {
  const detail = demoWindowDetail.value.get(name);
  return detail ? `${name} · ${detail}` : name;
}

function countryLabel(code: string): string {
  const flag = flagEmoji(code);
  return flag ? `${flag} ${code}` : code;
}

const ratingTotal = computed(() => (live.value?.tutor.thumbs_up ?? 0) + (live.value?.tutor.thumbs_down ?? 0));
const upPct = computed(() =>
  ratingTotal.value === 0 ? 0 : Math.round(((live.value?.tutor.thumbs_up ?? 0) / ratingTotal.value) * 100),
);

const statusText = computed(() => {
  if (stale.value && lastOkAt.value !== null) {
    return t('admin.live.status.stale', { time: formatClock(lastOkAt.value) });
  }
  if (lastOkAt.value !== null) return t('admin.live.status.updated', { time: formatClock(lastOkAt.value) });
  return loading.value ? t('admin.live.status.loading') : t('admin.live.status.waiting');
});

function onFullscreenChange() {
  fullscreen.value = document.fullscreenElement !== null;
}

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen();
    else await root.value?.requestFullscreen();
  } catch {
    fullscreen.value = document.fullscreenElement !== null;
  }
}

onMounted(() => document.addEventListener('fullscreenchange', onFullscreenChange));
onBeforeUnmount(() => document.removeEventListener('fullscreenchange', onFullscreenChange));
</script>

<template>
  <div ref="root" class="live" data-testid="admin-live">
    <header class="live__bar">
      <div class="live__heading">
        <Radio :size="28" :stroke-width="2" class="live__icon" aria-hidden="true" />
        <div>
          <h1 class="live__title">{{ t('admin.live.title') }}</h1>
          <p class="live__subtitle">{{ t('admin.live.subtitle') }}</p>
        </div>
      </div>

      <div class="live__controls">
        <span
          class="live__status"
          :class="{ 'live__status--stale': stale }"
          role="status"
          data-testid="live-status"
        >
          <WifiOff v-if="stale" :size="16" :stroke-width="2" aria-hidden="true" />
          {{ statusText }}
        </span>

        <div class="live__segmented" role="group" :aria-label="t('admin.live.controls.window')">
          <button
            v-for="opt in windowOptions"
            :key="opt.minutes"
            type="button"
            class="live__seg"
            :class="{ 'live__seg--active': windowMinutes === opt.minutes }"
            :aria-pressed="windowMinutes === opt.minutes"
            :data-window="opt.minutes"
            @click="windowMinutes = opt.minutes"
          >
            {{ t(opt.key) }}
          </button>
        </div>

        <button
          type="button"
          class="live__toggle"
          :class="{ 'live__toggle--on': presentation }"
          :aria-pressed="presentation"
          data-testid="live-presentation-toggle"
          @click="presentation = !presentation"
        >
          <Presentation :size="18" :stroke-width="2" aria-hidden="true" />
          {{ t('admin.live.controls.presentation') }}
        </button>

        <button
          type="button"
          class="live__toggle"
          :title="fullscreen ? t('admin.live.controls.exitFullscreen') : t('admin.live.controls.fullscreen')"
          :aria-label="fullscreen ? t('admin.live.controls.exitFullscreen') : t('admin.live.controls.fullscreen')"
          data-testid="live-fullscreen"
          @click="toggleFullscreen"
        >
          <Minimize v-if="fullscreen" :size="18" :stroke-width="2" aria-hidden="true" />
          <Maximize v-else :size="18" :stroke-width="2" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-if="!live" class="live__placeholder">{{ statusText }}</div>

    <template v-else>
      <section class="live__hero" aria-live="off">
        <div class="hero-card hero-card--primary">
          <span class="hero-card__label">{{ t('admin.live.hero.connected') }}</span>
          <span class="hero-card__value" data-testid="live-connected">{{ live.now.sessions_active }}</span>
        </div>
        <div class="hero-card">
          <span class="hero-card__label">{{ t('admin.live.hero.withAccount') }}</span>
          <span class="hero-card__value" data-testid="live-with-account">{{ live.now.sessions_with_user }}</span>
          <span class="hero-card__aside">
            {{ t('admin.live.hero.anonymous') }}: {{ live.now.sessions_anonymous }}
          </span>
        </div>
        <div class="hero-card">
          <span class="hero-card__label">{{ t('admin.live.hero.peak') }}</span>
          <span class="hero-card__value" data-testid="live-peak">{{ live.now.peak_sessions }}</span>
        </div>
        <div class="hero-card">
          <span class="hero-card__label">{{ t('admin.live.hero.questions') }}</span>
          <span class="hero-card__value" data-testid="live-questions">{{ live.tutor.questions }}</span>
          <span class="hero-card__aside">
            {{ t('admin.live.tutor.askers') }}: {{ live.tutor.askers }}
          </span>
        </div>
      </section>

      <div class="live__grid">
        <section class="panel">
          <h2 class="panel__title">{{ t('admin.live.sections.screensNow') }}</h2>
          <LiveBars :items="live.now.sections" :empty-label="emptyLabel" :label-for="sectionLabel" />
        </section>

        <section class="panel">
          <h2 class="panel__title">{{ t('admin.live.sections.demosNow') }}</h2>
          <LiveBars :items="live.now.demos" :empty-label="emptyLabel" />
        </section>

        <section class="panel panel--wide">
          <h2 class="panel__title">{{ t('admin.live.sections.activity') }}</h2>
          <LiveSparkline
            :points="live.window.activity_per_minute"
            :label="t('admin.live.activity.chart')"
            :events-label="t('admin.live.activity.events')"
            :questions-label="t('admin.live.activity.questions')"
          />
          <dl class="facts">
            <div class="facts__item">
              <dt>{{ t('admin.live.window.uniqueUsers') }}</dt>
              <dd>{{ live.window.unique_users }}</dd>
            </div>
            <div class="facts__item">
              <dt>{{ t('admin.live.window.uniqueSessions') }}</dt>
              <dd>{{ live.window.unique_sessions }}</dd>
            </div>
          </dl>
        </section>

        <section class="panel" data-testid="live-tutor">
          <h2 class="panel__title">{{ t('admin.live.sections.tutor') }}</h2>
          <dl class="facts">
            <div class="facts__item">
              <dt>{{ t('admin.live.tutor.p50') }}</dt>
              <dd>{{ formatLatency(live.tutor.latency_p50_ms) }}</dd>
            </div>
            <div class="facts__item">
              <dt>{{ t('admin.live.tutor.p95') }}</dt>
              <dd>{{ formatLatency(live.tutor.latency_p95_ms) }}</dd>
            </div>
            <div class="facts__item">
              <dt>{{ t('admin.live.tutor.degraded') }}</dt>
              <dd>{{ live.tutor.degraded }}</dd>
            </div>
          </dl>

          <div class="ratio" role="group" :aria-label="t('admin.live.tutor.ratio')">
            <div class="ratio__counts">
              <span class="ratio__up"><ThumbsUp :size="22" :stroke-width="2" aria-hidden="true" />{{ live.tutor.thumbs_up }}</span>
              <span class="ratio__down"><ThumbsDown :size="22" :stroke-width="2" aria-hidden="true" />{{ live.tutor.thumbs_down }}</span>
            </div>
            <div v-if="ratingTotal > 0" class="ratio__bar" aria-hidden="true">
              <span class="ratio__bar-up" :style="{ width: upPct + '%' }" />
            </div>
            <p v-else class="ratio__empty">{{ t('admin.live.tutor.noRatings') }}</p>
          </div>

          <h3 class="panel__subtitle">{{ t('admin.live.sections.topics') }}</h3>
          <LiveBars
            :items="topicItems"
            :empty-label="emptyLabel"
            :limit="5"
            :label-for="stripCitationBrackets"
          />
        </section>

        <section class="panel">
          <h2 class="panel__title">{{ t('admin.live.sections.screensWindow') }}</h2>
          <LiveBars :items="pageViewItems" :empty-label="emptyLabel" :label-for="screenLabel" />
        </section>

        <section class="panel">
          <h2 class="panel__title">{{ t('admin.live.sections.demosWindow') }}</h2>
          <LiveBars :items="demoWindowItems" :empty-label="emptyLabel" :label-for="demoLabel" />
        </section>

        <section class="panel">
          <h2 class="panel__title">{{ t('admin.live.sections.countries') }}</h2>
          <LiveBars :items="countryItems" :empty-label="emptyLabel" :label-for="countryLabel" />
        </section>

        <section v-if="!presentation" class="panel panel--wide" data-testid="live-recent">
          <h2 class="panel__title">{{ t('admin.live.sections.recent') }}</h2>
          <ul v-if="live.tutor.recent.length" class="recent">
            <li v-for="q in live.tutor.recent" :key="`${q.ts}-${q.question}`" class="recent__item">
              <time class="recent__time">{{ formatClock(q.ts) }}</time>
              <span class="recent__text">{{ q.question }}</span>
              <span v-if="q.degraded" class="recent__tag">{{ t('admin.live.degradedAnswer') }}</span>
              <span class="recent__rating">
                <ThumbsUp v-if="q.rating === 1" :size="18" :stroke-width="2" aria-hidden="true" />
                <ThumbsDown v-else-if="q.rating === -1" :size="18" :stroke-width="2" aria-hidden="true" />
                <span v-else class="recent__unrated">{{ t('admin.live.unrated') }}</span>
              </span>
            </li>
          </ul>
          <p v-else class="ratio__empty">{{ emptyLabel }}</p>
        </section>

        <p v-else class="live__hint" data-testid="live-presentation-hint">
          {{ t('admin.live.presentationHint') }}
        </p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.live {
  min-height: 100%;
  padding: var(--space-5);
  background: var(--surface-0);
  color: var(--text);
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  overflow-y: auto;
}

.live__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
}

.live__heading {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.live__icon { color: var(--accent); }

.live__title {
  margin: 0;
  font-size: clamp(28px, 3vw, 48px);
  line-height: 1.1;
  letter-spacing: 0.04em;
  color: var(--accent);
}

.live__subtitle {
  margin: 0;
  color: var(--text-dim);
  font-size: clamp(14px, 1.2vw, 20px);
}

.live__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}

.live__status {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: clamp(14px, 1.2vw, 20px);
  color: var(--text-dim);
  font-variant-numeric: tabular-nums;
}

.live__status--stale {
  color: var(--red-soft);
  font-weight: 700;
}

.live__segmented {
  display: inline-flex;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.live__seg,
.live__toggle {
  min-height: 44px;
  padding: 0 var(--space-4);
  font: inherit;
  font-size: clamp(14px, 1.2vw, 18px);
  color: var(--text);
  background: var(--surface-2);
  border: 1px solid transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.live__seg { border-radius: 0; border-right: 1px solid var(--border); }
.live__seg:last-child { border-right: 0; }

.live__toggle {
  border-color: var(--border-strong);
  border-radius: var(--radius-md);
}

.live__seg:hover,
.live__toggle:hover { background: var(--surface-3); }

.live__seg:focus-visible,
.live__toggle:focus-visible { outline: none; box-shadow: var(--focus-ring); }

.live__seg--active,
.live__toggle--on {
  background: var(--accent);
  color: var(--core-fg);
  font-weight: 700;
}

.live__seg--active:hover,
.live__toggle--on:hover { background: var(--accent); }

.live__placeholder {
  padding: var(--space-8) 0;
  text-align: center;
  font-size: clamp(20px, 2vw, 32px);
  color: var(--text-dim);
}

.live__hero {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-4);
}

.hero-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
  background: var(--surface-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
}

.hero-card--primary {
  border-color: var(--accent);
  box-shadow: var(--shadow-glow-amber);
}

.hero-card__label {
  font-size: clamp(14px, 1.4vw, 24px);
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.hero-card__value {
  font-size: clamp(56px, 8vw, 140px);
  line-height: 1;
  font-weight: 800;
  color: var(--accent);
  font-variant-numeric: tabular-nums;
}

.hero-card__aside {
  font-size: clamp(14px, 1.3vw, 22px);
  color: var(--text-dim);
}

.live__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
}

.panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-5);
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  min-width: 0;
}

.panel--wide { grid-column: 1 / -1; }

.panel__title {
  margin: 0;
  font-size: clamp(18px, 1.8vw, 30px);
  color: var(--text);
}

.panel__subtitle {
  margin: var(--space-2) 0 0;
  font-size: clamp(15px, 1.4vw, 22px);
  color: var(--text-dim);
}

.facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-5);
  margin: 0;
}

.facts__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.facts dt {
  font-size: clamp(13px, 1.1vw, 18px);
  color: var(--text-dim);
}

.facts dd {
  margin: 0;
  font-size: clamp(26px, 3vw, 52px);
  font-weight: 700;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}

.ratio {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ratio__counts {
  display: flex;
  gap: var(--space-5);
  font-size: clamp(22px, 2.4vw, 40px);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.ratio__up,
.ratio__down {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.ratio__up { color: var(--green); }
.ratio__down { color: var(--red-soft); }

.ratio__bar {
  height: clamp(12px, 1.2vw, 20px);
  background: var(--red-soft);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.ratio__bar-up {
  display: block;
  height: 100%;
  background: var(--green);
  transition: width var(--transition-slow);
}

.ratio__empty {
  margin: 0;
  color: var(--text-muted);
  font-size: clamp(14px, 1.2vw, 20px);
}

.recent {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.recent__item {
  display: grid;
  grid-template-columns: auto 1fr auto auto;
  align-items: baseline;
  gap: var(--space-3);
  font-size: clamp(15px, 1.4vw, 24px);
}

.recent__time {
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.recent__text { overflow-wrap: anywhere; }

.recent__tag {
  color: var(--yellow);
  font-size: 0.8em;
}

.recent__rating {
  display: inline-flex;
  align-items: center;
  color: var(--accent);
}

.recent__unrated {
  color: var(--text-muted);
  font-size: 0.8em;
}

.live__hint {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--text-muted);
  font-size: clamp(13px, 1.1vw, 18px);
  text-align: center;
}

@media (max-width: 1100px) {
  .live__hero { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 768px) {
  .live { padding: var(--space-4); }
  .live__grid { grid-template-columns: minmax(0, 1fr); }
  .live__hero { grid-template-columns: minmax(0, 1fr); }
  .recent__item { grid-template-columns: auto 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .ratio__bar-up { transition: none; }
}
</style>
