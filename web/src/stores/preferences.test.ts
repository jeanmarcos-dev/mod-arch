import { describe, it, expect, beforeEach } from 'vitest';
import { nextTick } from 'vue';
import { createPinia, setActivePinia } from 'pinia';
import { usePreferencesStore, detectA11yPreferences } from './preferences';

describe('preferences store', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('starts in pedagogic mode (animations on) with sidebar expanded', () => {
    const prefs = usePreferencesStore();
    expect(prefs.mode).toBe('pedagogic');
    expect(prefs.sidebarCollapsed).toBe(false);
    expect(prefs.animationsEnabled).toBe(true);
  });

  it('falls back to compact when explicitly requested', () => {
    const prefs = usePreferencesStore();
    prefs.setMode('compact');
    expect(prefs.animationsEnabled).toBe(false);
  });

  it('toggles the sidebar', () => {
    const prefs = usePreferencesStore();
    prefs.toggleSidebar();
    expect(prefs.sidebarCollapsed).toBe(true);
    prefs.toggleSidebar();
    expect(prefs.sidebarCollapsed).toBe(false);
  });

  it('enables animations and narrators when switching to pedagogic', () => {
    const prefs = usePreferencesStore();
    prefs.setMode('pedagogic');
    expect(prefs.animationsEnabled).toBe(true);
    expect(prefs.narratorsEnabled).toBe(true);
  });

  it('slows animation speed in projector mode', () => {
    const prefs = usePreferencesStore();
    prefs.setMode('projector');
    expect(prefs.animationSpeed).toBe(0.5);
  });

  it('starts with every side panel expanded', () => {
    const prefs = usePreferencesStore();
    expect(prefs.panelsCollapsed.cores).toBe(false);
    expect(prefs.panelsCollapsed.processes).toBe(false);
    expect(prefs.panelsCollapsed.queues).toBe(false);
    expect(prefs.panelsCollapsed.ipc).toBe(false);
  });

  it('toggles individual side panels independently', () => {
    const prefs = usePreferencesStore();
    prefs.togglePanel('queues');
    expect(prefs.panelsCollapsed.queues).toBe(true);
    expect(prefs.panelsCollapsed.processes).toBe(false);
    prefs.togglePanel('queues');
    expect(prefs.panelsCollapsed.queues).toBe(false);
    prefs.togglePanel('cores', true);
    expect(prefs.panelsCollapsed.cores).toBe(true);
    prefs.togglePanel('cores', true);
    expect(prefs.panelsCollapsed.cores).toBe(true);
  });

  it('persists column widths and clamps to the minimum', () => {
    const prefs = usePreferencesStore();
    prefs.setColumnWidth('left', 50);
    expect(prefs.columnWidths.leftPx).toBe(140);
    prefs.setColumnWidth('left', 220);
    expect(prefs.columnWidths.leftPx).toBe(220);
    prefs.setColumnWidth('right', 100);
    expect(prefs.columnWidths.rightPx).toBe(240);
    prefs.setColumnWidth('right', 380);
    expect(prefs.columnWidths.rightPx).toBe(380);
  });

  it('shows init by default and respects the manual toggle', () => {
    const prefs = usePreferencesStore();
    expect(prefs.showInit).toBe(true);
    prefs.setShowInit(false);
    expect(prefs.showInit).toBe(false);
    prefs.setShowInit(true);
    expect(prefs.showInit).toBe(true);
  });

  it('applyShowInitDefault always reveals init while user has not touched the toggle', () => {
    const prefs = usePreferencesStore();
    prefs.applyShowInitDefault(1);
    expect(prefs.showInit).toBe(true);
    prefs.applyShowInitDefault(2);
    expect(prefs.showInit).toBe(true);
    prefs.applyShowInitDefault(0);
    expect(prefs.showInit).toBe(true);
  });

  it('applyShowInitDefault stops overriding once the user touches the toggle', () => {
    const prefs = usePreferencesStore();
    prefs.applyShowInitDefault(2);
    expect(prefs.showInit).toBe(true);
    prefs.setShowInit(false);
    prefs.applyShowInitDefault(5);
    expect(prefs.showInit).toBe(false);
    prefs.applyShowInitDefault(1);
    expect(prefs.showInit).toBe(false);
  });

  it('persists the user override of showInit across reloads', async () => {
    const first = usePreferencesStore();
    first.setShowInit(true);
    await nextTick();
    setActivePinia(createPinia());
    const second = usePreferencesStore();
    expect(second.showInit).toBe(true);
    second.applyShowInitDefault(1);
    expect(second.showInit).toBe(true);
  });

  describe('presenter mirror', () => {
    it('reflects the host mode + narrators through the effective getters', () => {
      const prefs = usePreferencesStore();
      prefs.setNarratorsEnabled(true);
      prefs.setMode('pedagogic');
      prefs.setMirror({ mode: 'compact', narrators: false });
      expect(prefs.effectiveMode).toBe('compact');
      expect(prefs.effectiveNarratorsEnabled).toBe(false);
      // The local preference is untouched by the mirror.
      expect(prefs.mode).toBe('pedagogic');
      expect(prefs.narratorsEnabled).toBe(true);
    });

    it('falls back to the local preference once the mirror is cleared', () => {
      const prefs = usePreferencesStore();
      prefs.setNarratorsEnabled(true);
      prefs.setMirror({ mode: 'projector', narrators: false });
      expect(prefs.effectiveNarratorsEnabled).toBe(false);
      prefs.clearMirror();
      expect(prefs.effectiveNarratorsEnabled).toBe(true);
      expect(prefs.effectiveMode).toBe(prefs.mode);
    });

    it('never persists the mirrored value: the local pick survives a reload', async () => {
      const first = usePreferencesStore();
      first.setNarratorsEnabled(false);
      first.setMirror({ mode: 'projector', narrators: true });
      await nextTick();
      const raw = window.localStorage.getItem('modos-cpu:prefs:v1');
      expect(raw).toBeTruthy();
      expect(JSON.parse(raw as string).narratorsEnabled).toBe(false);
      setActivePinia(createPinia());
      const second = usePreferencesStore();
      expect(second.narratorsEnabled).toBe(false);
      expect(second.effectiveNarratorsEnabled).toBe(false);
    });
  });

  describe('admin defaults (sticky)', () => {
    it('applies admin defaults to fields the user has not touched', () => {
      const prefs = usePreferencesStore();
      prefs.applyAdminDefaults({ narratorsEnabled: false, mode: 'compact', theme: 'green', density: 'compact', narratorMinTickMs: 400 });
      expect(prefs.narratorsEnabled).toBe(false);
      expect(prefs.mode).toBe('compact');
      expect(prefs.theme).toBe('green');
      expect(prefs.density).toBe('compact');
      expect(prefs.narratorMinTickMs).toBe(400);
      // Applying defaults must not mark the fields as touched.
      expect(prefs.touched.narratorsEnabled).toBe(false);
      expect(prefs.touched.mode).toBe(false);
    });

    it('does not override a field the user already touched', () => {
      const prefs = usePreferencesStore();
      prefs.setNarratorsEnabled(true);
      prefs.setTheme('dark');
      prefs.applyAdminDefaults({ narratorsEnabled: false, theme: 'green' });
      expect(prefs.narratorsEnabled).toBe(true);
      expect(prefs.theme).toBe('dark');
    });

    it('treats a mode change as touching narrators (coupling)', () => {
      const prefs = usePreferencesStore();
      prefs.setMode('projector');
      expect(prefs.touched.mode).toBe(true);
      expect(prefs.touched.narratorsEnabled).toBe(true);
      prefs.applyAdminDefaults({ narratorsEnabled: false, mode: 'compact' });
      expect(prefs.narratorsEnabled).toBe(true);
      expect(prefs.mode).toBe('projector');
    });

    it('migrates a legacy payload: a diverging value counts as touched', () => {
      // Legacy payload written before sticky tracking existed: no `touched`
      // map, narrators stored off (differs from the code default of on).
      window.localStorage.setItem('modos-cpu:prefs:v1', JSON.stringify({
        panelsResetVersion: 4,
        narratorsEnabled: false,
      }));
      const prefs = usePreferencesStore();
      expect(prefs.touched.narratorsEnabled).toBe(true);
      prefs.applyAdminDefaults({ narratorsEnabled: true });
      expect(prefs.narratorsEnabled).toBe(false);
    });

    it('migrates a legacy payload: a default-matching value stays eligible', () => {
      window.localStorage.setItem('modos-cpu:prefs:v1', JSON.stringify({
        panelsResetVersion: 4,
        theme: 'amber',
      }));
      const prefs = usePreferencesStore();
      expect(prefs.touched.theme).toBe(false);
      prefs.applyAdminDefaults({ theme: 'green' });
      expect(prefs.theme).toBe('green');
    });
  });

  describe('accessibility preferences', () => {
    beforeEach(() => {
      setActivePinia(createPinia());
      window.localStorage.clear();
    });

    it('defaults to legible font off, normal text scale, announcer on', () => {
      const prefs = usePreferencesStore();
      expect(prefs.legibleFont).toBe(false);
      expect(prefs.textScale).toBe('normal');
      expect(prefs.screenReaderAnnounce).toBe(true);
    });

    it('updates the accessibility knobs through their setters', () => {
      const prefs = usePreferencesStore();
      prefs.setLegibleFont(true);
      prefs.setTextScale('large');
      prefs.setScreenReaderAnnounce(false);
      expect(prefs.legibleFont).toBe(true);
      expect(prefs.textScale).toBe('large');
      expect(prefs.screenReaderAnnounce).toBe(false);
    });

    it('persists the accessibility knobs across reloads', async () => {
      const first = usePreferencesStore();
      first.setLegibleFont(true);
      first.setTextScale('xlarge');
      first.setScreenReaderAnnounce(false);
      await nextTick();
      setActivePinia(createPinia());
      const second = usePreferencesStore();
      expect(second.legibleFont).toBe(true);
      expect(second.textScale).toBe('xlarge');
      expect(second.screenReaderAnnounce).toBe(false);
    });

    it('rejects an invalid stored text scale', () => {
      window.localStorage.setItem('modos-cpu:prefs:v1', JSON.stringify({
        panelsResetVersion: 4,
        textScale: 'huge',
      }));
      const prefs = usePreferencesStore();
      expect(prefs.textScale).toBe('normal');
    });

    it('maps a strong OS contrast signal to the high-contrast theme on first visit', () => {
      const original = window.matchMedia;
      window.matchMedia = ((query: string) => ({
        matches: query.includes('prefers-contrast: more'),
        media: query,
        onchange: null,
        addEventListener() {},
        removeEventListener() {},
        addListener() {},
        removeListener() {},
        dispatchEvent() { return false; },
      })) as unknown as typeof window.matchMedia;
      try {
        expect(detectA11yPreferences()).toEqual({ theme: 'accesible', legibleFont: true });
        setActivePinia(createPinia());
        window.localStorage.clear();
        const prefs = usePreferencesStore();
        expect(prefs.theme).toBe('accesible');
        expect(prefs.legibleFont).toBe(true);
      } finally {
        window.matchMedia = original;
      }
    });

    it('leaves the defaults untouched when no strong contrast signal is present', () => {
      const original = window.matchMedia;
      window.matchMedia = ((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener() {},
        removeEventListener() {},
        addListener() {},
        removeListener() {},
        dispatchEvent() { return false; },
      })) as unknown as typeof window.matchMedia;
      try {
        expect(detectA11yPreferences()).toEqual({});
        setActivePinia(createPinia());
        window.localStorage.clear();
        const prefs = usePreferencesStore();
        expect(prefs.theme).toBe('amber');
        expect(prefs.legibleFont).toBe(false);
      } finally {
        window.matchMedia = original;
      }
    });
  });
});
