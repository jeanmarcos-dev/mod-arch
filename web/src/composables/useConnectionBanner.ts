import { computed, ref, watch } from 'vue';
import { useCpuStore } from '../stores/cpu';

// On cold start we wait this long before painting anything. If the WebSocket
// opens within the window, the user never sees the banner — and no layout
// shift is triggered downstream by the shell either, because consumers gate
// padding/offset adjustments on the same `visible` flag exposed here.
const COLD_GRACE_MS = 600;

const coldGraceElapsed = ref(false);
window.setTimeout(() => { coldGraceElapsed.value = true; }, COLD_GRACE_MS);

const dismissed = ref(false);

let watcherInited = false;

export function useConnectionBanner() {
  const cpu = useCpuStore();

  if (!watcherInited) {
    watcherInited = true;
    watch(() => cpu.connected, (online) => {
      if (online) dismissed.value = false;
    });
  }

  const phase = computed<'connecting' | 'lost' | null>(() => {
    if (cpu.connected) return null;
    return cpu.hasEverConnected ? 'lost' : 'connecting';
  });

  const visible = computed(() => {
    if (phase.value === null || dismissed.value) return false;
    if (phase.value === 'lost') return true;
    return coldGraceElapsed.value;
  });

  function dismiss() { dismissed.value = true; }

  return { phase, visible, dismiss };
}
