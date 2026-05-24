import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { apiFetch } from '../lib/api';

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

export interface AccessSettings {
  access_mode: 'open' | 'invite_only' | 'closed';
  viewer_password_required: boolean;
  viewer_password_set: boolean;
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
  const sessions = ref<SessionRow[]>([]);
  const invites = ref<InviteRow[]>([]);
  const settings = ref<AccessSettings | null>(null);
  const users = ref<UserRow[]>([]);
  const audit = ref<AuditRow[]>([]);
  const lastError = ref<string | null>(null);

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
    const [s, i, me, set, u, a] = await Promise.all([
      send<SessionRow[]>('GET', '/admin/api/sessions').catch(() => []),
      send<InviteRow[]>('GET', '/admin/api/invites').catch(() => []),
      send<MeResponse>('GET', '/admin/api/me'),
      send<AccessSettings>('GET', '/admin/api/settings').catch(() => null),
      send<UserRow[]>('GET', '/admin/api/users').catch(() => []),
      send<AuditRow[]>('GET', '/admin/api/audit').catch(() => []),
    ]);
    sessions.value = s ?? [];
    invites.value = i ?? [];
    setMe(me);
    settings.value = set;
    users.value = u ?? [];
    audit.value = a ?? [];
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
    lastError,
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
  };
});
