import { onUnmounted, ref, type Ref } from 'vue';

export interface ResizableDrawerOptions {
  getHeight: () => number;
  setHeight: (px: number) => void;
  minPx?: number;
  maxPx?: () => number;
}

export interface ResizableDrawerHandle {
  isResizing: Ref<boolean>;
  start: (event: PointerEvent) => void;
}

// useResizableDrawer drives a horizontal handle anchored to the TOP edge of a
// bottom-pinned drawer. Dragging up grows the drawer; dragging down shrinks
// it. Unlike useResizableRow this does not split a flex pair — the drawer has
// a single pixel height that is clamped to [minPx, maxPx()] and persisted by
// the caller via setHeight.
export function useResizableDrawer(opts: ResizableDrawerOptions): ResizableDrawerHandle {
  const isResizing = ref(false);
  const minPx = opts.minPx ?? 200;
  const getMax = opts.maxPx ?? (() => (typeof window !== 'undefined' ? window.innerHeight * 0.5 : 600));

  let pointerId = -1;
  let startY = 0;
  let startHeight = 0;

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    const deltaY = e.clientY - startY;
    // Drag-up (deltaY < 0) → taller drawer. Drag-down → shorter.
    const next = Math.min(Math.max(minPx, startHeight - deltaY), getMax());
    opts.setHeight(next);
  }

  function onUp(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    isResizing.value = false;
    pointerId = -1;
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  }

  function start(event: PointerEvent) {
    if (event.button !== 0) return;
    pointerId = event.pointerId;
    startY = event.clientY;
    startHeight = opts.getHeight();
    isResizing.value = true;
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
    event.preventDefault();
  }

  onUnmounted(() => {
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  });

  return { isResizing, start };
}
