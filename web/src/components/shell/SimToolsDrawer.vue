<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  Settings,
  Share2,
  Square,
  Crosshair,
  ChevronLeft,
  Minus,
} from '@lucide/vue';
import { useSimBarStore } from '../../stores/simBar';
import { useSimSettings } from '../../composables/useSimSettings';
import SpeedSlider from './SpeedSlider.vue';
import HandRaiseBadge from './HandRaiseBadge.vue';
import ShareModal from '../ShareModal.vue';
import Term from '../glossary/Term.vue';

const bar = useSimBarStore();
const settings = useSimSettings();
const { t } = useI18n();

// Two visual states:
//   rail — slim opaque tab pinned to the right edge of the viewport. Hover
//          shows a tooltip; click opens the panel directly. No middle/preview
//          state — the previous "hover expands to icons" was extra friction.
//   open — full panel with sections (after the user clicks).
const state = computed<'rail' | 'open'>(() =>
  bar.toolsDrawerOpen ? 'open' : 'rail',
);

function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape' && bar.toolsDrawerOpen) bar.closeTools();
}

onMounted(() => {
  document.addEventListener('keydown', onEsc);
});
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onEsc);
});

watch(() => bar.toolsDrawerOpen, (open) => {
  if (!open) settings.shareOpen.value = false;
});

function openPanel() {
  bar.openTools();
}
</script>

<template>
  <Teleport to="body">
    <div
      class="tools-dock"
      :data-state="state"
      :aria-label="t('simtools.title')"
    >
      <!-- Rail mode — slim opaque tab pinned to the right edge. Single
           left-pointing chevron: minimal, unambiguous "click to slide out."
           Hovering shows the tooltip; click opens the full panel. -->
      <button
        v-if="state === 'rail'"
        type="button"
        class="tools-dock__rail-btn"
        :title="t('simtools.title')"
        :aria-label="t('simbar.toolsBtn')"
        @click="openPanel"
      >
        <ChevronLeft :size="16" :stroke-width="2" class="tools-dock__rail-icon" />
      </button>

      <!-- Open mode — full sections. Closing is via the header's minimise
           button or the Escape key (handled by the global keydown listener
           above). The rail's pull-tab disappears while the panel is open. -->
      <aside v-else class="tools-dock__panel" role="dialog" :aria-label="t('simtools.title')" aria-modal="false">
        <header class="tools-dock__header">
          <h2 class="tools-dock__title">{{ t('simtools.title') }}</h2>
          <button
            type="button"
            class="tools-dock__close"
            :title="t('simtools.closeAria')"
            :aria-label="t('simtools.closeAria')"
            @click="bar.closeTools()"
          ><Minus :size="14" :stroke-width="2.5" /></button>
        </header>

        <div class="tools-dock__body">
          <section v-if="settings.canShare.value" class="tools-dock__section">
            <h3 class="tools-dock__section-title">
              <Share2 :size="13" :stroke-width="2" />
              <span>{{ t('simtools.shareSection') }}</span>
            </h3>
            <HandRaiseBadge v-if="settings.isShared.value" class="tools-dock__hand" />
            <button
              type="button"
              :class="['tools-dock__btn', { 'tools-dock__btn--on': settings.whiteboardOn.value }]"
              :title="settings.whiteboardOn.value ? t('simbar.whiteboardOnHint') : t('simbar.whiteboardOffHint')"
              @click="settings.toggleWhiteboard"
            >
              <Crosshair :size="14" :stroke-width="2" />
              <span>{{ settings.whiteboardOn.value ? t('simtools.whiteboardOnLabel') : t('simtools.whiteboardActivateLabel') }}</span>
            </button>
            <button
              type="button"
              :class="['tools-dock__btn', { 'tools-dock__btn--on': settings.isShared.value }]"
              :title="settings.isShared.value ? t('topbar.shareManageHint') : t('topbar.shareHint')"
              @click="settings.openShare"
            >
              <component :is="settings.isShared.value ? Square : Share2" :size="14" :stroke-width="2" />
              <span>{{ settings.isShared.value ? t('simtools.shareActiveLabel') : t('simtools.shareInactiveLabel') }}</span>
            </button>
          </section>

          <section class="tools-dock__section">
            <h3 class="tools-dock__section-title">
              <Settings :size="13" :stroke-width="2" />
              <span>{{ t('simtools.configSection') }}</span>
            </h3>
            <SpeedSlider class="tools-dock__speed" :viewer="settings.viewer.value" />
            <label class="tools-dock__field">
              <span class="tools-dock__field-label">{{ t('simbar.settings.clockHz') }}</span>
              <input
                type="number"
                min="1"
                max="60"
                :value="settings.cpu.clockHz"
                :disabled="settings.viewer.value"
                :aria-label="t('simbar.settings.clockHzAria')"
                @input="settings.onClockHz"
              />
              <span class="tools-dock__field-help">{{ t('simbar.settings.clockHzHint') }}</span>
            </label>
            <label class="tools-dock__field">
              <Term term="Quantum" mode="label" class="tools-dock__field-label">{{ t('simbar.settings.quantum') }}</Term>
              <input
                type="number"
                min="0"
                max="200"
                :value="settings.cpu.timerQuantum"
                :disabled="settings.viewer.value"
                :aria-label="t('simbar.settings.quantumAria')"
                @input="settings.onQuantum"
              />
              <span class="tools-dock__field-help">{{ t('simbar.settings.quantumHint') }}</span>
            </label>
            <div class="tools-dock__field">
              <span class="tools-dock__field-label">{{ t('simbar.settings.cores') }}</span>
              <div class="tools-dock__core-picker" role="radiogroup" :aria-label="t('simbar.settings.coresAria')">
                <button
                  v-for="n in 4"
                  :key="n"
                  type="button"
                  role="radio"
                  :aria-checked="settings.cpu.numCores === n"
                  :class="['tools-dock__core-pick', { 'tools-dock__core-pick--active': settings.cpu.numCores === n }]"
                  :disabled="settings.viewer.value"
                  :title="t(n === 1 ? 'simbar.settings.coresChangeOne' : 'simbar.settings.coresChangeMany', { n })"
                  @click="settings.cpu.setNumCores(n)"
                >{{ n }}</button>
              </div>
              <span class="tools-dock__field-help">{{ t('simbar.settings.coresRestartHint') }}</span>
            </div>
          </section>
        </div>

        <ShareModal :open="settings.shareOpen.value" @close="settings.shareOpen.value = false" />
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
/* The dock lives as an overlay pinned to the right edge of the viewport.
   It stretches full-height so the open panel can fill the screen vertically;
   the rail uses margin-top to keep its tab below the topbar without needing
   the wrapper to start at the topbar's bottom. */
