import { defineStore } from 'pinia';
import { ref } from 'vue';
import { apiFetch } from '../lib/api';

export interface PublicAppConfig {
  turnstile_sitekey: string;
}

/**
 * appConfig holds the bootstrap settings exposed by GET /api/config. The
 * SPA fetches it once at boot so the Login screen knows whether to render
 * the Turnstile widget.
 */
export const useAppConfigStore = defineStore('appConfig', () => {
  const turnstileSitekey = ref<string>('');
  const loaded = ref(false);

  async function load(): Promise<void> {
    if (loaded.value) return;
    try {
      const r = await apiFetch('/api/config');
      if (r.ok) {
        const data = (await r.json()) as Partial<PublicAppConfig>;
        turnstileSitekey.value = data.turnstile_sitekey ?? '';
      }
    } catch {
      /* network blip: keep defaults */
    } finally {
      loaded.value = true;
    }
  }

  return { turnstileSitekey, loaded, load };
});
