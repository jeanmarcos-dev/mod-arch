import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import TutorMessage from './TutorMessage.vue';
import type { TutorMessage as Msg } from '../../stores/tutor';
import { i18n } from '../../i18n';

function answer(extra: Partial<Msg> = {}): Msg {
  return {
    id: 'a-r-1',
    role: 'tutor',
    text: 'Una respuesta',
    requestId: 'r-1',
    createdAt: 1,
    ...extra,
  };
}

describe('TutorMessage', () => {
  beforeEach(() => {
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  it('shows thumbs on answers that have a request id', () => {
    const w = mount(TutorMessage, { props: { msg: answer() } });
    expect(w.find('[data-rating="up"]').exists()).toBe(true);
    expect(w.find('[data-rating="down"]').exists()).toBe(true);
  });

  it('hides thumbs on student messages, pending answers, errors and answers without request id', () => {
    const cases: Msg[] = [
      { id: 'q-1', role: 'student', text: 'hola', requestId: 'r-1', createdAt: 1 },
      answer({ pending: true }),
      answer({ error: 'falló' }),
      answer({ requestId: undefined }),
    ];
    for (const msg of cases) {
      const w = mount(TutorMessage, { props: { msg } });
      expect(w.find('[data-rating]').exists()).toBe(false);
    }
  });

  it('emits the clicked rating', async () => {
    const w = mount(TutorMessage, { props: { msg: answer() } });
    await w.find('[data-rating="up"]').trigger('click');
    await w.find('[data-rating="down"]').trigger('click');
    expect(w.emitted('rate')).toEqual([[1], [-1]]);
  });

  it('marks the stored rating as pressed', () => {
    const w = mount(TutorMessage, { props: { msg: answer({ rating: -1 }) } });
    expect(w.find('[data-rating="down"]').attributes('aria-pressed')).toBe('true');
    expect(w.find('[data-rating="up"]').attributes('aria-pressed')).toBe('false');
  });

  it('translates the tutor_unavailable code per locale', () => {
    const msg = answer({ error: 'tutor desactivado en este servidor', errorCode: 'tutor_unavailable' });
    const w = mount(TutorMessage, { props: { msg } });
    expect(w.text()).toContain('ARCHIUS no está disponible');
    (i18n.global.locale as unknown as { value: string }).value = 'en';
    const en = mount(TutorMessage, { props: { msg } });
    expect(en.text()).toContain('ARCHIUS is not available');
    (i18n.global.locale as unknown as { value: string }).value = 'pt';
    const pt = mount(TutorMessage, { props: { msg } });
    expect(pt.text()).toContain('ARCHIUS não está disponível');
  });

  it('falls back to the raw server text for unknown codes', () => {
    const w = mount(TutorMessage, { props: { msg: answer({ error: 'texto crudo', errorCode: 'desconocido' }) } });
    expect(w.text()).toContain('texto crudo');
  });
});
