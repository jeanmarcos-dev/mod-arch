<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  Download,
  StepForward,
  Play,
  Pause,
  RotateCcw,
  Eraser,
  Pin,
  PinOff,
  GripHorizontal,
} from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useSimBarStore } from '../../stores/simBar';
import { useDraggable } from '../../composables/useDraggable';
import { useBreakpoint } from '../../composables/useBreakpoint';
import { useSimSettings } from '../../composables/useSimSettings';
import ProgramPicker from '../ProgramPicker.vue';
import SimStatusRow from './SimStatusRow.vue';
import SimToolsDrawer from './SimToolsDrawer.vue';

const cpu = useCpuStore();
const bar = useSimBarStore();
const settings = useSimSettings();
const { isMobile } = useBreakpoint();
const { t } = useI18n();

const panelRef = ref<HTMLElement | null>(null);

const viewer = settings.viewer;

function onLoad() {
  if (!cpu.selectedProgram) return;
  cpu.load(cpu.selectedProgram);
}

function onClear() {
  cpu.clearLog();
}

// Floating mode: the panel teleports to <body> with a fixed position. We
// drive coords via useDraggable so the small "handle" row above the bar
// doubles as a drag grip and the store persists the final position.
const drag = useDraggable({
  initial: { x: bar.x, y: bar.y },
  elementRef: panelRef,
  clampToViewport: true,
  onEnd: (pos) => { bar.setPosition(pos.x, pos.y); },
});

watch(() => bar.mode, async (m) => {
  if (m === 'floating') {
    await nextTick();
    drag.setPosition({ x: bar.x, y: bar.y });
  }
});

// Mobile viewports cannot host a draggable floating toolbar without crowding
// the content. Force docked mode while isMobile is true and restore the
// user's choice automatically when the viewport widens (the store mode ref
// stays at whatever it was last set to via float()).
watch(isMobile, (mobile) => {
  if (mobile && bar.mode === 'floating') bar.dock();
}, { immediate: true });

function dockHeaderDblClick() {
  bar.dock();
}
</script>

<template>
  <Teleport to="body" :disabled="bar.mode === 'docked'">
    <section
      ref="panelRef"
      :class="['sim-bar', `sim-bar--${bar.mode}`, { 'sim-bar--viewer': viewer, 'sim-bar--dragging': drag.isDragging.value }]"
      :style="bar.mode === 'floating' ? { left: drag.position.value.x + 'px', top: drag.position.value.y + 'px' } : undefined"
      role="region"
      :aria-label="t('simbar.controlsAria')"
      data-pizarra-anchor="sim-bar"
    >
      <header
        v-if="bar.mode === 'floating'"
        class="sim-bar__handle"
        @pointerdown="drag.start($event)"
        @dblclick="dockHeaderDblClick()"
      >
        <GripHorizontal :size="14" :stroke-width="2" class="sim-bar__handle-grip" />
        <span class="sim-bar__title">{{ t('simbar.title') }}</span>
        <button
          type="button"
          class="sim-bar__dock-btn"
          :title="t('simbar.dockHint')"
          @click="bar.dock()"
        >
          <Pin :size="13" :stroke-width="2" />
          <span>{{ t('simbar.dock') }}</span>
        </button>
      </header>

      <div class="sim-bar__row sim-bar__row--main">
        <div class="sim-bar__center">
          <ProgramPicker class="sim-bar__picker" />

          <div class="sim-bar__btns" role="group" :aria-label="t('simbar.execGroupAria')">
            <button
              type="button"
              class="sim-btn sim-btn--anim-load"
              :disabled="!cpu.selectedProgram || viewer"
              :title="t('simbar.tooltips.load')"
              @click="onLoad"
            ><Download :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.load') }}</span></button>
            <button
              type="button"
              class="sim-btn sim-btn--anim-step"
              :disabled="(cpu.halted && cpu.readyQueue.length === 0) || cpu.running || viewer"
              :title="t('simbar.tooltips.step')"
              @click="cpu.step()"
            ><StepForward :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.step') }}</span></button>
            <button
              v-if="!cpu.running"
              type="button"
              class="sim-btn sim-btn--anim-run"
              :disabled="(cpu.halted && cpu.readyQueue.length === 0) || viewer"
              :title="t('simbar.tooltips.run')"
              @click="cpu.run(0)"
            ><Play :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.run') }}</span></button>
            <button
              v-else
              type="button"
              class="sim-btn sim-btn--anim-run is-running"
              :disabled="viewer"
              :title="t('simbar.tooltips.pause')"
              @click="cpu.pause()"
            ><Pause :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.pause') }}</span></button>
            <button
              type="button"
              class="sim-btn sim-btn--reset sim-btn--anim-reset"
              :disabled="viewer"
              :title="t('simbar.tooltips.reset')"
              @click="cpu.reset()"
            ><RotateCcw :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.reset') }}</span></button>
            <button
              type="button"
              class="sim-btn sim-btn--clear sim-btn--anim-clear"
              :disabled="viewer"
              :title="t('simbar.tooltips.clear')"
              @click="onClear"
            ><Eraser :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.clear') }}</span></button>
          </div>
        </div>

        <SimStatusRow inline class="sim-bar__status-inline" />

        <div class="sim-bar__right">
          <button
            v-if="bar.mode === 'docked' && !isMobile"
            type="button"
            class="icon-btn sim-bar__float-btn"
            :title="t('simbar.undockHint')"
            @click="bar.float()"
          ><PinOff :size="14" :stroke-width="2" /></button>
        </div>
      </div>
    </section>
    <SimToolsDrawer />
  </Teleport>
