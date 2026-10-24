<template>
  <div class="a11y-menu">
    <button
      id="a11y-menu"
      ref="triggerRef"
      type="button"
      class="a11y-menu__trigger"
      :aria-label="t('a11y.panelTitle')"
      :aria-expanded="open"
      aria-haspopup="dialog"
      @click="toggle"
    >
      <Accessibility :size="20" aria-hidden="true" />
    </button>

    <div
      v-if="open"
      class="a11y-menu__panel"
      role="dialog"
      :aria-label="t('a11y.panelTitle')"
      @keydown.esc.stop.prevent="close"
    >
      <p class="a11y-menu__title">{{ t('a11y.panelTitle') }}</p>
      <p class="a11y-menu__hint">{{ t('a11y.panelHint') }}</p>

      <label class="a11y-menu__row">
        <span>{{ t('preferences.theme.accesibleLabel') }}</span>
        <button
          type="button"
          class="a11y-menu__switch"
          role="switch"
          :aria-checked="prefs.theme === 'accesible'"
          @click="toggleAccessibleTheme"
        >{{ prefs.theme === 'accesible' ? t('a11y.on') : t('a11y.off') }}</button>
      </label>

      <label class="a11y-menu__row">
        <span>{{ t('a11y.legibleFont') }}</span>
        <button
          type="button"
          class="a11y-menu__switch"
          role="switch"
          :aria-checked="prefs.legibleFont"
          @click="prefs.setLegibleFont(!prefs.legibleFont)"
        >{{ prefs.legibleFont ? t('a11y.on') : t('a11y.off') }}</button>
      </label>

      <div class="a11y-menu__row a11y-menu__row--stack">
        <span id="a11y-textscale-label">{{ t('a11y.textScale') }}</span>
        <div class="a11y-menu__seg" role="radiogroup" aria-labelledby="a11y-textscale-label">
          <button
            v-for="opt in textScaleOptions"
            :key="opt.value"
            type="button"
            role="radio"
            :aria-checked="prefs.textScale === opt.value"
            :class="['a11y-menu__seg-btn', { 'is-active': prefs.textScale === opt.value }]"
            @click="prefs.setTextScale(opt.value)"
          >{{ opt.label }}</button>
        </div>
      </div>

      <label class="a11y-menu__row">
        <span>{{ t('a11y.screenReaderAnnounce') }}</span>
        <button
          type="button"
          class="a11y-menu__switch"
          role="switch"
          :aria-checked="prefs.screenReaderAnnounce"
          @click="prefs.setScreenReaderAnnounce(!prefs.screenReaderAnnounce)"
        >{{ prefs.screenReaderAnnounce ? t('a11y.on') : t('a11y.off') }}</button>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Accessibility } from '@lucide/vue';
import { usePreferencesStore, type TextScale } from '../../stores/preferences';

const { t } = useI18n();
const prefs = usePreferencesStore();

const open = ref(false);
const triggerRef = ref<HTMLButtonElement | null>(null);

const textScaleOptions = computed<{ value: TextScale; label: string }[]>(() => [
  { value: 'normal', label: t('a11y.textScaleNormal') },
  { value: 'large', label: t('a11y.textScaleLarge') },
  { value: 'xlarge', label: t('a11y.textScaleXlarge') },
]);

let lastTheme: typeof prefs.theme = 'amber';

function toggleAccessibleTheme() {
  if (prefs.theme === 'accesible') {
    prefs.setTheme(lastTheme === 'accesible' ? 'amber' : lastTheme);
  } else {
    lastTheme = prefs.theme;
    prefs.setTheme('accesible');
  }
}

function toggle() {
  open.value = !open.value;
}

function close() {
  open.value = false;
  triggerRef.value?.focus();
}

function onDocClick(ev: MouseEvent) {
  if (!open.value) return;
  const root = triggerRef.value?.closest('.a11y-menu');
  if (root && ev.target instanceof Node && !root.contains(ev.target)) open.value = false;
}

onMounted(() => document.addEventListener('click', onDocClick));
onBeforeUnmount(() => document.removeEventListener('click', onDocClick));
</script>

<style scoped>
.a11y-menu {
  position: fixed;
  left: 16px;
  bottom: 16px;
  z-index: 900;
}

.a11y-menu__trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 50%;
  border: 1px solid var(--border, #334);
  background: var(--surface-1, #12151c);
  color: var(--text, #cfd6e4);
  cursor: pointer;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.3);
}

.a11y-menu__trigger:focus-visible {
  outline: 2px solid var(--accent, #0049b0);
  outline-offset: 2px;
}

.a11y-menu__panel {
  position: absolute;
  left: 0;
  bottom: 52px;
  width: min(320px, calc(100vw - 32px));
  max-height: calc(100vh - 90px);
  overflow-y: auto;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--border, #334);
  background: var(--surface-1, #12151c);
  color: var(--text, #cfd6e4);
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.4);
  font-family: var(--font-body);
}

.a11y-menu__title {
  margin: 0 0 2px;
  font-weight: 700;
  font-size: 0.95rem;
}

.a11y-menu__hint {
  margin: 0 0 12px;
  font-size: 0.78rem;
  color: var(--text-muted, #8b93a7);
}

.a11y-menu__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 0;
  font-size: 0.86rem;
}

.a11y-menu__row > span {
  flex: 1 1 auto;
  min-width: 0;
}

.a11y-menu__row--stack {
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
}

.a11y-menu__switch,
.a11y-menu__seg-btn {
  text-transform: none;
  letter-spacing: normal;
  min-height: 0;
  line-height: 1.2;
}

.a11y-menu__switch {
  flex: 0 0 auto;
  white-space: nowrap;
  min-width: 96px;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid var(--border, #334);
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-family: var(--font-code);
  font-size: 0.78rem;
}

.a11y-menu__switch[aria-checked="true"] {
  background: var(--accent, #0049b0);
  border-color: var(--accent, #0049b0);
  color: #fff;
}

.a11y-menu__seg {
  display: flex;
  gap: 4px;
}

.a11y-menu__seg-btn {
  flex: 1;
  padding: 6px 4px;
  border-radius: 8px;
  border: 1px solid var(--border, #334);
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 0.78rem;
}

.a11y-menu__seg-btn.is-active {
  background: var(--accent, #0049b0);
  border-color: var(--accent, #0049b0);
  color: #fff;
}

.a11y-menu__switch:focus-visible,
.a11y-menu__seg-btn:focus-visible {
  outline: 2px solid var(--accent, #0049b0);
  outline-offset: 2px;
}
</style>
