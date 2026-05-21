import { beforeEach, describe, expect, it } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useViewerStore } from './viewer';

describe('viewer store', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('defaults to autofollow on, no hand raises', () => {
    const v = useViewerStore();
    expect(v.autofollow).toBe(true);
    expect(v.handRaiseCount).toBe(0);
    expect(v.lastHandRaiseLabel).toBe('');
  });

  it('toggleAutofollow flips the flag', () => {
    const v = useViewerStore();
    v.toggleAutofollow();
    expect(v.autofollow).toBe(false);
    v.toggleAutofollow();
    expect(v.autofollow).toBe(true);
  });

  it('setAutofollow forces the value', () => {
    const v = useViewerStore();
    v.setAutofollow(false);
    expect(v.autofollow).toBe(false);
    v.setAutofollow(false);
    expect(v.autofollow).toBe(false);
    v.setAutofollow(true);
    expect(v.autofollow).toBe(true);
  });

  it('noteHandRaise increments count and tracks last label', () => {
    const v = useViewerStore();
    v.noteHandRaise('Ana');
    v.noteHandRaise('Beto');
    expect(v.handRaiseCount).toBe(2);
    expect(v.lastHandRaiseLabel).toBe('Beto');
  });

  it('clearHandRaises resets both counter and label', () => {
    const v = useViewerStore();
    v.noteHandRaise('Ana');
    v.clearHandRaises();
    expect(v.handRaiseCount).toBe(0);
    expect(v.lastHandRaiseLabel).toBe('');
  });
});
