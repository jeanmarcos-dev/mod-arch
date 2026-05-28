import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { apiFetch } from '../lib/api';

export interface PublicAppConfig {
  turnstile_sitekey: string;
  disabled_routes: string[];
  disabled_panels: string[];
  disabled_demos: string[];
  default_route: string;
}

export const DEFAULT_ROUTE_NAME = 'overview';

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is string => typeof v === 'string');
}

/**
 * appConfig holds the bootstrap settings exposed by GET /api/config. The
 * SPA fetches it once at boot so the Login screen knows whether to render
 * the Turnstile widget and which routes/panels/demos the admin has hidden.
 */
export const useAppConfigStore = defineStore('appConfig', () => {
  const turnstileSitekey = ref<string>('');
  const disabledRoutesList = ref<string[]>([]);
  const disabledPanelsList = ref<string[]>([]);
  const disabledDemosList = ref<string[]>([]);
  const defaultRoute = ref<string>(DEFAULT_ROUTE_NAME);
  const loaded = ref(false);

  const disabledRoutes = computed(() => new Set(disabledRoutesList.value));
  const disabledPanels = computed(() => new Set(disabledPanelsList.value));
  const disabledDemos = computed(() => new Set(disabledDemosList.value));

  async function load(force = false): Promise<void> {
    if (loaded.value && !force) return;
    try {
      const r = await apiFetch('/api/config');
      if (r.ok) {
        const data = (await r.json()) as Partial<PublicAppConfig>;
        turnstileSitekey.value = data.turnstile_sitekey ?? '';
        disabledRoutesList.value = normalizeStringArray(data.disabled_routes);
        disabledPanelsList.value = normalizeStringArray(data.disabled_panels);
        disabledDemosList.value = normalizeStringArray(data.disabled_demos);
        defaultRoute.value =
          typeof data.default_route === 'string' && data.default_route !== ''
            ? data.default_route
            : DEFAULT_ROUTE_NAME;
      }
    } catch {
      /* network blip: keep defaults */
    } finally {
      loaded.value = true;
    }
  }

  return {
    turnstileSitekey,
    disabledRoutes,
    disabledPanels,
    disabledDemos,
    defaultRoute,
    loaded,
    load,
  };
});
