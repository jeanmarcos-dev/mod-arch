<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import CollapsiblePanel from './ui/CollapsiblePanel.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

// Hide init from both queues whenever the PROCESOS toggle hides it from the
// tree, so the wait-pill "P1 wait(any)" stops cluttering single-proc demos.
// Init's PID is discovered by name since the kernel reserves it as PID 1 but
// the frontend should not hard-code that.
const initPID = computed(() => cpu.procs.find(p => p.name === 'init')?.pid ?? 0);
const hideInit = computed(() => !prefs.showInit && initPID.value > 0);
const readyQueue = computed(() =>
  hideInit.value ? cpu.readyQueue.filter(pid => pid !== initPID.value) : cpu.readyQueue,
);
const waitQueue = computed(() =>
  hideInit.value ? cpu.waitQueue.filter(w => w.pid !== initPID.value) : cpu.waitQueue,
);

// TransitionGroup applies enter/leave classes only when its `name` prop is
// truthy. Swapping the name to a sentinel ("rq-off"/"wq-off") with no
// matching CSS rules silently disables the animation without us tearing out
// the wrapper. We keep the ready and wait wrappers on separate names so the
// two flows can ever diverge stylistically without touching the template.
const rqTransitionName = computed(() => prefs.schedulerAnimationEnabled ? 'rq' : 'rq-off');
const wqTransitionName = computed(() => prefs.schedulerAnimationEnabled ? 'wq' : 'wq-off');
</script>

<template>
  <CollapsiblePanel
    class="panel queues"
    :title="t('panels.queues.title')"
    :title-hint="t('panels.queues.titleHint')"
    glossary-key="Ready queue"
    :collapsed="prefs.panelsCollapsed.queues"
    @update:collapsed="prefs.togglePanel('queues', $event)"
  >
    <div class="ready-queue">
      <div
        class="rq-label clickable-label"
        :title="t('panels.processes.readyQueueHint')"
        @click="cpu.openGlossary('Ready queue')"
      >{{ t('panels.processes.readyQueue') }}</div>
      <TransitionGroup
        :name="rqTransitionName"
        tag="div"
        class="rq-pills rq-pills--ready"
      >
        <span v-if="readyQueue.length === 0" key="__empty__" class="rq-empty">{{ t('panels.processes.empty') }}</span>
        <span
          v-for="(pid, idx) in readyQueue"
          :key="pid"
          class="rq-pill"
          :class="{ 'rq-pill--head': idx === 0 && prefs.schedulerAnimationEnabled }"
        >P{{ pid }}</span>
      </TransitionGroup>
    </div>

    <div class="wait-queue">
      <div
        class="rq-label clickable-label"
        :title="t('panels.processes.waitQueueHint')"
        @click="cpu.openGlossary('Waiting queue')"
      >{{ t('panels.processes.waitQueue') }}
        <span v-if="waitQueue.length > 0" class="wq-hint">{{ t('panels.processes.waitQueueHelp') }}</span>
      </div>
      <TransitionGroup
        :name="wqTransitionName"
        tag="div"
        class="rq-pills rq-pills--wait"
      >
        <span v-if="waitQueue.length === 0" key="__empty__" class="rq-empty">{{ t('panels.processes.empty') }}</span>
        <span
          v-for="w in waitQueue"
          :key="w.pid"
          class="wq-pill"
          :title="t('panels.processes.waitTooltip', { pid: w.pid, reason: w.reason })"
        >
          P{{ w.pid }}
          <span class="wq-reason">{{ w.reason }}</span>
        </span>
      </TransitionGroup>
    </div>
  </CollapsiblePanel>
</template>

<style scoped>
.clickable-label {
  cursor: pointer;
  transition: color 0.15s ease;
}

.clickable-label:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

.ready-queue {
  padding-bottom: 10px;
}

