import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { apiFetch } from '../lib/api';

export interface TelemetryMetrics {
  active_ws_connections: number;
  total_ws_connections: number;
  llm_queries: number;
  llm_errors: number;
  avg_llm_latency_ms: number;
}

export interface LiveCount { name: string; count: number }
export interface LivePageView { name: string; views: number; avg_ms: number }
export interface LiveDemoUse { name: string; loads: number; runs: number }
export interface LiveActivityPoint { minute: number; events: number; questions: number }
export interface LiveTopic { label: string; count: number }
export interface LiveQuestion { ts: number; question: string; rating?: number | null; degraded: boolean }
export interface LiveCountry { code: string; count: number }

export interface LiveStats {
  generated_at: number;
  window_minutes: number;
  now: {
    sessions_active: number;
    ws_connections: number;
    sessions_with_user: number;
    sessions_anonymous: number;
    peak_sessions: number;
    sessions_created_total: number;
    sections: LiveCount[];
    demos: LiveCount[];
  };
  window: {
    unique_users: number;
    unique_sessions: number;
    page_views: LivePageView[];
    demos: LiveDemoUse[];
    activity_per_minute: LiveActivityPoint[];
  };
  tutor: {
    questions: number;
    askers: number;
    latency_p50_ms: number;
    latency_p95_ms: number;
    degraded: number;
    thumbs_up: number;
    thumbs_down: number;
    top_topics: LiveTopic[];
    recent: LiveQuestion[];
  };
  countries: LiveCountry[];
}

export const LIVE_WINDOWS_MINUTES = [30, 120, 1440] as const;

export function normalizeLive(raw: LiveStats): LiveStats {
  return {
    ...raw,
    now: {
      ...raw.now,
      sections: raw.now?.sections ?? [],
      demos: raw.now?.demos ?? [],
    },
    window: {
      ...raw.window,
      page_views: raw.window?.page_views ?? [],
      demos: raw.window?.demos ?? [],
      activity_per_minute: raw.window?.activity_per_minute ?? [],
    },
    tutor: {
      ...raw.tutor,
      top_topics: raw.tutor?.top_topics ?? [],
      recent: raw.tutor?.recent ?? [],
    },
    countries: raw.countries ?? [],
  };
}

export interface AdminStats {
  full: number;
  privileged: number;
  max_public: number;
  max_privileged: number;
  total_created: number;
  peak: number;
}

export interface SessionRow {
  id: string;
  mode: 'full' | 'privileged' | string;
  label: string;
  public: boolean;
  has_share: boolean;
  view_token?: string;
  created_at: number;
  last_active: number;
}

export interface InviteRow {
  token: string;
  label: string;
  created_at: number;
  url: string;
}

export interface AdminDefaults {
  narratorsEnabled: boolean;
  mode: 'compact' | 'pedagogic' | 'projector';
  theme: 'amber' | 'green' | 'dark' | 'accesible';
  density: 'comfortable' | 'compact';
  speed: number;
  narratorMinTickMs: number;
}

export interface AccessSettings {
  access_mode: 'open' | 'invite_only' | 'closed';
  viewer_password_required: boolean;
  viewer_password_set: boolean;
  disabled_routes: string[];
  disabled_panels: string[];
  disabled_demos: string[];
  default_route: string;
  defaults: AdminDefaults;
  schedule_tz: string;
  allowed_domains: string[];
  allowed_emails: string[];
}

export type ScheduleKind = 'demo' | 'panel' | 'route' | 'defaults';
export type ScheduleAction = 'show' | 'hide' | 'apply';

