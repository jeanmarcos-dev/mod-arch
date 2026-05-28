<script setup lang="ts">
import { onMounted, ref } from 'vue';

withDefaults(
  defineProps<{ size?: number; strokeWidth?: number }>(),
  { size: 24, strokeWidth: 1.8 },
);

const root = ref<SVGSVGElement | null>(null);

// Some browsers / mount conditions skip SMIL auto-start for SVGs that
// appear after the initial document load (e.g. when Vue mounts the icon
// inside a v-if branch). beginElement() restarts every animateTransform
// inside this instance to guarantee the rotation/blink are running.
onMounted(() => {
  const svg = root.value;
  if (!svg) return;
  const anims = svg.querySelectorAll('animateTransform') as unknown as NodeListOf<SVGAnimationElement & { beginElement?: () => void }>;
  anims.forEach((a) => {
    try { a.beginElement?.(); } catch { /* SMIL unsupported — fallback to declarative begin="0s" */ }
  });
});
</script>

<template>
  <svg
    ref="root"
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="strokeWidth"
    stroke-linecap="round"
    stroke-linejoin="round"
    class="tutor-bot-icon"
    aria-hidden="true"
  >
    <path d="M12 6V2H8" />

    <!-- Right eye: drawn at local origin (0,0), positioned to (15,12) via
         translate. Additive scale collapses around the eye's own center. -->
    <g transform="translate(15 12)">
      <path d="M0 -1v2" />
      <animateTransform
        attributeName="transform"
        attributeType="XML"
        type="scale"
        additive="sum"
        values="1 1; 1 1; 1 0.05; 1 1; 1 1"
        keyTimes="0; 0.90; 0.94; 0.98; 1"
        dur="4.5s"
        begin="0s"
        repeatCount="indefinite"
        fill="freeze"
      />
    </g>

    <path d="M2 12h2" />
    <path d="M20 12h2" />
    <path d="M20 16a2 2 0 0 1-2 2H8.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 4 20.286V8a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2z" />

    <!-- Left eye: same blink as right eye, positioned at (9,12). -->
    <g transform="translate(9 12)">
      <path d="M0 -1v2" />
      <animateTransform
        attributeName="transform"
        attributeType="XML"
        type="scale"
        additive="sum"
        values="1 1; 1 1; 1 0.05; 1 1; 1 1"
        keyTimes="0; 0.90; 0.94; 0.98; 1"
        dur="4.5s"
        begin="0s"
        repeatCount="indefinite"
        fill="freeze"
      />
    </g>
  </svg>
</template>
