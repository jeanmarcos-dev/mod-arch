// Cyclic palette used by any view that needs to colour content by PID.
// Six entries keep the cycle long enough for the MaxProcesses=4 limit plus
// some headroom for ZOMBIE PIDs that linger in the timeline ring buffer.
// Tones are picked from the CRT theme so they all sit comfortably on the
// dark surface and pair with the amber/cyan/green accent variables.
const PALETTE = [
  '#ffb347', // amber  — P1
  '#7fc7ff', // cyan   — P2
  '#7fffaa', // green  — P3
  '#ffd24f', // yellow — P4
  '#ff7a9c', // rose   — P5
  '#b591ff', // violet — P6
] as const;

export function pidColor(pid: number): string {
  if (pid <= 0) return 'var(--text-muted, #6a6a6a)';
  return PALETTE[(pid - 1) % PALETTE.length];
}

export const PID_PALETTE = PALETTE;
