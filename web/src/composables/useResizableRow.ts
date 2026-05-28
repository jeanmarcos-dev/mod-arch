import { onUnmounted, ref, type Ref, type ComponentPublicInstance } from 'vue';

type ElTarget = HTMLElement | ComponentPublicInstance | null;

function resolveEl(target: ElTarget): HTMLElement | null {
  if (!target) return null;
  if (target instanceof HTMLElement) return target;
  const el = (target as ComponentPublicInstance).$el;
  return el instanceof HTMLElement ? el : null;
}

export interface ResizableRowOptions {
  topEl: Ref<ElTarget>;
  bottomEl: Ref<ElTarget>;
  getTopWeight: () => number;
  getBottomWeight: () => number;
  setTopWeight: (w: number) => void;
  setBottomWeight: (w: number) => void;
  minPx?: number;
  minWeight?: number;
}

export interface ResizableRowHandle {
  isResizing: Ref<boolean>;
  start: (event: PointerEvent) => void;
}

// useResizableRow drives a horizontal handle between two flex-grow siblings.
// The two siblings can be at any positions in their parent flex column; this
// composable just needs the live DOM nodes (for offsetHeight at drag start)
// and getters/setters for the weights that control their `flex-grow`.
//
// During drag we rewrite both weights on every pointermove. Vue's reactivity
// flows the new weights back to the inline style, so the panels resize
// continuously without us touching the DOM directly. The persistence watcher
// in the preferences store batches writes per frame.
export function useResizableRow(opts: ResizableRowOptions): ResizableRowHandle {
  const isResizing = ref<boolean>(false);
  const minPx = opts.minPx ?? 80;
  const minWeight = opts.minWeight ?? 0.15;

  let pointerId = -1;
  let startY = 0;
  let startTopHeight = 0;
  let sumH = 0;
  let sumW = 0;

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    const deltaY = e.clientY - startY;
    const newTop = Math.max(minPx, Math.min(sumH - minPx, startTopHeight + deltaY));
    const ratio = sumH > 0 ? newTop / sumH : 0.5;
    const topW = Math.max(minWeight, ratio * sumW);
    const bottomW = Math.max(minWeight, sumW - topW);
    opts.setTopWeight(topW);
    opts.setBottomWeight(bottomW);
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
    const topEl = resolveEl(opts.topEl.value);
    const bottomEl = resolveEl(opts.bottomEl.value);
    if (!topEl || !bottomEl) return;
    pointerId = event.pointerId;
    startY = event.clientY;
    startTopHeight = topEl.offsetHeight;
    sumH = startTopHeight + bottomEl.offsetHeight;
    sumW = opts.getTopWeight() + opts.getBottomWeight();
    if (sumW <= 0) sumW = 2;
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
