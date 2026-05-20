import { onUnmounted, ref, type Ref } from 'vue';

export interface Position { x: number; y: number; }

export interface DraggableOptions {
  initial?: Position;
  onEnd?: (pos: Position) => void;
  clampToViewport?: boolean;
  elementRef?: Ref<HTMLElement | null>;
}

export interface DraggableHandle {
  position: Ref<Position>;
  isDragging: Ref<boolean>;
  start: (event: PointerEvent) => void;
  setPosition: (pos: Position) => void;
}

export function useDraggable(opts: DraggableOptions = {}): DraggableHandle {
  const position = ref<Position>({ x: opts.initial?.x ?? 0, y: opts.initial?.y ?? 0 });
  const isDragging = ref(false);

  let pointerId = -1;
  let offsetX = 0;
  let offsetY = 0;

  function clamp(p: Position): Position {
    if (!opts.clampToViewport || typeof window === 'undefined') return p;
    const el = opts.elementRef?.value;
    const w = el?.offsetWidth ?? 0;
    const h = el?.offsetHeight ?? 0;
    const maxX = Math.max(0, window.innerWidth - w);
    const maxY = Math.max(0, window.innerHeight - h);
    return {
      x: Math.min(Math.max(0, p.x), maxX),
      y: Math.min(Math.max(0, p.y), maxY),
    };
  }

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    position.value = clamp({ x: e.clientX - offsetX, y: e.clientY - offsetY });
  }

  function onUp(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    isDragging.value = false;
    pointerId = -1;
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
    opts.onEnd?.(position.value);
  }

  function start(event: PointerEvent) {
    if (event.button !== 0) return;
    pointerId = event.pointerId;
    isDragging.value = true;
    offsetX = event.clientX - position.value.x;
    offsetY = event.clientY - position.value.y;
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
    event.preventDefault();
  }

  function setPosition(pos: Position) {
    position.value = clamp(pos);
  }

  onUnmounted(() => {
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  });

  return { position, isDragging, start, setPosition };
}
