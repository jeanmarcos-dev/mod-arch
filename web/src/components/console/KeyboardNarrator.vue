<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';
import { usePreferencesStore } from '../../stores/preferences';

// KeyboardNarrator turns each keypress into a 3-line story:
//   line 1 — character + hex code → IRQ 0x21
//   line 2 — kernel.handleKeyboard()
//   line 3 — outcome: a reader was woken OR the byte stayed in KBuf
//
// It does not subscribe through narratorBus because the bus does not surface
// process_state — and the outcome line depends on that event. Instead we
// listen straight on the cpu store and gate visibility on
// prefs.narratorsEnabled to match the rest of the narrators.

interface NarratorCard {
  seq: number;
  char: string;
  hex: string;
  status: 'pending' | 'delivered' | 'buffered';
  deliveredPid?: number;
  deliveredProg?: string;
  bufferedQueue?: number;
}

const MAX_CARDS = 6;
const MIN_PENDING_MS = 600;

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

// pendingTimeoutMs scales the wait window with the simulator's tick interval
// so a slow clock (1 Hz → 1000 ms/tick) still gives the kernel enough wall
// time to deliver the byte and emit process_state before the narrator gives
// up and labels the card as merely buffered. Three ticks is a conservative
// upper bound on the round-trip; the 600 ms floor preserves the snappy feel
// at the default 4 Hz clock.
function pendingTimeoutMs(): number {
  return Math.max(MIN_PENDING_MS, 3 * cpu.tickIntervalMs);
}

const cards = ref<NarratorCard[]>([]);
const feedRef = ref<HTMLElement | null>(null);
let seqCounter = 0;
// Each pending card has a wall-clock timer that flips it to buffered if no
// process_state arrives in time. The full state is tracked so we can also
// pause/resume the countdown when the user explicitly pauses (cpu.paused),
// otherwise wall-clock would keep ticking while the user inspects the freeze
// frame and the card would silently flip.
interface PendingTimerState {
  timeoutId: number | null;
  deadline: number; // ms epoch when the timer should fire
  remaining: number; // ms left if paused
}
const pendingTimers = new Map<number, PendingTimerState>();

watch(() => cards.value.length, () => {
  nextTick(() => {
    if (feedRef.value) feedRef.value.scrollTop = feedRef.value.scrollHeight;
  });
});

function clearPendingTimer(seq: number) {
  const st = pendingTimers.get(seq);
  if (st && st.timeoutId !== null) window.clearTimeout(st.timeoutId);
  pendingTimers.delete(seq);
}

function armPendingTimer(seq: number, ms: number) {
  // If the user is already paused when the key is typed (rare but possible
  // when injecting a key into a frozen simulator), arm the timer in the
  // paused state so wall-clock cannot tick it down behind the user's back.
  if (cpu.paused) {
    pendingTimers.set(seq, { timeoutId: null, deadline: Date.now() + ms, remaining: ms });
    return;
  }
  const id = window.setTimeout(() => resolveBuffered(seq), ms);
  pendingTimers.set(seq, { timeoutId: id, deadline: Date.now() + ms, remaining: ms });
}

// Freeze (paused=true) or resume (paused=false) every running countdown.
// On freeze we keep the remaining ms so resume restarts from where it
// stopped instead of restarting the full timeout.
watch(() => cpu.paused, (p) => {
  if (p) {
    for (const [, st] of pendingTimers) {
      if (st.timeoutId === null) continue;
      window.clearTimeout(st.timeoutId);
      st.remaining = Math.max(0, st.deadline - Date.now());
      st.timeoutId = null;
    }
  } else {
    for (const [seq, st] of pendingTimers) {
      if (st.timeoutId !== null) continue;
      st.timeoutId = window.setTimeout(() => resolveBuffered(seq), st.remaining);
      st.deadline = Date.now() + st.remaining;
    }
  }
});

function pruneCards() {
  if (cards.value.length > MAX_CARDS) {
    const dropped = cards.value.splice(0, cards.value.length - MAX_CARDS);
    for (const d of dropped) clearPendingTimer(d.seq);
  }
}

// pushKey() is patched at runtime so the narrator captures the character that
// was sent. This is the cleanest signal — server-side echo events strip
// non-printable chars and the IRQ event has no char payload.
const originalPushKey = cpu.pushKey;
cpu.pushKey = (key: string) => {
  if (prefs.narratorsEnabled && key !== '') {
    const ch = key[0];
    const seq = ++seqCounter;
    cards.value.push({
      seq,
      char: ch,
      hex: '0x' + ch.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0'),
      status: 'pending',
    });
    pruneCards();
    // Start the wait timer immediately. If cpu.paused is already true when
    // the user types (e.g. injects a key while the sim is frozen), the
    // pause watcher above will freeze it on the next microtask.
    armPendingTimer(seq, pendingTimeoutMs());
  }
  originalPushKey(key);
};

function resolveDelivered(pid: number) {
  // Resolve the oldest unresolved card (pending OR buffered). The buffered
  // case happens when process_state arrived after the timeout — we upgrade
  // the card rather than leave it stuck on a wrong label.
  const card = cards.value.find(c => c.status !== 'delivered');
  if (!card) return;
  clearPendingTimer(card.seq);
  card.status = 'delivered';
  card.deliveredPid = pid;
  card.deliveredProg = cpu.processNameOf(pid) || t('panels.console.pidUnknown');
  card.bufferedQueue = undefined;
}

function resolveBuffered(seq: number) {
  const card = cards.value.find(c => c.seq === seq);
  if (!card || card.status !== 'pending') return;
  clearPendingTimer(seq);
  card.status = 'buffered';
  card.bufferedQueue = cpu.kbdBufUsed;
}

