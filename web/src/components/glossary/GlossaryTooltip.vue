<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue';
import type { GlossaryEntry } from '../../data/glossary';

const props = defineProps<{
  entry: GlossaryEntry;
  anchor: HTMLElement | null;
  id: string;
}>();

const emit = defineEmits<{ (e: 'close'): void }>();

const tipRef = ref<HTMLElement | null>(null);
const x = ref(0);
const y = ref(0);
const ready = ref(false);

const GAP = 8;
const MARGIN = 8;

function measure(): void {
  const anchor = props.anchor;
  const tip = tipRef.value;
  if (!anchor || !tip) return;
  const a = anchor.getBoundingClientRect();
  const t = tip.getBoundingClientRect();
  const vw = window.innerWidth;
  const vh = window.innerHeight;

  // Default: place above the anchor, centered horizontally.
  let top = a.top - t.height - GAP;
  if (top < MARGIN) {
    // Flip below if no room above.
    top = a.bottom + GAP;
  }
  // Last-resort clamp to keep the tip on screen.
  if (top + t.height > vh - MARGIN) top = vh - t.height - MARGIN;
  if (top < MARGIN) top = MARGIN;

  let left = a.left + a.width / 2 - t.width / 2;
  left = Math.max(MARGIN, Math.min(left, vw - t.width - MARGIN));

  x.value = left;
  y.value = top;
  ready.value = true;
}

function onScrollOrResize(): void {
  // Closing on scroll is simpler than chasing the anchor and avoids
  // visually-stale tooltips. The user just re-hovers.
  emit('close');
}

function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close');
}

onMounted(() => {
  nextTick(measure);
  window.addEventListener('scroll', onScrollOrResize, { passive: true, capture: true });
  window.addEventListener('resize', onScrollOrResize);
  window.addEventListener('keydown', onKey);
});

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScrollOrResize, { capture: true } as EventListenerOptions);
  window.removeEventListener('resize', onScrollOrResize);
  window.removeEventListener('keydown', onKey);
});

watch(() => props.entry, () => nextTick(measure));

function displayTerm(t: string): string {
  return t.split('—')[0].trim().toUpperCase();
}
</script>

<template>
  <Teleport to="body">
    <div
      :id="id"
      ref="tipRef"
      class="g-tip"
      :class="{ 'g-tip--ready': ready }"
      role="tooltip"
      :style="{ left: `${x}px`, top: `${y}px` }"
    >
      <div class="g-tip__head">
        <span class="g-tip__term">{{ displayTerm(entry.term) }}</span>
        <span class="g-tip__cat">{{ entry.category }}</span>
      </div>
      <p class="g-tip__short">{{ entry.short }}</p>
      <div class="g-tip__hint">clic para más info ▸</div>
    </div>
  </Teleport>
</template>

<style scoped>
.g-tip {
  position: fixed;
  z-index: var(--z-overlay);
  max-width: 320px;
  min-width: 180px;
  padding: 10px 12px;
  background: var(--bg-1);
  border: 1px solid var(--amber);
  box-shadow: var(--shadow-glow-amber);
  font-family: 'JetBrains Mono', monospace;
  color: var(--text);
  pointer-events: none;
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.g-tip--ready {
  opacity: 1;
}

.g-tip__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 4px;
  margin-bottom: 6px;
  border-bottom: 1px dashed var(--border-strong);
}

.g-tip__term {
  font-family: var(--font-display);
  font-size: 12px;
  color: var(--amber);
  letter-spacing: 2px;
  text-transform: uppercase;
}

.g-tip__cat {
  font-size: 10px;
  color: var(--cyan);
  letter-spacing: 1px;
  text-transform: uppercase;
}

.g-tip__short {
  font-size: 12px;
  line-height: 1.5;
  color: var(--text);
  font-style: italic;
  margin: 0;
}

.g-tip__hint {
  margin-top: 8px;
  font-size: 10px;
  color: var(--text-muted);
  text-align: right;
  letter-spacing: 0.5px;
}
</style>
