import { describe, expect, it } from 'vitest';
import es from './es';
import en from './en';
import pt from './pt';

describe.each([
  ['es', es],
  ['en', en],
  ['pt', pt],
])('student-facing copy in %s', (_, messages) => {
  it('does not tell students to reconfigure the server', () => {
    const concurrency = JSON.stringify(messages.sections.concurrency);
    expect(concurrency).not.toMatch(/MODOS_CPU_CORES/);
    expect(concurrency).not.toMatch(/operador|operator/i);
  });

  it('does not require an institutional email to talk to ARCHIUS', () => {
    expect(messages.tutor.launcher.gateText).not.toMatch(/institucional|institutional/i);
  });
});
