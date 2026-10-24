import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import Login from './Login.vue';
import { useAppConfigStore } from '../stores/appConfig';
import { i18n } from '../i18n';

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ query: {} }),
  RouterLink: { template: '<a><slot /></a>' },
}));

function mountLogin() {
  const cfg = useAppConfigStore();
  vi.spyOn(cfg, 'load').mockResolvedValue(undefined as never);
  return mount(Login, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } });
}

describe('Login privacy notice', () => {
  beforeEach(() => {
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  it('shows the notice inside the form before the submit button', () => {
    const w = mountLogin();
    const form = w.find('form');
    const notice = form.find('[data-testid="login-privacy"]');
    expect(notice.exists()).toBe(true);
    expect(notice.text()).toContain('ARCHIUS');
    const html = form.html();
    expect(html.indexOf('login-privacy')).toBeLessThan(html.indexOf('type="submit"'));
  });

  it.each([
    ['es', 'con fines académicos'],
    ['en', 'academic purposes'],
    ['pt', 'fins acadêmicos'],
  ])('is translated in %s', (locale, fragment) => {
    (i18n.global.locale as unknown as { value: string }).value = locale;
    const w = mountLogin();
    expect(w.find('[data-testid="login-privacy"]').text()).toContain(fragment);
  });
});
