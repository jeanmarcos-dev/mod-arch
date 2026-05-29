<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  Settings,
  Share2,
  Sparkles,
  Square,
  Crosshair,
  Minus,
} from '@lucide/vue';
import { useSimBarStore } from '../../stores/simBar';
import { usePreferencesStore } from '../../stores/preferences';
import { useSimSettings } from '../../composables/useSimSettings';
import SpeedSlider from './SpeedSlider.vue';
import HandRaiseBadge from './HandRaiseBadge.vue';
import ShareModal from '../ShareModal.vue';
import Term from '../glossary/Term.vue';

const bar = useSimBarStore();
const prefs = usePreferencesStore();
const settings = useSimSettings();
const { t } = useI18n();

function toggleNarrators() {
  prefs.narratorsEnabled = !prefs.narratorsEnabled;
}

function onToggleIdleTask() {
  const next = !settings.cpu.idleTaskEnabled;
  settings.cpu.setIdleTaskEnabled(next);
  // When the student turns the swapper on, also reveal init so the full
  // kernel side is visible at once. Respect explicit user choice: once
  // they have toggled showInit themselves, leave it alone.
  if (next && !prefs.showInitTouched) prefs.setShowInit(true, /*fromCoupling*/ true);
}

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
</script>

<template>
  <Teleport to="body">
    <div
      v-if="bar.toolsDrawerOpen"
      class="tools-dock__backdrop"
      :aria-label="t('simtools.closeAria')"
      @click="bar.closeTools()"
    ></div>
    <aside
      v-if="bar.toolsDrawerOpen"
      class="tools-dock__panel"
      role="dialog"
      :aria-label="t('simtools.title')"
      aria-modal="false"
    >
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
          <div class="tools-dock__field">
            <span class="tools-dock__field-label">{{ t('simbar.settings.idleTask') }}</span>
            <button
              type="button"
              :class="['tools-dock__btn', { 'tools-dock__btn--on': settings.cpu.idleTaskEnabled }]"
              :aria-pressed="settings.cpu.idleTaskEnabled"
              :disabled="settings.viewer.value"
              :title="settings.cpu.idleTaskEnabled ? t('simbar.settings.idleTaskTooltipOn') : t('simbar.settings.idleTaskTooltipOff')"
              @click="onToggleIdleTask"
            >
              <span>{{ settings.cpu.idleTaskEnabled ? t('simbar.settings.idleTaskOn') : t('simbar.settings.idleTaskOff') }}</span>
            </button>
            <span class="tools-dock__field-help">{{ t('simbar.settings.idleTaskHint') }}</span>
          </div>
        </section>

        <section class="tools-dock__section">
          <h3 class="tools-dock__section-title">
            <Sparkles :size="13" :stroke-width="2" />
            <span>{{ t('simtools.narratorsSection') }}</span>
          </h3>
          <button
            type="button"
            :class="['tools-dock__btn', { 'tools-dock__btn--on': prefs.narratorsEnabled }]"
            :title="prefs.narratorsEnabled ? t('simtools.narratorsToggleOnHint') : t('simtools.narratorsToggleOffHint')"
            @click="toggleNarrators"
          >
            <Sparkles :size="14" :stroke-width="2" />
            <span>{{ prefs.narratorsEnabled ? t('simtools.narratorsToggleOn') : t('simtools.narratorsToggleOff') }}</span>
          </button>
          <span class="tools-dock__field-help">{{ t('simtools.narratorsHint') }}</span>
        </section>
      </div>

      <ShareModal :open="settings.shareOpen.value" @close="settings.shareOpen.value = false" />
    </aside>
  </Teleport>
</template>

<style scoped>
/* Backdrop sits below the panel but above everything else, dims the simulator
   just enough to flag the modal state, and applies a light blur so the user
   reads the drawer as the focus. Click on it closes the drawer (the panel
   itself swallows clicks since it sits at a higher z-index). */
.tools-dock__backdrop {
  position: fixed;
  inset: 0;
  z-index: calc(var(--z-overlay, 500) + 10);
  background: rgba(0, 0, 0, 0.28);
  backdrop-filter: blur(2px);
  -webkit-backdrop-filter: blur(2px);
  cursor: pointer;
  animation: tools-dock-fade 140ms ease-out;
}

@keyframes tools-dock-fade {
  from { opacity: 0; }
  to   { opacity: 1; }
}

/* Right-edge overlay panel — full viewport height, pinned over the layout.
   Triggered from the Settings button in the SimBar; no permanent rail/handle
   sits at the edge while the panel is closed. */
.tools-dock__panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(340px, 100vw);
  z-index: calc(var(--z-overlay, 500) + 20);
  display: flex;
  flex-direction: column;
  background: var(--surface-1);
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
  .tools-dock__panel { width: 100vw; }
}
</style>
