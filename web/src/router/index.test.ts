import { beforeEach, describe, expect, it, vi } from 'vitest';
import { envAdminRedirect, router } from './index';
import { useAdminStore } from '../stores/admin';

describe('envAdminRedirect', () => {
  it('lets env admins reach env-only routes', () => {
    expect(envAdminRedirect({ meta: { envAdminOnly: true } }, true)).toBe(true);
  });

  it('sends everyone else to the admin dashboard', () => {
    expect(envAdminRedirect({ meta: { envAdminOnly: true } }, false)).toEqual({ name: 'admin-dashboard' });
  });

  it('does not touch regular routes', () => {
    expect(envAdminRedirect({ meta: {} }, false)).toBe(true);
  });
});

describe('admin live route guard', () => {
  beforeEach(async () => {
    await router.replace('/');
  });

  function signIn(viaEnv: boolean) {
    const admin = useAdminStore();
    vi.spyOn(admin, 'fetchMeStatus').mockImplementation(async () => {
      admin.role = 'admin';
      admin.viaEnv = viaEnv;
      return 200;
    });
    return admin;
  }

  it('is flagged as env-admin only', () => {
    expect(router.resolve('/admin/live').meta.envAdminOnly).toBe(true);
    expect(router.resolve('/admin/live').name).toBe('admin-live');
  });

  it('redirects a database admin to the dashboard', async () => {
    signIn(false);
    await router.push('/admin/live');
    expect(router.currentRoute.value.name).toBe('admin-dashboard');
  });

  it('redirects when navigating from inside the admin shell', async () => {
    const admin = signIn(false);
    await router.push('/admin/sesiones');
    admin.viaEnv = false;
    await router.push('/admin/live');
    expect(router.currentRoute.value.name).toBe('admin-dashboard');
  });

  it('lets the env super admin in', async () => {
    signIn(true);
    await router.push('/admin/live');
    expect(router.currentRoute.value.name).toBe('admin-live');
  });
});
