import { describe, it, expect, vi, afterEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import Term from './Term.vue';
import { useCpuStore } from '../../stores/cpu';

afterEach(() => {
  vi.useRealTimers();
});

describe('Term', () => {
  it('renders the slot when present and falls back to text/term otherwise', () => {
    const a = mount(Term, { props: { term: 'EIP' }, slots: { default: 'IP' } });
    expect(a.text()).toBe('IP');

    const b = mount(Term, { props: { term: 'EIP', text: 'Pointer' } });
    expect(b.text()).toBe('Pointer');

    const c = mount(Term, { props: { term: 'EIP' } });
    expect(c.text()).toBe('EIP');
  });

  it('is interactive when the term is in the glossary', () => {
    const w = mount(Term, { props: { term: 'EIP' } });
    const el = w.element as HTMLElement;
    expect(el.getAttribute('tabindex')).toBe('0');
    expect(el.getAttribute('role')).toBe('button');
    expect(w.classes()).toContain('term--interactive');
  });

  it('renders inert when the term is unknown', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const w = mount(Term, { props: { term: 'no-such-term' } });
    const el = w.element as HTMLElement;
    expect(el.getAttribute('tabindex')).toBeNull();
    expect(el.getAttribute('role')).toBeNull();
    expect(w.classes()).toContain('term--inert');
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it('triggers cpu.openGlossary on click with the term key', async () => {
    const w = mount(Term, { props: { term: 'EIP' } });
    const cpu = useCpuStore();
    const before = cpu.glossaryOpenSignal;
    await w.trigger('click');
    expect(cpu.glossarySearch).toBe('EIP');
    expect(cpu.glossaryOpenSignal).toBe(before + 1);
  });

  it('does not trigger when disabled or unknown', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const unknown = mount(Term, { props: { term: 'no-such' } });
    const cpu = useCpuStore();
    const before = cpu.glossaryOpenSignal;
    await unknown.trigger('click');
    expect(cpu.glossaryOpenSignal).toBe(before);

    const disabled = mount(Term, { props: { term: 'EIP', disabled: true } });
    await disabled.trigger('click');
    expect(cpu.glossaryOpenSignal).toBe(before);
  });

  it('opens the tooltip after the hover delay', async () => {
    vi.useFakeTimers();
    const w = mount(Term, { props: { term: 'EIP' }, attachTo: document.body });
    await w.trigger('mouseenter');
    expect(document.body.querySelector('.g-tip')).toBeNull();

    vi.advanceTimersByTime(200);
    await flushPromises();
    expect(document.body.querySelector('.g-tip')).not.toBeNull();

    await w.trigger('mouseleave');
    await flushPromises();
    expect(document.body.querySelector('.g-tip')).toBeNull();
    w.unmount();
  });

  it('opens the tooltip immediately on focus (keyboard path)', async () => {
    const w = mount(Term, { props: { term: 'EIP' }, attachTo: document.body });
    await w.trigger('focus');
    await flushPromises();
    expect(document.body.querySelector('.g-tip')).not.toBeNull();
    w.unmount();
  });

  it('applies the mode class', () => {
    const w = mount(Term, { props: { term: 'EIP', mode: 'title' } });
    expect(w.classes()).toContain('term--title');
  });

  it('handles Enter and Space as click', async () => {
    const w = mount(Term, { props: { term: 'EIP' } });
    const cpu = useCpuStore();
    const before = cpu.glossaryOpenSignal;
    await w.trigger('keydown', { key: 'Enter' });
    expect(cpu.glossaryOpenSignal).toBe(before + 1);
    await w.trigger('keydown', { key: ' ' });
    expect(cpu.glossaryOpenSignal).toBe(before + 2);
  });

  it('renders as the requested element', () => {
    const w = mount(Term, { props: { term: 'EIP', as: 'code' } });
    expect(w.element.tagName.toLowerCase()).toBe('code');
  });
});
