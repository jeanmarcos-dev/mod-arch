import { describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import ViewerShell from './ViewerShell.vue';
import ViewerTopBar from '../components/shell/ViewerTopBar.vue';
import { useCpuStore } from '../stores/cpu';

async function mountViewerShell() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:p(.*)*', component: { render: () => h('div') } }],
  });
  router.push('/procesos');
  await router.isReady();
  const cpu = useCpuStore();
  vi.spyOn(cpu, 'connect').mockResolvedValue(undefined);
  const wrapper = mount(ViewerShell, {
    global: {
      plugins: [router],
      stubs: {
        Sidebar: true,
        SimStatusRow: true,
        ConnectionBanner: true,
        MobileMenuButton: true,
        GlossaryModal: true,
        WhiteboardCursor: true,
        WhiteboardClickRipple: true,
        WhiteboardFocusPill: true,
        EventAnnouncer: true,
        AccessibilityMenu: true,
        RouterView: true,
        ViewerTopBar: defineComponent({ render: () => h('header', { 'data-stub': 'topbar' }) }),
        TutorLauncher: defineComponent({ render: () => h('div', { 'data-stub': 'tutor-launcher' }) }),
        ReportModal: defineComponent({ render: () => h('div', { 'data-stub': 'report-modal' }) }),
      },
    },
  });
  await flushPromises();
  return wrapper;
}

describe('ViewerShell', () => {
  it('mounts ARCHIUS and the report modal for spectators', async () => {
    const wrapper = await mountViewerShell();
    expect(wrapper.find('[data-stub="tutor-launcher"]').exists()).toBe(true);
    expect(wrapper.find('[data-stub="report-modal"]').exists()).toBe(true);
  });
});

describe('ViewerTopBar', () => {
  it('offers the report button to spectators', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:p(.*)*', component: { render: () => h('div') } }],
    });
    const wrapper = mount(ViewerTopBar, { global: { plugins: [router] } });
    expect(wrapper.find('.report-btn').exists()).toBe(true);
  });
});
