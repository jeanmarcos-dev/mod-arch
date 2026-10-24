import { describe, expect, it } from 'vitest';
import es from './locales/es';
import en from './locales/en';
import pt from './locales/pt';

const catalogs = { es, en, pt };

function findKey(node: unknown, key: string): string | undefined {
  if (typeof node !== 'object' || node === null) return undefined;
  for (const [k, v] of Object.entries(node)) {
    if (k === key && typeof v === 'string') return v;
    const nested = findKey(v, key);
    if (nested !== undefined) return nested;
  }
  return undefined;
}

describe('content accuracy of simulator explanations', () => {
  it.each(Object.entries(catalogs))('%s says the kernel logic is written in Go', (_locale, c) => {
    expect(c.panels.codeAsm.explainP3Post).toMatch(/Go/);
  });

  it.each(Object.entries(catalogs))('%s no longer claims the whole behavior is real', (_locale, c) => {
    expect(c.panels.codeAsm.explainP3Bold).not.toMatch(/SÍ es real|is real|É real/);
  });

  it.each(Object.entries(catalogs))('%s says the C programs were translated by hand', (_locale, c) => {
    const text = findKey(c, 'codeC_long');
    expect(text).toBeDefined();
    expect(text).not.toMatch(/compil/i);
    expect(text).toMatch(/mano|hand|mão/);
  });

  it.each(Object.entries(catalogs))('%s mentions ksoftirqd and workqueues for kthreads', (_locale, c) => {
    const card = c.panels.processes.kthreadCardTitle;
    expect(card).toMatch(/ksoftirqd/);
    expect(card).toMatch(/kworker/);
    expect(card).toMatch(/workqueues/);
  });

  it.each(Object.entries(catalogs))('%s qualifies MUL/DIV on EDX as real x86 behavior', (_locale, c) => {
    expect(c.cpu.regHelp.EDX).toMatch(/x86/);
    expect(c.cpu.regHelp.EDX).toMatch(/EDX:EAX/);
  });
});
