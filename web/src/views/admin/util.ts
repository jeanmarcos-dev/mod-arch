// Shared helpers used across the admin sub-views. Kept stateless so each
// sub-view can be loaded independently by vue-router's code splitting.

export function formatTimestamp(unix: number): string {
  if (!unix) return '—';
  const d = new Date(unix * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function formatAgo(unix: number): string {
  const d = Date.now() / 1000 - unix;
  if (d < 60) return Math.floor(d) + 's';
  if (d < 3600) return Math.floor(d / 60) + 'm';
  return Math.floor(d / 3600) + 'h';
}
