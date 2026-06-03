<script setup lang="ts">
import { onUnmounted, ref, watch, computed } from 'vue';
import { useNarrators, type ModeSwitch } from '../../lib/narratorBus';
import { useCpuStore } from '../../stores/cpu';
import { useNarratorAnimation } from './useNarratorAnimation';

// TrapFrameNarrator animates what the x86 HW does on every IRQ entry and
// IRET: push (or pop) EFLAGS/CS/EIP of the trapped process between the
// CPU register file and the kernel stack. Three small cards fly between
// the [data-narrator-anchor="cpu-N"] of the affected core and the
// [data-narrator-anchor="memory-panel"] target — staggered so the alumno
// can read them as a sequence. Used together with ModeFlipNarrator: that
// one tells the alumno "the mode flipped"; this one tells them "and these
// are the three things the HW stashed for safekeeping".

const props = withDefaults(
  defineProps<{
    // Gate animations to the visibility of the memory panel. When false the
    // bus event is observed but no card is materialised: the destination
    // anchor lives inside MemoryPanel (v-show), and its bounding rect is
    // collapsed to (0,0) while hidden — cards would otherwise fly off-screen.
    enabled?: boolean;
  }>(),
  { enabled: true },
);

const cpu = useCpuStore();
const animator = useNarratorAnimation();

interface Card {
  id: number;
  label: 'EFLAGS' | 'CS' | 'EIP';
  value: string;
  coreId: number;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  kind: 'push' | 'pop';
  delayMs: number;
}

const cards = ref<Card[]>([]);
const root = ref<HTMLElement | null>(null);

function rectCenter(el: Element | null): { x: number; y: number } | null {
  if (!el || !root.value) return null;
  const local = root.value.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2 - local.left, y: r.top + r.height / 2 - local.top };
}

function hex(n: number, w = 8): string {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

let seq = 0;

function launch(kind: 'push' | 'pop', ev: ModeSwitch) {
  if (!props.enabled) return;
  if (!cpu.isCoreVisible(ev.coreId)) return;
  if (!animator.shouldFire()) return;
  const cpuEl = document.querySelector(`[data-narrator-anchor="cpu-${ev.coreId}"]`)
    ?? document.querySelector(`[data-narrator-anchor="cpu"]`);
  const memEl = document.querySelector(`[data-narrator-anchor="memory-panel"]`);
  if (!cpuEl || !memEl) return;
  const a = rectCenter(cpuEl);
  const b = rectCenter(memEl);
  if (!a || !b) return;

  const c = cpu.cores.find(x => x.id === ev.coreId);
  const regs = c?.regs ?? {};
  // Order matches the x86 push order: EFLAGS, then CS, then EIP. The pop
  // (IRET) order is reverse: EIP, CS, EFLAGS — we reverse the delays so
  // the cards fly out in the opposite sequence.
  const items: Array<{ name: 'EIP' | 'CS' | 'EFLAGS'; val: number; w: number }> = [
    { name: 'EFLAGS', val: regs.EFLAGS ?? 0, w: 8 },
    { name: 'CS',     val: regs.CS     ?? 0, w: 4 },
    { name: 'EIP',    val: regs.EIP    ?? 0, w: 8 },
  ];

  for (let i = 0; i < 3; i++) {
    const id = ++seq;
    const idx = kind === 'push' ? i : 2 - i;
    const item = items[idx];
    cards.value.push({
      id,
      label: item.name,
      value: hex(item.val, item.w),
      coreId: ev.coreId,
      fromX: kind === 'push' ? a.x : b.x,
      fromY: kind === 'push' ? a.y + i * 14 : b.y + i * 14,
      toX: kind === 'push' ? b.x : a.x,
      toY: kind === 'push' ? b.y + i * 14 : a.y + i * 14,
      kind,
      delayMs: i * 80,
    });

    const dummy = document.createElement('div');
    const anim = animator.play(dummy, [{ opacity: 0 }, { opacity: 0 }], { fill: 'none' });
    if (anim) {
      anim.finished
        .then(() => { cards.value = cards.value.filter(c => c.id !== id); })
        .catch(() => { /* cancelled */ });
    }
  }
}

const narrators = useNarrators();
narrators.onModeSwitch((ev: ModeSwitch) => {
  if (ev.from === 'user' && ev.to === 'kernel') launch('push', ev);
  else if (ev.cause === 'IRET') launch('pop', ev);
});

watch(
  () => cpu.paused,
  (p) => {
    if (root.value) root.value.style.setProperty('--anim-state', p ? 'paused' : 'running');
  },
  { immediate: true },
);

const ttlCss = computed(() => `${animator.tickMs.value}ms`);

onUnmounted(() => { cards.value = []; });
</script>

<template>
  <div ref="root" class="trap-frame-narrator" aria-hidden="true">
    <div
      v-for="c in cards"
      :key="c.id"
      :class="['trap-card', `trap-card--${c.kind}`]"
      :style="{
        '--from-x': `${c.fromX}px`,
        '--from-y': `${c.fromY}px`,
        '--to-x':   `${c.toX}px`,
        '--to-y':   `${c.toY}px`,
        '--ttl':    ttlCss,
        '--core-color': `var(--core-${c.coreId % 4})`,
        '--delay':  `${c.delayMs}ms`,
      }"
    >
      <span class="trap-card__verb">{{ c.kind === 'push' ? 'PUSH' : 'POP' }}</span>
      <span class="trap-card__label">{{ c.label }}</span>
      <span class="trap-card__val">{{ c.value }}</span>
    </div>
  </div>
</template>

<style scoped>
.trap-frame-narrator {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
  z-index: var(--z-overlay);
}

.trap-card {
  position: absolute;
  left: 0;
  top: 0;
  padding: 3px 6px;
  background: var(--surface-2);
  border: 1px solid var(--amber);
  border-radius: 3px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--amber);
  white-space: nowrap;
  transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%));
  animation: trap-fly var(--ttl) cubic-bezier(0.25, 0.5, 0.25, 1) forwards;
  animation-delay: var(--delay);
  animation-play-state: var(--anim-state, running);
  opacity: 0;
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.4);
}

.trap-card--pop {
  border-color: var(--green);
  color: var(--green);
  box-shadow: 0 0 8px color-mix(in oklab, var(--green) 40%, transparent);
}

.trap-card__verb {
  margin-right: 6px;
  font-size: 8px;
  letter-spacing: 1px;
  opacity: 0.75;
}

.trap-card__label {
  margin-right: 6px;
  font-weight: 700;
}

.trap-card__val {
  font-variant-numeric: tabular-nums;
}

@keyframes trap-fly {
  0%   { transform: translate(calc(var(--from-x) - 50%), calc(var(--from-y) - 50%)); opacity: 0; }
  15%  { opacity: 1; }
  85%  { opacity: 1; }
  100% { transform: translate(calc(var(--to-x) - 50%), calc(var(--to-y) - 50%));     opacity: 0; }
}
</style>
