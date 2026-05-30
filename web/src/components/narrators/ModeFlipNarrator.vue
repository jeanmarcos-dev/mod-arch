<script setup lang="ts">
import { computed, ref } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { useNarrators, type ModeSwitch } from '../../lib/narratorBus';
import { useNarratorAnimation } from './useNarratorAnimation';

// ModeFlipNarrator stages the USER↔KERNEL transition. Two layers:
//
//   1. An always-on ambient tint over its host element so the student can tell
//      at a glance which privilege level the CPU is in right now. This stays
//      on even when narrators are toggled off — it is context, not animation.
//   2. A transient pulse + banner on each mode_switch event. Each lasts exactly
//      one simulator tick (cpu.tickIntervalMs) so the visual matches the event
//      it represents. When the simulator pauses, the banner freezes in place
//      mid-life and continues from the same frame on resume.
//
// The component must be placed inside a position: relative parent — it covers
// it via inset: 0 and lets pointer events fall through to the content below.

const cpu = useCpuStore();
const narrators = useNarrators();
const animator = useNarratorAnimation();

// suppressBanner hides the transient flip card without touching the ambient
// tint. Callers that already show mode_switch events somewhere else (e.g. the
// multi-column Event Log) pass true so the user does not see the same chip
// twice. The tint stays on because it is context, not animation.
const props = defineProps<{ suppressBanner?: boolean }>();

interface BannerInfo {
  id: number;
  coreId: number;
  from: 'user' | 'kernel';
  to: 'user' | 'kernel';
  cause: string;
}

const banner = ref<BannerInfo | null>(null);
const tintRef = ref<HTMLElement | null>(null);
const bannerRef = ref<HTMLElement | null>(null);
let seq = 0;

narrators.onModeSwitch((ev: ModeSwitch) => {
  // Respect the SimBar filter: do not announce flips on hidden cores.
  if (!cpu.isCoreVisible(ev.coreId)) return;
  // At very high effective rates we skip the narrator entirely — see the
  // narratorMinTickMs preference.
  if (!animator.shouldFire()) return;

  seq++;
  const myId = seq;
  banner.value = { id: myId, coreId: ev.coreId, from: ev.from, to: ev.to, cause: ev.cause };

  // Brightness pulse on the tint, mirrors the legacy 450ms mf-pulse keyframes
  // but stretched/compressed to one simulator tick so it stays in sync with
  // the event.
  if (tintRef.value) {
    animator.play(
      tintRef.value,
      [
        { filter: 'brightness(1) saturate(1)', offset: 0 },
        { filter: 'brightness(1.9) saturate(1.4)', offset: 0.35 },
        { filter: 'brightness(1) saturate(1)', offset: 1 },
      ],
      { fill: 'none' },
    );
  }

  // Banner dwell: animate an invisible opacity hold for one tick so we get a
  // pause/resume-aware timer "for free". rAF defers until Vue mounts the v-if
  // wrapper and bannerRef is populated.
  requestAnimationFrame(() => {
    if (banner.value?.id !== myId || !bannerRef.value) return;
    const anim = animator.play(
      bannerRef.value,
      [{ opacity: 1 }, { opacity: 1 }],
      { fill: 'forwards' },
    );
    if (!anim) {
      banner.value = null;
      return;
    }
    anim.finished
      .then(() => { if (banner.value?.id === myId) banner.value = null; })
      .catch(() => { /* cancelled on unmount or rapid follow-up event */ });
  });
});

// cpu.mode is 3 for user, 0 for kernel. Map to a readable class hook.
const modeName = computed<'user' | 'kernel'>(() => (cpu.mode === 3 ? 'user' : 'kernel'));

// Boot idle (no PID on any core, not running) is neutral — no halo.
const showTint = computed(() => cpu.running || cpu.cores.some(c => c.current_pid > 0));

function label(m: 'user' | 'kernel') {
  return m === 'user' ? 'USER' : 'KERNEL';
}
</script>

<template>
  <div
    :class="[
      'mode-flip',
      showTint ? `mode-flip--${modeName}` : 'mode-flip--idle',
    ]"
    aria-hidden="true"
  >
    <div ref="tintRef" class="mode-flip__tint"></div>

    <Transition name="mf-banner">
      <div
        v-if="banner && !props.suppressBanner"
        ref="bannerRef"
        :key="banner.id"
        class="mode-flip__banner"
        :style="{ '--core-color': `var(--core-${banner.coreId % 4})`, '--core-color-fg': `var(--core-fg)` }"
      >
        <span v-if="cpu.numCores > 1" class="mode-flip__core">C{{ banner.coreId }}</span>
        <span class="mode-flip__chip mode-flip__chip--from">{{ label(banner.from) }}</span>
        <span class="mode-flip__arrow">→</span>
        <span class="mode-flip__chip mode-flip__chip--to">{{ label(banner.to) }}</span>
        <span class="mode-flip__cause">{{ banner.cause }}</span>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.mode-flip {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: var(--z-overlay);
  overflow: hidden;
}

.mode-flip__tint {
  position: absolute;
  inset: 0;
  transition: background var(--transition-slow), box-shadow var(--transition-slow);
}

.mode-flip--user .mode-flip__tint {
  background:
    radial-gradient(circle at top right, rgba(106, 212, 255, 0.05), transparent 55%),
    radial-gradient(circle at bottom left, rgba(106, 212, 255, 0.03), transparent 60%);
}

.mode-flip--kernel .mode-flip__tint {
  background:
    radial-gradient(circle at top right, rgba(var(--accent-rgb), 0.12), transparent 55%),
    radial-gradient(circle at bottom left, rgba(var(--accent-rgb), 0.07), transparent 60%);
  box-shadow: inset 0 0 0 1px rgba(var(--accent-rgb), 0.22), inset 0 0 38px rgba(var(--accent-rgb), 0.14);
}

.mode-flip__banner {
  position: absolute;
  /* Clears the middle-tabs row in Simulator.vue (layout pad 8 + tab height
     ~32 + gap). At 12px it landed directly on the EVENT LOG tab label. */
  top: 56px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 6px 14px;
  background: var(--surface-2);
  border: 1px solid var(--amber);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-glow-amber-strong);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  letter-spacing: 1px;
}

.mode-flip__chip {
  padding: 1px 6px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  font-weight: 700;
}

.mode-flip__chip--from { color: var(--text-dim); }
.mode-flip__chip--to   { color: var(--amber); text-shadow: var(--shadow-glow-amber); }

.mode-flip__core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  background: var(--core-color, var(--amber));
  color: var(--core-color-fg, var(--core-fg, #0e0e10));
  border: 1px solid var(--core-color, var(--amber));
  margin-right: 2px;
}

.mode-flip__arrow {
  color: var(--text-dim);
  font-size: 14px;
}

.mode-flip__cause {
  color: var(--text-dim);
  font-size: 10px;
  text-transform: uppercase;
  padding-left: var(--space-2);
  border-left: 1px solid var(--border);
}

.mf-banner-enter-from {
  opacity: 0;
  transform: translate(-50%, -10px) scale(0.95);
}
.mf-banner-leave-to {
  opacity: 0;
  transform: translate(-50%, -4px);
}
.mf-banner-enter-active,
.mf-banner-leave-active {
  transition: opacity var(--transition-base), transform var(--transition-base);
}

@media (prefers-reduced-motion: reduce) {
  .mode-flip__tint,
  .mf-banner-enter-active,
  .mf-banner-leave-active { transition-duration: 0.001ms !important; }
}
</style>
