import { defineStore } from 'pinia';
import { ref } from 'vue';
import { apiFetch, apiJSON } from '../lib/api';

export interface AuthUser {
  id: string;
  name: string;
  email?: string;
  // Role is reported by GET /api/auth/me so the student shell can decide
  // whether to surface a "Admin panel" link.
  role?: 'student' | 'moderator' | 'admin';
}

export interface LoginPayload {
  name: string;
  email?: string;
  student_id?: string;
  turnstile_token?: string;
}

// LoginOutcome describes the server's response shape so the Login view can
// pick the right next step. Three legitimate states:
//   - "direct":  SMTP off, the session was attached immediately, user is set.
//   - "pending": SMTP on, an email was sent; the caller must click the link.
//   - "error":   server refused (turnstile, send failed, bad input).
export type LoginOutcome = 'direct' | 'pending' | 'error';

export const useAuthStore = defineStore('auth', () => {
  const user = ref<AuthUser | null>(null);
  const lastError = ref<string | null>(null);
  // pendingEmail holds the address the magic link was just sent to. The
  // Login view uses it to render the "we sent you a link" screen with the
  // recipient highlighted.
  const pendingEmail = ref<string | null>(null);

  async function fetchMe(): Promise<boolean> {
    try {
      const res = await apiFetch('/api/auth/me');
      if (!res.ok) {
        user.value = null;
        return false;
      }
      const data = (await res.json()) as { user: AuthUser };
      user.value = data.user;
      return true;
    } catch (err) {
      lastError.value = (err as Error).message;
      return false;
    }
  }

  async function login(payload: LoginPayload): Promise<LoginOutcome> {
    lastError.value = null;
    pendingEmail.value = null;
    try {
      const data = await apiJSON<{
        ok: boolean;
        smtp: 'on' | 'off';
        pending?: boolean;
        email?: string;
        user?: AuthUser;
      }>('POST', '/api/auth/login', payload);
      if (data.smtp === 'on' && data.pending) {
        pendingEmail.value = data.email ?? payload.email ?? null;
        return 'pending';
      }
      if (data.user) {
        user.value = data.user;
      }
      return 'direct';
    } catch (err) {
      lastError.value = (err as Error).message;
      return 'error';
    }
  }

  async function logout(): Promise<void> {
    try {
      await apiFetch('/api/auth/logout', { method: 'POST' });
    } finally {
      user.value = null;
      pendingEmail.value = null;
    }
  }

  return { user, lastError, pendingEmail, fetchMe, login, logout };
});