// ScheduleRule mirrors the server wire shape: starts_at/ends_at are epoch UTC
// for sorting/status, while starts_local/ends_local are pre-formatted wall-clock
// strings in `timezone` so the UI never has to do IANA math itself.
export interface ScheduleRule {
  id: string;
  kind: ScheduleKind;
  target: string;
  action: ScheduleAction;
  starts_at: number;
  ends_at: number | null;
  starts_local: string;
  ends_local: string;
  timezone: string;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface ScheduleRuleInput {
  kind: ScheduleKind;
  target: string;
  action: ScheduleAction;
  starts_local: string;
  ends_local: string;
  timezone: string;
  enabled: boolean;
}

export interface UserSessionBrief {
  id: string;
  mode: string;
  label: string;
  last_active: number;
}

export type UserRole = 'student' | 'moderator' | 'admin';

export interface UserRow {
  id: string;
  name: string;
  email?: string;
  student_id?: string;
  role: UserRole;
  last_login_at: number;
  sessions: UserSessionBrief[];
}

export interface AuditRow {
  id: number;
  ts: number;
  actor: string;
  action: string;
  target?: string;
  meta?: string;
}

export interface SessionHistoryRow {
  id: number;
  original_session_id: string;
  user_id?: string;
  user_label?: string;
  mode: string;
  label?: string;
  public: boolean;
  view_token?: string;
  created_at: number;
  ended_at: number;
  duration_seconds: number;
  end_reason: 'idle' | 'admin_kill' | 'replaced' | 'shutdown' | 'user_logout' | string;
  client_ip_hash?: string;
  client_ip?: string;
  ua_short?: string;
  geo_country?: string;
  referrer_host?: string;
  total_commands: number;
  total_tutor_queries: number;
}

export interface SessionHistoryFilter {
  user_id?: string;
  end_reason?: string;
  country?: string;
  from?: number;
  to?: number;
}

export interface SessionHistoryPage {
  rows: SessionHistoryRow[];
  page: number;
  page_size: number;
  total: number;
  filters: SessionHistoryFilter;
}

export interface ActivityByDay {
  Day: string;
  Sessions: number;
  ActiveSeconds: number;
  TutorQueries: number;
}

export interface ActivityByCountry {
  Country: string;
  Sessions: number;
}

export interface UserActivity {
  user_id: string;
  user_label?: string;
  window: { from: number; to: number };
  total_sessions: number;
  total_active_seconds: number;
  avg_session_minutes: number;
  last_seen_at: number;
  last_seen_country?: string;
  total_commands: number;
  total_tutor_queries: number;
  by_day: ActivityByDay[];
  by_country: ActivityByCountry[];
}

export interface ReportRow {
  ID: number;
  TS: number;
  SessionID: string;
  UserID: string;
  ClientIPHash: string;
  Kind: 'feedback' | 'bug' | 'idea' | string;
  Title: string;
  Body: string;
  ContextJSON: string;
  Status: 'open' | 'reviewing' | 'resolved' | 'dismissed' | string;
  ResolvedAt: number;
  ResolvedBy: string;
}

export interface ReportFilter {
  kind?: string;
  status?: string;
  user_id?: string;
}

export interface ReportPage {
  rows: ReportRow[];
  page: number;
  page_size: number;
  total: number;
}

export interface AdminProfileView {
  user_id: string;
  name: string;
  email?: string;
  avatar_url?: string;
  cohort: string;
  institution: string;
  career: string;
  bio: string;
  github: string;
  linkedin: string;
  preferred_language: string;
  updated_at?: number;
}

export interface AuditFilter {
  actor?: string;
  action?: string;
  from?: number;
  to?: number;
}

export interface AuditPage {
  rows: AuditRow[];
  page: number;
  page_size: number;
  total: number;
}

export interface SmtpView {
  host: string;
  port: number;
  user: string;
  password_set: boolean;
  from_name: string;
  from_email: string;
  reply_to: string;
  subject_prefix: string;
  bcc: string;
  tls_mode: 'starttls' | 'tls' | 'none' | string;
  enabled: boolean;
}

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = { method };
  if (body !== undefined) {
    opts.headers = { 'Content-Type': 'application/json' };
    opts.body = JSON.stringify(body);
  }
  const r = await apiFetch(path, opts);
  if (!r.ok) {
    const e = await r.json().catch(() => ({ error: 'Error HTTP ' + r.status }));
    throw new Error(e.error || 'Error HTTP ' + r.status);
  }
  return (await r.json()) as T;
}

interface MeResponse {
  user: string;
  user_id?: string;
  role?: UserRole;
  via_env?: boolean;
  stats: AdminStats;
}

export const ENV_ADMIN_USER_ID = 'env-admin';

