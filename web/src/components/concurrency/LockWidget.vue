<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore, type MutexState, type SemaphoreState } from '../../stores/cpu';
import { pidColor } from '../../lib/pidColors';
import { useNarrators, type LockOp } from '../../lib/narratorBus';
import { useNarratorAnimation } from '../narrators/useNarratorAnimation';

const props = defineProps<{
  kind: 'mutex' | 'sem';
  lock: MutexState | SemaphoreState;
  inCycle?: boolean;
}>();

const { t } = useI18n();
const cpu = useCpuStore();

const mutex = computed(() => (props.kind === 'mutex' ? (props.lock as MutexState) : null));
const sem = computed(() => (props.kind === 'sem' ? (props.lock as SemaphoreState) : null));

const anchor = computed(() => `${props.kind === 'mutex' ? 'mutex' : 'sem'}-${props.lock.id}`);
const idLabel = computed(() => `${props.kind === 'mutex' ? 'M' : 'S'}${props.lock.id}`);

const status = computed<'free' | 'held' | 'contended'>(() => {
  if (props.lock.waiters.length > 0) return 'contended';
  if (mutex.value) return mutex.value.holder >= 0 ? 'held' : 'free';
  return (sem.value?.count ?? 0) > 0 ? 'free' : 'held';
});

function procName(pid: number): string {
  return cpu.procs.find(p => p.pid === pid)?.name ?? '';
}

const heldLabel = computed(() => {
  const holder = mutex.value?.holder ?? -1;
  if (holder < 0) return '';
  const name = procName(holder);
  return name
    ? t('sections.concurrency.lock.heldByNamed', { pid: holder, name })
    : t('sections.concurrency.lock.heldBy', { pid: holder });
});

// The wire carries no semaphore capacity, so remember the highest count seen
// this run: tokens render as `count` filled out of that cap.
const maxSeen = ref(0);
watch(
  () => sem.value?.count ?? 0,
  (c) => { if (c > maxSeen.value) maxSeen.value = c; },
  { immediate: true },
);
const tokenCap = computed(() => Math.max(maxSeen.value, sem.value?.count ?? 0));

// Same source as Concurrency.vue's blockedFor: prefer the live waitQueue
// stamp, fall back to the snapshot's blocked_since.
function blockedTicks(pid: number): string {
  const w = cpu.waitQueue.find(x => x.pid === pid);
  const bs = w?.blocked_since ?? cpu.procs.find(p => p.pid === pid)?.blocked_since;
  if (typeof bs !== 'number' || bs <= 0) return '';
  return t('sections.concurrency.blockedForFmt', { n: Math.max(0, cpu.tick - bs) });
}

// Handoff/rejection narration. Bus handlers run before Vue flushes the DOM
// patch, so measuring at event time usually sees the giver's side and the
// receiver still at the head of the waiter list. Batch-edge approximation:
// two ops on the same lock in one flush make the second measurement see the
// pre-batch DOM — acceptable for a 300ms ghost.
const rootEl = ref<HTMLElement | null>(null);
const flyKeyEl = ref<SVGSVGElement | null>(null);
const flyTokenEl = ref<HTMLElement | null>(null);
const narrators = useNarrators();
const animator = useNarratorAnimation();

// Flies the ghost between the centers of two elements, in root coordinates.
function flyGhost(ghost: HTMLElement, fromEl: Element, toEl: Element) {
  const root = rootEl.value;
  if (!root) return;
  const rr = root.getBoundingClientRect();
  const gr = ghost.getBoundingClientRect();
  const a = fromEl.getBoundingClientRect();
  const b = toEl.getBoundingClientRect();
  const dx = gr.width / 2 + rr.left;
  const dy = gr.height / 2 + rr.top;
  animator.play(ghost, [
    { transform: `translate(${a.left + a.width / 2 - dx}px, ${a.top + a.height / 2 - dy}px)`, opacity: 1 },
    { transform: `translate(${b.left + b.width / 2 - dx}px, ${b.top + b.height / 2 - dy}px)`, opacity: 0 },
  ], { durationMs: 300, fill: 'none' });
}

