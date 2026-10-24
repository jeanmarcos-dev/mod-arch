import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { answerErrorCode, useTutorStore } from './tutor';

const STORAGE_KEY = 'modos-cpu:tutor:v1';

function answered(store: ReturnType<typeof useTutorStore>, requestId = 'r-1') {
  store.startAsk('¿Qué es un page fault?', requestId);
  store.applyAnswer({ type: 'tutor_answer', request_id: requestId, text: 'Es una excepción.' });
  return `a-${requestId}`;
}

describe('tutor store — ratings', () => {
  beforeEach(() => {
    window.localStorage.clear();
    setActivePinia(createPinia());
  });

  it('keeps the request id on the answered message', () => {
    const store = useTutorStore();
    const id = answered(store);
    expect(store.messages.find((m) => m.id === id)?.requestId).toBe('r-1');
  });

  it('rates an answer and returns what must be sent', () => {
    const store = useTutorStore();
    const id = answered(store);
    expect(store.rate(id, 1)).toEqual({ requestId: 'r-1', rating: 1 });
    expect(store.messages.find((m) => m.id === id)?.rating).toBe(1);
  });

  it('replaces the previous rating', () => {
    const store = useTutorStore();
    const id = answered(store);
    store.rate(id, 1);
    expect(store.rate(id, -1)).toEqual({ requestId: 'r-1', rating: -1 });
    expect(store.messages.find((m) => m.id === id)?.rating).toBe(-1);
  });

  it('clears the rating when the same button is pressed again', () => {
    const store = useTutorStore();
    const id = answered(store);
    store.rate(id, 1);
    expect(store.rate(id, 1)).toEqual({ requestId: 'r-1', rating: 0 });
    expect(store.messages.find((m) => m.id === id)?.rating).toBeUndefined();
  });

  it('does not rate student messages, pending answers or errors', () => {
    const store = useTutorStore();
    store.startAsk('hola', 'r-2');
    expect(store.rate('q-r-2', 1)).toBeNull();
    expect(store.rate('a-r-2', 1)).toBeNull();
    store.applyAnswer({ type: 'tutor_answer', request_id: 'r-2', error: 'falló' });
    expect(store.rate('a-r-2', 1)).toBeNull();
    expect(store.rate('inexistente', 1)).toBeNull();
  });

  it('persists the rating and restores it after a reload', async () => {
    const store = useTutorStore();
    const id = answered(store);
    store.rate(id, -1);
    await Promise.resolve();
    const raw = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? '{}');
    expect(raw.messages.find((m: { id: string }) => m.id === id).rating).toBe(-1);

    setActivePinia(createPinia());
    const reloaded = useTutorStore();
    const restored = reloaded.messages.find((m) => m.id === id);
    expect(restored?.rating).toBe(-1);
    expect(restored?.requestId).toBe('r-1');
  });

  it('derives the request id of messages persisted before ratings existed', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        displayMode: 'docked',
        messages: [{ id: 'a-old-1', role: 'tutor', text: 'respuesta', createdAt: 1 }],
      }),
    );
    const store = useTutorStore();
    expect(store.rate('a-old-1', 1)).toEqual({ requestId: 'old-1', rating: 1 });
  });

  it('ignores invalid persisted ratings', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        displayMode: 'docked',
        messages: [{ id: 'a-x', role: 'tutor', text: 't', rating: 5, createdAt: 1 }],
      }),
    );
    expect(useTutorStore().messages[0].rating).toBeUndefined();
  });
});

describe('tutor store — error codes', () => {
  beforeEach(() => {
    window.localStorage.clear();
    setActivePinia(createPinia());
  });

  it('extracts the code from error_code, code or a bare code in error', () => {
    expect(answerErrorCode({ type: 'tutor_answer', request_id: 'a', error: 'x', error_code: 'tutor_unavailable' })).toBe('tutor_unavailable');
    expect(answerErrorCode({ type: 'tutor_answer', request_id: 'a', error: 'x', code: 'tutor_unavailable' })).toBe('tutor_unavailable');
    expect(answerErrorCode({ type: 'tutor_answer', request_id: 'a', error: 'tutor_unavailable' })).toBe('tutor_unavailable');
    expect(answerErrorCode({ type: 'tutor_answer', request_id: 'a', error: 'otra cosa' })).toBeUndefined();
    expect(answerErrorCode({ type: 'tutor_answer', request_id: 'a', text: 'ok' })).toBeUndefined();
  });

  it('stores the code on a failed answer', () => {
    const store = useTutorStore();
    store.startAsk('hola', 'r-9');
    store.applyAnswer({ type: 'tutor_answer', request_id: 'r-9', error: 'tutor_unavailable', error_code: 'tutor_unavailable' });
    expect(store.messages.find((m) => m.id === 'a-r-9')?.errorCode).toBe('tutor_unavailable');
  });

  it('marks interrupted messages restored from storage with a code', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        displayMode: 'docked',
        messages: [{ id: 'a-p', role: 'tutor', text: '', pending: true, createdAt: 1 }],
      }),
    );
    const msg = useTutorStore().messages[0];
    expect(msg.pending).toBe(false);
    expect(msg.errorCode).toBe('interrupted');
  });
});
