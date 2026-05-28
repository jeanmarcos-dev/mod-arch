import { describe, it, expect, beforeEach, vi } from 'vitest';
import { nextTick } from 'vue';
import { i18n, readStoredLocale, applyLocale } from './index';
import es from './locales/es';
import en from './locales/en';
import { usePreferencesStore } from '../stores/preferences';

// flattenKeys walks the message tree and emits every leaf path (e.g.
// "panels.events.title"). Used to compare the structure of es and en without
// caring about value content. The recursion stops at strings; arrays would
// also be valid leaves, but we don't use them in this catalog.
function flattenKeys(obj: unknown, prefix = ''): string[] {
  if (typeof obj !== 'object' || obj === null) return [prefix];
  const out: string[] = [];
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    out.push(...flattenKeys(v, key));
  }
  return out;
}

describe('i18n', () => {
  beforeEach(() => {
    window.localStorage.clear();
    // Reset to a known starting locale so the test suite is independent of
    // execution order — every previous switchLocale() leaks into the next
    // describe block otherwise.
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  describe('locale catalogs', () => {
    it('es and en have the same set of keys (defense against satisfies leniency)', () => {
      const esKeys = flattenKeys(es).sort();
      const enKeys = flattenKeys(en).sort();
      expect(enKeys).toEqual(esKeys);
    });

    it('translates a sample key in both locales', () => {
      const t = (i18n.global as unknown as {
        t: (k: string) => string;
        locale: { value: string };
      });
      t.locale.value = 'es';
      const esLoad = t.t('simbar.buttons.load');
      t.locale.value = 'en';
      const enLoad = t.t('simbar.buttons.load');
      expect(esLoad).toBe('LOAD');
      expect(enLoad).toBe('LOAD');

      t.locale.value = 'es';
      const esConsole = t.t('panels.console.noOutput');
      t.locale.value = 'en';
      const enConsole = t.t('panels.console.noOutput');
      expect(esConsole).toBe('(sin output todavía)');
      expect(enConsole).toBe('(no output yet)');
      expect(esConsole).not.toBe(enConsole);
    });
  });

  describe('readStoredLocale', () => {
    it('respects a stored locale even when the browser disagrees', () => {
      window.localStorage.setItem(
        'modos-cpu:prefs:v1',
        JSON.stringify({ locale: 'en' }),
      );
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('pt-BR');
      expect(readStoredLocale()).toBe('en');
    });

    it('falls back to en when navigator.language starts with en and storage is empty', () => {
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('en-US');
      expect(readStoredLocale()).toBe('en');
    });

    it('defaults to es when navigator.language is unrelated', () => {
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('pt-BR');
      expect(readStoredLocale()).toBe('es');
    });

    it('survives corrupt localStorage entries', () => {
      window.localStorage.setItem('modos-cpu:prefs:v1', '{not json');
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('es-VE');
      expect(readStoredLocale()).toBe('es');
    });

    it('ignores unknown locale values in storage', () => {
      window.localStorage.setItem(
        'modos-cpu:prefs:v1',
        JSON.stringify({ locale: 'fr' }),
      );
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('es-VE');
      expect(readStoredLocale()).toBe('es');
    });
  });

  describe('applyLocale', () => {
    it('writes <html lang> for accessibility / SEO', () => {
      applyLocale('en');
      expect(document.documentElement.getAttribute('lang')).toBe('en');
      applyLocale('es');
      expect(document.documentElement.getAttribute('lang')).toBe('es');
    });
  });

  describe('preferences store integration', () => {
    it('setLocale updates the live i18n instance, html lang, and localStorage', async () => {
      const prefs = usePreferencesStore();
      prefs.setLocale('en');

      const locale = (i18n.global.locale as unknown as { value: string }).value;
      expect(locale).toBe('en');
      expect(document.documentElement.getAttribute('lang')).toBe('en');

      // The persistence watcher flushes on the next microtask — wait for it
      // before reading localStorage.
      await nextTick();
      const raw = window.localStorage.getItem('modos-cpu:prefs:v1');
      expect(raw).toBeTruthy();
      const stored = JSON.parse(raw as string);
      expect(stored.locale).toBe('en');
    });

    it('rejects invalid locale values silently', () => {
      const prefs = usePreferencesStore();
      const before = prefs.locale;
      // @ts-expect-error - testing runtime guard
      prefs.setLocale('zz');
      expect(prefs.locale).toBe(before);
    });
  });
});
