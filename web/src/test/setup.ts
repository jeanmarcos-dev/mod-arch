// Shared vitest setup. Initialises Pinia for every test that mounts a
// component depending on a store. Components import their store via
// useXStore(), which expects an active Pinia instance.
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach } from 'vitest';

beforeEach(() => {
  setActivePinia(createPinia());
});
