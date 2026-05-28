import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useCpuStore } from '../stores/cpu';
import { useAuthStore } from '../stores/auth';
import { useWhiteboardStore } from '../stores/whiteboard';

// Both HZ and QUANTUM apply on every keystroke through a small debounce so the
// backend gets one command per "settled" value instead of one per digit. The
// 200ms window also feels instant during a class while protecting the WS from
// a flurry of writes when the student scrubs the spinner.
const SETTING_DEBOUNCE_MS = 200;

export function useSimSettings() {
  const cpu = useCpuStore();
  const auth = useAuthStore();
  const whiteboard = useWhiteboardStore();
  const router = useRouter();

  const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));
  const viewer = computed(() => cpu.isViewer());
  const canShare = computed(() => (cpu.sessionInfo?.role ?? 'none') === 'owner');
  const isShared = computed(() => Boolean(cpu.sessionInfo?.view_token));
  const whiteboardOn = computed(() => whiteboard.broadcasting);

  const shareOpen = ref(false);

  let hzTimer: number | null = null;
  let qTimer: number | null = null;

  function onClockHz(e: Event) {
    const v = Number((e.target as HTMLInputElement).value);
    if (!Number.isFinite(v)) return;
    const clamped = Math.max(1, Math.min(60, Math.round(v)));
    if (hzTimer !== null) window.clearTimeout(hzTimer);
    hzTimer = window.setTimeout(() => {
      hzTimer = null;
      cpu.setClockHz(clamped);
    }, SETTING_DEBOUNCE_MS);
  }

  function onQuantum(e: Event) {
    const v = Number((e.target as HTMLInputElement).value);
    if (!Number.isFinite(v)) return;
    const clamped = Math.max(0, Math.min(200, Math.round(v)));
    if (qTimer !== null) window.clearTimeout(qTimer);
    qTimer = window.setTimeout(() => {
      qTimer = null;
      cpu.setQuantum(clamped);
    }, SETTING_DEBOUNCE_MS);
  }

  function toggleWhiteboard() {
    const next = !whiteboard.broadcasting;
    whiteboard.setBroadcasting(next);
    if (!next) cpu.sendPointerOff();
  }

  function openShare() {
    shareOpen.value = true;
  }

  async function goLogin() {
    await router.push('/login');
  }

  async function doLogout() {
    await auth.logout();
  }

  return {
    cpu,
    auth,
    whiteboard,
    loggedIn,
    viewer,
    canShare,
    isShared,
    whiteboardOn,
    shareOpen,
    onClockHz,
    onQuantum,
    toggleWhiteboard,
    openShare,
    goLogin,
    doLogout,
  };
}