.tools-dock {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: calc(var(--z-overlay, 500) + 20);
  display: flex;
  align-items: stretch;
  justify-content: flex-end;
  transition: width var(--transition-base);
  pointer-events: none; /* the inner buttons re-enable */
}

.tools-dock[data-state='rail'] { width: 22px; }
.tools-dock[data-state='open'] { width: min(340px, 100vw); }

/* ─── Rail (collapsed) ──────────────────────────────────────────────── */

/* Slim, OPAQUE tab pinned to the right edge. The gear advertises "settings
   live in here," the chevron says "click and I slide out." No alpha
   gradient — the rail reads as a physical handle, not a translucent tease.
   Click opens the full panel directly; no intermediate hover state. */
.tools-dock__rail-btn {
  pointer-events: auto;
  align-self: flex-start;
  width: 22px;
  height: 28px;
  margin-top: 120px; /* clear the SimBar top + brand row visually */
  padding: 0;
  border: 1px solid rgba(var(--accent-rgb), 0.55);
  border-right: none;
  border-top-left-radius: var(--radius-sm);
  border-bottom-left-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--amber);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  /* Resting state: tucked ~10px into the right edge so only a sliver pokes
     out. Hover slides it back into full view. The amber glow pulses softly
     so the rail keeps advertising itself without being loud. */
  transform: translateX(10px);
  box-shadow:
    0 0 0 2px rgba(var(--accent-rgb), 0.04),
    -2px 0 8px rgba(0, 0, 0, 0.45),
    0 0 12px rgba(var(--accent-rgb), 0.30);
  animation: rail-idle-pulse 3.6s ease-in-out infinite;
  transition: transform 180ms var(--ease-out-soft),
              background var(--transition-fast),
              box-shadow var(--transition-fast),
              color var(--transition-fast),
              border-color var(--transition-fast);
}
.tools-dock__rail-btn:hover,
.tools-dock__rail-btn:focus-visible {
  transform: translateX(0);
  /* Background stays exactly the same as idle — no tinting, no alpha mix.
     The visible feedback is the slide-out + the brighter glow + a fully
     amber border + the chevron switching to the foreground colour. */
  border-color: var(--amber);
  color: var(--text);
  box-shadow:
    0 0 0 3px rgba(var(--accent-rgb), 0.10),
    -4px 0 14px rgba(0, 0, 0, 0.55),
    0 0 22px rgba(var(--accent-rgb), 0.60);
  animation: none;
}
.tools-dock__rail-btn:focus { outline: none; }
.tools-dock__rail-btn:focus-visible {
  outline: 1px solid var(--amber);
  outline-offset: -2px;
}

/* The button itself slides 10px to the right when at rest (so only a sliver
   pokes out of the viewport). To keep the chevron visible inside that
   sliver, the icon counter-translates a few pixels to the left so it sits
   in the exposed area instead of behind the right edge. On hover, both
   button and icon return to their natural positions. The icon also pulses
   its own glow/opacity in sync with the button's box-shadow pulse, so the
   arrow itself reads as alive — not just the surrounding tab. */
