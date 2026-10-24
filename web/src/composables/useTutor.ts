import { onBeforeUnmount, onMounted } from 'vue';

import { useConcurrencyState } from './useConcurrencyState';
import { useCpuStore } from '../stores/cpu';
import { useTutorStore, type IncomingAnswer, type IncomingProgress, type TutorRating } from '../stores/tutor';

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
  const concurrency = useConcurrencyState();

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
    //
    // For tutor turns, re-attach the inline citation labels (e.g.
    // "[Demo: x] [Docs: y]") that the server strips before display. The LLM
    // uses prior assistant turns as a few-shot signal for how to format new
    // answers; without the labels in history it stops emitting them, and
    // buildCitationsFromText on the server finds no matches — so the
    // citation chips disappear entirely.
    const history = tutor.messages
      .filter((m) => !m.error && !m.pending && m.text.trim().length > 0)
      .slice(-20)
      .map((m) => {
        let text = m.text;
        if (m.role === 'tutor' && m.citations && m.citations.length > 0) {
          const labels = m.citations.map((c) => c.label).join(' ');
          text = `${text} ${labels}`;
        }
        return {
          role: (m.role === 'student' ? 'user' : 'assistant') as 'user' | 'assistant',
          text,
        };
      });
    // Mode is controlled by the admin globally; the client just sends the
    // question and lets the server pick the system prompt.
    // A short Spanish summary of the live concurrency detectors (lost updates,
    // active deadlock cycle). Empty when nothing notable is happening; the
    // server only appends a context section when it is non-empty.
    const detectionContext = concurrency.tutorSummary();
    tutor.startAsk(trimmed, requestId);
    try {
      cpu.sendTutorAsk(trimmed, requestId, history, detectionContext);
    } catch (err) {
      tutor.failPending(requestId, String(err));
    }
  }

  function rate(messageId: string, value: TutorRating) {
    const result = tutor.rate(messageId, value);
    if (result) cpu.sendTutorFeedback(result.requestId, result.rating);
  }

  return { ask, rate, tutor };
}
