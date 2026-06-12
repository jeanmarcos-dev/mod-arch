import type { PCBInfo } from '../stores/cpu/types';

// A "user process" is anything the student's demo spawned: everything except
// the kernel-side machinery (swapper, kthreads and init).
export function isUserProc(p: PCBInfo): boolean {
  return p.is_idle !== true && p.is_kthread !== true && p.name !== 'init';
}