function flyHandoff() {
  const root = rootEl.value;
  if (!root) return;
  const waiter = root.querySelector('.lw__waiter');
  if (!waiter) return;
  // Mutex: the key leaves the holder capsule. Semaphore: a token (●) leaves
  // the token row — a semaphore has no key to hand over.
  const ghost = props.kind === 'mutex'
    ? (flyKeyEl.value as unknown as HTMLElement | null)
    : flyTokenEl.value;
  const from = root.querySelector(props.kind === 'mutex' ? '[data-lock-key]' : '[data-sem-token]');
  if (!ghost || !from) return;
  flyGhost(ghost, from, waiter);
}

function shake() {
  if (!rootEl.value) return;
  animator.play(rootEl.value, [
    { transform: 'translateX(0)' },
    { transform: 'translateX(-3px)' },
    { transform: 'translateX(3px)' },
    { transform: 'translateX(0)' },
  ], { durationMs: 250, fill: 'none' });
}

function onLockOp(e: LockOp) {
  if (e.lockId !== props.lock.id) return;
  if (e.op === 'handoff') flyHandoff();
  // 'rejected' belongs to the mutex vocabulary only (unlock by a non-holder);
  // the sem channel never carries it, so the shake is mutex-gated.
  else if (e.op === 'rejected' && props.kind === 'mutex') shake();
}

if (props.kind === 'mutex') narrators.onMutexOp(onLockOp);
else narrators.onSemOp(onLockOp);
</script>

<template>
  <article
    ref="rootEl"
    class="lw"
    :class="[`lw--${status}`, { 'lw--cycle': inCycle }]"
    :data-narrator-anchor="anchor"
  >
    <header class="lw__head">
      <code class="lw__id">{{ idLabel }}</code>
      <span v-if="mutex" class="lw__state">{{ mutex.holder < 0 ? t('sections.concurrency.lock.free') : heldLabel }}</span>
      <span v-else class="lw__state lw__state--sem" :title="t('sections.concurrency.lock.semAvailable', { count: sem!.count })">
        <span class="lw__tokens" data-sem-token aria-hidden="true">
          <span
            v-for="i in tokenCap"
            :key="i"
            class="lw__token"
            :class="{ 'lw__token--on': i <= sem!.count }"
          >{{ i <= sem!.count ? '●' : '○' }}</span>
        </span>
        <code class="lw__count">{{ sem!.count }}</code>
      </span>
    </header>

    <div v-if="mutex" class="lw__body">
      <span
        v-if="mutex.holder < 0"
        class="lw__hook"
      >
        <svg class="lw__key" data-lock-key viewBox="0 0 26 12" aria-hidden="true">
          <circle cx="5.5" cy="6" r="3.6" fill="none" stroke="currentColor" stroke-width="1.6" />
          <path d="M9 6 H23 M19 6 v3.4 M22.5 6 v3.4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
        </svg>
      </span>
      <span
        v-else
        class="lw__capsule"
        :style="{ '--holder-color': pidColor(mutex.holder) }"
      >
        <svg class="lw__key" data-lock-key viewBox="0 0 26 12" aria-hidden="true">
          <circle cx="5.5" cy="6" r="3.6" fill="none" stroke="currentColor" stroke-width="1.6" />
          <path d="M9 6 H23 M19 6 v3.4 M22.5 6 v3.4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
        </svg>
        <code class="lw__capsule-pid">P{{ mutex.holder }}</code>
      </span>
    </div>

    <svg v-if="kind === 'mutex'" ref="flyKeyEl" class="lw__fly lw__fly--key" viewBox="0 0 26 12" aria-hidden="true">
      <circle cx="5.5" cy="6" r="3.6" fill="none" stroke="currentColor" stroke-width="1.6" />
      <path d="M9 6 H23 M19 6 v3.4 M22.5 6 v3.4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
    </svg>
    <span v-else ref="flyTokenEl" class="lw__fly lw__fly--token" aria-hidden="true">●</span>

    <ol v-if="lock.waiters.length > 0" class="lw__waiters">
      <li
        v-for="(pid, idx) in lock.waiters"
        :key="pid"
        class="lw__waiter"
        :class="{ 'lw__waiter--head': idx === 0 }"
        :style="{ '--pid-color': pidColor(pid) }"
      >
        <code class="lw__waiter-pid">P{{ pid }}</code>
        <span v-if="blockedTicks(pid)" class="lw__waiter-time">· {{ blockedTicks(pid) }}</span>
      </li>
    </ol>
  </article>
