import { beforeEach, describe, expect, it } from 'vitest';
import { h } from 'vue';
import { mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import Sidebar from './Sidebar.vue';
import { i18n } from '../../i18n';

function setLocale(locale: string) {
  (i18n.global.locale as unknown as { value: string }).value = locale;
}

function mountSidebar() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:p(.*)*', component: { render: () => h('div') } }],
  });
  return mount(Sidebar, { props: { items: [] }, global: { plugins: [router] } });
}

describe('Sidebar footer links', () => {
  beforeEach(() => setLocale('es'));

  it.each([
    ['es', 'https://www.jeanmarcos.dev', 'https://www.jeanmarcos.dev/work/mod-arch'],
    ['en', 'https://www.jeanmarcos.dev/en', 'https://www.jeanmarcos.dev/en/work/mod-arch'],
    ['pt', 'https://www.jeanmarcos.dev/en', 'https://www.jeanmarcos.dev/en/work/mod-arch'],
  ])('points the author and project links to the %s site', (locale, site, project) => {
    setLocale(locale);
    const w = mountSidebar();
    expect(w.get('[data-testid="sidebar-author"]').attributes('href')).toBe(site);
    expect(w.get('[data-testid="sidebar-project"]').attributes('href')).toBe(project);
    expect(w.get('[data-testid="sidebar-repo"]').attributes('href')).toBe('https://github.com/jeanmarcos-dev/mod-arch');
  });

  it('opens every footer link in a new tab without exposing the opener', () => {
    const w = mountSidebar();
    for (const id of ['sidebar-author', 'sidebar-repo', 'sidebar-project']) {
      const a = w.get(`[data-testid="${id}"]`);
      expect(a.attributes('target')).toBe('_blank');
      expect(a.attributes('rel')).toContain('noopener');
    }
  });

  it('gives the icon-only links a localized accessible name', () => {
    const w = mountSidebar();
    expect(w.get('[data-testid="sidebar-repo"]').attributes('aria-label')).toBe('Código fuente en GitHub');
    expect(w.get('[data-testid="sidebar-project"]').attributes('aria-label')).toBe('Sobre el proyecto');
    expect(w.get('[data-testid="sidebar-author"]').text()).toBe('Jeanmarcos Juárez');
  });
});
