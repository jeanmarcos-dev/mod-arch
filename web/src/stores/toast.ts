import { defineStore } from 'pinia';
import { ref } from 'vue';

export type ToastTone = 'ok' | 'err' | 'info';

export interface ToastItem {
  id: number;
  message: string;
  tone: ToastTone;
}

let nextId = 1;

export const useToastStore = defineStore('toast', () => {
  const items = ref<ToastItem[]>([]);

  function push(message: string, tone: ToastTone = 'ok', ttlMs = 2500) {
    const id = nextId++;
    items.value.push({ id, message, tone });
    window.setTimeout(() => {
      items.value = items.value.filter((t) => t.id !== id);
    }, ttlMs);
  }

  function ok(message: string, ttlMs?: number) { push(message, 'ok', ttlMs); }
  function err(message: string, ttlMs?: number) { push(message, 'err', ttlMs); }
  function info(message: string, ttlMs?: number) { push(message, 'info', ttlMs); }

  return { items, push, ok, err, info };
});
