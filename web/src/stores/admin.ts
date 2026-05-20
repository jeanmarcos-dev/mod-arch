import { defineStore } from 'pinia';
import { ref } from 'vue';

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

export interface UserRow {
  id: string;
  name: string;
  email?: string;
  student_id?: string;
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

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    credentials: 'same-origin',
  };
  if (body !== undefined) opts.body = JSON.stringify(body);
  const r = await fetch(path, opts);
  if (!r.ok) {
    const e = await r.json().catch(() => ({ error: 'Error HTTP ' + r.status }));
    throw new Error(e.error || 'Error HTTP ' + r.status);
  }
  return (await r.json()) as T;
}

export const useAdminStore = defineStore('admin', () => {
  const user = ref<string | null>(null);
  const stats = ref<AdminStats | null>(null);
  const sessions = ref<SessionRow[]>([]);
  const invites = ref<InviteRow[]>([]);
  const settings = ref<AccessSettings | null>(null);
  const users = ref<UserRow[]>([]);
  const audit = ref<AuditRow[]>([]);
  const lastError = ref<string | null>(null);

  async function fetchMe(): Promise<boolean> {
    try {
      const r = await send<{ user: string; stats: AdminStats }>('GET', '/admin/api/me');
      user.value = r.user;
      stats.value = r.stats;
      return true;
    } catch {
      user.value = null;
      stats.value = null;
      return false;
    }
  }

  async function login(creds: { user: string; pass: string }): Promise<boolean> {
    lastError.value = null;
    try {
      const r = await send<{ ok: boolean; user: string }>('POST', '/admin/login', creds);
      user.value = r.user;
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
      user.value = null;
      stats.value = null;
      sessions.value = [];
      invites.value = [];
    }
  }

  async function refresh() {
    const [s, i, me, set, u, a] = await Promise.all([
      send<SessionRow[]>('GET', '/admin/api/sessions'),
      send<InviteRow[]>('GET', '/admin/api/invites'),
      send<{ user: string; stats: AdminStats }>('GET', '/admin/api/me'),
      send<AccessSettings>('GET', '/admin/api/settings').catch(() => null),
      send<UserRow[]>('GET', '/admin/api/users').catch(() => []),
      send<AuditRow[]>('GET', '/admin/api/audit').catch(() => []),
    ]);
    sessions.value = s ?? [];
    invites.value = i ?? [];
    user.value = me.user;
    stats.value = me.stats;
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

  return {
    user,
    stats,
    sessions,
    invites,
    settings,
    users,
    audit,
    lastError,
    fetchMe,
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
  };
});
