import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { router } from './router';
import { applyTheme, readStoredTheme } from './stores/preferences';
import './style.css';

// Apply the stored theme before Vue mounts so the first paint already wears
// the user's chosen palette. Without this the page would flash amber for a
// frame before the preferences store boots and the watcher catches up.
applyTheme(readStoredTheme());

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.mount('#app');
