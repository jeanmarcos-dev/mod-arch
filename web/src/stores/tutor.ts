import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

export type TutorDisplayMode = 'docked' | 'expanded';

const STORAGE_KEY = 'modos-cpu:tutor:v1';
// Cap how many messages we keep in localStorage. Each TutorMessage is small
// (~a few hundred bytes), so 50 entries comfortably fit a long classroom
// session without pushing the per-origin quota.
const MESSAGES_CAP = 50;

export const INTERRUPTED_ERROR_CODE = 'interrupted';
export const TUTOR_UNAVAILABLE_CODE = 'tutor_unavailable';

export type TutorRating = 1 | -1;

interface PersistedTutorState {
  displayMode: TutorDisplayMode;
  messages: TutorMessage[];
}

function loadPersisted(): PersistedTutorState {
  if (typeof window === 'undefined') return { displayMode: 'docked', messages: [] };
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return { displayMode: 'docked', messages: [] };
    const parsed = JSON.parse(raw) as Partial<PersistedTutorState>;
    const displayMode = parsed.displayMode === 'expanded' ? 'expanded' : 'docked';
    const messages = Array.isArray(parsed.messages) ? hydrateMessages(parsed.messages) : [];
    return { displayMode, messages };
  } catch {
    return { displayMode: 'docked', messages: [] };
  }
}

// hydrateMessages restores the persisted list and finalizes any message that
// was still mid-flight when the page closed. A spinner stuck forever after a
// reload is worse than telling the student the request was interrupted.
function hydrateMessages(raw: unknown[]): TutorMessage[] {
  const out: TutorMessage[] = [];
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue;
    const m = item as Partial<TutorMessage>;
    if (!m.id || (m.role !== 'student' && m.role !== 'tutor')) continue;
    const message: TutorMessage = {
      id: m.id,
      role: m.role,
      text: typeof m.text === 'string' ? m.text : '',
      citations: m.citations,
      latencyMs: m.latencyMs,
      cacheHit: m.cacheHit,
      degraded: m.degraded,
      error: m.error,
      errorCode: m.errorCode,
      retryAfterSec: m.retryAfterSec,
      toolsUsed: m.toolsUsed,
      requestId: m.requestId ?? (m.role === 'tutor' && m.id.startsWith('a-') ? m.id.slice(2) : undefined),
      rating: m.rating === 1 || m.rating === -1 ? m.rating : undefined,
      createdAt: typeof m.createdAt === 'number' ? m.createdAt : Date.now(),
    };
    if (m.pending || m.toolInFlight) {
      message.pending = false;
      message.toolInFlight = undefined;
      message.queueWaitSeconds = undefined;
      if (!message.error) {
        message.error = INTERRUPTED_ERROR_CODE;
        message.errorCode = INTERRUPTED_ERROR_CODE;
      }
    }
    out.push(message);
  }
  // Trim from the head if the persisted blob exceeded the cap (e.g. the cap
  // was lowered between versions).
  return out.length > MESSAGES_CAP ? out.slice(out.length - MESSAGES_CAP) : out;
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
  errorCode?: string;
  retryAfterSec?: number;
  requestId?: string;
  rating?: TutorRating;
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
  source: string; // 'glossary' | 'silberschatz' | ...
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
  error_code?: string;
  code?: string;
  retry_after_sec?: number;
}

export function answerErrorCode(ans: IncomingAnswer): string | undefined {
  if (!ans.error) return undefined;
  const code = ans.error_code ?? ans.code ?? (ans.error === TUTOR_UNAVAILABLE_CODE ? ans.error : undefined);
  return code || undefined;
}

/**
 * tutor store: history + open/close state for the tutor panel. Has no direct
 * link to the WebSocket — the useTutor composable bridges the two so cpu.ts
 * does not need to import this store.
 */
export const useTutorStore = defineStore('tutor', () => {
  const persisted = loadPersisted();
  const messages = ref<TutorMessage[]>(persisted.messages);
  const open = ref(false);
  const sending = ref(false);
  const displayMode = ref<TutorDisplayMode>(persisted.displayMode);
  const unreadCount = ref(0);

  // Tracks request_id → message index so out-of-order answers still land in
  // the right slot. Cleared when the answer arrives.
  const pendingByRequest = new Map<string, number>();
  // Request ids already counted toward unreadCount, so streaming + final
  // answer for the same turn do not double-count.
  const countedAsUnread = new Set<string>();

  function bumpUnreadIfClosed(requestId: string) {
    if (open.value) return;
    if (countedAsUnread.has(requestId)) return;
    countedAsUnread.add(requestId);
    unreadCount.value += 1;
  }

  function persist() {
    try {
      const trimmed =
        messages.value.length > MESSAGES_CAP
          ? messages.value.slice(messages.value.length - MESSAGES_CAP)
          : messages.value;
      const payload: PersistedTutorState = { displayMode: displayMode.value, messages: trimmed };
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
    } catch {
      /* private mode / quota — keep state in memory only */
    }
  }

  // One deep watcher persists both displayMode and the message list. Volume
  // is tiny (a few writes per turn) so no debounce is needed.
  watch([displayMode, messages], persist, { deep: true });

  function toggleOpen() {
    open.value = !open.value;
    if (open.value) unreadCount.value = 0;
  }

  function setOpen(v: boolean) {
    open.value = v;
    if (v) unreadCount.value = 0;
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
    unreadCount.value = 0;
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
        errorCode: answerErrorCode(ans),
        requestId: ans.request_id || undefined,
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
    msg.errorCode = answerErrorCode(ans);
    msg.requestId = ans.request_id;
    msg.retryAfterSec = ans.retry_after_sec;
    if (!ans.error) bumpUnreadIfClosed(ans.request_id);
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
          bumpUnreadIfClosed(p.request_id);
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

  function rate(messageId: string, value: TutorRating): { requestId: string; rating: TutorRating | 0 } | null {
    const msg = messages.value.find((m) => m.id === messageId);
    if (!msg || msg.role !== 'tutor' || msg.pending || msg.error || !msg.requestId) return null;
    const next: TutorRating | 0 = msg.rating === value ? 0 : value;
    msg.rating = next === 0 ? undefined : next;
    return { requestId: msg.requestId, rating: next };
  }

  const lastMessage = computed(() => messages.value[messages.value.length - 1] ?? null);

  return {
    messages,
    open,
    sending,
    displayMode,
    lastMessage,
    unreadCount,
    toggleOpen,
    setOpen,
    setDisplayMode,
    toggleDisplayMode,
    clear,
    startAsk,
    applyAnswer,
    applyProgress,
    failPending,
    rate,
  };
});