export const useAdminStore = defineStore('admin', () => {
  const user = ref<string | null>(null);
  const userId = ref<string | null>(null);
  const role = ref<UserRole | null>(null);
  const viaEnv = ref<boolean>(false);
  const stats = ref<AdminStats | null>(null);
  const metrics = ref<TelemetryMetrics | null>(null);
  const sessions = ref<SessionRow[]>([]);
  const invites = ref<InviteRow[]>([]);
  const settings = ref<AccessSettings | null>(null);
  const users = ref<UserRow[]>([]);
  const audit = ref<AuditRow[]>([]);
  const auditPage = ref<AuditPage | null>(null);
  const sessionHistory = ref<SessionHistoryPage | null>(null);
  const userActivity = ref<Record<string, UserActivity>>({});
  const schedules = ref<ScheduleRule[]>([]);
  const scheduleTZ = ref<string>('UTC');
  const lastError = ref<string | null>(null);
  const live = ref<LiveStats | null>(null);

  // Capability derivatives. Admins do everything; moderators are limited to
  // observation + kick. Components gate UI off these instead of reading the
  // role string directly so the rules live in one place.
  const isAdmin = computed(() => role.value === 'admin');
  const isModerator = computed(() => role.value === 'moderator');
  const canManageSettings = computed(() => isAdmin.value);
  const canManageInvites = computed(() => isAdmin.value);
  const canChangeRoles = computed(() => isAdmin.value);

  async function fetchMe(): Promise<boolean> {
    return (await fetchMeStatus()) === 200;
  }

  function setMe(data: MeResponse | null) {
    if (data) {
      user.value = data.user;
      userId.value = data.user_id ?? null;
      role.value = data.role ?? null;
      viaEnv.value = Boolean(data.via_env);
      stats.value = data.stats;
    } else {
      user.value = null;
      userId.value = null;
      role.value = null;
      viaEnv.value = false;
      stats.value = null;
    }
  }

  // fetchMeStatus returns the raw HTTP status of /admin/api/me so the router
  // can distinguish "unauthenticated" (401) from "admin disabled" (404) and
  // hide the entire /admin namespace in the latter case.
  async function fetchMeStatus(): Promise<number> {
    try {
      const r = await apiFetch('/admin/api/me');
      if (r.ok) {
        setMe((await r.json()) as MeResponse);
      } else {
        setMe(null);
      }
      return r.status;
    } catch {
      setMe(null);
      return 0;
    }
  }

  async function login(creds: { user: string; pass: string }): Promise<boolean> {
    lastError.value = null;
    try {
      await send<{ ok: boolean; user: string }>('POST', '/admin/login', creds);
      // /admin/api/me is the source of truth for role + identity now that
      // moderators can also log in via mc_sid; refresh from there.
      await fetchMe();
      return true;
    } catch (e) {
      lastError.value = e instanceof Error ? e.message : 'Error desconocido';
      return false;
    }
  }

  async function logout() {
    try {
      await send<{ ok: boolean }>('POST', '/admin/logout');
    } finally {
      setMe(null);
      sessions.value = [];
      invites.value = [];
      users.value = [];
      audit.value = [];
    }
  }

  // refresh pulls every admin-side table at once. Moderators are restricted
  // from invites/settings, so those calls are best-effort and fall through to
  // empty/null on 401/403 without breaking the rest of the dashboard.
  async function refresh() {
    const [s, i, me, set, u, a, m] = await Promise.all([
      send<SessionRow[]>('GET', '/admin/api/sessions').catch(() => []),
      send<InviteRow[]>('GET', '/admin/api/invites').catch(() => []),
      send<MeResponse>('GET', '/admin/api/me'),
      send<AccessSettings>('GET', '/admin/api/settings').catch(() => null),
      send<UserRow[]>('GET', '/admin/api/users').catch(() => []),
      send<AuditRow[]>('GET', '/admin/api/audit').catch(() => []),
      send<TelemetryMetrics>('GET', '/admin/api/metrics').catch(() => null),
    ]);
    sessions.value = s ?? [];
    invites.value = i ?? [];
    setMe(me);
    settings.value = set;
    users.value = u ?? [];
    audit.value = a ?? [];
    if (m) metrics.value = m;
  }

  async function killSession(id: string) {
    await send<{ ok: boolean }>('POST', `/admin/api/sessions/${id}/kill`);
    await refresh();
  }

  async function setLabel(id: string, label: string) {
    await send<{ ok: boolean }>('POST', `/admin/api/sessions/${id}/label`, { label });
    await refresh();
  }

  async function togglePublic(id: string, pub: boolean) {
    await send<{ ok: boolean }>('POST', `/admin/api/sessions/${id}/public`, { public: pub });
    await refresh();
  }

  async function createInvite(label: string): Promise<InviteRow> {
    const inv = await send<{ token: string; label: string; url: string }>(
      'POST',
      '/admin/api/invites',
      { label },
    );
    await refresh();
    return { ...inv, created_at: Math.floor(Date.now() / 1000) };
  }

  async function revokeInvite(token: string) {
    await send<{ ok: boolean }>('DELETE', `/admin/api/invites/${token}`);
    await refresh();
  }

  async function setSetting(key: string, value: string) {
    await send<{ ok: boolean }>('PUT', '/admin/api/settings', { key, value });
    await refresh();
  }

  async function kickUser(id: string) {
    await send<{ ok: boolean; sessions_killed: number }>(
      'POST',
      `/admin/api/users/${id}/kick`,
    );
    await refresh();
  }

  async function setUserRole(id: string, newRole: UserRole) {
    await send<{ ok: boolean; role: UserRole }>(
      'POST',
      `/admin/api/users/${id}/role`,
      { role: newRole },
    );
    await refresh();
  }

  // Identity CRUD. The PATCH body uses field-as-optional semantics: only
  // properties explicitly present are touched, so callers can update just the
  // fields the form dirty-tracked.
  async function createUser(args: {
    name: string;
    email: string;
    student_id: string;
    role: UserRole;
  }) {
    await send<UserRow>('POST', '/admin/api/users', args);
    await refresh();
  }

  async function updateUser(
    id: string,
    args: { name?: string; email?: string; student_id?: string },
  ) {
    await send<UserRow>('PATCH', `/admin/api/users/${id}`, args);
    await refresh();
  }

  async function deleteUser(id: string) {
    await send<{ ok: boolean; sessions_killed: number }>(
      'DELETE',
      `/admin/api/users/${id}`,
    );
    await refresh();
  }

  // SMTP block: the panel lets an admin configure the magic-link mailer.
  async function fetchSmtpSettings(): Promise<SmtpView> {
    return await send<SmtpView>('GET', '/admin/api/smtp');
  }

  async function saveSmtpSettings(payload: Record<string, unknown>): Promise<SmtpView> {
    return await send<SmtpView>('PUT', '/admin/api/smtp', payload);
  }

  async function testSmtp(to: string): Promise<void> {
    await send<{ ok: boolean }>('POST', '/admin/api/smtp/test', { to });
  }

  // Session history (E4): paginated browse of expired/closed sessions. The
  // returned page is also cached on the store so the view can reactively
  // re-render when filters change.
  function buildQuery(params: Record<string, string | number | undefined>): string {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue;
      qs.set(k, String(v));
    }
    const s = qs.toString();
    return s ? `?${s}` : '';
  }

  async function loadSessionHistory(
    filter: SessionHistoryFilter = {},
    page = 1,
    pageSize = 50,
  ): Promise<SessionHistoryPage> {
    const qs = buildQuery({ ...filter, page, page_size: pageSize });
    const result = await send<SessionHistoryPage>('GET', `/admin/api/sessions/history${qs}`);
    sessionHistory.value = result;
    return result;
  }

  async function loadUserActivity(
    id: string,
    range?: { from?: number; to?: number },
  ): Promise<UserActivity> {
    const qs = buildQuery({ from: range?.from, to: range?.to });
    const act = await send<UserActivity>('GET', `/admin/api/users/${id}/activity${qs}`);
    userActivity.value = { ...userActivity.value, [id]: act };
    return act;
  }

  // userProfile (E2): cached lookup for the admin's user activity panel so
  // each navigation does not re-fetch.
  const userProfile = ref<Record<string, AdminProfileView>>({});

  async function loadUserProfile(id: string): Promise<AdminProfileView | null> {
    try {
      const p = await send<AdminProfileView>('GET', `/admin/api/users/${id}/profile`);
      userProfile.value = { ...userProfile.value, [id]: p };
      return p;
    } catch {
      return null;
    }
  }

  // Reports (E3): paginated list + status mutation. The page is cached on the
  // store so the AdminReports view stays reactive across filter changes.
  const reportsPage = ref<ReportPage | null>(null);

  async function loadReports(
    filter: ReportFilter = {},
    page = 1,
    pageSize = 50,
  ): Promise<ReportPage> {
    const qs = buildQuery({ ...filter, page, page_size: pageSize });
    const result = await send<ReportPage>('GET', `/admin/api/reports${qs}`);
    reportsPage.value = result;
    return result;
  }

  async function updateReportStatus(id: number, status: ReportRow['Status']) {
    await send<{ ok: boolean; status: string }>(
      'PATCH',
      `/admin/api/reports/${id}/status`,
      { status },
    );
    if (reportsPage.value) {
      // Optimistic local patch: avoid a full refetch round-trip for one row.
      const row = reportsPage.value.rows.find((r) => r.ID === id);
      if (row) {
        row.Status = status;
        if (status === 'resolved' || status === 'dismissed') {
          row.ResolvedAt = Math.floor(Date.now() / 1000);
        } else {
          row.ResolvedAt = 0;
          row.ResolvedBy = '';
        }
      }
    }
  }

  async function loadAuditPage(
    filter: AuditFilter = {},
    page = 1,
    pageSize = 50,
  ): Promise<AuditPage> {
    const qs = buildQuery({ ...filter, page, page_size: pageSize });
    const result = await send<AuditPage>('GET', `/admin/api/audit${qs}`);
    auditPage.value = result;
    return result;
  }


  async function loadLive(windowMinutes: number = 120): Promise<LiveStats> {
    const data = await send<LiveStats>('GET', `/admin/api/live?window=${windowMinutes}`);
    live.value = normalizeLive(data);
    return live.value;
  }

  async function loadMetrics() {
    const data = await send<TelemetryMetrics>('GET', '/admin/api/metrics');
    metrics.value = data;
  }

  // Scheduled delivery: CRUD over the time-windowed rule list. The list
  // endpoint also returns the configured authoring timezone so the form can
  // default to it.
  async function loadSchedules(): Promise<void> {
    const data = await send<{ rules: ScheduleRule[]; timezone: string }>('GET', '/admin/api/schedules');
    schedules.value = data.rules ?? [];
    scheduleTZ.value = data.timezone || 'UTC';
  }

  async function createSchedule(input: ScheduleRuleInput): Promise<void> {
    await send<ScheduleRule>('POST', '/admin/api/schedules', input);
    await loadSchedules();
  }

  async function updateSchedule(id: string, input: ScheduleRuleInput): Promise<void> {
    await send<ScheduleRule>('PUT', `/admin/api/schedules/${id}`, input);
    await loadSchedules();
  }

  async function deleteSchedule(id: string): Promise<void> {
    await send<{ ok: boolean }>('DELETE', `/admin/api/schedules/${id}`);
    await loadSchedules();
  }

  return {
    user,
    userId,
    role,
    viaEnv,
    stats,
    sessions,
    invites,
    settings,
    users,
    audit,
    auditPage,
    sessionHistory,
    userActivity,
    schedules,
    scheduleTZ,
    lastError,
    live,
    isAdmin,
    isModerator,
    canManageSettings,
    canManageInvites,
    canChangeRoles,
    fetchMe,
    fetchMeStatus,
    login,
    logout,
    refresh,
    killSession,
    setLabel,
    togglePublic,
    createInvite,
    revokeInvite,
    setSetting,
    kickUser,
    setUserRole,
    createUser,
    updateUser,
    deleteUser,
    fetchSmtpSettings,
    saveSmtpSettings,
    testSmtp,
    loadSessionHistory,
    loadUserActivity,
    loadAuditPage,
    userProfile,
    loadUserProfile,
    reportsPage,
    loadReports,
    updateReportStatus,
    metrics,
    loadMetrics,
    loadLive,
    loadSchedules,
    createSchedule,
    updateSchedule,
    deleteSchedule,
  };
});
