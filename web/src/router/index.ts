import { createRouter, createWebHistory } from 'vue-router';
import { useAdminStore } from '../stores/admin';

const Simulator = () => import('../views/Simulator.vue');
const StudentLogin = () => import('../views/Login.vue');
const NotFound = () => import('../views/NotFound.vue');
const AdminLogin = () => import('../views/admin/Login.vue');
const AdminDashboard = () => import('../views/admin/Dashboard.vue');

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'simulator', component: Simulator },
    // The viewer URL keeps /v/<token> in the address bar; the backend
    // rewrites the request to / when serving index.html so the SPA mounts,
    // but vue-router still needs a matching route or it would fall through
    // to the not-found catch-all below.
    { path: '/v/:token', name: 'viewer', component: Simulator },
    { path: '/login', name: 'login', component: StudentLogin },
    {
      path: '/admin/login',
      name: 'admin-login',
      component: AdminLogin,
      meta: { requiresAuth: false },
    },
    {
      path: '/admin',
      name: 'admin-dashboard',
      component: AdminDashboard,
      meta: { requiresAuth: true },
    },
    // Catch-all for /admin/* — sends unknown admin paths to the dashboard
    // (the guard below redirects to /admin/login when needed).
    {
      path: '/admin/:catchAll(.*)',
      redirect: { name: 'admin-dashboard' },
    },
    // Generic catch-all: any other unknown path renders the 404 view.
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFound },
  ],
});

router.beforeEach(async (to) => {
  // not-found is always allowed — even when its URL happens to start with
  // /admin, otherwise the disabled-admin redirect would loop.
  if (to.name === 'not-found') return true;
  const isAdminPath = to.path === '/admin' || to.path.startsWith('/admin/');
  if (!isAdminPath) return true;

  const admin = useAdminStore();
  const status = await admin.fetchMeStatus();

  // The whole /admin namespace pretends not to exist when the server has no
  // admin credentials configured. fetch returning 404 (or a network failure)
  // both fall under "hide the surface". We pass the original path as
  // pathMatch so the URL bar keeps showing what the user typed.
  if (status === 404 || status === 0) {
    return {
      name: 'not-found',
      params: { pathMatch: to.path.replace(/^\//, '').split('/') },
    };
  }

  if (to.name === 'admin-login') {
    return status === 200 ? { name: 'admin-dashboard' } : true;
  }
  if (status !== 200) {
    return { name: 'admin-login', query: { next: to.fullPath } };
  }
  return true;
});
