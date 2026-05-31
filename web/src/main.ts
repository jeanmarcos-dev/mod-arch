import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { router } from './router';
import { applyTheme, readStoredTheme } from './stores/preferences';
import { i18n, applyLocale, readStoredLocale } from './i18n';
import './style.css';
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css';

// Apply the stored theme and locale before Vue mounts so the first paint
// already wears the user's chosen palette and language. Without this the
// page would flash amber/Spanish for a frame before the preferences store
// boots and the watcher catches up.
applyTheme(readStoredTheme());
applyLocale(readStoredLocale());

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.use(i18n);
app.mount('#app');
