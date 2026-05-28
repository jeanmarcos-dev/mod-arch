import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

// PresentationMode tunes density, animation speed and narrator visibility in
// one place. The three presets are exposed to the user as "preferences" so a
// teacher can flip a switch before class without remembering which knobs to
// turn.
export type PresentationMode = 'compact' | 'pedagogic' | 'projector';

export type Density = 'comfortable' | 'compact';

// Theme controls the color palette + font family of the whole shell. The
// active theme is reflected on <html data-theme="..."> so it reaches portals
// and modals mounted outside the shell root.
export type Theme = 'amber' | 'green' | 'dark' | 'accesible';

interface PrefsState {
  sidebarCollapsed: boolean;
  mode: PresentationMode;
  density: Density;
  narratorsEnabled: boolean;
  // Below this tick interval (ms) narrators stop firing. Pedagogically:
  // visual narration of every event is meaningless once the CPU ticks faster
  // than the eye can resolve. Configurable so the teacher can widen or
  // narrow the readable band.
  narratorMinTickMs: number;
  theme: Theme;
}

const STORAGE_KEY = 'modos-cpu:prefs:v1';

// Pedagogic is the default preset so a fresh visitor sees the animated
// transitions out of the box (animationsEnabled is derived from mode in
// the computed below). Narrators default on too: the simulator is built
// around explaining what the CPU is doing, and the animated overlays are
// the most direct way to surface that — a first-time visitor should see
// them straight away rather than having to discover the toggle.
const DEFAULTS: PrefsState = {
  sidebarCollapsed: false,
  mode: 'pedagogic',
  density: 'comfortable',
  narratorsEnabled: true,
  narratorMinTickMs: 200,
  theme: 'amber',
};

// applyTheme writes the active theme to <html data-theme="..."> so the
// :root[data-theme] rules in style.css take effect. Exposed at module scope
// so main.ts can call it before Vue mounts, avoiding a brief flash of the
// default amber palette on top of a saved non-amber pick.
export function applyTheme(theme: Theme) {
  if (typeof document === 'undefined') return;
  document.documentElement.setAttribute('data-theme', theme);
}

export function readStoredTheme(): Theme {
  if (typeof window === 'undefined') return DEFAULTS.theme;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS.theme;
    const parsed = JSON.parse(raw) as Partial<PrefsState>;
    const t = parsed.theme as string | undefined;
    // The "modern" theme was merged into "dark" — silently migrate any
    // pre-existing preference so returning users see the new (sharper) dark
    // instead of having their localStorage entry rejected as invalid.
    if (t === 'modern') return 'dark';
    if (t === 'amber' || t === 'green' || t === 'dark' || t === 'accesible') return t;
  } catch { /* corrupt entry → default */ }
  return DEFAULTS.theme;
}

function loadFromStorage(): PrefsState {
  if (typeof window === 'undefined') return DEFAULTS;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS;
    return { ...DEFAULTS, ...(JSON.parse(raw) as Partial<PrefsState>) };
  } catch {
    return DEFAULTS;
  }
}

export const usePreferencesStore = defineStore('preferences', () => {
  const initial = loadFromStorage();
  const sidebarCollapsed = ref(initial.sidebarCollapsed);
  const mode = ref<PresentationMode>(initial.mode);
  const density = ref<Density>(initial.density);
  const narratorsEnabled = ref(initial.narratorsEnabled);
  const narratorMinTickMs = ref(initial.narratorMinTickMs);
  const theme = ref<Theme>(initial.theme);

  // Mobile drawer state is volatile by design: always starts closed so a
  // fresh reload doesn't reveal the sidebar over the content. Kept separate
  // from sidebarCollapsed (desktop, persisted) so the two never fight.
  const sidebarMobileOpen = ref(false);

  // Keep the document attribute in sync with the store. main.ts already calls
  // applyTheme() before mount to avoid a flash, but if the theme changes at
  // runtime this watcher handles the swap without each component having to
  // observe the store itself.
  watch(theme, (t) => applyTheme(t), { immediate: false });

  watch(
    [sidebarCollapsed, mode, density, narratorsEnabled, narratorMinTickMs, theme],
    ([sc, m, d, n, nMin, t]) => {
      try {
        window.localStorage.setItem(
          STORAGE_KEY,
          JSON.stringify({
            sidebarCollapsed: sc,
            mode: m,
            density: d,
            narratorsEnabled: n,
            narratorMinTickMs: nMin,
            theme: t,
          }),
        );
      } catch {
        // Quota or private mode — silently fall back to in-memory state.
      }
    },
  );

  // Auto-derived knobs that components consume directly. Keeping these as
  // computed refs means a change to `mode` propagates everywhere at once.
  const rowDensity = computed(() => (density.value === 'compact' ? 0.7 : 1));
  const animationsEnabled = computed(() => mode.value !== 'compact');
  const animationSpeed = computed(() => {
    if (mode.value === 'projector') return 0.5;
    if (mode.value === 'pedagogic') return 0.85;
    return 1;
  });

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value;
  }

  function openSidebarMobile() {
    sidebarMobileOpen.value = true;
  }

  function closeSidebarMobile() {
    sidebarMobileOpen.value = false;
  }

  function toggleSidebarMobile() {
    sidebarMobileOpen.value = !sidebarMobileOpen.value;
  }

  function setMode(m: PresentationMode) {
    mode.value = m;
    // Pedagogic / projector default to narrators on for first-time users; they
    // can still toggle narrators off explicitly afterwards.
    if (m !== 'compact') narratorsEnabled.value = true;
  }

  function setTheme(t: Theme) {
    theme.value = t;
  }

  return {
    sidebarCollapsed,
    sidebarMobileOpen,
    mode,
    density,
    narratorsEnabled,
    narratorMinTickMs,
    theme,
    rowDensity,
    animationsEnabled,
    animationSpeed,
    toggleSidebar,
    openSidebarMobile,
    closeSidebarMobile,
    toggleSidebarMobile,
    setMode,
    setTheme,
  };
});
