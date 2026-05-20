import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import Btn from './Btn.vue';

describe('Btn', () => {
  it('renders the default slot', () => {
    const wrapper = mount(Btn, { slots: { default: 'Hola' } });
    expect(wrapper.text()).toContain('Hola');
  });

  it('emits click when not disabled', async () => {
    const wrapper = mount(Btn, { slots: { default: 'Click' } });
    await wrapper.trigger('click');
    expect(wrapper.emitted('click')).toBeTruthy();
  });

  it('does not emit click when disabled', async () => {
    const wrapper = mount(Btn, { props: { disabled: true }, slots: { default: 'X' } });
    await wrapper.trigger('click');
    expect(wrapper.emitted('click')).toBeFalsy();
  });

  it('disables itself while loading', () => {
    const wrapper = mount(Btn, { props: { loading: true }, slots: { default: 'X' } });
    expect((wrapper.element as HTMLButtonElement).disabled).toBe(true);
  });

  it('applies the variant class', () => {
    const wrapper = mount(Btn, { props: { variant: 'danger' }, slots: { default: 'Eliminar' } });
    expect(wrapper.classes()).toContain('btn--danger');
  });
});
