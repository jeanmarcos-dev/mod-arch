import { computed, type ComputedRef } from 'vue';
import { useAuthStore } from '../stores/auth';

// useIsStudent reports whether the active session is a student-level one.
// Anonymous visitors and explicit role="student" return true; admin and
// moderator return false. The composable is the single gate for "should I
// apply the admin-curated lockdowns on the simulator UI?" — anything not
// proven privileged is treated as restricted (defense in depth).
export function useIsStudent(): ComputedRef<boolean> {
  const auth = useAuthStore();
  return computed(() => {
    const role = auth.user?.role;
    return role !== 'admin' && role !== 'moderator';
  });
}
