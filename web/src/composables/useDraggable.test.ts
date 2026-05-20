import { describe, it, expect } from 'vitest';
import { defineComponent, h, ref, nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { useDraggable } from './useDraggable';

function makePointer(type: string, x: number, y: number, button = 0, pointerId = 1): PointerEvent {
  return new PointerEvent(type, { clientX: x, clientY: y, button, pointerId, bubbles: true });
}

describe('useDraggable', () => {
  it('exposes initial position', () => {
    const Host = defineComponent({
      setup() {
        const handle = useDraggable({ initial: { x: 10, y: 20 } });
        return () => h('div', String(handle.position.value.x) + ',' + String(handle.position.value.y));
      },
    });
    const wrapper = mount(Host);
    expect(wrapper.text()).toBe('10,20');
  });

  it('updates position while dragging', async () => {
    const Host = defineComponent({
      setup() {
        const elementRef = ref<HTMLElement | null>(null);
        const handle = useDraggable({ initial: { x: 0, y: 0 }, elementRef });
        return () => h('div', { ref: elementRef, onPointerdown: handle.start },
          handle.position.value.x + '|' + handle.position.value.y + '|' + (handle.isDragging.value ? '1' : '0'));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 5, 5));
    window.dispatchEvent(makePointer('pointermove', 30, 40));
    await nextTick();
    expect(wrapper.text()).toBe('25|35|1');
    window.dispatchEvent(makePointer('pointerup', 30, 40));
    await nextTick();
    expect(wrapper.text()).toBe('25|35|0');
    wrapper.unmount();
  });

  it('ignores non-primary button', async () => {
    const Host = defineComponent({
      setup() {
        const handle = useDraggable({ initial: { x: 0, y: 0 } });
        return () => h('div', { onPointerdown: handle.start },
          handle.position.value.x + ',' + (handle.isDragging.value ? '1' : '0'));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 10, 10, 2));
    window.dispatchEvent(makePointer('pointermove', 50, 50));
    await nextTick();
    expect(wrapper.text()).toBe('0,0');
    wrapper.unmount();
  });

  it('calls onEnd with final position', async () => {
    let captured: { x: number; y: number } | null = null;
    const Host = defineComponent({
      setup() {
        const handle = useDraggable({ initial: { x: 0, y: 0 }, onEnd: (p) => { captured = p; } });
        return () => h('div', { onPointerdown: handle.start }, '');
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 0, 0));
    window.dispatchEvent(makePointer('pointermove', 100, 200));
    window.dispatchEvent(makePointer('pointerup', 100, 200));
    await nextTick();
    expect(captured).toEqual({ x: 100, y: 200 });
    wrapper.unmount();
  });
});
