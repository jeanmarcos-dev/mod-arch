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
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<AuthUser | null>(null);
  const lastError = ref<string | null>(null);
  const magicSent = ref(false);
  // magicLink is populated when the server could not deliver the link by
  // email (SMTP off / failed). The login page surfaces it as a copyable URL
  // so the student is not stranded.
  const magicLink = ref<string | null>(null);

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

  async function login(payload: LoginPayload): Promise<boolean> {
    lastError.value = null;
    magicSent.value = false;
    magicLink.value = null;
    try {
      const data = await apiJSON<{ user: AuthUser; magic_sent: boolean; magic_link?: string }>(
        'POST',
        '/api/auth/login',
        payload,
      );
      user.value = data.user;
      magicSent.value = Boolean(data.magic_sent);
      magicLink.value = data.magic_link ?? null;
      return true;
    } catch (err) {
      lastError.value = (err as Error).message;
      return false;
    }
  }

  async function logout(): Promise<void> {
    try {
      await apiFetch('/api/auth/logout', { method: 'POST' });
    } finally {
      user.value = null;
      magicSent.value = false;
      magicLink.value = null;
    }
  }

  return { user, lastError, magicSent, magicLink, fetchMe, login, logout };
});
