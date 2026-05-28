import { computed, onBeforeUnmount, watch } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';

// useNarratorAnimation drives the visual narrators with the same time scale
// the simulator runs at. Each animation lasts exactly one tick (cpu.tickIntervalMs)
// so the on-screen pulse / flight / banner represents the event the kernel
// just emitted, instead of an arbitrary fixed dwell. Below narratorMinTickMs
// the simulator is ticking faster than the eye can resolve, so play() returns
// null and the caller skips the animation entirely.
//
// When the run loop pauses (cpu.running === false) every live animation is
// frozen mid-flight via WAAPI's Animation.pause(); resume continues from the
// exact same frame. This is the freeze-in-the-air behavior the design calls
// for, and it works whether the event was kept alive by a long tick interval
// or by the user pausing midway through it.

export interface PlayOptions {
  fill?: FillMode;
  /**
   * Override the duration in ms. Use sparingly — defaults to cpu.tickIntervalMs
   * which is the right answer for kernel-event narrators. The override exists
   * for sub-elements that share the parent tick (e.g. a staggered arrow chain
   * inside InterruptNarrator) and need a fraction of it.
   */
  durationMs?: number;
  /**
   * Delay before the animation starts, in ms. Same fraction-of-tick caveat
   * as durationMs.
   */
  delayMs?: number;
}

export function useNarratorAnimation() {
  const cpu = useCpuStore();
  const prefs = usePreferencesStore();
  const liveAnimations = new Set<Animation>();

  function shouldFire(): boolean {
    return cpu.tickIntervalMs >= prefs.narratorMinTickMs;
  }

  function play(
    el: HTMLElement,
    keyframes: Keyframe[],
    opts: PlayOptions = {},
  ): Animation | null {
    if (!shouldFire()) return null;
    const duration = opts.durationMs ?? cpu.tickIntervalMs;
    const anim = el.animate(keyframes, {
      duration,
      delay: opts.delayMs ?? 0,
      fill: opts.fill ?? 'forwards',
      easing: 'ease-out',
    });
    liveAnimations.add(anim);
    // Drop the handle when the animation finishes naturally OR is cancelled —
    // .finished rejects on cancel, hence the catch.
    anim.finished
      .catch(() => { /* cancelled — still want to clear the set */ })
      .finally(() => { liveAnimations.delete(anim); });
    if (!cpu.running) anim.pause();
    return anim;
  }

  const stopWatch = watch(
    () => cpu.running,
    (on) => {
      liveAnimations.forEach((a) => (on ? a.play() : a.pause()));
    },
  );

  onBeforeUnmount(() => {
    liveAnimations.forEach((a) => a.cancel());
    liveAnimations.clear();
    stopWatch();
  });

  return {
    shouldFire,
    play,
    tickMs: computed(() => cpu.tickIntervalMs),
  };
}
