import { onBeforeUnmount, ref, type Ref } from 'vue';

const TOUCH_QUERY = '(hover: none)';

export function useTouchInput(): Ref<boolean> {
  const isTouch = ref(false);
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return isTouch;

  const mql = window.matchMedia(TOUCH_QUERY);
  const onChange = (ev: MediaQueryListEvent) => { isTouch.value = ev.matches; };
  isTouch.value = mql.matches;
  mql.addEventListener('change', onChange);
  onBeforeUnmount(() => mql.removeEventListener('change', onChange));
  return isTouch;
}
