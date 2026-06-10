import { createI18n } from 'vue-i18n';
import es from './locales/es';
import en from './locales/en';
import pt from './locales/pt';
import { DEFAULT_LOCALE, SUPPORTED_LOCALES, type Locale, type MessageSchema } from './schema';

const STORAGE_KEY = 'modos-cpu:prefs:v1';

function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (SUPPORTED_LOCALES as string[]).includes(v);
}

// readStoredLocale mirrors readStoredTheme in stores/preferences.ts: a saved
// pick wins, otherwise we sniff navigator.language and fall back to the
// default. Called both from main.ts (anti-flash, before mount) and from the
// preferences store on boot.
export function readStoredLocale(): Locale {
  if (typeof window === 'undefined') return DEFAULT_LOCALE;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as { locale?: unknown };
      if (isLocale(parsed.locale)) return parsed.locale;
    }
  } catch { /* corrupt entry → fall through to detection */ }
  if (typeof navigator !== 'undefined') {
    const nav = (navigator.language || '').toLowerCase();
    if (nav.startsWith('pt')) return 'pt';
    if (nav.startsWith('en')) return 'en';
  }
  return DEFAULT_LOCALE;
}

// applyLocale sets <html lang="..."> so assistive tech and search engines
// see the active locale. Mirrors applyTheme's role for data-theme. Exposed
// at module scope so main.ts can call it before Vue mounts.
export function applyLocale(locale: Locale) {
  if (typeof document === 'undefined') return;
  document.documentElement.setAttribute('lang', locale);
}

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: readStoredLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: { es, en, pt },
});

// setI18nLocale is the canonical mutation point for runtime switches. The
// preferences store calls this so cpu.ts (via formatLog.ts) and every Vue
// component re-render in the new locale immediately.
export function setI18nLocale(locale: Locale) {
  (i18n.global.locale as unknown as { value: Locale }).value = locale;
  applyLocale(locale);
}

declare module 'vue-i18n' {
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  export interface DefineLocaleMessage extends MessageSchema {}
}
