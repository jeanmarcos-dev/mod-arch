<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useCpuStore } from '../../stores/cpu';

// SpeedSlider is the TopBar's primary speed control. It manipulates the
// wall-clock multiplier (speedMul ∈ [0.1, 5]) while clockHz stays fixed in
// Settings. A continuous range input lets the user reach any value; magnetic
// snap-points at [0.25, 0.5, 1, 2, 4] make the common YouTube-style presets
// effortless to land on. A read-only line under the control surfaces the
// derived values (clock Hz, effective Hz, tick ms) so students can see what
// each position means without doing the math.

const cpu = useCpuStore();

interface Props {
  viewer?: boolean;
}
const props = defineProps<Props>();

const PRESETS = [0.25, 0.5, 1.0, 2.0, 4.0];
const SNAP_RADIUS = 0.05;
const MIN = 0.1;
const MAX = 5.0;
const STEP = 0.05;
const DEBOUNCE_MS = 120;

const local = ref(cpu.speedMul);
watch(() => cpu.speedMul, (v) => { local.value = v; });

let pendingTimer: number | null = null;

function snap(raw: number): number {
  for (const p of PRESETS) {
    if (Math.abs(raw - p) <= SNAP_RADIUS) return p;
  }
  return Math.round(raw * 100) / 100;
}

function flush(value: number) {
  if (pendingTimer !== null) window.clearTimeout(pendingTimer);
  pendingTimer = window.setTimeout(() => {
    pendingTimer = null;
    cpu.setSpeedMul(value);
  }, DEBOUNCE_MS);
}

function onInput(e: Event) {
  if (props.viewer) return;
  const raw = Number((e.target as HTMLInputElement).value);
  if (!Number.isFinite(raw)) return;
  const snapped = snap(raw);
  local.value = snapped;
  flush(snapped);
}

function selectPreset(value: number) {
  if (props.viewer) return;
  local.value = value;
  if (pendingTimer !== null) window.clearTimeout(pendingTimer);
  pendingTimer = null;
  cpu.setSpeedMul(value);
}

const displayMul = computed(() => local.value.toFixed(2));
const effectiveHz = computed(() => (cpu.clockHz * local.value).toFixed(2));
const tickMs = computed(() => {
  const eff = cpu.clockHz * local.value;
  return eff > 0 ? Math.round(1000 / eff) : 0;
});

const trackFillPct = computed(() => {
  const ratio = (local.value - MIN) / (MAX - MIN);
  return `${Math.max(0, Math.min(100, ratio * 100))}%`;
});

function isActive(p: number): boolean {
  return Math.abs(local.value - p) < SNAP_RADIUS / 2;
}
</script>

<template>
  <div class="speed-slider" :class="{ 'speed-slider--viewer': viewer }">
    <div class="speed-slider__row">
      <span class="speed-slider__label">SPEED</span>
      <span class="speed-slider__value">{{ displayMul }}x</span>
    </div>

    <div class="speed-slider__bar">
      <div class="speed-slider__track">
        <div class="speed-slider__fill" :style="{ width: trackFillPct }"></div>
      </div>
      <input
        type="range"
        :min="MIN"
        :max="MAX"
        :step="STEP"
        :value="local"
        :disabled="viewer"
        class="speed-slider__input"
        aria-label="Multiplicador de velocidad"
        @input="onInput"
      />
    </div>

    <div class="speed-slider__presets">
      <button
        v-for="p in PRESETS"
        :key="p"
        type="button"
        class="speed-slider__preset"
        :class="{ 'speed-slider__preset--active': isActive(p) }"
        :disabled="viewer"
        :title="`${p}x`"
        @click="selectPreset(p)"
      >{{ p === 1 ? '1×' : `${p}x` }}</button>
    </div>

    <div class="speed-slider__readout">
      <span>clock {{ cpu.clockHz }} Hz</span>
      <span class="speed-slider__sep">·</span>
      <span>efectivo {{ effectiveHz }} Hz</span>
      <span class="speed-slider__sep">·</span>
      <span>tick {{ tickMs }}ms</span>
    </div>
  </div>
</template>

<style scoped>
.speed-slider {
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 6px);
  width: 100%;
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.74rem;
  color: var(--color-fg);
}

.speed-slider__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
  letter-spacing: 0.08em;
  line-height: 1;
}

.speed-slider__label {
  font-weight: 600;
  font-size: 0.7rem;
  letter-spacing: 0.08em;
  opacity: 0.7;
}

.speed-slider__value {
  font-weight: 700;
  font-size: 0.9rem;
  color: var(--color-accent, #ffae42);
  font-variant-numeric: tabular-nums;
}

.speed-slider__bar {
  position: relative;
  height: 14px;
}

.speed-slider__track {
  position: absolute;
  top: 5px;
  left: 0;
  right: 0;
  height: 4px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--color-fg) 16%, transparent);
  pointer-events: none;
}

.speed-slider__fill {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--color-accent, #ffae42) 55%, transparent),
    var(--color-accent, #ffae42)
  );
  border-radius: 2px;
  transition: width 80ms linear;
}

.speed-slider__input {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  width: 100%;
  height: 14px;
  margin: 0;
  background: transparent;
  appearance: none;
  -webkit-appearance: none;
  cursor: pointer;
}

.speed-slider__input:disabled { cursor: not-allowed; }

.speed-slider__input::-webkit-slider-thumb {
  appearance: none;
  -webkit-appearance: none;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--color-accent, #ffae42);
  border: 2px solid var(--color-bg, #0d0d0d);
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-accent, #ffae42) 50%, transparent);
  cursor: grab;
}
.speed-slider__input::-moz-range-thumb {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--color-accent, #ffae42);
  border: 2px solid var(--color-bg, #0d0d0d);
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-accent, #ffae42) 50%, transparent);
  cursor: grab;
}
.speed-slider__input:active::-webkit-slider-thumb { cursor: grabbing; }
.speed-slider__input:active::-moz-range-thumb { cursor: grabbing; }

.speed-slider__presets {
  display: flex;
  gap: 3px;
  justify-content: space-between;
}

.speed-slider__preset {
  flex: 1;
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--color-fg) 22%, transparent);
  color: color-mix(in srgb, var(--color-fg) 72%, transparent);
  font-family: inherit;
  font-size: 0.66rem;
  letter-spacing: 0.02em;
  padding: 3px 0;
  border-radius: 3px;
  cursor: pointer;
  font-variant-numeric: tabular-nums;
  line-height: 1;
  transition: color 100ms, border-color 100ms, background 100ms;
}

.speed-slider__preset:hover:not(:disabled) {
  color: var(--color-fg);
  border-color: var(--color-accent, #ffae42);
}

.speed-slider__preset--active {
  background: var(--color-accent, #ffae42);
  border-color: var(--color-accent, #ffae42);
  color: var(--color-bg, #0d0d0d);
}

.speed-slider__preset:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.speed-slider__readout {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  font-size: 0.62rem;
  opacity: 0.6;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
  line-height: 1.2;
}

.speed-slider__sep { opacity: 0.6; }

.speed-slider--viewer { opacity: 0.65; }
</style>
