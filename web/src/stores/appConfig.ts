import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { apiFetch } from '../lib/api';
import type { AdminDefaults } from './preferences';

export interface PublicAppConfig {
  turnstile_sitekey: string;
  disabled_routes: string[];
  disabled_panels: string[];
  disabled_demos: string[];
  default_route: string;
  defaults?: Partial<AdminDefaults>;
}

export const DEFAULT_ROUTE_NAME = 'overview';

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is string => typeof v === 'string');
}

// sanitizeDefaults mirrors the per-field guards in the preferences store: the
// admin baseline is untrusted wire data, so each field is validated before it
// can flow into applyAdminDefaults. Unknown / malformed fields are dropped so a
// partial or future-shaped payload still applies its valid parts.
function sanitizeDefaults(value: unknown): Partial<AdminDefaults> | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const src = value as Record<string, unknown>;
  const out: Partial<AdminDefaults> = {};
  if (typeof src.narratorsEnabled === 'boolean') out.narratorsEnabled = src.narratorsEnabled;
  if (src.mode === 'compact' || src.mode === 'pedagogic' || src.mode === 'projector') out.mode = src.mode;
  if (src.theme === 'amber' || src.theme === 'green' || src.theme === 'dark' || src.theme === 'accesible') out.theme = src.theme;
  if (src.density === 'comfortable' || src.density === 'compact') out.density = src.density;
  if (typeof src.speed === 'number' && Number.isFinite(src.speed) && src.speed > 0) out.speed = src.speed;
  if (typeof src.narratorMinTickMs === 'number' && Number.isFinite(src.narratorMinTickMs) && src.narratorMinTickMs >= 0) {
    out.narratorMinTickMs = src.narratorMinTickMs;
  }
  return out;
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
  const adminDefaults = ref<Partial<AdminDefaults> | null>(null);
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
        adminDefaults.value = sanitizeDefaults(data.defaults) ?? null;
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
    adminDefaults,
    loaded,
    load,
  };
});
