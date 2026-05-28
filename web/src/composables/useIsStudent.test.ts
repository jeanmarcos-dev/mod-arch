import { describe, it, expect, beforeEach } from 'vitest';
import { useIsStudent } from './useIsStudent';
import { useAuthStore } from '../stores/auth';

describe('useIsStudent composable', () => {
  let auth: ReturnType<typeof useAuthStore>;

  beforeEach(() => {
    auth = useAuthStore();
  });

  it('treats anonymous visitors as students (defense in depth)', () => {
    const isStudent = useIsStudent();
    expect(isStudent.value).toBe(true);
  });

  it('flags explicit student role as student', () => {
    auth.user = { id: 'u1', name: 'Alice', role: 'student' };
    const isStudent = useIsStudent();
    expect(isStudent.value).toBe(true);
  });

  it('does not treat moderators as students', () => {
    auth.user = { id: 'u2', name: 'Bob', role: 'moderator' };
    const isStudent = useIsStudent();
    expect(isStudent.value).toBe(false);
  });

  it('does not treat admins as students', () => {
    auth.user = { id: 'u3', name: 'Carol', role: 'admin' };
    const isStudent = useIsStudent();
    expect(isStudent.value).toBe(false);
  });

  it('reacts to role changes', () => {
    const isStudent = useIsStudent();
    expect(isStudent.value).toBe(true);
    auth.user = { id: 'u4', name: 'Dan', role: 'admin' };
    expect(isStudent.value).toBe(false);
    auth.user = { id: 'u4', name: 'Dan', role: 'student' };
    expect(isStudent.value).toBe(true);
  });
});
