<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
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
  PanelBottomOpen,
  PanelBottomClose,
} from '@lucide/vue';
import { useCpuStore, type CoreInfo } from '../../stores/cpu';
import { useSimBarStore } from '../../stores/simBar';
import { useDraggable } from '../../composables/useDraggable';
import { useBreakpoint } from '../../composables/useBreakpoint';
import { useSimSettings } from '../../composables/useSimSettings';
import {
  hex,
  coreModeLabel,
  coreModeClass,
  coreStateLabel,
  coreStateClass,
  coreStateTerm,
} from '../../composables/useCoreFormat';
import ProgramPicker from '../ProgramPicker.vue';
import SimStatusRow from './SimStatusRow.vue';
import SimToolsDrawer from './SimToolsDrawer.vue';
import Term from '../glossary/Term.vue';

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

// Cores panel — strip with one line per visible core. The accordion handle
// hangs from the bottom edge of the SimBar. Behavior is identical whether
// there's 1 core or many: handle always present, panel toggleable.
const visibleCores = computed<CoreInfo[]>(() =>
  cpu.cores.filter((c) => cpu.isCoreVisible(c.id)).slice().sort((a, b) => a.id - b.id),
);

// Idle-wait: every core HALT but at least one process is parked in I/O
// (today only sys_read on stdin). The run loop autostops in this state to
// avoid a spinning ticker, yet semantically the simulation is still alive —
// a keypress wakes it up. The user must be able to PAUSE it explicitly so
// the byte sits in the buffer instead of auto-resuming, hence the PAUSE
// button has to stay visible here even though cpu.running is false.
const idleWait = computed(() => !cpu.running && cpu.kbdReaders.length > 0);

// Permanently dead: every core halted, ready queue empty, no I/O readers.
// Nothing can advance until the user resets or loads a new program. We
// disable step/run and explain why in the tooltip.
const haltedDead = computed(
  () => cpu.halted && cpu.readyQueue.length === 0 && cpu.kbdReaders.length === 0,
);

// Show PAUSE whenever the sim is "intended to be running": actually ticking,
// or parked in idle-wait without an explicit user pause. RUN takes over in
// every other case (paused, dead-halt, or never started).
const showPause = computed(() => cpu.running || (idleWait.value && !cpu.paused));

// useSimSettings returns viewer as a ComputedRef<boolean>; .value is required
// when consumed inside another computed (only the template auto-unwraps refs).
const stepDisabled = computed<boolean>(
  () => viewer.value || cpu.running || haltedDead.value,
);
const runDisabled = computed<boolean>(() => viewer.value || haltedDead.value);

const stepTooltip = computed(() => {
  if (viewer.value) return t('simbar.tooltips.viewerLocked');
  if (cpu.running) return t('simbar.tooltips.stepWhileRunning');
  if (haltedDead.value) return t('simbar.tooltips.stepDisabledDead');
  return t('simbar.tooltips.step');
});

const runTooltip = computed(() => {
  if (viewer.value) return t('simbar.tooltips.viewerLocked');
  if (haltedDead.value) return t('simbar.tooltips.runDisabledDead');
  if (cpu.paused) return t('simbar.tooltips.runResumeFromPause');
  return t('simbar.tooltips.run');
});

