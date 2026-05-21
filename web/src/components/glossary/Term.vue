<script setup lang="ts">
import { computed, ref, onBeforeUnmount, watch } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import { findEntry } from '../../data/glossary';
import GlossaryTooltip from './GlossaryTooltip.vue';

const props = withDefaults(
  defineProps<{
    term: string;
    mode?: 'inline' | 'label' | 'title';
    disabled?: boolean;
    as?: 'span' | 'code' | 'strong' | 'em';
    text?: string;
  }>(),
  { mode: 'inline', disabled: false, as: 'span' },
);

const cpu = useCpuStore();
const entry = computed(() => findEntry(props.term));
const interactive = computed(() => !props.disabled && entry.value !== null);

if (import.meta.env.DEV && entry.value === null) {
  console.warn(`[Term] No glossary entry for "${props.term}"`);
}

const triggerRef = ref<HTMLElement | null>(null);
const localHovered = ref(false);
let hoverTimer = 0;
const HOVER_DELAY_MS = 180;

// Cross-shell tooltip: when a viewer is connected and the owner hovers a
// <Term> with this exact term, light it up here too. Reading from the cpu
// store keeps the subscription centralized — every Term shares the same
// upstream listener instead of N parallel cpu.subscribe() calls.
const isViewer = computed(() => cpu.sessionInfo.role === 'viewer');
const remoteHovered = computed(() => isViewer.value && cpu.ownerGlossaryHover === props.term);
const hovered = computed(() => localHovered.value || remoteHovered.value);

const tipId = computed(() => {
  const slug = props.term.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  return `term-tip-${slug}`;
});

function openTip(immediate = false): void {
  if (!interactive.value) return;
  window.clearTimeout(hoverTimer);
  if (immediate) {
    localHovered.value = true;
    cpu.sendGlossaryHover(props.term);
    return;
  }
  hoverTimer = window.setTimeout(() => {
    localHovered.value = true;
    cpu.sendGlossaryHover(props.term);
  }, HOVER_DELAY_MS);
}

function closeTip(): void {
  window.clearTimeout(hoverTimer);
  if (localHovered.value) {
    cpu.sendGlossaryHoverOff();
  }
  localHovered.value = false;
}

// If the owner opens a tooltip on the same term locally (mouseenter), the
// remote-driven render goes back to mirroring the local state. No special
// handling needed — the computed already covers it.
// Watcher keeps the subscription tidy: when the cross-shell hover ends,
// we do not need to clean local hover state.
watch(remoteHovered, () => { /* reactive consumer */ });

function onClick(): void {
  if (!interactive.value) return;
  cpu.openGlossary(props.term);
  closeTip();
}

function onKey(e: KeyboardEvent): void {
  if (!interactive.value) return;
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault();
    onClick();
  } else if (e.key === 'Escape') {
    closeTip();
  }
}

onBeforeUnmount(() => {
  window.clearTimeout(hoverTimer);
});
</script>

<template>
  <component
    :is="as"
    ref="triggerRef"
    :class="['term', `term--${mode}`, interactive ? 'term--interactive' : 'term--inert']"
    :tabindex="interactive ? 0 : undefined"
    :role="interactive ? 'button' : undefined"
    :aria-describedby="interactive && hovered ? tipId : undefined"
    :aria-haspopup="interactive ? 'dialog' : undefined"
    @mouseenter="openTip(false)"
    @mouseleave="closeTip"
    @focus="openTip(true)"
    @blur="closeTip"
    @click="onClick"
    @keydown="onKey"
  ><slot>{{ text ?? term }}</slot><GlossaryTooltip
      v-if="hovered && entry"
      :anchor="triggerRef"
      :entry="entry"
      :id="tipId"
      @close="closeTip"
    /></component>
</template>

<style scoped>
.term {
  transition: color var(--transition-fast),
              text-shadow var(--transition-fast),
              outline-color var(--transition-fast),
              border-color var(--transition-fast);
}

.term--inert {
  /* No entry → render as plain text. */
  cursor: inherit;
}

.term--interactive {
  cursor: help;
}

/* Inline: dashed underline cyan; hover amber + glow. For terms inside prose. */
.term--inline.term--interactive {
  border-bottom: 1px dashed var(--cyan);
}

.term--inline.term--interactive:hover,
.term--inline.term--interactive:focus-visible {
  color: var(--amber);
  border-bottom-color: var(--amber);
  text-shadow: 0 0 6px rgba(var(--accent-rgb), 0.35);
}

/* Label: invisible dashed outline that lights up on hover. Mirrors the
   existing .reg-row.clickable pattern in CpuPanel. For table headers, pill
   labels, and other compact labels where an underline would feel busy. */
.term--label.term--interactive {
  outline: 1px dashed transparent;
  outline-offset: 2px;
  padding: 0 1px;
}

.term--label.term--interactive:hover,
.term--label.term--interactive:focus-visible {
  outline-color: var(--cyan);
  color: var(--cyan);
}

/* Title: keep the typography intact; underline only on hover so the headline
   composition is not broken by a permanent decoration. */
.term--title.term--interactive {
  border-bottom: 1px dashed transparent;
}

.term--title.term--interactive:hover,
.term--title.term--interactive:focus-visible {
  color: var(--amber);
  border-bottom-color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}
</style>
