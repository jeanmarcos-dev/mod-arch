<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';

// WhiteboardClickRipple renders the owner's clicks as a short pulse on the
// viewer's screen. Companion to WhiteboardCursor: the cursor shows where the
// host's mouse is, the ripple shows where they just clicked. Each ripple is a
// DOM-anchored circle that grows and fades over 600ms, then unmounts itself.
// Mounted by both AppShell (no-op for the host) and ViewerShell (the actual
// audience). Cheap to keep around — the list is empty until an event lands.

const cpu = useCpuStore();
const route = useRoute();

interface Ripple {
  id: number;
  anchor: string;
  offsetX: number;
  offsetY: number;
  route: string;
  // Cached layout snapshot at ripple time; we re-read on resize/scroll too
  // so the pulse stays glued to the same anchor while the audience scrolls.
  left: number;
  top: number;
}

const RIPPLE_MS = 600;
const ripples = ref<Ripple[]>([]);
let nextId = 0;
const layoutTick = ref(0);
function bumpLayout() { layoutTick.value = (layoutTick.value + 1) & 0xffff; }

function anchorPosition(anchor: string, offsetX: number, offsetY: number): { left: number; top: number } | null {
  const host = document.querySelector<HTMLElement>(`[data-pizarra-anchor="${anchor}"]`);
  if (!host) return null;
  const rect = host.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  return {
    left: rect.left + offsetX * rect.width,
    top: rect.top + offsetY * rect.height,
  };
}

function pushRipple(ev: { anchor?: string; offset_x?: number; offset_y?: number; route?: string }) {
  if (typeof ev.anchor !== 'string' || typeof ev.offset_x !== 'number' || typeof ev.offset_y !== 'number') return;
  if (typeof ev.route !== 'string') return;
  if (ev.route !== route.path) return;
  if (cpu.sessionInfo.role === 'owner') return; // No self-echo.
  const pos = anchorPosition(ev.anchor, ev.offset_x, ev.offset_y);
  if (!pos) return;
  const id = nextId++;
  ripples.value.push({
    id,
    anchor: ev.anchor,
    offsetX: ev.offset_x,
    offsetY: ev.offset_y,
    route: ev.route,
    left: pos.left,
    top: pos.top,
  });
  window.setTimeout(() => {
    ripples.value = ripples.value.filter((r) => r.id !== id);
  }, RIPPLE_MS + 50);
}

let unsubscribe: () => void = () => { /* placeholder */ };

function onLayoutChange() {
  // Recompute every active ripple's screen coords; cheap since at most a few
  // are live at once.
  if (ripples.value.length === 0) return;
  ripples.value = ripples.value.map((r) => {
    const pos = anchorPosition(r.anchor, r.offsetX, r.offsetY);
    return pos ? { ...r, left: pos.left, top: pos.top } : r;
  });
  bumpLayout();
}

onMounted(() => {
  unsubscribe = cpu.subscribe((ev: CpuEvent) => {
    if (ev.type === 'owner_click') pushRipple(ev);
  });
  window.addEventListener('resize', onLayoutChange);
  window.addEventListener('scroll', onLayoutChange, true);
});

onBeforeUnmount(() => {
  unsubscribe();
  window.removeEventListener('resize', onLayoutChange);
  window.removeEventListener('scroll', onLayoutChange, true);
});
</script>

<template>
  <Teleport to="body">
    <div class="wb-ripple-layer" aria-hidden="true">
      <span
        v-for="r in ripples"
        :key="r.id"
        class="wb-ripple"
        :style="{ left: r.left + 'px', top: r.top + 'px' }"
      />
    </div>
  </Teleport>
</template>

<style scoped>
.wb-ripple-layer {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: var(--z-overlay);
}

.wb-ripple {
  position: absolute;
  left: 0;
  top: 0;
  width: 0;
  height: 0;
  border-radius: 50%;
  border: 2px solid var(--accent);
  background: rgba(var(--accent-rgb), 0.18);
  transform: translate(-50%, -50%) scale(0.2);
  opacity: 1;
  animation: wb-ripple-grow 600ms ease-out forwards;
  filter: drop-shadow(0 0 8px rgba(var(--accent-rgb), 0.5));
}

@keyframes wb-ripple-grow {
  0%   { width: 0;     height: 0;     opacity: 0.9; transform: translate(-50%, -50%) scale(0.2); }
  60%  { width: 56px;  height: 56px;  opacity: 0.6; transform: translate(-50%, -50%) scale(1.0); }
  100% { width: 88px;  height: 88px;  opacity: 0;   transform: translate(-50%, -50%) scale(1.3); }
}

@media (prefers-reduced-motion: reduce) {
  .wb-ripple { animation-duration: 1ms; opacity: 0; }
}
</style>
