import { describe, it, expect, beforeEach } from 'vitest';
import { usePreferencesStore } from './preferences';

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
});
