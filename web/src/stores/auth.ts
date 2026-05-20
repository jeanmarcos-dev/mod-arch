import { defineStore } from 'pinia';
import { ref } from 'vue';

export interface AuthUser {
  id: string;
  name: string;
  email?: string;
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

  async function fetchMe(): Promise<boolean> {
    try {
      const res = await fetch('/api/auth/me', { credentials: 'same-origin' });
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
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json();
      if (!res.ok) {
        lastError.value = data.error ?? `error ${res.status}`;
        return false;
      }
      user.value = data.user as AuthUser;
      magicSent.value = Boolean(data.magic_sent);
      return true;
    } catch (err) {
      lastError.value = (err as Error).message;
      return false;
    }
  }

  async function logout(): Promise<void> {
    try {
      await fetch('/api/auth/logout', {
        method: 'POST',
        credentials: 'same-origin',
      });
    } finally {
      user.value = null;
      magicSent.value = false;
    }
  }

  return { user, lastError, magicSent, fetchMe, login, logout };
});
