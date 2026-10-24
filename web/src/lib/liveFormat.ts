const SECTION_TITLE_KEYS: Record<string, string> = {
  '/': 'router.titles.overview',
  '/procesos': 'router.titles.processes',
  '/memoria': 'router.titles.memory',
  '/interrupciones': 'router.titles.interrupts',
  '/concurrencia': 'router.titles.concurrency',
  '/x86': 'router.titles.x86',
  '/glosario': 'router.titles.glossary',
  '/preferencias': 'router.titles.preferences',
  '/perfil': 'router.titles.profile',
};

export function sectionTitleKey(path: string): string | null {
  return SECTION_TITLE_KEYS[path] ?? null;
}

export function formatClock(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function formatShortDuration(ms: number): string {
  const total = Math.max(0, Math.round(ms / 1000));
  if (total < 60) return `${total}s`;
  const m = Math.floor(total / 60);
  const s = total % 60;
  return s === 0 ? `${m}m` : `${m}m ${s}s`;
}

export function formatLatency(ms: number): string {
  if (ms <= 0) return '—';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

export function stripCitationBrackets(label: string): string {
  return label.replace(/^\[/, '').replace(/\]$/, '');
}

export function flagEmoji(country: string): string {
  if (!/^[A-Za-z]{2}$/.test(country)) return '';
  const base = 0x1f1e6;
  const [a, b] = country.toUpperCase().split('').map((c) => c.charCodeAt(0) - 65);
  return String.fromCodePoint(base + a) + String.fromCodePoint(base + b);
}
