import { describe, it, expect, beforeEach } from 'vitest';
import { defineComponent, h, nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { useResizableColumn } from './useResizableColumn';

function makePointer(type: string, x: number, button = 0, pointerId = 1): PointerEvent {
  return new PointerEvent(type, { clientX: x, clientY: 0, button, pointerId, bubbles: true });
}

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width });
}

describe('useResizableColumn', () => {
  beforeEach(() => {
    setViewport(1200);
  });

  it('exposes initial width clamped to the configured maximum', () => {
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'right',
          initialPx: 9000,
          minPx: 200,
          maxPercent: 0.33,
        });
        return () => h('div', String(handle.width.value));
      },
    });
    const wrapper = mount(Host);
    expect(wrapper.text()).toBe(String(Math.floor(1200 * 0.33)));
    wrapper.unmount();
  });

  it('grows the right column as the pointer moves left', async () => {
    let captured = 0;
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'right',
          initialPx: 300,
          minPx: 200,
          maxPercent: 0.5,
          onChange: (px) => { captured = px; },
        });
        return () => h('div', { onPointerdown: handle.start }, String(handle.width.value));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 800));
    window.dispatchEvent(makePointer('pointermove', 700));
    await nextTick();
    expect(wrapper.text()).toBe('400');
    window.dispatchEvent(makePointer('pointerup', 700));
    await nextTick();
    expect(captured).toBe(400);
    wrapper.unmount();
  });

  it('shrinks the left column as the pointer moves left', async () => {
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'left',
          initialPx: 300,
          minPx: 140,
          maxPercent: 0.5,
        });
        return () => h('div', { onPointerdown: handle.start }, String(handle.width.value));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 300));
    window.dispatchEvent(makePointer('pointermove', 240));
    await nextTick();
    expect(wrapper.text()).toBe('240');
    wrapper.unmount();
  });

  it('clamps to the minimum width while dragging', async () => {
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'left',
          initialPx: 200,
          minPx: 140,
          maxPercent: 0.5,
        });
        return () => h('div', { onPointerdown: handle.start }, String(handle.width.value));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 200));
    window.dispatchEvent(makePointer('pointermove', 50));
    await nextTick();
    expect(wrapper.text()).toBe('140');
    wrapper.unmount();
  });

  it('snaps softly to 25% when releasing nearby', async () => {
    setViewport(1000);
    let captured = 0;
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'right',
          initialPx: 200,
          minPx: 140,
          maxPercent: 0.5,
          snapPercents: [0.25, 0.33],
          onChange: (px) => { captured = px; },
        });
        return () => h('div', { onPointerdown: handle.start }, String(handle.width.value));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 800));
    // 1000*0.25 = 250 ; arrastramos a width=255 (delta=-55), dentro de la
    // ventana de 2% (20px) → debería snappear a 250.
    window.dispatchEvent(makePointer('pointermove', 745));
    window.dispatchEvent(makePointer('pointerup', 745));
    await nextTick();
    expect(captured).toBe(250);
    wrapper.unmount();
  });

  it('skips snap when released outside the tolerance window', async () => {
    setViewport(1000);
    let captured = 0;
    const Host = defineComponent({
      setup() {
        const handle = useResizableColumn({
          side: 'right',
          initialPx: 200,
          minPx: 140,
          maxPercent: 0.5,
          snapPercents: [0.25, 0.33],
          onChange: (px) => { captured = px; },
        });
        return () => h('div', { onPointerdown: handle.start }, String(handle.width.value));
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    const el = wrapper.element as HTMLElement;
    el.dispatchEvent(makePointer('pointerdown', 800));
    // delta=-90 → width=290 ; ningún snap dentro de 20px de 250 o 330.
    window.dispatchEvent(makePointer('pointermove', 710));
    window.dispatchEvent(makePointer('pointerup', 710));
    await nextTick();
    expect(captured).toBe(290);
    wrapper.unmount();
  });
});
