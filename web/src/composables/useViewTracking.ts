import { onBeforeUnmount, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useCpuStore } from '../stores/cpu';
import { createViewTracker } from '../lib/viewTracker';

export function useViewTracking() {
  const cpu = useCpuStore();
  const router = useRouter();
  const route = useRoute();
  const tracker = createViewTracker((cmd) => cpu.sendTrack(cmd));

  const removeGuard = router.afterEach((to) => tracker.enter(to.path));

  function onVisibility() {
    if (document.visibilityState === 'hidden') tracker.hide();
    else tracker.show();
  }

  function onPageHide() {
    tracker.hide();
  }

  onMounted(() => {
    tracker.enter(route.path);
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pagehide', onPageHide);
  });

  onBeforeUnmount(() => {
    removeGuard();
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('pagehide', onPageHide);
    tracker.hide();
  });

  return tracker;
}
