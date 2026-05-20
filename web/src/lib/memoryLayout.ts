// Mirror of the physical memory layout defined in pkg/cpu/memory.go.
// Kept in TS so the frontend can render the region map and resolve any
// address to a region without an extra endpoint. The layout is stable —
// changing it requires a coordinated change with the Go side anyway.

export interface MemoryRegion {
  name: string;
  label: string;
  start: number;
  end: number;
  userPerm: string;
  kernPerm: string;
}

export const MEM_TOTAL = 0x400000; // 4 MB total physical space

export const MEMORY_REGIONS: readonly MemoryRegion[] = [
  { name: 'reserved',   label: 'rsvd',       start: 0x000000, end: 0x0FFFFF, userPerm: '---', kernPerm: 'RW-' },
  { name: 'kernel',     label: 'kernel',     start: 0x100000, end: 0x1FFFFF, userPerm: '---', kernPerm: 'RWX' },
  { name: 'user_code',  label: 'user code',  start: 0x200000, end: 0x2FFFFF, userPerm: 'R-X', kernPerm: 'RWX' },
  { name: 'user_stack', label: 'user stack', start: 0x300000, end: 0x3FFFFF, userPerm: 'RW-', kernPerm: 'RW-' },
];

export function regionFor(addr: number): MemoryRegion | undefined {
  return MEMORY_REGIONS.find(r => addr >= r.start && addr <= r.end);
}

export function fmtAddr(a: number): string {
  return '0x' + (a >>> 0).toString(16).toUpperCase().padStart(8, '0');
}

export function fmtByte(b: number | undefined): string {
  if (b === undefined) return '··';
  return (b & 0xff).toString(16).toUpperCase().padStart(2, '0');
}

// Resolve a core id to the matching CSS custom property name. The palette
// is small (--core-0..3); higher ids cycle to keep colors distinguishable.
export function coreColorVar(coreId: number): string {
  return `var(--core-${coreId % 4})`;
}

export function coreGlowVar(coreId: number): string {
  return `var(--shadow-glow-core-${coreId % 4})`;
}
