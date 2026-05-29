import { describe, it, expect, beforeEach } from 'vitest';
import { nextTick } from 'vue';
import { createPinia, setActivePinia } from 'pinia';
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
});
