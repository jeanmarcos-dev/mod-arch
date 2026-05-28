import { defineStore } from 'pinia';
import { ref } from 'vue';

export type ToastTone = 'ok' | 'err' | 'info';

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface ToastItem {
  id: number;
  message: string;
  tone: ToastTone;
  action?: ToastAction;
}

let nextId = 1;

export const useToastStore = defineStore('toast', () => {
  const items = ref<ToastItem[]>([]);

  function push(message: string, tone: ToastTone = 'ok', ttlMs = 2500, action?: ToastAction) {
    const id = nextId++;
    items.value.push({ id, message, tone, action });
    window.setTimeout(() => {
      items.value = items.value.filter((t) => t.id !== id);
    }, ttlMs);
    return id;
  }

  function dismiss(id: number) {
    items.value = items.value.filter((t) => t.id !== id);
  }

  function ok(message: string, ttlMs?: number) { push(message, 'ok', ttlMs); }
  function err(message: string, ttlMs?: number) { push(message, 'err', ttlMs); }
  function info(message: string, ttlMs?: number) { push(message, 'info', ttlMs); }

  // pushAction surfaces an inline button (typically "Undo"). The toast lingers
  // longer than a plain notification so the user has time to react.
  function pushAction(message: string, action: ToastAction, tone: ToastTone = 'info', ttlMs = 6000) {
    return push(message, tone, ttlMs, action);
  }

  return { items, push, dismiss, ok, err, info, pushAction };
});
