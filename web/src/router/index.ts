import { createRouter, createWebHistory } from 'vue-router';
import { useAdminStore } from '../stores/admin';
import { useAppConfigStore } from '../stores/appConfig';
import { useAuthStore } from '../stores/auth';

// Route meta carries a translation key instead of a literal string so the
// document.title watcher can resolve it through vue-i18n on locale changes.
declare module 'vue-router' {
  interface RouteMeta {
    titleKey?: string;
    section?: boolean;
    category?: string;
    requiresAuth?: boolean;
  }
}

const AppShell = () => import('../layouts/AppShell.vue');
const Overview = () => import('../views/section/Overview.vue');
const Processes = () => import('../views/section/Processes.vue');
const Memory = () => import('../views/section/Memory.vue');
const Interrupts = () => import('../views/section/Interrupts.vue');
const Concurrency = () => import('../views/section/Concurrency.vue');
const X86Real = () => import('../views/section/X86Real.vue');
const GlossaryPage = () => import('../views/secondary/Glossary.vue');
const Preferences = () => import('../views/secondary/Preferences.vue');
const Profile = () => import('../views/secondary/Profile.vue');
const ViewerEntry = () => import('../views/ViewerEntry.vue');
const StudentLogin = () => import('../views/Login.vue');
const NotFound = () => import('../views/NotFound.vue');
const AdminLogin = () => import('../views/admin/Login.vue');
const AdminShell = () => import('../layouts/AdminShell.vue');
const AdminOverview = () => import('../views/admin/AdminOverview.vue');
const AdminSessions = () => import('../views/admin/AdminSessions.vue');
const AdminInvites = () => import('../views/admin/AdminInvites.vue');
const AdminAccess = () => import('../views/admin/AdminAccess.vue');
const AdminUsers = () => import('../views/admin/AdminUsers.vue');
const AdminAudit = () => import('../views/admin/AdminAudit.vue');
const AdminSmtp = () => import('../views/admin/AdminSmtp.vue');
const AdminTutorMetrics = () => import('../views/admin/AdminTutorMetrics.vue');
const AdminTutorConfig = () => import('../views/admin/AdminTutorConfig.vue');
const AdminSecurity = () => import('../views/admin/AdminSecurity.vue');
const AdminSimulator = () => import('../views/admin/AdminSimulator.vue');

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', name: 'overview', component: Overview, meta: { titleKey: 'router.titles.overview', section: false } },
        { path: 'procesos', name: 'processes', component: Processes, meta: { titleKey: 'router.titles.processes', section: true } },
        { path: 'memoria', name: 'memory', component: Memory, meta: { titleKey: 'router.titles.memory', section: true } },
        { path: 'interrupciones', name: 'interrupts', component: Interrupts, meta: { titleKey: 'router.titles.interrupts', section: true } },
        { path: 'concurrencia', name: 'concurrency', component: Concurrency, meta: { titleKey: 'router.titles.concurrency', section: true, category: 'concurrencia' } },
        { path: 'x86', name: 'x86-real', component: X86Real, meta: { titleKey: 'router.titles.x86', section: true } },
        // Legacy URL: /sesiones used to host a list of sessions, but with
        // the "one session per user" policy the list collapses to a single
        // row. Profile now carries the session info inline, so this path
        // redirects to /perfil instead of 404'ing for existing bookmarks.
        { path: 'sesiones', redirect: '/perfil' },
        { path: 'glosario', name: 'glossary', component: GlossaryPage, meta: { titleKey: 'router.titles.glossary', section: false } },
        { path: 'preferencias', name: 'preferences', component: Preferences, meta: { titleKey: 'router.titles.preferences', section: false } },
        { path: 'perfil', name: 'profile', component: Profile, meta: { titleKey: 'router.titles.profile', section: false } },
      ],
    },
    // The backend sets the mc_view cookie when serving /v/<token>, so by the
    // time this route mounts the SPA is already identified as a viewer.
    // ViewerEntry just waits for the initial session_info and redirects to
    // the section the host is presenting; the actual shell is mounted at /
    // and AppShell delegates to ViewerShell based on role.
    { path: '/v/:token', name: 'viewer', component: ViewerEntry },
    { path: '/login', name: 'login', component: StudentLogin },
    {
      path: '/admin/login',
      name: 'admin-login',
      component: AdminLogin,
      meta: { requiresAuth: false },
    },
    {
      path: '/admin',
      component: AdminShell,
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'admin-dashboard', component: AdminOverview, meta: { titleKey: 'router.titles.adminDashboard' } },
        { path: 'sesiones', name: 'admin-sessions', component: AdminSessions, meta: { titleKey: 'router.titles.adminSessions' } },
        { path: 'usuarios', name: 'admin-users', component: AdminUsers, meta: { titleKey: 'router.titles.adminUsers' } },
        { path: 'invitaciones', name: 'admin-invites', component: AdminInvites, meta: { titleKey: 'router.titles.adminInvites' } },
        { path: 'accesos', name: 'admin-access', component: AdminAccess, meta: { titleKey: 'router.titles.adminAccess' } },
        { path: 'smtp', name: 'admin-smtp', component: AdminSmtp, meta: { titleKey: 'router.titles.adminSmtp' } },
        { path: 'auditoria', name: 'admin-audit', component: AdminAudit, meta: { titleKey: 'router.titles.adminAudit' } },
        { path: 'tutor', name: 'admin-tutor-metrics', component: AdminTutorMetrics, meta: { titleKey: 'router.titles.adminTutorMetricsLong' } },
        { path: 'tutor/config', name: 'admin-tutor-config', component: AdminTutorConfig, meta: { titleKey: 'router.titles.adminTutorConfigLong' } },
        { path: 'seguridad', name: 'admin-security', component: AdminSecurity, meta: { titleKey: 'router.titles.adminSecurity' } },
        { path: 'simulador', name: 'admin-simulator', component: AdminSimulator, meta: { titleKey: 'router.titles.adminSimulator' } },
      ],
    },
    {
      path: '/admin/:catchAll(.*)',
      redirect: { name: 'admin-dashboard' },
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFound },
  ],
});

