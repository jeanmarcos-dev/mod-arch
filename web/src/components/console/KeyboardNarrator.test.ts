import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import KeyboardNarrator from './KeyboardNarrator.vue';
import { useCpuStore } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';

// Helper: force narrators on so the component reacts to keypresses; the
// default mode is pedagogic which already enables them, but being explicit
// keeps these tests robust to default changes.
function enableNarrators() {
  const prefs = usePreferencesStore();
  prefs.setMode('pedagogic');
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('KeyboardNarrator', () => {
  it('upgrades a buffered card to delivered when process_state arrives late', async () => {
    enableNarrators();
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'state', running: true, paused: false, clockHz: 4, speedMul: 1 } as any);

    const w = mount(KeyboardNarrator);
    cpu.pushKey('a');
    await flushPromises();

    // Advance well past the legacy 600 ms threshold so the card transitions
    // to buffered. With a 250 ms tick that is 3 * 250 = 750 ms, but we go
    // further to be unambiguous.
    vi.advanceTimersByTime(2000);
    await flushPromises();

    expect(w.text()).toContain(cpu.kbdBufUsed + ''); // buffered label includes the queue count

    cpu.__applyEvent({
      type: 'process_state',
      pid: 7,
      state: 'READY',
      reason: 'keyboard delivered',
    } as any);
    await flushPromises();

    // After the late delivery, the row must reflect the delivered outcome
    // (the delivered class is set on .kbd-narrator__row).
    const row = w.find('.kbd-narrator__row');
    expect(row.classes()).toContain('is-delivered');
  });

  it('does not flip pending→buffered while the simulator is paused', async () => {
    enableNarrators();
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'state', running: false, paused: true, clockHz: 4, speedMul: 1 } as any);

    const w = mount(KeyboardNarrator);
    cpu.pushKey('b');
    await flushPromises();

    // 5 s of wall-clock would normally be far past the pending timeout, but
    // because cpu.paused === true the narrator must hold the card in
    // pending state until the user resumes.
    vi.advanceTimersByTime(5000);
    await flushPromises();

    const row = w.find('.kbd-narrator__row');
    expect(row.classes()).toContain('is-pending');
    expect(row.classes()).not.toContain('is-buffered');
  });

  it('scales the pending timeout with tickIntervalMs at slow clocks', async () => {
    enableNarrators();
    const cpu = useCpuStore();
    // 1 Hz simulator: tickIntervalMs = 1000. The legacy hard-coded 600 ms
    // would have prematurely flipped the card to buffered; the new policy
    // must use at least 3 ticks = 3000 ms.
    cpu.__applyEvent({ type: 'state', running: true, paused: false, clockHz: 1, speedMul: 1 } as any);

    const w = mount(KeyboardNarrator);
    cpu.pushKey('c');
    await flushPromises();

    vi.advanceTimersByTime(800);
    await flushPromises();

    let row = w.find('.kbd-narrator__row');
    expect(row.classes()).toContain('is-pending');

    cpu.__applyEvent({
      type: 'process_state',
      pid: 1,
      state: 'READY',
      reason: 'keyboard delivered',
    } as any);
    await flushPromises();

    row = w.find('.kbd-narrator__row');
    expect(row.classes()).toContain('is-delivered');
    expect(row.classes()).not.toContain('is-buffered');
  });
});
