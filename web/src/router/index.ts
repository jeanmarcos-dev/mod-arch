import { createRouter, createWebHistory } from 'vue-router';
import { useAdminStore } from '../stores/admin';

const AppShell = () => import('../layouts/AppShell.vue');
const Overview = () => import('../views/section/Overview.vue');
const Processes = () => import('../views/section/Processes.vue');
const Memory = () => import('../views/section/Memory.vue');
const Interrupts = () => import('../views/section/Interrupts.vue');
const Concurrency = () => import('../views/section/Concurrency.vue');
const X86Real = () => import('../views/section/X86Real.vue');
const MySessions = () => import('../views/secondary/MySessions.vue');
const GlossaryPage = () => import('../views/secondary/Glossary.vue');
const Preferences = () => import('../views/secondary/Preferences.vue');
const Profile = () => import('../views/secondary/Profile.vue');
const Simulator = () => import('../views/Simulator.vue');
const StudentLogin = () => import('../views/Login.vue');
const NotFound = () => import('../views/NotFound.vue');
const AdminLogin = () => import('../views/admin/Login.vue');
const AdminDashboard = () => import('../views/admin/Dashboard.vue');

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', name: 'overview', component: Overview, meta: { title: 'Vista general', section: false } },
        { path: 'procesos', name: 'processes', component: Processes, meta: { title: 'Procesos', section: true } },
        { path: 'memoria', name: 'memory', component: Memory, meta: { title: 'Memoria', section: true } },
        { path: 'interrupciones', name: 'interrupts', component: Interrupts, meta: { title: 'Interrupciones', section: true } },
        { path: 'concurrencia', name: 'concurrency', component: Concurrency, meta: { title: 'Concurrencia', section: true } },
        { path: 'x86', name: 'x86-real', component: X86Real, meta: { title: 'x86 real', section: true } },
        { path: 'sesiones', name: 'my-sessions', component: MySessions, meta: { title: 'Mis sesiones', section: false } },
        { path: 'glosario', name: 'glossary', component: GlossaryPage, meta: { title: 'Glosario', section: false } },
        { path: 'preferencias', name: 'preferences', component: Preferences, meta: { title: 'Preferencias', section: false } },
        { path: 'perfil', name: 'profile', component: Profile, meta: { title: 'Perfil', section: false } },
      ],
    },
    // Viewer mode keeps the bare Simulator without the shell so projected
    // viewers see only the simulation, not the navigation.
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
    {
      path: '/admin/:catchAll(.*)',
      redirect: { name: 'admin-dashboard' },
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFound },
  ],
});

router.beforeEach(async (to) => {
  if (to.name === 'not-found') return true;
  const isAdminPath = to.path === '/admin' || to.path.startsWith('/admin/');
  if (!isAdminPath) return true;

  const admin = useAdminStore();
  const status = await admin.fetchMeStatus();

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
