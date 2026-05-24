<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { MousePointer2 } from '@lucide/vue';
import { useWhiteboardStore } from '../../stores/whiteboard';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';

// WhiteboardCursor renders the owner's pointer as an overlay on viewer
// clients. Both AppShell (host view, no-op render) and ViewerShell mount it
// so the underlying cpu WS subscription is active wherever the user lands.
// The render only paints when the pointer payload exists, its route matches
// the viewer's current path, and the named anchor exists in the local DOM.

const cpu = useCpuStore();
const whiteboard = useWhiteboardStore();
const route = useRoute();

// anchorTick increments whenever the anchor host might have moved or
// resized (window resize, scroll, route change). It forces the position
// computed below to re-read getBoundingClientRect — Vue cannot observe
// layout mutations directly so we drive the re-read with a reactive nonce.
const anchorTick = ref(0);
function bumpAnchorTick() {
  anchorTick.value = (anchorTick.value + 1) & 0xffff;
}

// Owner identity for the label. Falls back to a generic "Owner" tag when the
// viewer cannot resolve it (e.g. anonymous viewer of a public share).
const ownerLabel = computed(() => {
  const name = cpu.sessionInfo.user_name;
  return name && name.length > 0 ? name : 'Owner';
});

interface Positioned {
  x: number;
  y: number;
}

const positioned = computed<Positioned | null>(() => {
  // anchorTick.value is read here purely as a reactive dependency so the
  // computed re-runs after resize/scroll/route changes (see bumpAnchorTick).
  void anchorTick.value;
  const p = whiteboard.ownerPointer;
  if (!p) return null;
  if (p.route !== route.path) return null;
  if (cpu.sessionInfo.role === 'owner') return null;
  const host = document.querySelector<HTMLElement>(`[data-pizarra-anchor="${p.anchor}"]`);
  if (!host) return null;
  const rect = host.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  return {
    x: rect.left + p.offsetX * rect.width,
    y: rect.top + p.offsetY * rect.height,
  };
});

const visible = computed<boolean>(() => positioned.value !== null);

const style = computed(() => {
  const pos = positioned.value;
  if (!pos) return undefined;
  return {
    transform: `translate(${pos.x}px, ${pos.y}px)`,
  } as Record<string, string>;
});

// Wire the owner_pointer event from the cpu WS stream into the whiteboard
// store. Done here (not in the cpu store) so the dependency lives close to
// the consumer; removing the WhiteboardCursor component cleans up the
// subscription automatically.
let unsubscribe: () => void = () => { /* placeholder */ };
let bumpRaf = 0;
let resizeObserver: ResizeObserver | null = null;

function scheduleBump() {
  if (bumpRaf !== 0) return;
  bumpRaf = window.requestAnimationFrame(() => {
    bumpRaf = 0;
    bumpAnchorTick();
  });
}

onMounted(() => {
  window.addEventListener('resize', scheduleBump);
  // orientationchange fires on phone rotation. Modern browsers also emit
  // resize but the two come at slightly different times, so listening to
  // both keeps the cursor pinned during the in-between frames.
  window.addEventListener('orientationchange', scheduleBump);
  // Scroll inside any internal panel can move an anchor; capturing is needed
  // because individual panels scroll, not the window. The listener is cheap
  // (only bumps a counter) so the overhead is negligible.
  window.addEventListener('scroll', bumpAnchorTick, true);
  // ResizeObserver covers viewport size changes that don't fire `resize`,
  // such as the iOS Safari toolbar collapsing as the user scrolls.
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(scheduleBump);
    resizeObserver.observe(document.documentElement);
  }
  unsubscribe = cpu.subscribe((ev: CpuEvent) => {
    if (ev.type === 'owner_pointer') {
      whiteboard.setOwnerPointer(ev);
    }
  });
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', scheduleBump);
  window.removeEventListener('orientationchange', scheduleBump);
  window.removeEventListener('scroll', bumpAnchorTick, true);
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  if (bumpRaf !== 0) {
    window.cancelAnimationFrame(bumpRaf);
    bumpRaf = 0;
  }
  unsubscribe();
  whiteboard.reset();
});
</script>

<template>
  <div v-if="visible" class="wb-cursor" :style="style" aria-hidden="true">
    <MousePointer2 :size="22" :stroke-width="2" class="wb-cursor__icon" />
    <span class="wb-cursor__label">{{ ownerLabel }}</span>
  </div>
</template>

<style scoped>
.wb-cursor {
  position: fixed;
  left: 0;
  top: 0;
  /* The icon's tip aligns with (0, 0) of the wrapper, so the translate sets
     the tip exactly at the broadcast coordinate without an offset hack. */
  pointer-events: none;
  z-index: var(--z-overlay);
  color: var(--accent);
  filter: drop-shadow(0 0 6px rgba(var(--accent-rgb), 0.55));
  transition: transform 80ms linear;
  will-change: transform;
}

.wb-cursor__icon {
  display: block;
}

.wb-cursor__label {
  display: inline-block;
  margin-top: 2px;
  margin-left: 16px;
  padding: 2px 6px;
  background: var(--surface-2);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  font-family: var(--font-body);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--accent);
  white-space: nowrap;
}
</style>
