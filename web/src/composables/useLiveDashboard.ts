import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { LIVE_WINDOWS_MINUTES, useAdminStore } from '../stores/admin';

export const LIVE_REFRESH_MS = 3000;
export const LIVE_STORAGE_KEY = 'modos-cpu:admin-live:v1';
const DEFAULT_WINDOW_MINUTES = 120;

interface StoredLivePrefs {
  presentation: boolean;
  windowMinutes: number;
}

function isAllowedWindow(v: unknown): v is number {
  return typeof v === 'number' && (LIVE_WINDOWS_MINUTES as readonly number[]).includes(v);
}

export function readLivePrefs(): StoredLivePrefs {
  const fallback = { presentation: true, windowMinutes: DEFAULT_WINDOW_MINUTES };
  try {
    const raw = window.localStorage.getItem(LIVE_STORAGE_KEY);
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as Partial<StoredLivePrefs>;
    return {
      presentation: parsed.presentation === false ? false : true,
      windowMinutes: isAllowedWindow(parsed.windowMinutes) ? parsed.windowMinutes : DEFAULT_WINDOW_MINUTES,
    };
  } catch {
    return fallback;
  }
}

function writeLivePrefs(prefs: StoredLivePrefs) {
  try {
    window.localStorage.setItem(LIVE_STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    return;
  }
}

export function useLiveDashboard() {
  const admin = useAdminStore();
  const initial = readLivePrefs();
  const presentation = ref(initial.presentation);
  const windowMinutes = ref(initial.windowMinutes);
  const lastOkAt = ref<number | null>(null);
  const failed = ref(false);
  const loading = ref(false);
  let inFlight = false;
  let queued = false;
  let timer: number | undefined;

  const stale = computed(() => failed.value && lastOkAt.value !== null);

  async function refresh(force = false) {
    if (inFlight) {
      if (force) queued = true;
      return;
    }
    inFlight = true;
    loading.value = admin.live === null;
    try {
      await admin.loadLive(windowMinutes.value);
      lastOkAt.value = Date.now();
      failed.value = false;
    } catch {
      failed.value = true;
    } finally {
      inFlight = false;
      loading.value = false;
    }
    if (queued) {
      queued = false;
      await refresh();
    }
  }

  watch([presentation, windowMinutes], () => {
    writeLivePrefs({ presentation: presentation.value, windowMinutes: windowMinutes.value });
  });

  watch(windowMinutes, () => {
    refresh(true);
  });

  onMounted(() => {
    refresh();
    timer = window.setInterval(() => refresh(), LIVE_REFRESH_MS);
  });

  onBeforeUnmount(() => {
    if (timer !== undefined) window.clearInterval(timer);
  });

  return { presentation, windowMinutes, lastOkAt, failed, stale, loading, refresh };
}
