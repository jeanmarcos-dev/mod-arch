<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { MousePointer2 } from '@lucide/vue';
import { useWhiteboardStore } from '../../stores/whiteboard';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';

// WhiteboardCursor renders the owner's pointer as an overlay on viewer
// clients. Both AppShell (host view, no-op render) and ViewerShell mount it
// so the underlying cpu WS subscription is active wherever the user lands.
// The render only paints when both conditions hold: the latest pointer
// payload exists AND its route matches the viewer's current path. Otherwise
// rendering is a no-op so the overlay is cheap to keep on screen.

const cpu = useCpuStore();
const whiteboard = useWhiteboardStore();
const route = useRoute();

const viewport = ref({ w: 0, h: 0 });

function refreshViewport() {
  viewport.value = {
    w: window.innerWidth,
    h: window.innerHeight,
  };
}

// Owner identity for the label. Falls back to a generic "Owner" tag when the
// viewer cannot resolve it (e.g. anonymous viewer of a public share).
const ownerLabel = computed(() => {
  const name = cpu.sessionInfo.user_name;
  return name && name.length > 0 ? name : 'Owner';
});

const visible = computed<boolean>(() => {
  const p = whiteboard.ownerPointer;
  if (!p) return false;
  // Only show the cursor when the viewer is currently looking at the same
  // route the owner is broadcasting from. Cross-route visualization would be
  // confusing (a cursor floating over the wrong section).
  if (p.route !== route.path) return false;
  // If the viewer is the owner themselves, do not echo back. The role check
  // is enough — only owners send pointer commands in the first place.
  if (cpu.sessionInfo.role === 'owner') return false;
  return true;
});

const style = computed(() => {
  const p = whiteboard.ownerPointer;
  if (!p) return undefined;
  return {
    transform: `translate(${p.x * viewport.value.w}px, ${p.y * viewport.value.h}px)`,
  } as Record<string, string>;
});

// Wire the owner_pointer event from the cpu WS stream into the whiteboard
// store. Done here (not in the cpu store) so the dependency lives close to
// the consumer; removing the WhiteboardCursor component cleans up the
// subscription automatically.
let unsubscribe: () => void = () => { /* placeholder */ };

onMounted(() => {
  refreshViewport();
  window.addEventListener('resize', refreshViewport);
  unsubscribe = cpu.subscribe((ev: CpuEvent) => {
    if (ev.type === 'owner_pointer') {
      whiteboard.setOwnerPointer(ev);
    }
  });
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', refreshViewport);
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