</template>

<style scoped>
.lw {
  position: relative;
  min-height: 90px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-2) var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-family: 'JetBrains Mono', monospace;
  transition: border-color var(--transition-base), box-shadow var(--transition-base);
}
.lw--free {
  border-color: color-mix(in oklab, var(--green) 45%, var(--border));
  box-shadow: 0 0 8px color-mix(in oklab, var(--green) 18%, transparent);
}
.lw--contended {
  border-color: color-mix(in oklab, var(--yellow, #ffd54f) 45%, var(--border));
}
.lw--cycle {
  border-color: var(--red);
  animation: lw-cycle-blink 1s step-end infinite;
}
@keyframes lw-cycle-blink {
  0%, 100% { box-shadow: 0 0 8px color-mix(in oklab, var(--red) 45%, transparent); }
  50%      { box-shadow: none; }
}

.lw__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
}
.lw__id {
  color: var(--amber);
  font-weight: bold;
  font-size: 12px;
}
.lw__state {
  color: var(--text-dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lw--free .lw__state { color: var(--green); text-shadow: 0 0 6px color-mix(in oklab, var(--green) 35%, transparent); }
.lw__state--sem {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.lw__tokens { letter-spacing: 2px; font-size: 11px; }
.lw__token { color: var(--text-muted); }
.lw__token--on { color: var(--green); text-shadow: 0 0 5px color-mix(in oklab, var(--green) 40%, transparent); }
.lw__count {
  color: var(--text);
  background: rgba(var(--accent-rgb), 0.1);
  padding: 0 4px;
  border-radius: var(--radius-sm);
}

.lw__body { display: flex; align-items: center; min-height: 22px; }

/* Free: the key hangs on its hook, faint phosphor-green glow. */
.lw__hook {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-bottom: 1px dashed color-mix(in oklab, var(--green) 35%, transparent);
  color: var(--green);
  filter: drop-shadow(0 0 3px color-mix(in oklab, var(--green) 35%, transparent));
}

/* Held: the key travels inside a capsule tinted with the holder's color. */
.lw__capsule {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  border: 1px solid var(--holder-color);
  border-radius: 999px;
  color: var(--holder-color);
  background: color-mix(in oklab, var(--holder-color) 12%, transparent);
}
.lw__capsule-pid { font-size: 11px; }

.lw__key { width: 24px; height: 11px; display: block; }

/* Handoff flight ghost: parked invisible at the widget origin, moved with
   WAAPI from the giver's side to the head waiter on each handoff. */
.lw__fly {
  position: absolute;
  left: 0;
  top: 0;
  opacity: 0;
  pointer-events: none;
}
.lw__fly--key {
  width: 24px;
  height: 11px;
  color: var(--amber);
  filter: drop-shadow(0 0 4px color-mix(in oklab, var(--amber) 55%, transparent));
}
.lw__fly--token {
  font-size: 11px;
  line-height: 1;
  color: var(--green);
  text-shadow: 0 0 5px color-mix(in oklab, var(--green) 40%, transparent);
}

.lw__waiters {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.lw__waiter {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: rgba(255, 213, 79, 0.06);
}
.lw__waiter-pid { color: var(--pid-color); }
.lw__waiter-time { color: var(--text-muted); font-style: italic; }
/* First in the FIFO = next owner once the lock is released; same pulse
   convention as the waiting tables. */
.lw__waiter--head {
  border-color: color-mix(in oklab, var(--yellow, #ffd54f) 60%, transparent);
  animation: lw-head-pulse 1.8s ease-in-out infinite;
}
@keyframes lw-head-pulse {
  0%, 100% { box-shadow: 0 0 0 0 transparent; }
  50%      { box-shadow: 0 0 8px 0 color-mix(in oklab, var(--yellow, #ffd54f) 35%, transparent); }
}
</style>
