import { ref } from 'vue';
import type { SessionInfo, Presence } from './types';

export function useCpuPresence() {
  const sessionInfo = ref<SessionInfo>({ id: '', mode: 'unknown', role: 'none', label: '', view_token: '', user_id: '', user_name: '', user_email: '' });
  const isViewer = () => sessionInfo.value.role === 'viewer';
  const lastPresence = ref<Presence>({ section: '', cores: [], mode: '', narrators: false });
  const handRaiseSignal = ref(0);
  const lastHandRaiseLabel = ref('');

  const ownerGlossaryHover = ref<string | null>(null);
  const GLOSSARY_STALE_MS = 5000;
  let glossaryStaleTimer: number | null = null;
  function clearGlossaryStaleTimer() {
    if (glossaryStaleTimer !== null) {
      window.clearTimeout(glossaryStaleTimer);
      glossaryStaleTimer = null;
    }
  }
  function armGlossaryStaleTimer() {
    clearGlossaryStaleTimer();
    glossaryStaleTimer = window.setTimeout(() => {
      ownerGlossaryHover.value = null;
      glossaryStaleTimer = null;
    }, GLOSSARY_STALE_MS);
  }

  const faultShake = ref(false);
  const glossarySearch = ref('');
  const glossaryOpenSignal = ref(0);
  function openGlossary(term?: string) {
    glossarySearch.value = term ?? '';
    glossaryOpenSignal.value++;
  }

  return {
    sessionInfo, isViewer, lastPresence, handRaiseSignal, lastHandRaiseLabel,
    ownerGlossaryHover, clearGlossaryStaleTimer, armGlossaryStaleTimer,
    faultShake, glossarySearch, glossaryOpenSignal, openGlossary
  };
}