</template>

<style scoped>
.sim-bar {
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  color: var(--text);
}

.sim-bar--floating {
  position: fixed;
  z-index: var(--z-floating-toolbar, 250);
  background: var(--surface-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-elevation-floating, 0 8px 28px rgba(0, 0, 0, 0.55));
  max-width: calc(100vw - var(--space-4));
  overflow: visible;
}

.sim-bar--dragging { user-select: none; }

.sim-bar__handle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
  border-bottom: 1px dashed var(--border);
  border-top-left-radius: var(--radius-md);
  border-top-right-radius: var(--radius-md);
  background: rgba(var(--accent-rgb), 0.04);
  min-height: 36px;
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
  cursor: grab;
}
.sim-bar--dragging .sim-bar__handle { cursor: grabbing; }

.sim-bar__title {
  font-family: var(--font-display);
  letter-spacing: 3px;
  color: var(--amber);
  flex: 1;
  font-size: 10px;
}

.sim-bar__handle-grip { color: var(--text-muted); flex-shrink: 0; }

.sim-bar__dock-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid var(--border);
  color: var(--amber);
  border-radius: var(--radius-sm);
  cursor: pointer;
  flex-shrink: 0;
  transition: background var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
}
.sim-bar__dock-btn:hover {
  background: rgba(var(--accent-rgb), 0.16);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.sim-bar__dock-btn > svg { flex-shrink: 0; }

/* ─── Main row (controls + status + accordion) ─────────────────────── */
.sim-bar__row {
  padding: var(--space-1) var(--space-3);
}

/* Single row that hosts both clusters: action controls on the left half
   (visually centered) and the CPU status chips on the right half. When the
   viewport can't keep them on the same line, flex-wrap pushes the status
   cluster onto a second line. The PinOff button stays absolute so it
   doesn't fight either cluster's positioning. */
.sim-bar__row--main {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  min-height: 48px;
  position: relative;
  padding-right: 48px; /* reserve room for the absolute PinOff */
}

.sim-bar__center {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex: 1 1 auto;
  justify-content: flex-start;
  min-width: 0;
  flex-wrap: wrap;
}

/* SimStatusRow inline mode renders header + cores as direct flex children
   of the parent row. The header is a regular flex item; the cores wrapper
   force-claims a fresh line via flex-basis: 100% (set inside the component
   itself, in inline mode). */
.sim-bar__status-inline {
  /* Marker class. The display:contents trick on the inner component handles
     the rest. */
}

.sim-bar__right {
  position: absolute;
  right: var(--space-3);
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.sim-bar__picker {
  flex: 0 1 200px;
  min-width: 140px;
}

.sim-bar__btns {
  display: flex;
  gap: var(--space-1);
  flex-wrap: nowrap;
}

@media (max-width: 768px) {
  .sim-bar__row--main {
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    padding-right: 48px;
    min-height: 0;
  }
  .sim-bar__center {
    flex: 1 1 100%;
    justify-content: flex-start;
  }
  .sim-bar__btns {
    flex-wrap: wrap;
    justify-content: flex-start;
  }
  .sim-bar--floating {
    position: static;
    max-width: none;
    border-radius: 0;
    box-shadow: none;
    border: none;
    border-bottom: 1px solid var(--border);
  }
}

@media (max-width: 600px) {
  .sim-bar__picker {
    flex: 1 1 100%;
    max-width: none;
  }
}

@media (max-width: 480px) {
  .sim-bar__row--main {
    padding-left: 52px;
    justify-content: flex-start;
  }
  .sim-bar__center {
    justify-content: flex-start;
    flex-wrap: nowrap;
  }
  .sim-bar__btns {
    flex-wrap: nowrap;
    justify-content: flex-start;
  }
}

.sim-btn {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 0 10px;
  height: 32px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.sim-btn > svg { flex-shrink: 0; }
.sim-btn:hover:not(:disabled) {
  background: rgba(var(--accent-rgb), 0.18);
  box-shadow: var(--shadow-glow-amber);
}
.sim-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.sim-btn--reset {
  border-color: rgba(255, 58, 58, 0.35);
  color: var(--red-soft);
}
.sim-btn--reset:hover:not(:disabled) {
  background: rgba(255, 58, 58, 0.12);
  box-shadow: var(--shadow-glow-red);
}
.sim-btn--clear {
  border-color: var(--border);
  color: var(--text-dim);
}
.sim-btn--clear:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.04);
  color: var(--text);
}

.icon-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  width: 28px;
  height: 28px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  padding: 0;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.icon-btn:hover {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--amber);
  border-color: var(--border-strong);
}
.icon-btn > svg { display: block; }
</style>
