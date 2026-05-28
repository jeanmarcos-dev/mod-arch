import type { CoreInfo } from '../stores/cpu';

export type CoreStateKind = 'RUN' | 'HALT' | 'IDLE';

export function hex(v: number | undefined): string {
  if (typeof v !== 'number') return '—';
  return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0');
}

export function coreModeLabel(c: CoreInfo): string {
  return c.mode === 3 ? 'USER' : 'KERNEL';
}

export function coreModeClass(c: CoreInfo): string {
  return c.mode === 3 ? 'core-cell--user' : 'core-cell--kernel';
}

export function coreStateLabel(c: CoreInfo): CoreStateKind {
  if (c.halted) return 'HALT';
  if (c.current_pid > 0) return 'RUN';
  return 'IDLE';
}

export function coreStateClass(c: CoreInfo): string {
  const l = coreStateLabel(c);
  if (l === 'HALT') return 'core-cell--halt';
  if (l === 'RUN') return 'core-cell--running';
  return 'core-cell--idle';
}

export function coreStateTerm(c: CoreInfo): string {
  const l = coreStateLabel(c);
  if (l === 'HALT') return 'HLT';
  if (l === 'RUN') return 'RUN';
  return 'Round-Robin';
}
