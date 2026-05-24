import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

export type TutorDisplayMode = 'docked' | 'expanded';

const STORAGE_KEY = 'modos-cpu:tutor:v1';

interface PersistedTutorState {
  displayMode: TutorDisplayMode;
}

function loadPersisted(): PersistedTutorState {
  if (typeof window === 'undefined') return { displayMode: 'docked' };
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return { displayMode: 'docked' };
    const parsed = JSON.parse(raw) as Partial<PersistedTutorState>;
    const m = parsed.displayMode === 'expanded' ? 'expanded' : 'docked';
    return { displayMode: m };
  } catch {
    return { displayMode: 'docked' };
  }
}

// One entry in the conversation. role='student' is the user, role='tutor' is
// the LLM. Errors are stored as a tutor message with `error` set so we keep
// a single ordered list instead of two parallel arrays.
export interface TutorMessage {
  id: string;
  role: 'student' | 'tutor';
  text: string;
  citations?: TutorCitation[];
  latencyMs?: number;
  cacheHit?: boolean;
  degraded?: boolean;
  error?: string;
  retryAfterSec?: number;
  pending?: boolean;
  // Currently-running tool the agent has called while still pending. Cleared
  // when the answer arrives or the agent moves on.
  toolInFlight?: string;
  toolsUsed?: string[];
  // queueWaitSeconds is the ETA the rate-limit bucket reports when this
  // request was parked. Cleared once the token is acquired.
  queueWaitSeconds?: number;
  createdAt: number;
}

export interface IncomingProgress {
  type: 'tutor_progress';
  request_id: string;
  stage: 'tool_call' | 'tool_result' | 'token' | 'queued' | 'retry';
  tool?: string;
  call_id?: string;
  ok?: boolean;
  error?: string;
  delta?: string;
  wait_seconds?: number;
  queue_depth?: number;
  attempt?: number;
}

export interface TutorCitation {
  source: string; // 'glossary' | 'stallings' | ...
  id: string;
  label: string;
  page?: number;
  excerpt?: string;
}

export interface IncomingAnswer {
  type: 'tutor_answer';
  request_id: string;
  text?: string;
  citations?: TutorCitation[];
  latency_ms?: number;
  cache_hit?: boolean;
  degraded?: boolean;
  error?: string;
  retry_after_sec?: number;
}

/**
 * tutor store: history + open/close state for the tutor panel. Has no direct
 * link to the WebSocket — the useTutor composable bridges the two so cpu.ts
 * does not need to import this store.
 */
export const useTutorStore = defineStore('tutor', () => {
  const messages = ref<TutorMessage[]>([]);
  const open = ref(false);
  const sending = ref(false);
  const displayMode = ref<TutorDisplayMode>(loadPersisted().displayMode);

  // Tracks request_id → message index so out-of-order answers still land in
  // the right slot. Cleared when the answer arrives.
  const pendingByRequest = new Map<string, number>();

  watch(displayMode, (m) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ displayMode: m }));
    } catch {
      /* private mode / quota — keep state in memory only */
    }
  });

  function toggleOpen() {
    open.value = !open.value;
  }

  function setOpen(v: boolean) {
    open.value = v;
  }

  function setDisplayMode(m: TutorDisplayMode) {
    displayMode.value = m;
  }

  function toggleDisplayMode() {
    displayMode.value = displayMode.value === 'docked' ? 'expanded' : 'docked';
  }

  function clear() {
    messages.value = [];
    pendingByRequest.clear();
    sending.value = false;
  }

  /** Adds the student's question and a placeholder for the pending answer. */
  function startAsk(question: string, requestId: string) {
    const now = Date.now();
    messages.value.push({
      id: `q-${requestId}`,
      role: 'student',
      text: question,
      createdAt: now,
    });
    const placeholderIndex = messages.value.push({
      id: `a-${requestId}`,
      role: 'tutor',
      text: '',
      pending: true,
      createdAt: now,
    }) - 1;
    pendingByRequest.set(requestId, placeholderIndex);
    sending.value = true;
  }

  /** Resolves the pending answer slot with whatever the server sent back. */
  function applyAnswer(ans: IncomingAnswer) {
    const idx = pendingByRequest.get(ans.request_id);
    pendingByRequest.delete(ans.request_id);
    sending.value = pendingByRequest.size > 0;
    if (idx === undefined) {
      // Out-of-band answer (e.g. server restart): append as a standalone
      // message so we do not silently drop it.
      messages.value.push({
        id: `a-${ans.request_id || `t-${Date.now()}`}`,
        role: 'tutor',
        text: ans.text ?? '',
        error: ans.error,
        citations: ans.citations,
        latencyMs: ans.latency_ms,
        cacheHit: ans.cache_hit,
        degraded: ans.degraded,
        retryAfterSec: ans.retry_after_sec,
        createdAt: Date.now(),
      });
      return;
    }
    const msg = messages.value[idx];
    msg.pending = false;
    msg.text = ans.text ?? '';
    msg.citations = ans.citations;
    msg.latencyMs = ans.latency_ms;
    msg.cacheHit = ans.cache_hit;
    msg.degraded = ans.degraded;
    msg.error = ans.error;
    msg.retryAfterSec = ans.retry_after_sec;
  }

  /** Updates the pending slot with the tool the agent just invoked, or
   *  appends streamed tokens to the answer text. */
  function applyProgress(p: IncomingProgress) {
    const idx = pendingByRequest.get(p.request_id);
    if (idx === undefined) return;
    const msg = messages.value[idx];
    if (!msg.pending) return;
    switch (p.stage) {
      case 'tool_call':
        if (p.tool) {
          msg.toolInFlight = p.tool;
          msg.toolsUsed = [...(msg.toolsUsed ?? []), p.tool];
        }
        msg.queueWaitSeconds = undefined;
        break;
      case 'tool_result':
        msg.toolInFlight = undefined;
        break;
      case 'token':
        if (p.delta) {
          msg.text += p.delta;
          // First token closes the "consultando…" state visually.
          msg.toolInFlight = undefined;
          msg.queueWaitSeconds = undefined;
        }
        break;
      case 'queued':
        msg.queueWaitSeconds = p.wait_seconds ?? 0;
        msg.toolInFlight = undefined;
        break;
      case 'retry':
        // Rate-limit retry kicked in; show how many seconds the orchestrator
        // is waiting before its next attempt.
        msg.queueWaitSeconds = p.wait_seconds ?? 0;
        break;
    }
  }

  /** Marks the pending slot as failed when the WS fails before any answer. */
  function failPending(requestId: string, reason: string) {
    const idx = pendingByRequest.get(requestId);
    pendingByRequest.delete(requestId);
    sending.value = pendingByRequest.size > 0;
    if (idx === undefined) return;
    const msg = messages.value[idx];
    msg.pending = false;
    msg.error = reason;
  }

  const lastMessage = computed(() => messages.value[messages.value.length - 1] ?? null);

  return {
    messages,
    open,
    sending,
    displayMode,
    lastMessage,
    toggleOpen,
    setOpen,
    setDisplayMode,
    toggleDisplayMode,
    clear,
    startAsk,
    applyAnswer,
    applyProgress,
    failPending,
  };
});
