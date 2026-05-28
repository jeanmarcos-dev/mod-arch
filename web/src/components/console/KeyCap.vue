<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';

// KeyCap shows the last key sent to the simulator as a mechanical keycap that
// dips when triggered. The press animation is driven by the bumpSeq prop —
// every increment plays one dip whose duration is tied to cpu.tickIntervalMs
// so it stays in step with the rest of the narrators.

const props = defineProps<{
  /** Character of the last key sent. Empty before the first press. */
  char: string;
  /** Monotonic counter — increment to retrigger the dip animation. */
  bumpSeq: number;
}>();

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

const pressed = ref(false);
const capRef = ref<HTMLElement | null>(null);

watch(() => props.bumpSeq, () => {
  if (!prefs.narratorsEnabled) return;
  pressed.value = true;
  const dur = Math.max(120, cpu.tickIntervalMs);
  if (cpu.running) {
    // Frozen-in-the-air: while paused the cap stays down until resume,
    // matching how the other narrators behave.
    window.setTimeout(() => { pressed.value = false; }, dur);
  }
});

watch(() => cpu.running, (running) => {
  if (running && pressed.value) {
    const dur = Math.max(120, cpu.tickIntervalMs);
    window.setTimeout(() => { pressed.value = false; }, dur);
  }
});

const display = computed(() => {
  if (props.char === '') return t('panels.console.keyCap.idle');
  if (props.char === '\n') return '⏎';
  if (props.char === ' ') return '␣';
  if (props.char === '\t') return '⇥';
  return props.char;
});

const hex = computed(() => {
  if (props.char === '') return '';
  const code = props.char.charCodeAt(0);
  return '0x' + code.toString(16).toUpperCase().padStart(2, '0');
});

const aria = computed(() => {
  if (props.char === '') return '';
  return t('panels.console.keyCap.ariaPressed', { char: display.value, hex: hex.value });
});
</script>

<template>
  <div
    ref="capRef"
    class="key-cap"
    :class="{ 'is-pressed': pressed, 'is-idle': char === '' }"
    role="img"
    :aria-label="aria"
  >
    <span class="key-cap__char">{{ display }}</span>
  </div>
</template>

<style scoped>
.key-cap {
  --cap-bg: var(--surface-2, #1a1f1a);
  --cap-fg: var(--green);
  width: 36px;
  height: 36px;
  border-radius: 0;
  background: var(--cap-bg);
  border: 1px solid var(--terminal-border);
  box-shadow:
    0 1px 0 0 rgba(0, 0, 0, 0.4),
    0 2px 6px -2px rgba(0, 0, 0, 0.5),
    inset 0 1px 0 0 rgba(255, 255, 255, 0.06);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--cap-fg);
  font-family: 'VT323', monospace;
  transition:
    transform 100ms ease-out,
    box-shadow 100ms ease-out;
  position: relative;
  flex-shrink: 0;
}

.key-cap.is-idle {
  --cap-fg: var(--text-muted);
  box-shadow:
    0 1px 0 0 rgba(0, 0, 0, 0.3),
    inset 0 1px 0 0 rgba(255, 255, 255, 0.04);
}

.key-cap.is-pressed {
  transform: translateY(2px);
  box-shadow:
    0 0 0 0 rgba(0, 0, 0, 0.4),
    inset 0 2px 4px 0 rgba(0, 0, 0, 0.5),
    0 0 8px 0 color-mix(in oklab, var(--cap-fg) 50%, transparent);
}

.key-cap__char {
  font-size: 22px;
  line-height: 1;
  text-shadow: 0 0 6px color-mix(in oklab, var(--cap-fg) 40%, transparent);
}

.key-cap.is-idle .key-cap__char {
  text-shadow: none;
}

@media (max-width: 768px) {
  .key-cap {
    width: 40px;
    height: 40px;
  }

  .key-cap__char {
    font-size: 24px;
  }
}
</style>
