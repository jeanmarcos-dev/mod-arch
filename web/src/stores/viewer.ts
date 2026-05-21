import { defineStore } from 'pinia';
import { ref } from 'vue';

// Viewer store keeps the spectator-side UI state (autofollow toggle, hand
// raise counters). The CPU store owns the cross-cutting WS payloads —
// lastPresence, handRaiseSignal — and this store layers UI-only intent on
// top of them so the topbar and section components do not have to know
// about WS plumbing.
export const useViewerStore = defineStore('viewer', () => {
  // Autofollow defaults to ON: a freshly joined viewer should land where
  // the professor is. The toggle is per-tab (no persistence) because a
  // student returning later usually wants to re-sync.
  const autofollow = ref(true);
  // Count of hand-raise events the host has seen since the page loaded.
  // Reset when the host clicks the badge.
  const handRaiseCount = ref(0);
  // Label of whoever raised their hand last — surfaced in the toast.
  const lastHandRaiseLabel = ref('');

  function setAutofollow(on: boolean) {
    autofollow.value = on;
  }
  function toggleAutofollow() {
    autofollow.value = !autofollow.value;
  }
  function noteHandRaise(label: string) {
    handRaiseCount.value++;
    lastHandRaiseLabel.value = label;
  }
  function clearHandRaises() {
    handRaiseCount.value = 0;
    lastHandRaiseLabel.value = '';
  }

  return {
    autofollow,
    handRaiseCount,
    lastHandRaiseLabel,
    setAutofollow,
    toggleAutofollow,
    noteHandRaise,
    clearHandRaises,
  };
});
