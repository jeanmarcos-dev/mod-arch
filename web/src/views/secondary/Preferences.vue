<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import Card from '../../components/ui/Card.vue';
import { usePreferencesStore, type Theme, type Locale } from '../../stores/preferences';

const prefs = usePreferencesStore();
const { t } = useI18n();

interface ThemeOption {
  id: Theme;
  labelKey: 'amberLabel' | 'greenLabel' | 'darkLabel' | 'accesibleLabel';
  descKey: 'amberDesc' | 'greenDesc' | 'darkDesc' | 'accesibleDesc';
  swatch: string;
}

const themes: ThemeOption[] = [
  { id: 'amber', labelKey: 'amberLabel', descKey: 'amberDesc', swatch: '#ffb547' },
  { id: 'green', labelKey: 'greenLabel', descKey: 'greenDesc', swatch: '#7fff95' },
  { id: 'dark', labelKey: 'darkLabel', descKey: 'darkDesc', swatch: '#58a6ff' },
  { id: 'accesible', labelKey: 'accesibleLabel', descKey: 'accesibleDesc', swatch: '#0049b0' },
];

const locales: { id: Locale; labelKey: 'es' | 'en' | 'pt' }[] = [
  { id: 'es', labelKey: 'es' },
  { id: 'en', labelKey: 'en' },
  { id: 'pt', labelKey: 'pt' },
];
</script>

<template>
  <div class="prefs">
    <Card :title="t('preferences.language.title')" :elevation="1" class="prefs__card">
      <p class="prefs__hint">{{ t('preferences.language.hint') }}</p>
      <div class="prefs__lang-switch" role="radiogroup" :aria-label="t('preferences.language.title')">
        <button
          v-for="l in locales"
          :key="l.id"
          type="button"
          role="radio"
          :aria-checked="prefs.locale === l.id"
          class="prefs__lang-pick"
          :class="{ 'prefs__lang-pick--active': prefs.locale === l.id }"
          @click="prefs.setLocale(l.id)"
        >{{ t(`preferences.language.${l.labelKey}`) }}</button>
      </div>
    </Card>

    <Card :title="t('preferences.theme.title')" :elevation="1" class="prefs__card">
      <p class="prefs__hint">{{ t('preferences.theme.hint') }}</p>
      <div class="prefs__options">
        <button
          v-for="th in themes"
          :key="th.id"
          type="button"
          class="prefs__option prefs__option--theme"
          :class="{ 'prefs__option--on': prefs.theme === th.id }"
          @click="prefs.setTheme(th.id)"
        >
          <span class="prefs__theme-row">
            <span class="prefs__theme-swatch" :style="{ background: th.swatch }" aria-hidden="true"></span>
            <span class="prefs__option-label">{{ t(`preferences.theme.${th.labelKey}`) }}</span>
          </span>
          <span class="prefs__option-desc">{{ t(`preferences.theme.${th.descKey}`) }}</span>
        </button>
      </div>
    </Card>

  </div>
</template>

<style scoped>
.prefs {
  flex: 1;
  padding: var(--space-5);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 880px;
  width: 100%;
  margin: 0 auto;
}

.prefs__card { width: 100%; }

.prefs__hint {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
  margin-bottom: var(--space-3);
}

.prefs__options {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
}

.prefs__options--row {
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.prefs__option {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
  padding: var(--space-3);
  text-align: left;
  font-family: inherit;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  border-radius: var(--radius-sm);
  transition:
    background var(--transition-fast),
    border-color var(--transition-fast),
    box-shadow var(--transition-fast),
    transform var(--transition-fast);
}

.prefs__option:hover {
  background: rgba(var(--accent-rgb), 0.06);
  border-color: var(--border-strong);
  transform: translateY(-1px);
}

.prefs__option--on {
  background: rgba(var(--accent-rgb), 0.14);
  border-color: var(--accent);
  box-shadow: var(--shadow-glow-amber);
}

.prefs__option-label {
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 2px;
  color: var(--accent);
}

.prefs__option-desc {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.prefs__theme-row {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.prefs__theme-swatch {
  width: 18px;
  height: 18px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-strong);
  box-shadow: 0 0 8px rgba(0, 0, 0, 0.45);
  flex-shrink: 0;
}

.prefs__toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  font-size: 13px;
  color: var(--text);
}

.prefs__toggle input {
  width: 16px;
  height: 16px;
  accent-color: var(--accent);
}

/* Segmented control for the language switch: visually distinct from the
   large theme cards so the user reads it as a quick toggle, not a deep
   choice. Matches the CRT button geometry from SimBar so the page hangs
   together. */
.prefs__lang-switch {
  display: inline-flex;
  gap: 0;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: var(--surface-0);
  width: fit-content;
  max-width: 100%;
}

.prefs__lang-pick {
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 2px;
  text-transform: uppercase;
  background: transparent;
  border: none;
  color: var(--text-dim);
  padding: 8px 22px;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
  min-width: 88px;
}

.prefs__lang-pick + .prefs__lang-pick {
  border-left: 1px solid var(--border);
}

.prefs__lang-pick:hover {
  background: rgba(var(--accent-rgb), 0.06);
  color: var(--text);
}

.prefs__lang-pick--active {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--accent);
  text-shadow: var(--shadow-glow-amber);
}

@media (max-width: 480px) {
  .prefs__lang-switch {
    display: flex;
    flex-direction: column;
    width: 100%;
  }
  .prefs__lang-pick {
    min-width: 0;
    min-height: 44px;
  }
  .prefs__lang-pick + .prefs__lang-pick {
    border-left: none;
    border-top: 1px solid var(--border);
  }
}
</style>
