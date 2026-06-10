<script setup lang="ts">
interface Props {
  size?: string
  label?: string
  compact?: boolean
}

withDefaults(defineProps<Props>(), {
  size: '160px',
  label: 'Booting MODARCH…',
  compact: false,
})
</script>

<template>
  <div
    class="loader-modarch"
    :class="{ 'loader-modarch--compact': compact }"
    :style="{ '--lm-size': size }"
    role="status"
    aria-live="polite"
    :aria-label="label"
  >
    <svg
      class="lm-svg"
      viewBox="0 0 240 240"
      xmlns="http://www.w3.org/2000/svg"
      role="presentation"
      aria-hidden="true"
    >
      <defs>
        <filter id="lmGlow" x="-20%" y="-20%" width="140%" height="140%">
          <feGaussianBlur stdDeviation="1.2" result="b" />
          <feMerge>
            <feMergeNode in="b" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>

      <g class="lm-buses" fill="none" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" filter="url(#lmGlow)" pathLength="1">
        <path class="lm-bus lm-bus--amber" d="M 72 88 L 30 88 L 30 30" pathLength="1" />
        <path class="lm-bus lm-bus--green" d="M 168 88 L 210 88 L 210 30" pathLength="1" />
        <path class="lm-bus lm-bus--green" d="M 72 152 L 30 152 L 30 210" pathLength="1" />
        <path class="lm-bus lm-bus--amber" d="M 168 152 L 210 152 L 210 210" pathLength="1" />
      </g>

      <g class="lm-bus-pads" fill="none" stroke-width="1.8" stroke-linecap="square" filter="url(#lmGlow)">
        <rect class="lm-bus-pad lm-bus-pad--amber" x="20" y="20"  width="20" height="14" rx="2" />
        <rect class="lm-bus-pad lm-bus-pad--green" x="200" y="20" width="20" height="14" rx="2" />
        <rect class="lm-bus-pad lm-bus-pad--green" x="20" y="206" width="20" height="14" rx="2" />
        <rect class="lm-bus-pad lm-bus-pad--amber" x="200" y="206" width="20" height="14" rx="2" />
      </g>

      <g class="lm-pins" stroke-width="3" stroke-linecap="round" filter="url(#lmGlow)">
        <line class="lm-pin" x1="92"  y1="60" x2="92"  y2="72" />
        <line class="lm-pin" x1="112" y1="60" x2="112" y2="72" />
        <line class="lm-pin" x1="128" y1="60" x2="128" y2="72" />
        <line class="lm-pin" x1="148" y1="60" x2="148" y2="72" />
        <line class="lm-pin" x1="92"  y1="168" x2="92"  y2="180" />
        <line class="lm-pin" x1="112" y1="168" x2="112" y2="180" />
        <line class="lm-pin" x1="128" y1="168" x2="128" y2="180" />
        <line class="lm-pin" x1="148" y1="168" x2="148" y2="180" />
      </g>

      <rect class="lm-chip" x="72" y="72" width="96" height="96" rx="4" fill="none" stroke-width="2.4" filter="url(#lmGlow)" />
      <circle class="lm-notch" cx="80" cy="80" r="2" fill="none" stroke-width="1.2" opacity="0.7" />

      <g class="lm-regs" filter="url(#lmGlow)">
        <rect class="lm-reg" x="86" y="90"  width="68" height="4" rx="1" />
        <rect class="lm-reg" x="86" y="102" width="68" height="4" rx="1" />
        <rect class="lm-reg" x="86" y="114" width="68" height="4" rx="1" />
        <rect class="lm-reg" x="86" y="126" width="68" height="4" rx="1" />
        <rect class="lm-reg" x="86" y="146" width="68" height="4" rx="1" />
      </g>

      <line class="lm-scan" x1="74" y1="76" x2="166" y2="76" stroke-width="1.6" stroke-linecap="round" />
    </svg>

    <span v-if="!compact" class="lm-label">{{ label }}</span>
  </div>
</template>

<style scoped>
.loader-modarch {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3, 12px);
  color: var(--accent);
  user-select: none;
}

.lm-svg {
  width: var(--lm-size, 160px);
  height: var(--lm-size, 160px);
  display: block;
  filter: drop-shadow(0 0 6px rgba(var(--accent-rgb), 0.22));
}

.lm-label {
  font-family: var(--font-body);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
  opacity: 0.85;
  animation: lm-label-pulse 2.4s var(--ease-out-soft, ease-in-out) infinite;
}

.lm-bus {
  stroke-dasharray: 1;
  stroke-dashoffset: 1;
  animation: lm-bus-draw 2.4s linear infinite;
}
.lm-bus--amber { stroke: var(--accent); }
.lm-bus--green { stroke: var(--green); }

