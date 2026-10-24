<template>
  <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ message }}</div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { usePreferencesStore } from '../../stores/preferences';
import { useNarrators } from '../../lib/narratorBus';

const { t } = useI18n();
const prefs = usePreferencesStore();
const message = ref('');

const MIN_INTERVAL_MS = 650;
let lastEmit = 0;
let trailing: ReturnType<typeof setTimeout> | null = null;
let pending = '';

function now(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now();
}

function emit(text: string) {
  if (!prefs.screenReaderAnnounce) return;
  pending = text;
  const wait = MIN_INTERVAL_MS - (now() - lastEmit);
  if (wait <= 0) {
    lastEmit = now();
    message.value = pending;
    return;
  }
  if (trailing === null) {
    trailing = setTimeout(() => {
      trailing = null;
      lastEmit = now();
      message.value = pending;
    }, wait);
  }
}

function hex(n: number): string {
  return '0x' + n.toString(16).toUpperCase();
}

const narrators = useNarrators({ force: true });

narrators.onModeSwitch((m) =>
  emit(t('a11y.announce.modeSwitch', { from: t(`a11y.mode.${m.from}`), to: t(`a11y.mode.${m.to}`) })),
);
narrators.onIrqFire((m) =>
  emit(t('a11y.announce.irq', { vector: hex(m.vector), source: m.source })),
);
narrators.onCtxSwitch((m) =>
  emit(t('a11y.announce.ctxSwitch', { from: m.fromPid, to: m.toPid })),
);

onBeforeUnmount(() => {
  if (trailing !== null) clearTimeout(trailing);
});
</script>