const pauseTooltip = computed(() => {
  if (viewer.value) return t('simbar.tooltips.viewerLocked');
  if (idleWait.value) return t('simbar.tooltips.pauseDuringIdleWait');
  return t('simbar.tooltips.pause');
});
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

        <SimStatusRow inline class="sim-bar__status-inline" />

        <button
          type="button"
          :class="['icon-btn', 'sim-bar__cores-btn', { 'sim-bar__cores-btn--on': bar.coresExpanded }]"
          :aria-expanded="bar.coresExpanded"
          :aria-label="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
          :title="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
          @click="bar.toggleCores()"
          @pointerdown.stop
          @dblclick.stop
        >
          <component
            :is="bar.coresExpanded ? PanelBottomClose : PanelBottomOpen"
            :size="14"
            :stroke-width="2"
          />
        </button>

        <button
          type="button"
          class="sim-bar__dock-btn"
          :title="t('simbar.dockHint')"
          @click="bar.dock()"
          @pointerdown.stop
          @dblclick.stop
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
              :disabled="stepDisabled"
              :title="stepTooltip"
              @click="cpu.step()"
            ><StepForward :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.step') }}</span></button>
            <button
              v-if="!showPause"
              type="button"
              class="sim-btn sim-btn--anim-run"
              :disabled="runDisabled"
              :title="runTooltip"
              @click="cpu.run(0)"
            ><Play :size="14" :stroke-width="2" /><span>{{ t('simbar.buttons.run') }}</span></button>
            <button
              v-else
              type="button"
              class="sim-btn sim-btn--anim-run is-running"
              :disabled="viewer"
              :title="pauseTooltip"
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

        <SimStatusRow v-if="bar.mode === 'docked'" inline class="sim-bar__status-inline" />

        <div v-if="bar.mode === 'docked'" class="sim-bar__right">
          <button
            type="button"
            :class="['icon-btn', 'sim-bar__cores-btn', { 'sim-bar__cores-btn--on': bar.coresExpanded }]"
            :aria-expanded="bar.coresExpanded"
            :aria-label="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
            :title="bar.coresExpanded ? t('simbar.coresToggleCollapse') : t('simbar.coresToggleExpand')"
            @click="bar.toggleCores()"
          >
            <component
              :is="bar.coresExpanded ? PanelBottomClose : PanelBottomOpen"
              :size="14"
              :stroke-width="2"
            />
          </button>
          <button
            v-if="!isMobile"
            type="button"
            class="icon-btn sim-bar__float-btn"
            :title="t('simbar.undockHint')"
            @click="bar.float()"
          ><PinOff :size="14" :stroke-width="2" /></button>
        </div>
      </div>

      <Transition name="sim-cores">
        <div v-if="bar.coresExpanded" class="sim-bar__cores-panel">
          <div
            v-for="c in visibleCores"
            :key="c.id"
            :class="['sim-core-row', {
              'sim-core-row--halted': c.halted,
              'sim-core-row--idle': c.current_pid === 0 && !c.halted,
            }]"
            :style="{ '--core-tint': `var(--core-${c.id % 4})` }"
            :title="t('shell.coreCell.coreStateTitle', { id: c.id })"
          >
            <span class="sim-core-row__id">C{{ c.id }}</span>
            <span class="core-cell core-cell--pid">
              <Term term="PID" mode="label" class="core-cell__k">PID</Term>
              <span class="core-cell__v">{{ c.current_pid }}</span>
            </span>
            <span :class="['core-cell', 'core-cell--mode', coreModeClass(c)]">
              <Term :term="c.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'" mode="label" class="core-cell__k">{{ t('shell.coreCell.modeLabel') }}</Term>
              <span class="core-cell__v">{{ coreModeLabel(c) }}</span>
            </span>
            <span class="core-cell core-cell--reg">
              <Term term="EIP" mode="label" class="core-cell__k">EIP</Term>
              <span class="core-cell__v">{{ hex(c.regs.EIP) }}</span>
            </span>
            <span class="core-cell core-cell--reg">
              <Term term="EAX" mode="label" class="core-cell__k">EAX</Term>
              <span class="core-cell__v">{{ hex(c.regs.EAX) }}</span>
            </span>
            <span class="core-cell core-cell--reg">
              <Term term="ESP" mode="label" class="core-cell__k">ESP</Term>
              <span class="core-cell__v">{{ hex(c.regs.ESP) }}</span>
            </span>
            <span
              :class="['core-cell', 'core-cell--state', coreStateClass(c)]"
              :title="c.halted
                ? t('shell.coreCell.haltedTitle', { id: c.id })
                : (c.current_pid > 0
                  ? t('shell.coreCell.runningTitle', { id: c.id, pid: c.current_pid })
                  : t('shell.coreCell.idleTitle', { id: c.id }))"
            >
              <Term :term="coreStateTerm(c)" mode="label" class="core-cell__k">{{ t('shell.coreCell.stateLabel') }}</Term>
              <span class="core-cell__v">{{ coreStateLabel(c) }}</span>
            </span>
          </div>
        </div>
      </Transition>

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
  position: relative; /* anchors the cores handle to the bottom edge */
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
  font-size: 10px;
  margin-right: auto; /* pushes the status row + cores-btn + dock cluster to the right */
  flex-shrink: 0;
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
  padding-right: 84px; /* reserve room for the absolute cores-btn + PinOff cluster (docked only) */
}