.tools-dock__rail-icon {
  transform: translateX(-5px);
  animation: rail-icon-pulse 3.6s ease-in-out infinite;
  transition: transform 180ms var(--ease-out-soft);
}
.tools-dock__rail-btn:hover .tools-dock__rail-icon,
.tools-dock__rail-btn:focus-visible .tools-dock__rail-icon {
  transform: translateX(0);
  animation: none;
  opacity: 1;
  filter: drop-shadow(0 0 4px rgba(var(--accent-rgb), 0.75));
}

@keyframes rail-icon-pulse {
  0%, 100% {
    opacity: 0.85;
    filter: drop-shadow(0 0 2px rgba(var(--accent-rgb), 0.45));
  }
  50% {
    opacity: 1;
    filter: drop-shadow(0 0 6px rgba(var(--accent-rgb), 0.85));
  }
}

@keyframes rail-idle-pulse {
  0%, 100% {
    box-shadow:
      0 0 0 2px rgba(var(--accent-rgb), 0.04),
      -2px 0 8px rgba(0, 0, 0, 0.45),
      0 0 12px rgba(var(--accent-rgb), 0.30);
  }
  50% {
    box-shadow:
      0 0 0 4px rgba(var(--accent-rgb), 0.10),
      -2px 0 10px rgba(0, 0, 0, 0.55),
      0 0 22px rgba(var(--accent-rgb), 0.55);
  }
}
@media (prefers-reduced-motion: reduce) {
  .tools-dock__rail-btn,
  .tools-dock__rail-icon { animation: none; }
  .tools-dock__rail-icon {
    opacity: 0.95;
    filter: drop-shadow(0 0 3px rgba(var(--accent-rgb), 0.55));
  }
}

/* ─── Open (full panel) ─────────────────────────────────────────────── */

.tools-dock__panel {
  pointer-events: auto;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--surface-1);
  /* Subtle border like the left sidebar — no heavy line, just enough to
     separate the panel from the content underneath. */
  border-left: 1px solid var(--border);
  box-shadow: -8px 0 28px rgba(0, 0, 0, 0.55);
}

.tools-dock__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--border);
  background: rgba(var(--accent-rgb), 0.04);
}

/* Minimise button — borrows the tutor panel's icon-btn vocabulary: borderless
   at rest with a muted glyph, soft amber tint on hover. The minus glyph is
   the universal "minimise" affordance (the X belongs to "close/destroy"). */
.tools-dock__close {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  padding: 0.4rem;
  border-radius: 0.3rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 120ms ease;
}
.tools-dock__close:hover {
  color: var(--accent);
  border-color: rgba(var(--accent-rgb), 0.4);
  background: rgba(var(--accent-rgb), 0.08);
}
.tools-dock__close:focus { outline: none; }
.tools-dock__close:focus-visible { outline: 1px solid var(--amber); outline-offset: -1px; }
.tools-dock__close > svg { display: block; }

.tools-dock__title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 3px;
  text-transform: uppercase;
  color: var(--amber);
}

.tools-dock__body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3) var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.tools-dock__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.tools-dock__section-title {
  margin: 0 0 var(--space-1);
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.tools-dock__hand { align-self: flex-start; }

.tools-dock__btn {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  height: 34px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
  text-align: left;
  width: 100%;
  box-sizing: border-box;
}
.tools-dock__btn:hover {
  background: rgba(var(--accent-rgb), 0.14);
  color: var(--amber);
  border-color: var(--border-strong);
}
.tools-dock__btn--on {
  background: rgba(127, 255, 170, 0.10);
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green);
}
.tools-dock__btn--on:hover {
  background: rgba(127, 255, 170, 0.16);
  color: var(--green);
  border-color: var(--green);
}
.tools-dock__btn--ghost {
  width: auto;
  height: 28px;
  padding: 0 8px;
  font-size: 10px;
  letter-spacing: 1px;
}

.tools-dock__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 11px;
}
.tools-dock__field-label {
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
}
.tools-dock__field input {
  font-family: 'JetBrains Mono', monospace;
  background: var(--surface-0);
  border: 1px solid var(--border);
  color: var(--text);
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  width: 100%;
  box-sizing: border-box;
}
.tools-dock__field-help { color: var(--text-muted); font-size: 10px; }

.tools-dock__core-picker {
  display: inline-flex;
  gap: 4px;
}
.tools-dock__core-pick {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  background: var(--surface-0);
  border: 1px solid var(--border);
  color: var(--text);
  width: 36px;
  height: 32px;
  padding: 0;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.tools-dock__core-pick:hover:not(:disabled) {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.10);
  border-color: var(--border-strong);
}
.tools-dock__core-pick--active {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.tools-dock__core-pick:disabled { opacity: 0.4; cursor: not-allowed; }

.tools-dock__speed { margin-bottom: var(--space-1); }

@media (max-width: 768px) {
  .tools-dock[data-state='open'] { width: 100vw; }
}
</style>
