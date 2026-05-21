import { describe, it, expect } from 'vitest';
import { findEntry } from './glossary';

describe('findEntry', () => {
  it('matches exact term case-insensitively', () => {
    expect(findEntry('EIP')?.term).toBe('EIP');
    expect(findEntry('eip')?.term).toBe('EIP');
    expect(findEntry('  EIP  ')?.term).toBe('EIP');
  });

  it('matches by prefix before the em-dash', () => {
    expect(findEntry('PL')?.term).toBe('PL — Privilege Level');
    expect(findEntry('ZF')?.term).toBe('ZF — Zero Flag');
    expect(findEntry('IRQ 0x20')?.term).toBe('IRQ 0x20 — Timer');
  });

  it('matches comma-separated tokens of a multi-term entry', () => {
    const entry = findEntry('EBX');
    expect(entry?.term).toBe('EAX, EBX, ECX, EDX');
    expect(findEntry('ecx')?.term).toBe('EAX, EBX, ECX, EDX');
    expect(findEntry('ESP')?.term).toBe('ESP, EBP');
  });

  it('strips parenthetical suffixes when indexing', () => {
    expect(findEntry('sys_write')?.term).toBe('sys_write (4)');
    expect(findEntry('sys_read')?.term).toBe('sys_read (3)');
    expect(findEntry('sys_exit')?.term).toBe('sys_exit (1)');
  });

  it('resolves explicit aliases', () => {
    expect(findEntry('semáforo')?.term).toBe('Semaphore');
    expect(findEntry('mutex')?.term).toBe('Mutex');
    expect(findEntry('interbloqueo')?.term).toBe('Deadlock');
    expect(findEntry('sección crítica')?.term).toBe('Critical section');
    expect(findEntry('int 0x81')?.term).toBe('INT 0x81 — Mutex/Semáforo');
    expect(findEntry('IRQ 0x80')?.term).toBe('INT 0x80 — Syscall');
    expect(findEntry('IRQ 0x0E')?.term).toBe('Vector 0x0E — Page Fault');
    expect(findEntry('IRQ 0x00')?.term).toBe('Vector 0x00 — Divide error');
    expect(findEntry('exception')?.term).toBe('Excepción');
    expect(findEntry('trap')?.term).toBe('Software interrupt');
  });

  it('returns null for unknown terms', () => {
    expect(findEntry('inexistente')).toBeNull();
    expect(findEntry('')).toBeNull();
    expect(findEntry('   ')).toBeNull();
  });

  it('does not let aliases clobber exact matches', () => {
    // Sanity check: every canonical term still resolves to itself even if
    // some other entry's alias happened to overlap.
    for (const t of ['Mutex', 'Semaphore', 'Deadlock', 'Race condition']) {
      expect(findEntry(t)?.term).toBe(t);
    }
  });
});