/* In floating mode the cores-btn + dock cluster live in the header, so the
   main row no longer needs the right-edge reservation. */
.sim-bar--floating .sim-bar__row--main {
  padding-right: var(--space-3);
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
    padding-right: 84px;
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

/* ─── Cores panel (accordion below the controls row) ───────────────────
   The strip is a sibling of .sim-bar__row--main, not a child. That keeps
   the main row at a fixed height (so the absolute PinOff button stays
   centered against it) while the panel grows underneath. */
.sim-bar__cores-panel {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: var(--space-2) var(--space-4);
  background: var(--surface-0);
  border-top: 1px solid var(--border-strong);
  box-shadow:
    inset 0 6px 10px -8px rgba(0, 0, 0, 0.55),
    inset 0 -2px 4px -3px rgba(0, 0, 0, 0.25);
  overflow: hidden;
}

.sim-core-row {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: var(--space-3);
  padding: 2px var(--space-2);
  padding-left: var(--space-2);
  border-left: 2px solid var(--core-tint, var(--border));
  transition: background var(--transition-fast);
  min-height: 24px;
  box-sizing: border-box;
}
.sim-core-row + .sim-core-row {
  margin-top: 2px;
}
.sim-core-row:hover {
  background: color-mix(in oklab, var(--core-tint, var(--amber)) 8%, transparent);
}
.sim-core-row--halted { opacity: 0.7; }
.sim-core-row--idle { opacity: 0.85; }

.sim-core-row__id {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 1.5px;
  font-weight: 700;
  color: var(--core-tint, var(--text));
  min-width: 24px;
  flex-shrink: 0;
}

/* Each label/value pair lives in a borderless cell. Fixed min-widths keep
   the columns visually aligned across rows without a real table. */
.core-cell {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  height: 20px;
}
.core-cell__k,
.core-cell__v {
  display: inline-flex;
  align-items: center;
  height: 20px;
  line-height: 1;
  font-family: 'JetBrains Mono', monospace;
}
.core-cell__k {
  color: var(--text-muted);
  letter-spacing: 1px;
  text-transform: uppercase;
  font-size: 9px;
  font-weight: 500;
}
.core-cell__v {
  color: var(--text);
  font-size: 10px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.core-cell--pid   { min-width: 56px; }
.core-cell--mode  { min-width: 84px; }
.core-cell--reg   { min-width: 130px; }
.core-cell--state { min-width: 64px; }

.core-cell--user .core-cell__v { color: var(--green); }
.core-cell--kernel .core-cell__v { color: var(--red-soft, #ff7a7a); }

.core-cell--halt .core-cell__v { color: var(--red); font-weight: 700; }
.core-cell--running .core-cell__v { color: var(--green); font-weight: 700; }
.core-cell--idle .core-cell__v { color: var(--text-muted); }

/* Accordion transition — max-height + opacity, scoped tight to the strip
   (max 4 cores ≈ 110px). */
.sim-cores-enter-active,
.sim-cores-leave-active {
  transition: max-height var(--transition-base) ease-out,
              opacity var(--transition-base) ease-out;
  overflow: hidden;
}
.sim-cores-enter-from,
.sim-cores-leave-to {
  max-height: 0;
  opacity: 0;
}
.sim-cores-enter-to,
.sim-cores-leave-from {
  max-height: 140px;
  opacity: 1;
}

/* Cores toggle — icon-btn sibling of the PinOff button. Same vocabulary
   (28px square, transparent at rest, amber tint on hover) so it reads as
   "another SimBar control" rather than a floating handle. */
.sim-bar__cores-btn--on {
  color: var(--amber);
  border-color: rgba(var(--accent-rgb), 0.45);
  background: rgba(var(--accent-rgb), 0.10);
}

@media (max-width: 480px) {
  .sim-core-row {
    flex-wrap: nowrap;
    overflow-x: auto;
    scrollbar-width: thin;
    -webkit-overflow-scrolling: touch;
    mask-image: linear-gradient(to right, black 88%, transparent);
  }
  .core-cell__k { font-size: 8px; }
  .core-cell__v { font-size: 9.5px; }
  .core-cell--pid   { min-width: 44px; }
  .core-cell--mode  { min-width: 68px; }
  .core-cell--reg   { min-width: 100px; }
  .core-cell--state { min-width: 50px; }
}
</style>
