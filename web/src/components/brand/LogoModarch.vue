<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  size?: number | string
  showWordmark?: boolean
  glow?: boolean
  ariaLabel?: string
}

const props = withDefaults(defineProps<Props>(), {
  showWordmark: true,
  glow: true,
  ariaLabel: 'MODARCH',
})

const viewBox = computed(() => (props.showWordmark ? '0 0 720 360' : '0 0 720 270'))

const sizeStyle = computed(() => {
  if (props.size == null) return undefined
  const v = typeof props.size === 'number' ? `${props.size}px` : props.size
  return { width: v, height: 'auto' }
})
</script>

<template>
  <svg
    class="logo-modarch"
    :class="{ 'logo-modarch--glow': glow }"
    :viewBox="viewBox"
    :style="sizeStyle"
    xmlns="http://www.w3.org/2000/svg"
    role="img"
    :aria-label="ariaLabel"
  >
    <defs>
      <filter id="logoGlow" x="-20%" y="-20%" width="140%" height="140%">
        <feGaussianBlur stdDeviation="1.2" result="b" />
        <feMerge>
          <feMergeNode in="b" />
          <feMergeNode in="SourceGraphic" />
        </feMerge>
      </filter>
      <filter id="logoGlowSoft" x="-20%" y="-20%" width="140%" height="140%">
        <feGaussianBlur stdDeviation="2.4" result="b" />
        <feMerge>
          <feMergeNode in="b" />
          <feMergeNode in="SourceGraphic" />
        </feMerge>
      </filter>
    </defs>

    <g class="lm-buses" fill="none" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" filter="url(#logoGlow)">
      <path d="M 270 80 L 220 80 L 220 58 L 110 58" class="lm-bus-green" />
      <path d="M 450 80 L 500 80 L 500 58 L 610 58" class="lm-bus-amber" />
      <path d="M 270 180 L 220 180 L 220 222 L 110 222" class="lm-bus-green" />
      <path d="M 450 180 L 500 180 L 500 222 L 610 222" class="lm-bus-amber" />
    </g>

    <g class="lm-pads">
      <g class="lm-pad lm-pad--green" filter="url(#logoGlow)">
        <rect x="60" y="42" width="50" height="32" rx="3" fill="none" stroke-width="2" />
        <line x1="72" y1="51" x2="98" y2="51" stroke-width="2" stroke-linecap="round" />
        <line x1="72" y1="58" x2="98" y2="58" stroke-width="2" stroke-linecap="round" />
        <line x1="72" y1="65" x2="98" y2="65" stroke-width="2" stroke-linecap="round" />
      </g>
      <g class="lm-pad lm-pad--amber" filter="url(#logoGlow)">
        <rect x="610" y="42" width="50" height="32" rx="3" fill="none" stroke-width="2" />
        <line x1="622" y1="51" x2="648" y2="51" stroke-width="2" stroke-linecap="round" />
        <line x1="622" y1="58" x2="648" y2="58" stroke-width="2" stroke-linecap="round" />
        <line x1="622" y1="65" x2="648" y2="65" stroke-width="2" stroke-linecap="round" />
      </g>
      <g class="lm-pad lm-pad--green" filter="url(#logoGlow)">
        <rect x="60" y="206" width="50" height="32" rx="3" fill="none" stroke-width="2" />
        <line x1="72" y1="215" x2="98" y2="215" stroke-width="2" stroke-linecap="round" />
        <line x1="72" y1="222" x2="98" y2="222" stroke-width="2" stroke-linecap="round" />
        <line x1="72" y1="229" x2="98" y2="229" stroke-width="2" stroke-linecap="round" />
      </g>
      <g class="lm-pad lm-pad--amber" filter="url(#logoGlow)">
        <rect x="610" y="206" width="50" height="32" rx="3" fill="none" stroke-width="2" />
        <line x1="622" y1="215" x2="648" y2="215" stroke-width="2" stroke-linecap="round" />
        <line x1="622" y1="222" x2="648" y2="222" stroke-width="2" stroke-linecap="round" />
        <line x1="622" y1="229" x2="648" y2="229" stroke-width="2" stroke-linecap="round" />
      </g>
    </g>

    <g class="lm-labels" font-size="11" font-weight="700" letter-spacing="1.5" filter="url(#logoGlow)">
      <text x="85" y="92" text-anchor="middle" class="lm-label--green">MEM</text>
      <text x="85" y="200" text-anchor="middle" class="lm-label--green">STACK</text>
      <text x="635" y="92" text-anchor="middle" class="lm-label--amber">SCHED</text>
      <text x="635" y="200" text-anchor="middle" class="lm-label--amber">IO</text>
    </g>

    <g class="lm-pins" stroke-width="3" stroke-linecap="round" filter="url(#logoGlow)">
      <line x1="300" y1="26" x2="300" y2="40" />
      <line x1="340" y1="26" x2="340" y2="40" />
      <line x1="380" y1="26" x2="380" y2="40" />
      <line x1="420" y1="26" x2="420" y2="40" />
      <line x1="300" y1="220" x2="300" y2="234" />
      <line x1="340" y1="220" x2="340" y2="234" />
      <line x1="380" y1="220" x2="380" y2="234" />
      <line x1="420" y1="220" x2="420" y2="234" />
    </g>

    <rect
      class="lm-chip"
      x="270" y="40" width="180" height="180" rx="6"
      fill="none" stroke-width="3"
      filter="url(#logoGlow)"
    />
    <circle class="lm-notch" cx="284" cy="54" r="3" fill="none" stroke-width="1.4" opacity="0.75" />

    <g class="lm-regs-text" font-size="16" font-weight="500" filter="url(#logoGlow)">
      <text x="298" y="76">R0</text>
      <text x="298" y="106">R1</text>
      <text x="298" y="136">R2</text>
      <text x="298" y="166" font-size="20">:</text>
      <text x="298" y="196">Rn</text>
    </g>
    <g class="lm-regs-bars" filter="url(#logoGlow)">
      <rect x="340" y="68"  width="90" height="6" rx="1" />
      <rect x="340" y="98"  width="90" height="6" rx="1" />
      <rect x="340" y="128" width="90" height="6" rx="1" />
      <rect x="340" y="188" width="90" height="6" rx="1" />
    </g>

    <g v-if="showWordmark" class="lm-wordmark" filter="url(#logoGlowSoft)">
      <text x="360" y="312" text-anchor="middle" font-size="44" font-weight="400" letter-spacing="14">MODARCH</text>
    </g>
  </svg>
</template>

<style scoped>
.logo-modarch {
  display: block;
  height: auto;
  max-width: 100%;
  color: var(--accent);
}

.logo-modarch--glow {
  filter: drop-shadow(0 0 8px rgba(var(--accent-rgb), 0.25));
}

.lm-buses .lm-bus-amber { stroke: var(--accent); }
.lm-buses .lm-bus-green { stroke: var(--green); }

.lm-pad--amber { stroke: var(--accent); }
.lm-pad--green { stroke: var(--green); }

.lm-label--amber { fill: var(--accent); }
.lm-label--green { fill: var(--green); }

.lm-pins { stroke: var(--accent-dim); }

.lm-chip { stroke: var(--accent); }
.lm-notch { stroke: var(--accent); }

.lm-regs-text { fill: var(--accent); font-family: var(--font-body); }
.lm-regs-bars { fill: var(--accent); }

.lm-wordmark text {
  fill: var(--text);
  font-family: var(--font-brand);
}
</style>
