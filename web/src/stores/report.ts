import { defineStore } from 'pinia';
import { ref } from 'vue';

// Tiny store that wires the report modal to the launcher buttons that live
// in different toolbars. Keeping it here avoids prop-drilling open/close
// state through the AppShell + SimBar + Topbar.
export const useReportStore = defineStore('report', () => {
  const open = ref(false);

  function show() {
    open.value = true;
  }
  function hide() {
    open.value = false;
  }

  return { open, show, hide };
});
