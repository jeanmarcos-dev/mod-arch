import { defineStore } from 'pinia';
import { ref, watch } from 'vue';

export type SimBarMode = 'docked' | 'floating';

interface SimBarState {
  mode: SimBarMode;
  x: number;
  y: number;
}

const STORAGE_KEY = 'modos-cpu:simbar:v1';

const DEFAULTS: SimBarState = {
  mode: 'docked',
  x: 24,
  y: 72,
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
    if (!inViewport(parsed)) return { ...DEFAULTS, mode: 'docked' };
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

  watch([mode, x, y], ([m, px, py]) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: m, x: px, y: py }));
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

  return { mode, x, y, setPosition, dock, float, toggle };
});
