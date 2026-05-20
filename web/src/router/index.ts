import { createRouter, createWebHistory } from 'vue-router';
import { useAdminStore } from '../stores/admin';

const Simulator = () => import('../views/Simulator.vue');
const StudentLogin = () => import('../views/Login.vue');
const AdminLogin = () => import('../views/admin/Login.vue');
const AdminDashboard = () => import('../views/admin/Dashboard.vue');

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'simulator', component: Simulator },
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
  ],
});

router.beforeEach(async (to) => {
  if (!to.meta.requiresAuth) return true;
  const admin = useAdminStore();
  if (admin.user) return true;
  const ok = await admin.fetchMe();
  if (!ok) {
    return { name: 'admin-login', query: { next: to.fullPath } };
  }
  return true;
});
