import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import TutorPanel from './TutorPanel.vue';
import { useTutorStore } from '../../stores/tutor';
import { useCpuStore } from '../../stores/cpu';
import { i18n } from '../../i18n';

describe('TutorPanel', () => {
  beforeEach(() => {
    window.localStorage.clear();
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  it('shows the privacy notice in the empty state', () => {
    const w = mount(TutorPanel);
    expect(w.text()).toContain('Las preguntas y respuestas se guardan para mejorar el curso.');
  });

  it('translates the empty state per locale', () => {
    (i18n.global.locale as unknown as { value: string }).value = 'en';
    const w = mount(TutorPanel);
    expect(w.text()).toContain('Questions and answers are saved to improve the course.');
    expect(w.text()).toContain('How can I help you?');
  });

  it('announces the answers area politely', () => {
    const tutor = useTutorStore();
    tutor.startAsk('hola', 'r-1');
    const w = mount(TutorPanel);
    const list = w.find('.list');
    expect(list.attributes('aria-live')).toBe('polite');
    expect(list.attributes('role')).toBe('log');
  });

  it('sends tutor_feedback and toggles the stored rating on click', async () => {
    const tutor = useTutorStore();
    tutor.startAsk('hola', 'r-1');
    tutor.applyAnswer({ type: 'tutor_answer', request_id: 'r-1', text: 'respuesta' });
    const cpu = useCpuStore();
    const send = vi.spyOn(cpu, 'sendTutorFeedback').mockImplementation(() => undefined);
    const w = mount(TutorPanel);

    await w.find('[data-rating="up"]').trigger('click');
    expect(send).toHaveBeenLastCalledWith('r-1', 1);
    expect(w.find('[data-rating="up"]').attributes('aria-pressed')).toBe('true');

    await w.find('[data-rating="down"]').trigger('click');
    expect(send).toHaveBeenLastCalledWith('r-1', -1);

    await w.find('[data-rating="down"]').trigger('click');
    expect(send).toHaveBeenLastCalledWith('r-1', 0);
    expect(w.find('[data-rating="down"]').attributes('aria-pressed')).toBe('false');
  });
});
