import { onBeforeUnmount, onMounted } from 'vue';

import { useCpuStore } from '../stores/cpu';
import { useTutorStore, type IncomingAnswer, type IncomingProgress } from '../stores/tutor';

/**
 * useTutor wires the tutor store to the WebSocket. cpu.ts re-dispatches
 * incoming `tutor_answer` events as the `modos:tutor-answer` window event
 * so this composable can subscribe without taking a hard dependency on the
 * cpu store's internals.
 *
 * Returns helpers the panel uses to send questions and clear history.
 */
export function useTutor() {
  const cpu = useCpuStore();
  const tutor = useTutorStore();

  function onAnswer(e: Event) {
    const ce = e as CustomEvent<unknown>;
    const detail = ce.detail as IncomingAnswer | null;
    if (!detail || detail.type !== 'tutor_answer') return;
    tutor.applyAnswer(detail);
  }

  function onProgress(e: Event) {
    const ce = e as CustomEvent<unknown>;
    const detail = ce.detail as IncomingProgress | null;
    if (!detail || detail.type !== 'tutor_progress') return;
    tutor.applyProgress(detail);
  }

  onMounted(() => {
    window.addEventListener('modos:tutor-answer', onAnswer);
    window.addEventListener('modos:tutor-progress', onProgress);
  });
  onBeforeUnmount(() => {
    window.removeEventListener('modos:tutor-answer', onAnswer);
    window.removeEventListener('modos:tutor-progress', onProgress);
  });

  /** Sends a question to the tutor. Falls back gracefully when offline. */
  function ask(question: string) {
    const trimmed = question.trim();
    if (!trimmed) return;
    const requestId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    // Build the multi-turn history from prior messages (newest last). The
    // server enforces both a hard cap (20 entries) and the admin-configured
    // HistoryWindowTurns, so a generous slice of 20 here is safe.
    const history = tutor.messages
      .filter((m) => !m.error && !m.pending && m.text.trim().length > 0)
      .slice(-20)
      .map((m) => ({
        role: (m.role === 'student' ? 'user' : 'assistant') as 'user' | 'assistant',
        text: m.text,
      }));
    // Mode is controlled by the admin globally; the client just sends the
    // question and lets the server pick the system prompt.
    tutor.startAsk(trimmed, requestId);
    try {
      cpu.sendTutorAsk(trimmed, requestId, history);
    } catch (err) {
      tutor.failPending(requestId, String(err));
    }
  }

  return { ask, tutor };
}
