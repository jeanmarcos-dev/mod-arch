<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRaceDetector, type UseRaceDetectorReturn, type OpenWindow } from '../../composables/useRaceDetector';
import { pidColor } from '../../lib/pidColors';

// An external detector lets the parent view share one instance across
// several panels; standalone usage falls back to a private one.
const props = defineProps<{
  detector?: UseRaceDetectorReturn;
}>();

const { t } = useI18n();
// props.detector is read once at setup: swapping the prop later is not
// supported (the subscription would have to be rebuilt).
const internal = props.detector ? null : useRaceDetector();
const det = props.detector ?? internal!;
const state = det.state;

const tracked = computed(() => state.trackedAddr !== null);
const addrHex = computed(() =>
  state.trackedAddr === null ? '' : '0x' + state.trackedAddr.toString(16).toUpperCase(),
);
const cellLabel = computed(() => state.trackedName ?? t('sections.concurrency.raceLens.cellFallback'));
const varSymbol = computed(() => state.trackedName ?? 'n');

const windows = computed<OpenWindow[]>(() =>
  Object.values(state.openWindows).sort((a, b) => a.pid - b.pid),
);
// Side is pinned by PID parity so a closing window never flips the
// surviving chip to the other column.
const leftWindows = computed(() => windows.value.filter(w => w.pid % 2 === 1));
const rightWindows = computed(() => windows.value.filter(w => w.pid % 2 === 0));

function ticksOpen(w: OpenWindow): number {
  return Math.max(0, state.currentTick - w.sinceTick);
}

const recentLost = computed(() => state.lostUpdates.slice(-5).reverse());

// Stale-store effects: a brief red flash on the cell plus the overwritten
// value falling away crossed-out. Both are keyed by detection seq so the
// element is replaced (and its animation restarted) on back-to-back losses;
// the watcher fires once per reactive flush, so one fall per flush.
const falling = ref<{ seq: number; value: number }[]>([]);
const flashSeq = ref(0);
let flashTimer: ReturnType<typeof setTimeout> | undefined;
const fallTimers = new Set<ReturnType<typeof setTimeout>>();

watch(
  () => (state.lostUpdates.length ? state.lostUpdates[state.lostUpdates.length - 1].seq : 0),
  (seqNow) => {
    if (!seqNow) return;
    const last = state.lostUpdates[state.lostUpdates.length - 1];
    falling.value.push({ seq: last.seq, value: last.lostValue });
    const fallTimer = setTimeout(() => {
      fallTimers.delete(fallTimer);
      falling.value = falling.value.filter(f => f.seq !== last.seq);
    }, 700);
    fallTimers.add(fallTimer);
    flashSeq.value = last.seq;
    clearTimeout(flashTimer);
    flashTimer = setTimeout(() => { flashSeq.value = 0; }, 650);
  },
);

onBeforeUnmount(() => {
  clearTimeout(flashTimer);
  for (const id of fallTimers) clearTimeout(id);
  fallTimers.clear();
});
</script>

<template>
  <div class="rl">
    <div v-if="!tracked" class="rl__empty">
      {{ t('sections.concurrency.raceLens.empty') }}
    </div>

    <template v-else>
      <div class="rl__score">
        <span class="rl__stat">
          <span class="rl__stat-label">{{ t('sections.concurrency.raceLens.actual') }}</span>
          <span class="rl__stat-value">{{ state.cellValue ?? '—' }}</span>
        </span>
        <span class="rl__sep" aria-hidden="true">·</span>
        <span class="rl__stat" :class="state.lostTotal > 0 ? 'rl__stat--bad' : 'rl__stat--good'">
          <template v-if="state.lostTotal > 0">
            <span class="rl__stat-label">{{ t('sections.concurrency.raceLens.lost') }}</span>
            <span class="rl__stat-value">{{ state.lostTotal }}</span>
          </template>
          <template v-else>
            <span class="rl__stat-label">{{ t('sections.concurrency.raceLens.lostZero') }}</span>
          </template>
        </span>
        <span class="rl__sep" aria-hidden="true">·</span>
        <span class="rl__stat">
          <span class="rl__stat-label">{{ t('sections.concurrency.raceLens.expected') }}</span>
          <span class="rl__stat-value">{{ det.expectedFinal.value ?? '—' }}</span>
        </span>
      </div>

      <div class="rl__stage">
        <TransitionGroup tag="div" name="rlchip" class="rl__side rl__side--l">
          <div
            v-for="w in leftWindows"
            :key="w.pid"
            class="rl__chip"
            :style="{ borderColor: pidColor(w.pid) }"
          >
            <span class="rl__chip-pid" :style="{ color: pidColor(w.pid) }">P{{ w.pid }}</span>
            <span class="rl__chip-copy">{{ t('sections.concurrency.raceLens.chipCopy') }} {{ varSymbol }}={{ w.value }}</span>
            <span class="rl__chip-ticks">{{ t('sections.concurrency.raceLens.ticksOpen', { n: ticksOpen(w) }) }}</span>
            <span v-if="w.preempted" class="rl__chip-badge">{{ t('sections.concurrency.raceLens.preempted') }}</span>
          </div>
        </TransitionGroup>

        <div class="rl__cell">
          <span class="rl__cell-label">{{ cellLabel }}</span>
          <span class="rl__cell-value">{{ state.cellValue ?? '—' }}</span>
          <span class="rl__cell-addr">{{ addrHex }}</span>
          <span v-if="flashSeq" :key="flashSeq" class="rl__flash" aria-hidden="true"></span>
          <span v-for="f in falling" :key="f.seq" class="rl__fall" aria-hidden="true">{{ f.value }}</span>
        </div>

        <TransitionGroup tag="div" name="rlchip" class="rl__side rl__side--r">
          <div
            v-for="w in rightWindows"
            :key="w.pid"
            class="rl__chip"
            :style="{ borderColor: pidColor(w.pid) }"
          >
            <span class="rl__chip-pid" :style="{ color: pidColor(w.pid) }">P{{ w.pid }}</span>
            <span class="rl__chip-copy">{{ t('sections.concurrency.raceLens.chipCopy') }} {{ varSymbol }}={{ w.value }}</span>
            <span class="rl__chip-ticks">{{ t('sections.concurrency.raceLens.ticksOpen', { n: ticksOpen(w) }) }}</span>
            <span v-if="w.preempted" class="rl__chip-badge">{{ t('sections.concurrency.raceLens.preempted') }}</span>
          </div>
        </TransitionGroup>
      </div>

      <div v-if="recentLost.length" class="rl__lost">
        <span class="rl__lost-title">{{ t('sections.concurrency.raceLens.lostListTitle') }}</span>
        <ul class="rl__lost-list">
          <li v-for="lu in recentLost" :key="lu.seq" class="rl__lost-item">
            {{ t('sections.concurrency.raceLens.lostEntry', { pid: lu.clobberPid, value: lu.newValue, over: lu.lostValue, n: lu.lostCount }) }}
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>