.wait-queue {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.rq-label {
  color: var(--cyan);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.rq-pills {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

/* The ready/wait pill rows own a positioning context so leaving pills
   (which switch to position:absolute mid-transition) anchor inside them
   instead of floating up to the document root. */
.rq-pills--ready,
.rq-pills--wait {
  position: relative;
  min-height: 22px;
}

.rq-pill {
  padding: 2px 8px;
  background: rgba(255, 213, 79, 0.08);
  border: 1px solid var(--yellow);
  color: var(--yellow);
  font-size: 13px;
  font-weight: 700;
}

/* Head of the ready queue: this PID is the next one the dispatcher will
   bind to an idle core, so it pulses to telegraph "you are about to RUN".
   Restricted to the moment the toggle is on; otherwise the panel looks
   static at glance, matching the legacy behavior. */
.rq-pill--head {
  animation: rq-head-pulse 1.6s ease-in-out infinite;
}

@keyframes rq-head-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 color-mix(in oklab, var(--yellow) 0%, transparent);
    border-color: var(--yellow);
  }
  50% {
    box-shadow: 0 0 10px 1px color-mix(in oklab, var(--yellow) 45%, transparent);
    border-color: color-mix(in oklab, var(--yellow) 100%, white 15%);
  }
}

/* Scheduler animation: arrivals slide in from the right (the "tail" of the
   FIFO conveyor belt) and departures slide out to the left (toward the CPU
   that just picked them up). Position:absolute on leave keeps the layout
   from jumping while the leaving pill animates. */
.rq-enter-from {
  transform: translateX(18px);
  opacity: 0;
}
.rq-enter-active {
  transition: transform 260ms cubic-bezier(.2,.8,.2,1), opacity 220ms ease-out;
}
.rq-leave-to {
  transform: translateX(-22px) scale(0.95);
  opacity: 0;
}
.rq-leave-active {
  transition: transform 220ms ease-in, opacity 200ms ease-in;
  position: absolute;
}
/* Moves (same key, new index) glide horizontally as the queue shrinks
   ahead of them. Without this the pills would snap to their new spots
   the instant the head leaves. */
.rq-move {
  transition: transform 240ms cubic-bezier(.2,.8,.2,1);
}

/* Wait queue animations: a process entering BLOCKED arrives from the
   right (it just stepped off a core into the wait list); a process being
   woken leaves to the left, telegraphing that it is heading toward the
   ready queue (which renders above). The directional pair makes the
   "events flow in one side, dispatch out the other" story explicit for
   the wait queue too, not just for ready. */
.wq-enter-from {
  transform: translateX(18px);
  opacity: 0;
}
.wq-enter-active {
  transition: transform 260ms cubic-bezier(.2,.8,.2,1), opacity 220ms ease-out;
}
.wq-leave-to {
  transform: translateX(-22px) scale(0.95);
  opacity: 0;
}
.wq-leave-active {
  transition: transform 220ms ease-in, opacity 200ms ease-in;
  position: absolute;
}
.wq-move {
  transition: transform 240ms cubic-bezier(.2,.8,.2,1);
}

.rq-empty {
  color: var(--text-muted);
  font-style: italic;
  font-size: 13px;
}

.wq-hint {
  color: var(--text-muted);
  font-style: italic;
  font-size: 10px;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 4px;
}

.wq-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  background: color-mix(in oklab, var(--red) 8%, transparent);
  border: 1px solid var(--red-soft);
  color: var(--red-soft);
  font-size: 13px;
  font-weight: 700;
  animation: wait-pulse 1.6s ease-in-out infinite;
}

.wq-reason {
  font-size: 11px;
  font-weight: 400;
  color: var(--text-dim);
  text-transform: lowercase;
  letter-spacing: 0;
}

@keyframes wait-pulse {
  0%, 100% { opacity: 1; box-shadow: none; }
  50%      { opacity: 0.7; box-shadow: 0 0 6px color-mix(in oklab, var(--red) 30%, transparent); }
}
</style>
