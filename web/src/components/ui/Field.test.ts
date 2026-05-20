import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { h } from 'vue';
import Field from './Field.vue';

describe('Field', () => {
  it('renders the label', () => {
    const wrapper = mount(Field, {
      props: { label: 'Nombre' },
      slots: { default: () => h('input') },
    });
    expect(wrapper.text()).toContain('Nombre');
  });

  it('exposes a slot-scope id used by label for', () => {
    const wrapper = mount(Field, {
      props: { label: 'Email' },
      slots: {
        default: ({ id }: { id: string }) => h('input', { id, name: 'email' }),
      },
    });
    const input = wrapper.find('input');
    const label = wrapper.find('label');
    const inputId = input.attributes('id');
    expect(inputId).toBeTruthy();
    expect(label.attributes('for')).toBe(inputId);
  });

  it('shows error text and hides help when error is set', () => {
    const wrapper = mount(Field, {
      props: { label: 'X', help: 'no me muestres', error: 'mal hecho' },
      slots: { default: () => h('input') },
    });
    expect(wrapper.text()).toContain('mal hecho');
    expect(wrapper.text()).not.toContain('no me muestres');
  });

  it('renders required asterisk', () => {
    const wrapper = mount(Field, {
      props: { label: 'X', required: true },
      slots: { default: () => h('input') },
    });
    expect(wrapper.text()).toContain('*');
  });
});
