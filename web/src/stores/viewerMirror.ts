import { defineStore } from 'pinia';
import { computed } from 'vue';
import { useCpuStore, type Focus, type MiddleTab, type MobileTab, type SimBarMode } from './cpu';

// useViewerMirrorStore exposes the host's inner-panel state — middle/mobile
// tab, memory base address, pinned PID, etc. — to viewer-side components so
// they can render exactly what the owner is presenting. The store reads
// reactively from cpu.lastPresence.focus and exposes a typed surface that
// components consume via computed refs. Each value falls back to undefined
// when the owner has not expressed an opinion yet so callers can keep their
// own local default.
//
// Owner-side components do NOT read this store: they own their local refs
// and emit them via cpu.sendPresence. The mirror is one-way — viewer reads
// what the owner publishes.
export const useViewerMirrorStore = defineStore('viewerMirror', () => {
  const cpu = useCpuStore();

  const focus = computed<Focus>(() => cpu.lastPresence.focus ?? {});

  const middleTab = computed<MiddleTab | undefined>(() => focus.value.middle_tab);
  const mobileTab = computed<MobileTab | undefined>(() => focus.value.mobile_tab);
  const memoryBaseAddr = computed<number | undefined>(() => focus.value.memory_base_addr);
  const memoryAutoFollow = computed<boolean | undefined>(() => focus.value.memory_auto_follow);
  const pinnedPID = computed<number | undefined>(() => focus.value.pinned_pid);
  const simBarMode = computed<SimBarMode | undefined>(() => focus.value.sim_bar_mode);
  const sidebarCollapsed = computed<boolean | undefined>(() => focus.value.sidebar_collapsed);
  const isMobile = computed<boolean | undefined>(() => focus.value.is_mobile);

  return {
    focus,
    middleTab,
    mobileTab,
    memoryBaseAddr,
    memoryAutoFollow,
    pinnedPID,
    simBarMode,
    sidebarCollapsed,
    isMobile,
  };
});
