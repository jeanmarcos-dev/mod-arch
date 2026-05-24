<script setup lang="ts">
import { usePreferencesStore, type Theme } from '../../stores/preferences';

const prefs = usePreferencesStore();

interface ThemeOption {
  id: Theme;
  label: string;
  swatch: string;
}

const themes: ThemeOption[] = [
  { id: 'amber',     label: 'CRT ámbar',       swatch: '#ffb547' },
  { id: 'green',     label: 'CRT verde',       swatch: '#7fff95' },
  { id: 'dark',      label: 'Oscuro estándar', swatch: '#58a6ff' },
  { id: 'accesible', label: 'Claro accesible', swatch: '#0049b0' },
];
</script>

<template>
  <div class="theme-switcher" role="group" aria-label="Cambiar tema visual">
    <button
      v-for="t in themes"
      :key="t.id"
      type="button"
      class="theme-switcher__swatch"
      :class="{ 'theme-switcher__swatch--on': prefs.theme === t.id }"
      :style="{ '--swatch': t.swatch }"
      :title="t.label"
      :aria-label="t.label"
      :aria-pressed="prefs.theme === t.id"
      @click="prefs.setTheme(t.id)"
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
