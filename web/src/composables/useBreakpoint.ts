import { onBeforeUnmount, ref, type Ref } from 'vue';

// Shared breakpoint convention used across the app. The two values must stay
// in sync with the @media queries hand-written in component <style> blocks
// (see the note near the global mobile block in style.css).
const MOBILE_QUERY = '(max-width: 768px)';
const NARROW_QUERY = '(max-width: 480px)';

export interface BreakpointHandle {
  /** True when the viewport is at most 768px wide. */
  isMobile: Ref<boolean>;
  /** True when the viewport is at most 480px wide — phone portrait. */
  isNarrow: Ref<boolean>;
}

export function useBreakpoint(): BreakpointHandle {
  const isMobile = ref(false);
  const isNarrow = ref(false);

  // Guard for SSR-like contexts (vitest jsdom is fine, but typing-wise
  // `window` may not exist on every entrypoint that imports the composable).
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return { isMobile, isNarrow };
  }

  const mobileMql = window.matchMedia(MOBILE_QUERY);
  const narrowMql = window.matchMedia(NARROW_QUERY);

  const onMobileChange = (ev: MediaQueryListEvent) => { isMobile.value = ev.matches; };
  const onNarrowChange = (ev: MediaQueryListEvent) => { isNarrow.value = ev.matches; };

  isMobile.value = mobileMql.matches;
  isNarrow.value = narrowMql.matches;

  mobileMql.addEventListener('change', onMobileChange);
  narrowMql.addEventListener('change', onNarrowChange);

  onBeforeUnmount(() => {
    mobileMql.removeEventListener('change', onMobileChange);
    narrowMql.removeEventListener('change', onNarrowChange);
  });

  return { isMobile, isNarrow };
}
