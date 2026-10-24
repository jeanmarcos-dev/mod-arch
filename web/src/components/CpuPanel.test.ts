import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import CpuPanel from './CpuPanel.vue';
import { useCpuStore } from '../stores/cpu';
import { i18n } from '../i18n';

function mockHover(none: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === '(hover: none)' ? none : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
    onchange: null,
  })) as unknown as typeof window.matchMedia;
}

describe('CpuPanel explanations', () => {
  beforeEach(() => {
    window.localStorage.clear();
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  function mountPanel() {
    const cpu = useCpuStore();
    const open = vi.spyOn(cpu, 'openGlossary').mockImplementation(() => undefined);
    const wrapper = mount(CpuPanel);
    return { wrapper, open };
  }

  it('shows the explanation inline on touch devices instead of opening the glossary', async () => {
    mockHover(true);
    const { wrapper, open } = mountPanel();
    await wrapper.get('[data-explain="reg-EAX"]').trigger('click');
    const tip = wrapper.get('[data-testid="cpu-tip"]');
    expect(tip.text()).toContain(i18n.global.t('cpu.regHelp.EAX'));
    expect(tip.text()).not.toContain('click para abrir el glosario');
    expect(open).not.toHaveBeenCalled();
  });

  it('opens the glossary from the tip on demand', async () => {
    mockHover(true);
    const { wrapper, open } = mountPanel();
    await wrapper.get('[data-explain="reg-EAX"]').trigger('click');
    await wrapper.get('[data-testid="cpu-tip-glossary"]').trigger('click');
    expect(open).toHaveBeenCalledWith('EAX');
  });

  it('toggles the tip when tapping the same element again and closes it with the button', async () => {
    mockHover(true);
    const { wrapper } = mountPanel();
    await wrapper.get('[data-explain="pl"]').trigger('click');
    expect(wrapper.find('[data-testid="cpu-tip"]').exists()).toBe(true);
    await wrapper.get('[data-explain="pl"]').trigger('click');
    expect(wrapper.find('[data-testid="cpu-tip"]').exists()).toBe(false);
    await wrapper.get('[data-explain="flag-ZF"]').trigger('click');
    expect(wrapper.get('[data-testid="cpu-tip"]').text()).toContain('ZF');
    await wrapper.get('[data-testid="cpu-tip-close"]').trigger('click');
    expect(wrapper.find('[data-testid="cpu-tip"]').exists()).toBe(false);
  });

  it('replaces the tip when another element is tapped', async () => {
    mockHover(true);
    const { wrapper } = mountPanel();
    await wrapper.get('[data-explain="reg-EAX"]').trigger('click');
    await wrapper.get('[data-explain="pl"]').trigger('click');
    expect(wrapper.findAll('[data-testid="cpu-tip"]')).toHaveLength(1);
    expect(wrapper.get('[data-testid="cpu-tip"]').text()).toContain('Privilege Level');
  });

  it('keeps opening the glossary directly with a pointer device', async () => {
    mockHover(false);
    const { wrapper, open } = mountPanel();
    await wrapper.get('[data-explain="reg-EBX"]').trigger('click');
    expect(open).toHaveBeenCalledWith('EBX');
    expect(wrapper.find('[data-testid="cpu-tip"]').exists()).toBe(false);
  });
});