// Routes the disabled-routes/default-route policy can touch. Excludes
// authentication, admin and viewer-token URLs so a misconfigured default or
// disabled list cannot lock a user out of the admin panel or the magic-link
// landing page.
const STUDENT_GATED_ROUTES = new Set([
  'overview',
  'processes',
  'memory',
  'interrupts',
  'concurrency',
  'x86-real',
  'glossary',
  'preferences',
  'profile',
]);

// Fallback path used when the configured default route is itself disabled or
// unknown. Keeps the student inside the SPA instead of bouncing to 404.
const SAFE_FALLBACK_PATH = '/';

function defaultPath(defaultRoute: string, disabled: Set<string>): string {
  if (!STUDENT_GATED_ROUTES.has(defaultRoute) || disabled.has(defaultRoute)) {
    return SAFE_FALLBACK_PATH;
  }
  const resolved = router.resolve({ name: defaultRoute });
  return resolved?.path || SAFE_FALLBACK_PATH;
}

router.beforeEach(async (to, from) => {
  if (to.name === 'not-found') return true;
  const isAdminPath = to.path === '/admin' || to.path.startsWith('/admin/');

  // Apply the disabled-routes lockdown to student-gated routes only.
  // Admin/moderator bypass: the auth store is the authority on role, but if
  // it has not been hydrated yet we treat the visit as a student visit
  // (defense in depth — anything not proven privileged is restricted).
  if (!isAdminPath && typeof to.name === 'string' && STUDENT_GATED_ROUTES.has(to.name)) {
    const appCfg = useAppConfigStore();
    const auth = useAuthStore();
    if (!appCfg.loaded) await appCfg.load();
    const role = auth.user?.role;
    const isPrivileged = role === 'admin' || role === 'moderator';
    if (!isPrivileged && appCfg.disabledRoutes.has(to.name)) {
      const fallback = defaultPath(appCfg.defaultRoute, appCfg.disabledRoutes);
      if (fallback !== to.path) return fallback;
      return SAFE_FALLBACK_PATH;
    }
  }

  if (!isAdminPath) return true;

  const admin = useAdminStore();

  // Skip the /me roundtrip when we are already inside the admin shell with a
  // known role — the AdminShell layout polls /me every 5s so the store stays
  // fresh on its own. Re-running the fetch on every navigation made
  // overlapping guards stack up; a slow or contended response could leave a
  // navigation pending forever, ending in a blank <main> with no error.
  const fromAdmin = from.path === '/admin' || from.path.startsWith('/admin/');
  if (fromAdmin && admin.role !== null && to.name !== 'admin-login') {
    return true;
  }

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

function isLazyRouteLoadError(err: unknown): boolean {
  const msg = err instanceof Error ? `${err.name} ${err.message}` : String(err);
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Loading chunk [\w-]+ failed|ChunkLoadError/i.test(msg);
}

// A lazy route chunk can disappear during or after a deploy while the browser
// is still running the previous HTML shell. Vue Router leaves the parent
// layout mounted in that case, so admin pages look like an empty content area
// with no backend logs. Reload once to force the no-cache HTML path.
router.onError((err, to) => {
  if (!isLazyRouteLoadError(err)) {
    console.error(err);
    return;
  }

  const key = `mc:route-reload:${to.fullPath}`;
  if (sessionStorage.getItem(key) === '1') {
    console.error(err);
    return;
  }
  sessionStorage.setItem(key, '1');
  window.location.assign(to.fullPath);
});