<style scoped>
.rl {
  width: 100%;
}

.rl__empty {
  color: var(--text-dim);
  font-style: italic;
  font-size: 12px;
  padding: var(--space-3);
  text-align: center;
}

.rl__score {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: center;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  font-family: var(--font-display);
  font-size: 11px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-dim);
}

.rl__stat {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
}

.rl__stat-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  color: var(--text);
}

.rl__stat--bad .rl__stat-label,
.rl__stat--bad .rl__stat-value {
  color: var(--red);
  text-shadow: 0 0 6px rgba(255, 58, 58, 0.45);
}

.rl__stat--good .rl__stat-label {
  color: var(--green);
}

.rl__sep {
  color: var(--text-muted);
}

.rl__stage {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: var(--space-3);
  min-height: 96px;
}

.rl__side {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.rl__side--l {
  align-items: flex-end;
}

.rl__side--r {
  align-items: flex-start;
}

.rl__cell {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  min-width: 110px;
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}

/* Overlay covering the cell border; keyed by loss seq so each loss mounts a
   fresh element and the animation restarts even on back-to-back losses. */
.rl__flash {
  position: absolute;
  inset: -1px;
  border: 1px solid var(--red);
  border-radius: var(--radius-sm);
  pointer-events: none;
  animation: rl-flash 0.6s ease-out forwards;
}

@keyframes rl-flash {
  0% {
    opacity: 1;
    box-shadow: 0 0 16px rgba(255, 58, 58, 0.7);
    background: rgba(255, 58, 58, 0.18);
  }
  100% {
    opacity: 0;
    box-shadow: none;
    background: transparent;
  }
}

.rl__cell-label {
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 0.5px;
}

.rl__cell-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 24px;
  color: var(--amber);
  text-shadow: 0 0 8px rgba(var(--accent-rgb), 0.4);
}

.rl__cell-addr {
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  color: var(--text-muted);
}

.rl__fall {
  position: absolute;
  left: 50%;
  top: 50%;
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  color: var(--red);
  text-decoration: line-through;
  pointer-events: none;
  animation: rl-fall 0.6s ease-in forwards;
}

@keyframes rl-fall {
  from {
    opacity: 1;
    transform: translate(-50%, -50%);
  }
  to {
    opacity: 0;
    transform: translate(-50%, 36px);
  }
}

.rl__chip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 8px;
  max-width: 100%;
  padding: 4px 8px;
  border: 1px dashed var(--amber);
  border-radius: var(--radius-sm);
  background: rgba(var(--accent-rgb), 0.06);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
  animation: rl-pulse 1.2s ease-in-out infinite;
}

@keyframes rl-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0);
  }
  50% {
    box-shadow: 0 0 7px 1px rgba(var(--accent-rgb), 0.35);
  }
}

.rl__chip-pid {
  font-weight: 700;
}

.rl__chip-ticks {
  color: var(--text-muted);
  font-size: 9px;
}

.rl__chip-badge {
  padding: 1px 4px;
  border: 1px solid var(--amber);
  border-radius: var(--radius-sm);
  color: var(--amber);
  font-size: 8px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.rlchip-enter-active {
  transition: transform 0.25s ease-out, opacity 0.25s ease-out;
}

.rlchip-leave-active {
  transition: transform 0.2s ease-in, opacity 0.2s ease-in;
}

.rlchip-enter-from {
  opacity: 0;
}

/* Chips fly out of the central cell toward their side. */
.rl__side--l .rlchip-enter-from {
  transform: translateX(36px);
}

.rl__side--r .rlchip-enter-from {
  transform: translateX(-36px);
}

.rlchip-leave-to {
  opacity: 0;
  transform: scale(0.9);
}

.rl__lost {
  margin-top: var(--space-3);
}

.rl__lost-title {
  display: block;
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  margin-bottom: var(--space-1);
}

.rl__lost-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.rl__lost-item {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
}

.rl__lost-item:first-child {
  color: var(--red-soft);
}
</style>
