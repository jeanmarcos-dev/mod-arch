import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue';

export type ResizableSide = 'left' | 'right';

export interface ResizableColumnOptions {
  side: ResizableSide;
  initialPx: number;
  minPx: number;
  maxPercent: number;
  snapPercents?: number[];
  snapToleranceFraction?: number;
  onChange?: (px: number) => void;
}

export interface ResizableColumnHandle {
  width: Ref<number>;
  isResizing: Ref<boolean>;
  start: (event: PointerEvent) => void;
}

function viewportWidth(): number {
  if (typeof window === 'undefined') return 1024;
  return window.innerWidth;
}

function maxPxFor(maxPercent: number): number {
  return Math.floor(viewportWidth() * maxPercent);
}

function clampWidth(px: number, minPx: number, maxPercent: number): number {
  const max = maxPxFor(maxPercent);
  return Math.max(minPx, Math.min(max, px));
}

// applySnap pulls the width onto one of the snap targets when the cursor is
// released within snapToleranceFraction*viewportWidth of that target. Anything
// outside the tolerance window is left untouched, so the user keeps fine
// control between presets.
function applySnap(
  px: number,
  snapPercents: number[] | undefined,
  tolerance: number,
  minPx: number,
  maxPercent: number,
): number {
  if (!snapPercents || snapPercents.length === 0) return px;
  const vw = viewportWidth();
  const toleranceWidth = vw * tolerance;
  let best = px;
  let bestDist = Infinity;
  for (const p of snapPercents) {
    if (p > maxPercent) continue;
    const target = vw * p;
    const dist = Math.abs(px - target);
    if (dist <= toleranceWidth && dist < bestDist) {
      best = target;
      bestDist = dist;
    }
  }
  return clampWidth(Math.round(best), minPx, maxPercent);
}

export function useResizableColumn(opts: ResizableColumnOptions): ResizableColumnHandle {
  const tolerance = opts.snapToleranceFraction ?? 0.02;
  const width: Ref<number> = ref(clampWidth(opts.initialPx, opts.minPx, opts.maxPercent));
  const isResizing: Ref<boolean> = ref(false);

  let pointerId = -1;
  let startX = 0;
  let startWidth = 0;

  function commit(px: number, persist: boolean) {
    const clamped = clampWidth(Math.round(px), opts.minPx, opts.maxPercent);
    if (clamped !== width.value) width.value = clamped;
    if (persist) opts.onChange?.(clamped);
  }

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    const delta = e.clientX - startX;
    const raw = opts.side === 'left' ? startWidth + delta : startWidth - delta;
    commit(raw, false);
  }

  function onUp(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    isResizing.value = false;
    pointerId = -1;
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
    const snapped = applySnap(width.value, opts.snapPercents, tolerance, opts.minPx, opts.maxPercent);
    commit(snapped, true);
  }

  function start(event: PointerEvent) {
    if (event.button !== 0) return;
    pointerId = event.pointerId;
    startX = event.clientX;
    startWidth = width.value;
    isResizing.value = true;
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
    event.preventDefault();
  }

  // External writes (e.g. preferences hydrated from another tab) flow in
  // through this watcher so the column reflects the new value, still clamped
  // against the live viewport.
  watch(
    () => opts.initialPx,
    (next) => {
      if (next === undefined || next === null) return;
      commit(next, false);
    },
  );

  function onWindowResize() {
    const reclamped = clampWidth(width.value, opts.minPx, opts.maxPercent);
    if (reclamped !== width.value) {
      width.value = reclamped;
      opts.onChange?.(reclamped);
    }
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('resize', onWindowResize);
    }
  });

  onUnmounted(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', onWindowResize);
    }
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  });

  return { width, isResizing, start };
}
