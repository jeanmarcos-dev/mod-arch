import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import AdminShell from './AdminShell.vue';
import { useAdminStore } from '../stores/admin';

const SidebarStub = defineComponent({
  name: 'Sidebar',
  props: { items: { type: Array, default: () => [] } },
  setup(props) {
    return () => h('nav', (props.items as Array<{ to: string }>).map((i) => h('a', { 'data-to': i.to })));
  },
});

async function mountShell(opts: { viaEnv: boolean; role: 'admin' | 'moderator' }) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:p(.*)*', component: { render: () => h('div') } }],
  });
  const admin = useAdminStore();
  vi.spyOn(admin, 'refresh').mockImplementation(async () => {
    admin.role = opts.role;
    admin.viaEnv = opts.viaEnv;
  });
  admin.role = opts.role;
  admin.viaEnv = opts.viaEnv;
  const wrapper = mount(AdminShell, {
    global: {
      plugins: [router],
      stubs: { Sidebar: SidebarStub, Topbar: true, Toast: true, RouterView: true },
    },
  });
  await flushPromises();
  return wrapper;
}

describe('AdminShell live entry', () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it('shows "En vivo" only to the env super admin', async () => {
    const wrapper = await mountShell({ viaEnv: true, role: 'admin' });
    expect(wrapper.find('[data-to="/admin/live"]').exists()).toBe(true);
  });

  it('hides it from database admins', async () => {
    const wrapper = await mountShell({ viaEnv: false, role: 'admin' });
    expect(wrapper.find('[data-to="/admin/live"]').exists()).toBe(false);
    expect(wrapper.find('[data-to="/admin/usuarios"]').exists()).toBe(true);
  });

  it('hides it from moderators', async () => {
    const wrapper = await mountShell({ viaEnv: false, role: 'moderator' });
    expect(wrapper.find('[data-to="/admin/live"]').exists()).toBe(false);
  });
});
