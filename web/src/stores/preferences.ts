import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

// PresentationMode tunes density, animation speed and narrator visibility in
// one place. The three presets are exposed to the user as "preferences" so a
// teacher can flip a switch before class without remembering which knobs to
// turn.
export type PresentationMode = 'compact' | 'pedagogic' | 'projector';

export type Density = 'comfortable' | 'compact';

interface PrefsState {
  sidebarCollapsed: boolean;
  mode: PresentationMode;
  density: Density;
  narratorsEnabled: boolean;
}

const STORAGE_KEY = 'modos-cpu:prefs:v1';

// Pedagogic is the default preset so a fresh visitor sees the animated
// transitions out of the box (animationsEnabled is derived from mode in
// the computed below). Narrators stay off until the user opts in or until
// a host broadcasts them via presence — they pull attention and are
// genuinely a teaching aid, not a default-on chrome.
const DEFAULTS: PrefsState = {
  sidebarCollapsed: false,
  mode: 'pedagogic',
  density: 'comfortable',
  narratorsEnabled: false,
};

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

  watch(
    [sidebarCollapsed, mode, density, narratorsEnabled],
    ([sc, m, d, n]) => {
      try {
        window.localStorage.setItem(
          STORAGE_KEY,
          JSON.stringify({
            sidebarCollapsed: sc,
            mode: m,
            density: d,
            narratorsEnabled: n,
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

  function setMode(m: PresentationMode) {
    mode.value = m;
    // Pedagogic / projector default to narrators on for first-time users; they
    // can still toggle narrators off explicitly afterwards.
    if (m !== 'compact') narratorsEnabled.value = true;
  }

  return {
    sidebarCollapsed,
    mode,
    density,
    narratorsEnabled,
    rowDensity,
    animationsEnabled,
    animationSpeed,
    toggleSidebar,
    setMode,
  };
});
