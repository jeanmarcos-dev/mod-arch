import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import ReportButton from './ReportButton.vue';
import { useReportStore } from '../stores/report';
import { i18n } from '../i18n';

describe('ReportButton', () => {
  beforeEach(() => {
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  it.each([
    ['es', 'Comentarios'],
    ['en', 'Feedback'],
    ['pt', 'Comentários'],
  ])('shows a visible text label in %s', (locale, label) => {
    (i18n.global.locale as unknown as { value: string }).value = locale;
    const w = mount(ReportButton);
    expect(w.get('[data-testid="report-btn-label"]').text()).toBe(label);
    expect(w.text()).toContain(label);
  });

  it('renders icon-only with an accessible name when compact', () => {
    const w = mount(ReportButton, { props: { compact: true } });
    expect(w.classes()).toContain('report-btn--compact');
    expect(w.attributes('aria-label')).toBe('Comentarios');
    expect(w.get('[data-testid="report-btn-label"]').classes()).toContain('report-btn__label--sr');
  });

  it('applies the bar variant used by the top bars', () => {
    const w = mount(ReportButton, { props: { variant: 'bar' } });
    expect(w.classes()).toContain('report-btn--bar');
    expect(w.get('[data-testid="report-btn-label"]').classes()).not.toContain('report-btn__label--sr');
  });

  it('keeps the visible label when not compact', () => {
    const w = mount(ReportButton);
    expect(w.classes()).not.toContain('report-btn--compact');
    expect(w.get('[data-testid="report-btn-label"]').classes()).not.toContain('report-btn__label--sr');
  });

  it('opens the report modal on click', async () => {
    const w = mount(ReportButton);
    const reports = useReportStore();
    expect(reports.open).toBe(false);
    await w.trigger('click');
    expect(reports.open).toBe(true);
  });
});