.lm-buses .lm-bus:nth-child(1) { animation-delay: 0s; }
.lm-buses .lm-bus:nth-child(2) { animation-delay: 0.6s; }
.lm-buses .lm-bus:nth-child(3) { animation-delay: 1.2s; }
.lm-buses .lm-bus:nth-child(4) { animation-delay: 1.8s; }

.lm-bus-pad--amber { stroke: var(--accent); }
.lm-bus-pad--green { stroke: var(--green); }
.lm-bus-pads .lm-bus-pad {
  opacity: 0.6;
  animation: lm-pad-blink 2.4s var(--ease-out-soft, ease-in-out) infinite;
}
.lm-bus-pads .lm-bus-pad:nth-child(1) { animation-delay: 0.5s; }
.lm-bus-pads .lm-bus-pad:nth-child(2) { animation-delay: 1.1s; }
.lm-bus-pads .lm-bus-pad:nth-child(3) { animation-delay: 1.7s; }
.lm-bus-pads .lm-bus-pad:nth-child(4) { animation-delay: 2.3s; }

.lm-pins {
  stroke: var(--accent-dim);
}
.lm-pin {
  animation: lm-pin-flow 2.4s var(--ease-out-soft, ease-in-out) infinite;
}
.lm-pins .lm-pin:nth-child(1) { animation-delay: 0s; }
.lm-pins .lm-pin:nth-child(2) { animation-delay: 0.15s; }
.lm-pins .lm-pin:nth-child(3) { animation-delay: 0.3s; }
.lm-pins .lm-pin:nth-child(4) { animation-delay: 0.45s; }
.lm-pins .lm-pin:nth-child(5) { animation-delay: 0.6s; }
.lm-pins .lm-pin:nth-child(6) { animation-delay: 0.75s; }
.lm-pins .lm-pin:nth-child(7) { animation-delay: 0.9s; }
.lm-pins .lm-pin:nth-child(8) { animation-delay: 1.05s; }

.lm-chip {
  stroke: var(--accent);
  animation: lm-chip-pulse 2.4s var(--ease-out-soft, ease-in-out) infinite;
}
.lm-notch {
  stroke: var(--accent);
}

.lm-reg {
  fill: var(--accent);
  opacity: 0;
  transform-origin: left center;
  animation: lm-reg-cascade 2.4s var(--ease-out-soft, ease-in-out) infinite;
}
.lm-regs .lm-reg:nth-child(1) { animation-delay: 0.2s; }
.lm-regs .lm-reg:nth-child(2) { animation-delay: 0.35s; }
.lm-regs .lm-reg:nth-child(3) { animation-delay: 0.5s; }
.lm-regs .lm-reg:nth-child(4) { animation-delay: 0.65s; }
.lm-regs .lm-reg:nth-child(5) { animation-delay: 0.95s; }

.lm-scan {
  stroke: var(--green);
  opacity: 0;
  animation: lm-scanline 2.4s linear infinite;
}

@keyframes lm-bus-draw {
  0%   { stroke-dashoffset: 1;  opacity: 0.15; }
  18%  { stroke-dashoffset: 0;  opacity: 1; }
  55%  { stroke-dashoffset: 0;  opacity: 1; }
  85%  { stroke-dashoffset: -1; opacity: 0; }
  100% { stroke-dashoffset: -1; opacity: 0; }
}

@keyframes lm-pin-flow {
  0%, 100% { opacity: 0.25; }
  50%      { opacity: 1; }
}

@keyframes lm-chip-pulse {
  0%, 100% { opacity: 0.78; }
  50%      { opacity: 1; }
}

@keyframes lm-reg-cascade {
  0%   { opacity: 0; transform: translateX(-3px) scaleX(0.6); }
  20%  { opacity: 1; transform: translateX(0)    scaleX(1); }
  70%  { opacity: 1; transform: translateX(0)    scaleX(1); }
  100% { opacity: 0; transform: translateX(0)    scaleX(1); }
}

@keyframes lm-pad-blink {
  0%, 100% { opacity: 0.35; }
  50%      { opacity: 0.95; }
}

@keyframes lm-scanline {
  0%   { opacity: 0; transform: translateY(0); }
  20%  { opacity: 0.55; }
  100% { opacity: 0; transform: translateY(86px); }
}

@keyframes lm-label-pulse {
  0%, 100% { opacity: 0.55; }
  50%      { opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .lm-bus,
  .lm-bus-pad,
  .lm-pin,
  .lm-chip,
  .lm-reg,
  .lm-scan,
  .lm-label {
    animation: none;
  }
  .lm-bus { stroke-dashoffset: 0; opacity: 0.85; }
  .lm-reg { opacity: 0.85; }
  .lm-pin { opacity: 0.7; }
  .lm-bus-pad { opacity: 0.7; }
  .lm-scan { opacity: 0; }
  .lm-svg { animation: lm-reduced-pulse 3s ease-in-out infinite; }
}

@keyframes lm-reduced-pulse {
  0%, 100% { opacity: 0.8; }
  50%      { opacity: 1; }
}
</style>
