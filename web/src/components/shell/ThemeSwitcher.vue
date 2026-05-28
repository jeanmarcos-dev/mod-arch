<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { usePreferencesStore, type Theme } from '../../stores/preferences';

const { t } = useI18n();
const prefs = usePreferencesStore();

interface ThemeOption {
  id: Theme;
  labelKey: 'preferences.theme.amberLabel' | 'preferences.theme.greenLabel' | 'preferences.theme.darkLabel' | 'preferences.theme.accesibleLabel';
  swatch: string;
}

const themes: ThemeOption[] = [
  { id: 'amber',     labelKey: 'preferences.theme.amberLabel',     swatch: '#ffb547' },
  { id: 'green',     labelKey: 'preferences.theme.greenLabel',     swatch: '#7fff95' },
  { id: 'dark',      labelKey: 'preferences.theme.darkLabel',      swatch: '#58a6ff' },
  { id: 'accesible', labelKey: 'preferences.theme.accesibleLabel', swatch: '#0049b0' },
];

const ariaGroup = computed(() => t('shell.themeSwitcher.aria'));
</script>

<template>
  <div class="theme-switcher" role="group" :aria-label="ariaGroup">
    <button
      v-for="opt in themes"
      :key="opt.id"
      type="button"
      class="theme-switcher__swatch"
      :class="{ 'theme-switcher__swatch--on': prefs.theme === opt.id }"
      :style="{ '--swatch': opt.swatch }"
      :title="t(opt.labelKey)"
      :aria-label="t(opt.labelKey)"
      :aria-pressed="prefs.theme === opt.id"
      @click="prefs.setTheme(opt.id)"
    />
  </div>
</template>

<style scoped>
.theme-switcher {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
  height: 32px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(var(--accent-rgb), 0.05);
  box-sizing: border-box;
}
.theme-switcher__swatch {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.18);
  background: var(--swatch);
  cursor: pointer;
  padding: 0;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
  position: relative;
}
.theme-switcher__swatch:hover {
  transform: scale(1.15);
}
.theme-switcher__swatch--on {
  box-shadow: 0 0 0 2px var(--surface-1), 0 0 0 3px var(--swatch);
}
.theme-switcher__swatch:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 3px;
}
</style>
