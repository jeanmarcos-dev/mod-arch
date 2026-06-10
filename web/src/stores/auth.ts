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

export interface UserProfile {
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

export type ProfileDraft = Pick<
  UserProfile,
  'cohort' | 'institution' | 'career' | 'bio' | 'github' | 'linkedin' | 'preferred_language'
>;

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
      profile.value = null;
    }
  }

  // Profile (E2): the extended student identity. fetchProfile loads the
  // current user's profile on demand; saveProfile pushes a draft. Both update
  // the cached `profile` ref so any consumer reacts to the latest data.
  const profile = ref<UserProfile | null>(null);

  async function fetchProfile(): Promise<UserProfile | null> {
    try {
      const res = await apiFetch('/api/profile/me');
      if (!res.ok) {
        profile.value = null;
        return null;
      }
      const data = (await res.json()) as UserProfile;
      profile.value = data;
      return data;
    } catch (err) {
      lastError.value = (err as Error).message;
      return null;
    }
  }

  async function saveProfile(draft: ProfileDraft): Promise<UserProfile | null> {
    try {
      const data = await apiJSON<UserProfile>('PUT', '/api/profile/me', draft);
      profile.value = data;
      return data;
    } catch (err) {
      lastError.value = (err as Error).message;
      return null;
    }
  }

  return { user, lastError, pendingEmail, profile, fetchMe, login, logout, fetchProfile, saveProfile };
});
