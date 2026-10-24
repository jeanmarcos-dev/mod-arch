<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  points: Array<{ minute: number; events: number; questions: number }>;
  label: string;
  eventsLabel: string;
  questionsLabel: string;
}>();

const { t } = useI18n();

const WIDTH = 600;
const HEIGHT = 140;
const PAD = 6;

const peak = computed(() =>
  Math.max(1, ...props.points.map((p) => Math.max(p.events, p.questions))),
);

function coordinates(pick: (p: { events: number; questions: number }) => number) {
  const n = props.points.length;
  if (n === 0) return [];
  const step = n > 1 ? (WIDTH - PAD * 2) / (n - 1) : 0;
  return props.points.map((p, i) => ({
    x: n > 1 ? PAD + i * step : WIDTH / 2,
    y: HEIGHT - PAD - (pick(p) / peak.value) * (HEIGHT - PAD * 2),
  }));
}

function toPath(coords: Array<{ x: number; y: number }>): string {
  return coords.map((c, i) => `${i === 0 ? 'M' : 'L'}${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(' ');
}

const eventsCoords = computed(() => coordinates((p) => p.events));
const questionsCoords = computed(() => coordinates((p) => p.questions));
const eventsPath = computed(() => toPath(eventsCoords.value));
const questionsPath = computed(() => toPath(questionsCoords.value));
const areaPath = computed(() => {
  const c = eventsCoords.value;
  if (c.length < 2) return '';
  return `${toPath(c)} L${c[c.length - 1].x.toFixed(1)},${HEIGHT - PAD} L${c[0].x.toFixed(1)},${HEIGHT - PAD} Z`;
});
</script>

<template>
  <figure class="spark">
    <svg
      class="spark__svg"
      :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
      preserveAspectRatio="none"
      role="img"
      :aria-label="label"
    >
      <line class="spark__axis" :x1="PAD" :y1="HEIGHT - PAD" :x2="WIDTH - PAD" :y2="HEIGHT - PAD" />
      <path v-if="areaPath" class="spark__area" :d="areaPath" />
      <path v-if="eventsPath" class="spark__line spark__line--events" :d="eventsPath" />
      <path v-if="questionsPath" class="spark__line spark__line--questions" :d="questionsPath" />
    </svg>
    <figcaption class="spark__legend">
      <span class="spark__key spark__key--events">{{ eventsLabel }}</span>
      <span class="spark__key spark__key--questions">{{ questionsLabel }}</span>
      <span class="spark__peak">{{ t('admin.live.activity.peak', { n: peak }) }}</span>
    </figcaption>
  </figure>
</template>

<style scoped>
.spark {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.spark__svg {
  width: 100%;
  height: clamp(110px, 14vh, 200px);
  display: block;
}

.spark__axis {
  stroke: var(--border-strong);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.spark__area {
  fill: rgba(var(--accent-rgb), 0.16);
}

.spark__line {
  fill: none;
  stroke-width: 3;
  stroke-linejoin: round;
  stroke-linecap: round;
  vector-effect: non-scaling-stroke;
}

.spark__line--events { stroke: var(--accent); }
.spark__line--questions { stroke: var(--cyan); }

.spark__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  font-size: clamp(13px, 1.1vw, 18px);
  color: var(--text-dim);
}

.spark__key::before {
  content: '';
  display: inline-block;
  width: 0.8em;
  height: 0.8em;
  margin-right: 0.4em;
  border-radius: var(--radius-sm);
  vertical-align: baseline;
}

.spark__key--events::before { background: var(--accent); }
.spark__key--questions::before { background: var(--cyan); }

.spark__peak {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
}
</style>
