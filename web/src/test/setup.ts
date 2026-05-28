// Shared vitest setup. Initialises Pinia for every test that mounts a
// component depending on a store. Components import their store via
// useXStore(), which expects an active Pinia instance. Vue-i18n is also
// registered as a global plugin so any setup() that calls useI18n() picks
// up the same catalogs we ship in production.
import { config } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach } from 'vitest';
import { i18n } from '../i18n';

config.global.plugins = [...(config.global.plugins ?? []), i18n];

beforeEach(() => {
  setActivePinia(createPinia());
});