const unsubscribe = cpu.subscribe((ev: CpuEvent) => {
  if (!prefs.narratorsEnabled) return;
  if (ev.type === 'process_state' && ev.reason === 'keyboard delivered') {
    resolveDelivered(ev.pid);
  }
});

onBeforeUnmount(() => {
  unsubscribe();
  for (const st of pendingTimers.values()) {
    if (st.timeoutId !== null) window.clearTimeout(st.timeoutId);
  }
  pendingTimers.clear();
  cpu.pushKey = originalPushKey;
});

function dropCard(seq: number) {
  clearPendingTimer(seq);
  const idx = cards.value.findIndex(c => c.seq === seq);
  if (idx >= 0) cards.value.splice(idx, 1);
}

function charLabel(card: NarratorCard): string {
  if (card.char === '\n') return '⏎';
  if (card.char === ' ') return '␣';
  if (card.char === '\t') return '⇥';
  return card.char;
}

function outcomeLabel(card: NarratorCard): string {
  if (card.status === 'delivered') {
    return t('panels.console.narrator.delivered', {
      pid: card.deliveredPid ?? 0,
      prog: card.deliveredProg ?? '',
    });
  }
  if (card.status === 'buffered') {
    return t('panels.console.narrator.buffered', { n: card.bufferedQueue ?? 0 });
  }
  return '…';
}
</script>

<template>
  <div class="kbd-narrator" role="status" aria-live="polite">
    <div class="kbd-narrator__title">{{ t('panels.console.narrator.title') }}</div>
    <div ref="feedRef" class="kbd-narrator__feed">
      <div v-if="cards.length === 0" class="kbd-narrator__idle">
        {{ t('panels.console.narrator.idle') }}
      </div>
      <transition-group v-else name="narrator-card" tag="ul" class="kbd-narrator__list">
        <li
          v-for="card in cards"
          :key="card.seq"
          class="kbd-narrator__row"
          :class="`is-${card.status}`"
          @click="dropCard(card.seq)"
        >
          <span class="kbd-narrator__char">{{ charLabel(card) }}</span>
          <span class="kbd-narrator__hex">{{ card.hex }}</span>
          <span class="kbd-narrator__arrow">→</span>
          <span class="kbd-narrator__outcome" :class="{ 'is-pending': card.status === 'pending' }">
            {{ outcomeLabel(card) }}
          </span>
        </li>
      </transition-group>
    </div>
  </div>
</template>

<style scoped>
.kbd-narrator {
  display: flex;
  flex-direction: column;
  background: var(--terminal-bg);
  border: 1px solid var(--terminal-border);
  box-shadow: inset 0 0 14px var(--surface-overlay-strong);
  position: relative;
  overflow: hidden;
  min-height: 0;
}

.kbd-narrator::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: repeating-linear-gradient(
    0deg,
    transparent 0,
    color-mix(in oklab, var(--amber) 4%, transparent) 1.5px,
    transparent 3px
  );
  opacity: 0.6;
  mix-blend-mode: screen;
}

.kbd-narrator__title {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--amber);
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 0 10px;
  height: 32px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  border-bottom: 1px solid var(--terminal-border);
  background: color-mix(in oklab, var(--terminal-bg) 88%, var(--amber) 12%);
  position: relative;
  z-index: 1;
}

.kbd-narrator__feed {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px;
  position: relative;
  z-index: 0;
  min-height: 0;
}

.kbd-narrator__idle {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-style: italic;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.kbd-narrator__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.kbd-narrator__row {
  display: grid;
  grid-template-columns: 22px 44px 14px 1fr;
  align-items: baseline;
  gap: 4px;
  padding: 2px 4px;
  border-left: 2px solid var(--amber);
  border-radius: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.3;
  color: var(--terminal-fg);
  cursor: pointer;
  background: color-mix(in oklab, var(--amber) 6%, transparent);
}

.kbd-narrator__row.is-delivered {
  border-left-color: var(--green);
  background: color-mix(in oklab, var(--green) 6%, transparent);
}

.kbd-narrator__row.is-buffered {
  border-left-color: var(--cyan);
  background: color-mix(in oklab, var(--cyan) 6%, transparent);
}

.kbd-narrator__char {
  font-family: 'VT323', monospace;
  font-size: 16px;
  color: var(--green);
  text-align: center;
}

.kbd-narrator__hex {
  color: var(--text-dim);
  font-size: 10px;
}

.kbd-narrator__arrow {
  color: var(--text-muted);
  text-align: center;
}

.kbd-narrator__outcome {
  color: var(--green);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kbd-narrator__row.is-buffered .kbd-narrator__outcome {
  color: var(--cyan);
}

.kbd-narrator__outcome.is-pending {
  color: var(--text-muted);
}

@media (max-width: 768px) {
  /* When stacked under the monitor, the narrator needs an explicit height
     so the monitor still gets most of the vertical real estate. The
     monitor + narrator together must leave enough room for the keyboard
     section underneath, so we cap this fairly tight. */
  .kbd-narrator {
    height: 120px;
    flex-shrink: 0;
  }
}

@media (max-width: 440px) {
  /* The arrow column is decorative; dropping it lets the hex + outcome
     keep readable widths down to 320px without overflowing. */
  .kbd-narrator__row {
    grid-template-columns: 20px 38px 1fr;
    font-size: 10px;
  }
  .kbd-narrator__arrow { display: none; }
  .kbd-narrator__char { font-size: 14px; }
  .kbd-narrator__hex { font-size: 9px; }
}

.narrator-card-enter-active,
.narrator-card-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}

.narrator-card-enter-from {
  opacity: 0;
  transform: translateX(-4px);
}

.narrator-card-leave-to {
  opacity: 0;
  transform: translateX(4px);
}
</style>
