import { defineStore } from 'pinia';
import { ref, watch } from 'vue';

export type SimBarMode = 'docked' | 'floating';

interface SimBarState {
  mode: SimBarMode;
  x: number;
  y: number;
  coresExpanded: boolean;
}

// Includes toolsDrawerOpen so the Reset UI snackbar can restore an open
// drawer on Undo — the drawer state is runtime-only otherwise.
export interface SimBarSnapshot extends SimBarState {
  toolsDrawerOpen: boolean;
}

const STORAGE_KEY = 'modos-cpu:simbar:v1';

const DEFAULTS: SimBarState = {
  mode: 'docked',
  x: 24,
  y: 72,
  coresExpanded: true,
};

function inViewport(s: SimBarState): boolean {
  if (typeof window === 'undefined') return true;
  if (!Number.isFinite(s.x) || !Number.isFinite(s.y)) return false;
  return s.x >= 0 && s.y >= 0 && s.x < window.innerWidth && s.y < window.innerHeight;
}

function loadFromStorage(): SimBarState {
  if (typeof window === 'undefined') return DEFAULTS;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS;
    const parsed = { ...DEFAULTS, ...(JSON.parse(raw) as Partial<SimBarState>) };
    if (parsed.mode !== 'docked' && parsed.mode !== 'floating') return DEFAULTS;
    if (typeof parsed.coresExpanded !== 'boolean') parsed.coresExpanded = DEFAULTS.coresExpanded;
    if (!inViewport(parsed)) return { ...DEFAULTS, coresExpanded: parsed.coresExpanded, mode: 'docked' };
    return parsed;
  } catch {
    return DEFAULTS;
  }
}

export const useSimBarStore = defineStore('simBar', () => {
  const initial = loadFromStorage();
  const mode = ref<SimBarMode>(initial.mode);
  const x = ref(initial.x);
  const y = ref(initial.y);
  const coresExpanded = ref<boolean>(initial.coresExpanded);

  // toolsDrawerOpen lives in runtime only — the drawer should always start
  // closed on page load so the visual hierarchy of the simulation is the
  // first thing the user sees.
  const toolsDrawerOpen = ref(false);

  watch([mode, x, y, coresExpanded], ([m, px, py, ce]) => {
    try {
      window.localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({ mode: m, x: px, y: py, coresExpanded: ce }),
      );
    } catch {
      // Quota or private mode — silently fall back to in-memory state.
    }
  });

  function setPosition(nx: number, ny: number) {
    x.value = nx;
    y.value = ny;
  }

  function dock() {
    mode.value = 'docked';
  }

  function float() {
    mode.value = 'floating';
  }

  function toggle() {
    mode.value = mode.value === 'docked' ? 'floating' : 'docked';
  }

  function toggleCores() {
    coresExpanded.value = !coresExpanded.value;
  }

  function openTools() {
    toolsDrawerOpen.value = true;
  }

  function closeTools() {
    toolsDrawerOpen.value = false;
  }

  function toggleTools() {
    toolsDrawerOpen.value = !toolsDrawerOpen.value;
  }

  // Snapshot/apply/reset support the "Reset UI" command in SimBar, which
  // captures the layout before reverting it and offers an Undo via the toast
  // snackbar. toolsDrawerOpen is included so an open drawer is restored on
  // undo even though it does not persist across reloads.
  function getState(): SimBarSnapshot {
    return {
      mode: mode.value,
      x: x.value,
      y: y.value,
      coresExpanded: coresExpanded.value,
      toolsDrawerOpen: toolsDrawerOpen.value,
    };
  }

  function applyState(s: SimBarSnapshot) {
    mode.value = s.mode;
    x.value = s.x;
    y.value = s.y;
    coresExpanded.value = s.coresExpanded;
    toolsDrawerOpen.value = s.toolsDrawerOpen;
  }

  function reset() {
    applyState({ ...DEFAULTS, toolsDrawerOpen: false });
  }

  return {
    mode,
    x,
    y,
    coresExpanded,
    toolsDrawerOpen,
    setPosition,
    dock,
    float,
    toggle,
    toggleCores,
    openTools,
    closeTools,
    toggleTools,
    getState,
    applyState,
    reset,
  };
});
